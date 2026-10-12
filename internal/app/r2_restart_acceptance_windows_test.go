//go:build windows

package app

import (
	"browserruntime/internal/engine"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"browserruntime/internal/processmanager"
)

// Only temporary data and the existing fake browser are used. Replacing the
// manager models lost runtime bookkeeping, not an actual browser crash/kill.
func TestR2RestartMustNotDeleteSurvivingBrowserProfile(t *testing.T) {
	s, root, previous := acceptanceService(t)
	p, err := s.CreateProfile("R2 isolated restart safety")
	if err != nil {
		t.Fatal("temporary profile setup failed")
	}
	if err := s.LaunchProfile(p.ID); err != nil {
		t.Fatal("simulated browser launch failed")
	}
	marker := filepath.Join(root, p.ID, "browser-data", "r2-persistence-marker")
	if err := os.WriteFile(marker, []byte("temporary browser data"), 0600); err != nil {
		t.Fatal("temporary browser data setup failed")
	}
	// The previous browser remains running while a restarted native manager
	// has no process records. No real process is terminated by this probe.
	s.processes = processmanager.NewProcessManager()
	status, err := s.processes.Status(p.ID)
	if err != nil {
		t.Fatal("restarted manager status failed")
	}
	closeErr := s.PrepareClose()
	deleteErr := s.DeleteProfile(p.ID)
	_, markerErr := os.Stat(marker)
	stillRunning, _ := previous.Status(p.ID)
	t.Logf("restart simulation: reported_status=%s surviving_browser_status=%s exit_accepted=%t delete_accepted=%t browser_data_removed=%t",
		status, stillRunning, closeErr == nil, deleteErr == nil, errors.Is(markerErr, os.ErrNotExist))
	if closeErr == nil || deleteErr == nil || errors.Is(markerErr, os.ErrNotExist) {
		t.Fatal("BLOCKER: restarted manager allowed exit or deletion while a browser survived")
	}
}

func TestR3OtherManagerLeaseBlocksAllDangerousActions(t *testing.T) {
	s, root, _ := acceptanceService(t)
	id := acceptanceProxy(t, s, "R3 cross-manager profile")
	executable := filepath.Join(root, "isolated-native-candidate.exe")
	a, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("manager A setup failed")
	}
	b, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("manager B setup failed")
	}
	s.processes = b
	before := acceptanceNetworkFiles(t, root, id)
	if err := a.WithStoppedProfiles([]string{id}, func() error {
		for _, action := range []func() error{
			func() error { return s.DeleteProfile(id) },
			func() error { return s.LaunchProfile(id) },
			func() error { _, err := s.SetDirect(id); return err },
			func() error {
				_, err := s.SetProxy(id, SetProxyInput{Scheme: "http", Host: "replacement.example", Port: 8080})
				return err
			},
			func() error { _, err := s.TestNetwork(id); return err },
			s.PrepareClose,
		} {
			if action() == nil {
				t.Fatal("other manager lease failed to guard action")
			}
		}
		return nil
	}); err != nil {
		t.Fatal("manager A guarded operation failed")
	}
	if before != acceptanceNetworkFiles(t, root, id) {
		t.Fatal("other manager lease rejection mutated network")
	}
	if _, err := s.profiles.Get(id); err != nil {
		t.Fatal("protected profile lost")
	}
	if _, err := s.SetDirect(id); err != nil {
		t.Fatal("stopped editing did not recover")
	}
}

type r3PausedDeleteStore struct {
	profileStore
	entered, resume chan struct{}
}

func (p r3PausedDeleteStore) Delete(id string) error {
	close(p.entered)
	<-p.resume
	return p.profileStore.Delete(id)
}

func TestR3DeleteHoldsLeaseThroughFilesystemAction(t *testing.T) {
	s, root, _ := acceptanceService(t)
	p, err := s.CreateProfile("R3 delete race temporary profile")
	if err != nil {
		t.Fatal("profile setup failed")
	}
	executable := filepath.Join(root, "isolated-native-candidate.exe")
	a, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("manager A setup failed")
	}
	b, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("manager B setup failed")
	}
	s.processes = a
	paused := r3PausedDeleteStore{s.profiles, make(chan struct{}), make(chan struct{})}
	s.profiles = paused
	done := make(chan error, 1)
	go func() { done <- s.DeleteProfile(p.ID) }()
	select {
	case <-paused.entered:
	case <-time.After(time.Second):
		close(paused.resume)
		t.Fatal("delete did not enter guarded filesystem action")
	}
	called := false
	err = b.LaunchPrepared(p.ID, func() (engine.LaunchSpec, error) { called = true; return engine.LaunchSpec{}, nil })
	close(paused.resume)
	if err == nil || called {
		t.Fatal("launch raced through guarded deletion")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("safe deletion failed")
		}
	case <-time.After(time.Second):
		t.Fatal("delete deadlock")
	}
}

func TestR3ExternalAndUnknownGuardsPreserveData(t *testing.T) {
	for _, status := range []processmanager.Status{processmanager.StatusExternalRunning, processmanager.StatusInUse, processmanager.StatusUnknown} {
		t.Run(string(status), func(t *testing.T) {
			s, root, pm := acceptanceService(t)
			id := acceptanceProxy(t, s, "R3 protected temporary profile")
			path := filepath.Join(root, id)
			marker := filepath.Join(path, "browser-data", "r3-marker")
			if os.WriteFile(marker, []byte("preserve"), 0600) != nil {
				t.Fatal("marker setup failed")
			}
			profileBefore, err := os.ReadFile(filepath.Join(path, "profile.json"))
			if err != nil {
				t.Fatal("metadata setup failed")
			}
			networkBefore := acceptanceNetworkFiles(t, root, id)
			pm.statuses[id] = status
			for _, operation := range []func() error{
				func() error { return s.DeleteProfile(id) },
				func() error { return s.LaunchProfile(id) },
				func() error { _, err := s.SetDirect(id); return err },
				func() error {
					_, err := s.SetProxy(id, SetProxyInput{Scheme: "http", Host: "replacement.example", Port: 8080})
					return err
				},
				func() error { _, err := s.TestNetwork(id); return err },
				s.PrepareClose,
			} {
				if operation() == nil {
					t.Fatal("unsafe occupancy allowed operation")
				}
			}
			views, err := s.ListProfiles()
			if err != nil || len(views) != 1 || views[0].LaunchReady || views[0].Status == "Stopped" {
				t.Fatal("unsafe UI readiness")
			}
			after, err := os.ReadFile(filepath.Join(path, "profile.json"))
			if err != nil || sha256.Sum256(after) != sha256.Sum256(profileBefore) || acceptanceNetworkFiles(t, root, id) != networkBefore {
				t.Fatal("rejection changed profile identity/network/DPAPI data")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" || len(pm.launched) != 0 {
				t.Fatal("data lost or extra browser created")
			}
			pm.statuses[id] = processmanager.StatusStopped
			if _, err := s.SetDirect(id); err != nil {
				t.Fatal("stopped editing failed")
			}
			if s.LaunchProfile(id) != nil || s.StopProfile(id) != nil || s.DeleteProfile(id) != nil || s.PrepareClose() != nil {
				t.Fatal("stopped normal path failed")
			}
		})
	}
}
