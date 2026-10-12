//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStandaloneTimezoneData(t *testing.T) {
	if os.Getenv("BR_PORTABLE_TZ_CHILD") == "1" {
		for _, zone := range []string{"America/Los_Angeles", "Asia/Shanghai"} {
			if _, err := time.LoadLocation(zone); err != nil {
				t.Fatal("IANA timezone unavailable without Go installation")
			}
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BR_PORTABLE_TZ_CHILD", "1")
	t.Setenv("GOROOT", t.TempDir())
	t.Setenv("ZONEINFO", filepath.Join(t.TempDir(), "absent"))
	if err = exec.Command(exe, "-test.run=^TestStandaloneTimezoneData$").Run(); err != nil {
		t.Fatal("standalone timezone data required")
	}
}
