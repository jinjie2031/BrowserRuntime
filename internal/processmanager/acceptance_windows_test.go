//go:build windows

package processmanager_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"browserruntime/internal/engine/cloak"
	"browserruntime/internal/processmanager"
	"browserruntime/internal/profile"
)

const acceptanceProfileName = "Task 2 Native Lifecycle Acceptance"

func TestCloakNativeLifecycleAcceptance(t *testing.T) {
	if os.Getenv("BR_RUN_CLOAK_ACCEPTANCE") != "1" {
		t.Skip("set BR_RUN_CLOAK_ACCEPTANCE=1 for the operator-approved native acceptance run")
	}
	executable := os.Getenv("BR_CLOAK_EXECUTABLE")
	profilesRoot := os.Getenv("BR_PROFILES_ROOT")
	if executable == "" || profilesRoot == "" {
		t.Fatal("BR_CLOAK_EXECUTABLE and BR_PROFILES_ROOT are required")
	}

	store, err := profile.NewStore(profilesRoot)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	p := findOrCreateAcceptanceProfile(t, store)
	seed := p.FingerprintSeed

	adapter, err := cloak.NewAdapter(executable, profilesRoot)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	firstSpec, err := adapter.LaunchSpec(*p)
	if err != nil {
		t.Fatalf("LaunchSpec(first) error = %v", err)
	}
	manager, err := processmanager.NewProcessManagerWithPaths(profilesRoot, executable)
	if err != nil {
		t.Fatal("configure native occupancy checks")
	}

	if err := manager.Launch(p.ID, firstSpec); err != nil {
		t.Fatalf("Launch(first) error = %v", err)
	}
	if status, err := manager.Status(p.ID); err != nil || status != processmanager.StatusRunning {
		t.Fatalf("Status(first) = %q, %v; want RUNNING", status, err)
	}
	waitForOpen(t, manager, p.ID)
	if err := manager.Launch(p.ID, firstSpec); !errors.Is(err, processmanager.ErrProfileAlreadyRunning) {
		t.Fatalf("duplicate Launch() error = %v, want ErrProfileAlreadyRunning", err)
	}
	if err := manager.Stop(p.ID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status, err := manager.Status(p.ID); err != nil || status != processmanager.StatusStopped {
		t.Fatalf("Status(after Stop) = %q, %v; want STOPPED", status, err)
	}

	reopenedStore, err := profile.NewStore(profilesRoot)
	if err != nil {
		t.Fatalf("NewStore(reopen) error = %v", err)
	}
	reopened, err := reopenedStore.Get(p.ID)
	if err != nil {
		t.Fatalf("Get(reopen) error = %v", err)
	}
	secondSpec, err := adapter.LaunchSpec(*reopened)
	if err != nil {
		t.Fatalf("LaunchSpec(second) error = %v", err)
	}
	if reopened.FingerprintSeed != seed || secondSpec.FingerprintSeed != firstSpec.FingerprintSeed {
		t.Fatal("fingerprint seed changed before reopen")
	}
	if !samePath(secondSpec.ProfilePath, firstSpec.ProfilePath) {
		t.Fatalf("profile path changed: %q -> %q", firstSpec.ProfilePath, secondSpec.ProfilePath)
	}
	if err := manager.Launch(p.ID, secondSpec); err != nil {
		t.Fatalf("Launch(second) error = %v", err)
	}
	waitForOpen(t, manager, p.ID)
	fmt.Printf("TASK2_MANUAL_CLOSE_READY profile_id=%s\n", p.ID)

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := manager.Status(p.ID)
		if err != nil {
			t.Fatalf("Status(wait manual close) error = %v", err)
		}
		if status == processmanager.StatusStopped {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("manual browser close was not detected within five minutes")
}

func findOrCreateAcceptanceProfile(t *testing.T, store *profile.Store) *profile.Profile {
	t.Helper()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for i := range profiles {
		if profiles[i].Name == acceptanceProfileName {
			return &profiles[i]
		}
	}
	created, err := store.Create(acceptanceProfileName)
	if err != nil {
		t.Fatalf("Create(acceptance profile) error = %v", err)
	}
	return created
}

func waitForOpen(t *testing.T, manager *processmanager.ProcessManager, profileID string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := manager.Open(profileID); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("Open() did not foreground browser within 20 seconds: %v", lastErr)
}

func samePath(first string, second string) bool {
	firstAbsolute, firstErr := filepath.Abs(first)
	secondAbsolute, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && firstAbsolute == secondAbsolute
}
