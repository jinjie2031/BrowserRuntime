//go:build windows

package main

import (
	"os"
	"strings"
	"testing"
)

func TestPortablePackageAndStartupContract(t *testing.T) {
	for _, file := range []string{"main.go", "bootstrap_windows.go", "engine_bootstrap_windows.go"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"internal/installation", "installerCommand", "BR_PROFILES_ROOT", "BR_CLOAK_EXECUTABLE", "UserHomeDir", "remote-debugging", "enable-automation"} {
			if strings.Contains(string(b), forbidden) {
				t.Fatalf("startup includes forbidden dependency: %s", forbidden)
			}
		}
	}
	manifest, _ := os.ReadFile("build/windows/wails.exe.manifest")
	if !strings.Contains(string(manifest), `level="asInvoker"`) {
		t.Fatal("portable privilege contract missing")
	}
	script, _ := os.ReadFile("scripts/build-portable.ps1")
	if !strings.Contains(string(script), "-webview2 error") || strings.Contains(string(script), "-nsis") || strings.Contains(string(script), "Compress-Archive") {
		t.Fatal("unsafe package contract")
	}
	if strings.Count(string(script), "CreateEntryFromFile") != 2 {
		t.Fatal("release allowlist changed")
	}
}
