//go:build windows

package portable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecutableAnchoredLayout(t *testing.T) {
	root := t.TempDir()
	t.Chdir(t.TempDir())
	l, err := FromExecutable(filepath.Join(root, "BrowserRuntime.exe"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{l.Data, l.Engines, l.Profiles, l.Settings, l.WebView} {
		rel, err := filepath.Rel(root, p)
		if err != nil || !filepath.IsAbs(p) || rel == ".." {
			t.Fatal("path outside portable root")
		}
	}
	if l.Profiles != filepath.Join(root, "data", "profiles") {
		t.Fatal("wrong profiles directory")
	}
	if err = l.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err = l.Prepare(); err != nil {
		t.Fatal("reopen failed")
	}
	if _, err = FromExecutable("BrowserRuntime.exe"); err == nil {
		t.Fatal("relative executable accepted")
	}
}

func TestCannotWriteDoesNotFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	l, _ := FromExecutable(filepath.Join(root, "BrowserRuntime.exe"))
	if l.Prepare() != ErrUnavailable {
		t.Fatal("unwritable root not rejected safely")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("unexpected fallback directory")
	}
}

func TestFolderRelocationPreservesRelativeProfileData(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	if os.Mkdir(first, 0700) != nil {
		t.Fatal("setup")
	}
	l, _ := FromExecutable(filepath.Join(first, "BrowserRuntime.exe"))
	if l.Prepare() != nil {
		t.Fatal("prepare")
	}
	if os.WriteFile(filepath.Join(l.Settings, "canary"), []byte("preference"), 0600) != nil {
		t.Fatal("write")
	}
	moved := filepath.Join(parent, "moved")
	if os.Rename(first, moved) != nil {
		t.Fatal("relocate")
	}
	next, _ := FromExecutable(filepath.Join(moved, "BrowserRuntime.exe"))
	if next.Prepare() != nil {
		t.Fatal("reopen")
	}
	b, err := os.ReadFile(filepath.Join(next.Settings, "canary"))
	if err != nil || string(b) != "preference" {
		t.Fatal("data lost")
	}
}

func TestRejectRedirectedData(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "data")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	l, _ := FromExecutable(filepath.Join(root, "BrowserRuntime.exe"))
	if l.Prepare() != ErrUnavailable {
		t.Fatal("redirected data accepted")
	}
}
