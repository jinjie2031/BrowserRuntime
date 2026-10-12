//go:build windows

package cloak

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	brnetwork "browserruntime/internal/network"
	"browserruntime/internal/processmanager"
	"browserruntime/internal/profile"
)

func TestUnavailableProxyBrowserHasNoDirectFallback(t *testing.T) {
	if os.Getenv("BR_RUN_CLOAK_NETWORK_ACCEPTANCE") != "1" {
		t.Skip("set BR_RUN_CLOAK_NETWORK_ACCEPTANCE=1 to run native browser acceptance")
	}
	executable := os.Getenv("BR_CLOAK_EXECUTABLE")
	if executable == "" {
		t.Fatal("native browser acceptance environment is incomplete")
	}
	root := filepath.Join(t.TempDir(), "profiles")
	profiles, err := profile.NewStore(root)
	if err != nil {
		t.Fatal("create acceptance profile store")
	}
	p, err := profiles.Create("Task 3 unavailable proxy acceptance")
	if err != nil {
		t.Fatal("create acceptance profile")
	}
	adapter, err := NewAdapter(executable, root)
	if err != nil {
		t.Fatal("create native adapter")
	}
	bridge, err := brnetwork.StartLoopbackBridge(&brnetwork.ResolvedProxy{Scheme: brnetwork.SchemeHTTP, Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal("create unavailable proxy bridge")
	}
	defer bridge.Close()
	engineProxy := bridge.EngineProxy()
	engineProxy.Locality = &brnetwork.Locality{ExitIP: "203.0.113.71", CountryCode: "US", City: "Los Angeles", Timezone: "America/Los_Angeles", Locale: "en-US"}
	spec, err := adapter.LaunchSpecWithNetwork(*p, engineProxy)
	if err != nil {
		t.Fatal("build fail-closed native launch specification")
	}
	var directHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	spec.Arguments = append(spec.Arguments, "--proxy-bypass-list=<-loopback>", target.URL)

	manager, err := processmanager.NewProcessManagerWithPaths(root, executable)
	if err != nil {
		t.Fatal("configure native occupancy checks")
	}
	if err := manager.Launch(p.ID, spec); err != nil {
		t.Fatal("launch native browser for unavailable proxy acceptance")
	}
	time.Sleep(5 * time.Second)
	if directHits.Load() != 0 {
		_ = manager.Stop(p.ID)
		t.Fatal("unavailable proxy produced direct browser egress")
	}
	if err := manager.Stop(p.ID); err != nil {
		t.Fatal("stop native browser after unavailable proxy acceptance")
	}
}
