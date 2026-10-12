package network

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafetyMissingConfigurationNeverInitializesDirect(t *testing.T) {
	root := t.TempDir()
	p := createTestProfile(t, root, "Uninitialized historical profile")
	s := newTestStore(t, root)
	if _, err := s.Get(p.ID); err == nil {
		t.Fatal("uninitialized profile guessed Direct")
	}
	if _, err := os.Stat(filepath.Join(root, p.ID, configFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing configuration recreated")
	}
	if err := s.SetDirect(p.ID); err != nil {
		t.Fatal("explicit Direct initialization failed")
	}
	if cfg, err := s.Get(p.ID); err != nil || cfg.Mode != ModeDirect {
		t.Fatal("explicit Direct did not persist")
	}
}
