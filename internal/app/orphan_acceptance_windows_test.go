//go:build windows

package app

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"browserruntime/internal/engine/cloak"
	brnetwork "browserruntime/internal/network"
	"browserruntime/internal/processmanager"
	"browserruntime/internal/profile"
)

// Explicitly opted-in, one targeted check against the operator-authorized
// temporary Browser launched by the real desktop Manager. Never auto-launches
// or terminates the external browser, and never reads browser storage.
func TestR3LiveOrphanGuardAcceptance(t *testing.T) {
	if os.Getenv("BR_RUN_R3_ORPHAN_ACCEPTANCE") != "1" {
		t.Skip("requires the approved temporary orphan acceptance run")
	}
	root, id, executable := os.Getenv("BR_R3_PROFILES_ROOT"), os.Getenv("BR_R3_PROFILE_ID"), os.Getenv("BR_CLOAK_EXECUTABLE")
	if profile.ValidateID(id) != nil || !filepath.IsAbs(root) || !filepath.IsAbs(executable) {
		t.Fatal("explicit temporary acceptance configuration required")
	}
	profiles, err := profile.NewStore(root)
	if err != nil {
		t.Fatal("profile store setup failed")
	}
	networks, err := brnetwork.NewStore(root)
	if err != nil {
		t.Fatal("network store setup failed")
	}
	adapter, err := cloak.NewAdapter(executable, root)
	if err != nil {
		t.Fatal("native adapter setup failed")
	}
	pm, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("occupancy setup failed")
	}
	s, err := NewService(profiles, networks, adapter, pm, brnetwork.StartLoopbackBridge, connectionStub{}, fixedResolver{})
	if err != nil {
		t.Fatal("application setup failed")
	}
	before := map[string][32]byte{}
	for _, name := range []string{"profile.json", "network.json"} {
		data, err := os.ReadFile(filepath.Join(root, id, name))
		if err != nil {
			t.Fatal("acceptance metadata missing")
		}
		before[name] = sha256.Sum256(data)
	}
	status, err := pm.Status(id)
	if err != nil || status != processmanager.StatusExternalRunning {
		t.Fatal("real orphan not classified ExternalRunning")
	}
	for _, operation := range []func() error{
		func() error { return s.DeleteProfile(id) }, func() error { return s.LaunchProfile(id) },
		func() error { _, err := s.SetDirect(id); return err },
		func() error {
			_, err := s.SetProxy(id, SetProxyInput{Scheme: "http", Host: "127.0.0.1", Port: 1})
			return err
		},
		func() error { _, err := s.TestNetwork(id); return err }, s.PrepareClose,
	} {
		if operation() == nil {
			t.Fatal("real orphan bypassed application guard")
		}
	}
	for name, hash := range before {
		data, err := os.ReadFile(filepath.Join(root, id, name))
		if err != nil || sha256.Sum256(data) != hash {
			t.Fatal("real orphan rejection changed metadata")
		}
	}
	if info, err := os.Stat(filepath.Join(root, id, "browser-data")); err != nil || !info.IsDir() {
		t.Fatal("real browser data lost")
	}
	t.Log("REAL_ORPHAN_GUARDS=PASS METADATA_UNCHANGED=true EXTERNAL_BROWSER_NOT_CONTROLLED=true")
}
