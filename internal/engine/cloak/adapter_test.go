package cloak

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"browserruntime/internal/engine"
	brnetwork "browserruntime/internal/network"
	"browserruntime/internal/profile"
)

func TestLaunchSpecUsesPersistentPathAndFixedSeed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	id := "br_0123456789abcdef0123456789abcdef"
	browserDataPath := filepath.Join(root, id, "browser-data")
	if err := os.MkdirAll(browserDataPath, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	executable := filepath.Join(t.TempDir(), "cloak.exe")
	if err := os.WriteFile(executable, nil, 0o700); err != nil {
		t.Fatalf("WriteFile(executable) error = %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root) error = %v", err)
	}
	wantBrowserDataPath := filepath.Join(canonicalRoot, id, "browser-data")

	adapter, err := NewAdapter(executable, root)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	now := time.Now().UTC()
	p := profile.Profile{
		ID:              id,
		Name:            "Test",
		EngineID:        profile.DefaultEngineID,
		ProfilePath:     filepath.ToSlash(filepath.Join(id, "browser-data")),
		FingerprintSeed: "12345",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	first, err := adapter.LaunchSpec(p)
	if err != nil {
		t.Fatalf("LaunchSpec(first) error = %v", err)
	}
	second, err := adapter.LaunchSpec(p)
	if err != nil {
		t.Fatalf("LaunchSpec(second) error = %v", err)
	}
	if first.ProfilePath != wantBrowserDataPath || second.ProfilePath != wantBrowserDataPath {
		t.Fatalf("ProfilePath = %q/%q, want %q", first.ProfilePath, second.ProfilePath, wantBrowserDataPath)
	}
	if first.FingerprintSeed != "12345" || second.FingerprintSeed != "12345" {
		t.Fatalf("FingerprintSeed = %q/%q, want fixed seed", first.FingerprintSeed, second.FingerprintSeed)
	}
	if len(first.Arguments) != len(second.Arguments) {
		t.Fatalf("argument lengths differ: %v vs %v", first.Arguments, second.Arguments)
	}
	for i := range first.Arguments {
		if first.Arguments[i] != second.Arguments[i] {
			t.Fatalf("argument %d differs: %q vs %q", i, first.Arguments[i], second.Arguments[i])
		}
	}

	wantArguments := map[string]bool{
		"--user-data-dir=" + wantBrowserDataPath: false,
		"--fingerprint=12345":                    false,
	}
	for _, argument := range first.Arguments {
		if _, ok := wantArguments[argument]; ok {
			wantArguments[argument] = true
		}
		lower := strings.ToLower(argument)
		for _, forbidden := range engine.ForbiddenLaunchFlags() {
			if lower == forbidden || strings.HasPrefix(lower, forbidden+"=") {
				t.Fatalf("LaunchSpec() included forbidden flag %q", argument)
			}
		}
	}
	for argument, found := range wantArguments {
		if !found {
			t.Errorf("LaunchSpec() missing %q; got %v", argument, first.Arguments)
		}
	}
	if err := engine.ValidateLaunchSpec(first); err != nil {
		t.Fatalf("ValidateLaunchSpec() error = %v", err)
	}
}

func TestLaunchSpecNetworkArguments(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	id := "br_0123456789abcdef0123456789abcdef"
	browserDataPath := filepath.Join(root, id, "browser-data")
	if err := os.MkdirAll(browserDataPath, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	executable := filepath.Join(t.TempDir(), "cloak.exe")
	if err := os.WriteFile(executable, nil, 0o700); err != nil {
		t.Fatalf("WriteFile(executable) error = %v", err)
	}
	adapter, err := NewAdapter(executable, root)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	p := profile.Profile{ID: id, EngineID: profile.DefaultEngineID, ProfilePath: filepath.ToSlash(filepath.Join(id, "browser-data")), FingerprintSeed: "12345"}

	direct, err := adapter.LaunchSpecWithNetwork(p, brnetwork.EngineProxy{Mode: brnetwork.ModeDirect})
	if err != nil {
		t.Fatalf("LaunchSpecWithNetwork(direct) error = %v", err)
	}
	for _, argument := range direct.Arguments {
		lower := strings.ToLower(argument)
		if strings.HasPrefix(lower, "--proxy-server") || strings.HasPrefix(lower, "--fingerprint-timezone") || strings.HasPrefix(lower, "--fingerprint-locale") || strings.HasPrefix(lower, "--lang") {
			t.Fatalf("direct launch included proxy-locality argument %q", argument)
		}
	}

	locality := &brnetwork.Locality{ExitIP: "203.0.113.70", CountryCode: "US", City: "Los Angeles", Timezone: "America/Los_Angeles", Locale: "en-US"}
	proxy, err := adapter.LaunchSpecWithNetwork(p, brnetwork.EngineProxy{Mode: brnetwork.ModeProxy, Scheme: brnetwork.SchemeHTTP, Host: "127.0.0.1", Port: 48123, Locality: locality})
	if err != nil {
		t.Fatalf("LaunchSpecWithNetwork(proxy) error = %v", err)
	}
	wantProxyArguments := map[string]bool{
		"--proxy-server=http://127.0.0.1:48123":      false,
		"--fingerprint-timezone=America/Los_Angeles": false,
		"--fingerprint-locale=en-US":                 false,
		"--lang=en-US":                               false,
	}
	for _, argument := range proxy.Arguments {
		if _, ok := wantProxyArguments[argument]; ok {
			wantProxyArguments[argument] = true
		}
		if strings.Contains(argument, "@") {
			t.Fatalf("proxy launch argument contains credentials")
		}
	}
	for argument, found := range wantProxyArguments {
		if !found {
			t.Fatalf("proxy launch arguments = %v, want %q", proxy.Arguments, argument)
		}
	}
}

func TestLaunchSpecWithNetworkFailsClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	id := "br_0123456789abcdef0123456789abcdef"
	if err := os.MkdirAll(filepath.Join(root, id, "browser-data"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	executable := filepath.Join(t.TempDir(), "cloak.exe")
	if err := os.WriteFile(executable, nil, 0o700); err != nil {
		t.Fatalf("WriteFile(executable) error = %v", err)
	}
	adapter, err := NewAdapter(executable, root)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	p := profile.Profile{ID: id, EngineID: profile.DefaultEngineID, ProfilePath: filepath.ToSlash(filepath.Join(id, "browser-data")), FingerprintSeed: "12345"}

	invalid := []brnetwork.EngineProxy{
		{Mode: brnetwork.ModeProxy},
		{Mode: brnetwork.ModeProxy, Scheme: "ftp", Host: "127.0.0.1", Port: 1},
		{Mode: brnetwork.ModeProxy, Scheme: brnetwork.SchemeHTTP, Host: "127.0.0.1", Port: 1},
		{Mode: brnetwork.ModeProxy, Scheme: brnetwork.SchemeHTTP, Host: "127.0.0.1", Port: 1, Locality: &brnetwork.Locality{ExitIP: "203.0.113.70", CountryCode: "US", Timezone: "", Locale: "en-US"}},
		{Mode: brnetwork.ModeProxy, Scheme: brnetwork.SchemeHTTP, Host: "127.0.0.1", Port: 1, Locality: &brnetwork.Locality{ExitIP: "203.0.113.70", CountryCode: "US", Timezone: "America/Los_Angeles", Locale: ""}},
		{Mode: "automatic"},
	}
	for _, config := range invalid {
		if _, err := adapter.LaunchSpecWithNetwork(p, config); err == nil {
			t.Fatalf("LaunchSpecWithNetwork(%#v) error = nil, want fail closed", config)
		}
	}
}

func TestLaunchSpecRejectsInvalidProfileIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("MkdirAll(root) error = %v", err)
	}
	executable := filepath.Join(t.TempDir(), "cloak.exe")
	if err := os.WriteFile(executable, nil, 0o700); err != nil {
		t.Fatalf("WriteFile(executable) error = %v", err)
	}
	adapter, err := NewAdapter(executable, root)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	tests := []profile.Profile{
		{ID: "../escape", ProfilePath: "../escape/browser-data", FingerprintSeed: "12345"},
		{ID: "br_0123456789abcdef0123456789abcdef", ProfilePath: "../escape", FingerprintSeed: "12345"},
		{ID: "br_0123456789abcdef0123456789abcdef", ProfilePath: "br_0123456789abcdef0123456789abcdef/browser-data", FingerprintSeed: ""},
	}
	for _, p := range tests {
		if _, err := adapter.LaunchSpec(p); err == nil {
			t.Fatalf("LaunchSpec(%#v) error = nil, want validation failure", p)
		}
	}
}
