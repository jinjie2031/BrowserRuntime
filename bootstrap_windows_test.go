//go:build windows

package main

import (
	"browserruntime/internal/enginebootstrap"
	"browserruntime/internal/network"
	"browserruntime/internal/portable"
	"browserruntime/internal/profile"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableProfileAndDPAPISecretSurviveSameUserFolderMove(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	if os.Mkdir(first, 0700) != nil {
		t.Fatal("setup")
	}
	l, _ := portable.FromExecutable(filepath.Join(first, "BrowserRuntime.exe"))
	if l.Prepare() != nil {
		t.Fatal("prepare")
	}
	profiles, _ := profile.NewStore(l.Profiles)
	p, err := profiles.Create("Untranslated user name 环境")
	if err != nil {
		t.Fatal(err)
	}
	networks, _ := network.NewStore(l.Profiles)
	const password = "portable-dpapi-canary-only"
	config := network.Config{Mode: network.ModeProxy, Scheme: network.SchemeHTTP, Host: "127.0.0.1", Port: 4444, Username: "portable-canary-user"}
	if networks.SetProxy(p.ID, config, password) != nil {
		t.Fatal("set proxy")
	}
	for _, name := range []string{"network.json", "network.secret", "profile.json"} {
		b, err := os.ReadFile(filepath.Join(l.Profiles, p.ID, name))
		if err != nil || bytes.Contains(b, []byte(password)) {
			t.Fatal("plaintext password on disk")
		}
	}
	moved := filepath.Join(parent, "moved")
	if os.Rename(first, moved) != nil {
		t.Fatal("relocate")
	}
	next, _ := portable.FromExecutable(filepath.Join(moved, "BrowserRuntime.exe"))
	if next.Prepare() != nil {
		t.Fatal("prepare moved")
	}
	newProfiles, _ := profile.NewStore(next.Profiles)
	saved, err := newProfiles.Get(p.ID)
	if err != nil || saved.ProfilePath != p.ProfilePath || saved.FingerprintSeed != p.FingerprintSeed || saved.Name != p.Name {
		t.Fatal("profile identity changed")
	}
	newNetworks, _ := network.NewStore(next.Profiles)
	resolved, err := newNetworks.ResolveProxy(p.ID)
	if err != nil || resolved.Password != password {
		t.Fatal("same-user DPAPI failed after move")
	}
}

func TestPortableServiceWithoutEngineIgnoresEnvironment(t *testing.T) {
	root := t.TempDir()
	l, err := portable.FromExecutable(filepath.Join(root, "BrowserRuntime.exe"))
	if err != nil || l.Prepare() != nil {
		t.Fatal("layout setup")
	}
	outside := filepath.Join(t.TempDir(), "must-not-use")
	t.Setenv("BR_PROFILES_ROOT", outside)
	t.Setenv("BR_CLOAK_EXECUTABLE", outside+".exe")
	m := enginebootstrap.New(filepath.Join(l.Engines, "cloakbrowser", enginebootstrap.Version))
	s, err := buildPortableService(l, m)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProfile("Portable")
	if err != nil {
		t.Fatal(err)
	}
	store, err := profile.NewStore(l.Profiles)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(p.ID); err != nil {
		t.Fatal("profile outside portable root")
	}
	if _, err = os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("environment override used")
	}
	if err = s.LaunchProfile(p.ID); err != enginebootstrap.ErrNotReady {
		t.Fatalf("missing engine must fail closed: %T", err)
	}
	if s.PrepareClose() != nil {
		t.Fatal("stopped portable cannot close")
	}
}
