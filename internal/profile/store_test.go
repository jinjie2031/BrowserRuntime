package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var profileIDPattern = regexp.MustCompile(`^br_[0-9a-f]{32}$`)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "profiles")
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store, root
}

func TestFingerprintSeedPersistsAcrossRenameAndStoreRecreation(t *testing.T) {
	store, root := newTestStore(t)
	created, err := store.Create("Stable identity")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if matched, _ := regexp.MatchString(`^[1-9][0-9]{4}$`, created.FingerprintSeed); !matched {
		t.Fatalf("Create() FingerprintSeed = %q, want five-digit secure seed", created.FingerprintSeed)
	}

	renamed, err := store.Rename(created.ID, "Stable identity renamed")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if renamed.FingerprintSeed != created.FingerprintSeed {
		t.Fatalf("Rename() seed = %q, want %q", renamed.FingerprintSeed, created.FingerprintSeed)
	}

	reopened, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(reopen) error = %v", err)
	}
	got, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() after recreation error = %v", err)
	}
	if got.FingerprintSeed != created.FingerprintSeed {
		t.Fatalf("Get() seed = %q, want stable %q", got.FingerprintSeed, created.FingerprintSeed)
	}
}

func TestGetMigratesLegacyProfileSeedExactlyOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	id := "br_0123456789abcdef0123456789abcdef"
	profileDir := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(profileDir, browserDataDirName), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	createdAt := time.Now().UTC().Add(-time.Hour)
	updatedAt := time.Now().UTC().Add(time.Hour)
	legacy := map[string]any{
		"id":           id,
		"name":         "Legacy profile",
		"engine_id":    DefaultEngineID,
		"profile_path": filepath.ToSlash(filepath.Join(id, browserDataDirName)),
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal(legacy) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, metadataFileName), data, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	migrated, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get(legacy) error = %v", err)
	}
	if matched, _ := regexp.MatchString(`^[1-9][0-9]{4}$`, migrated.FingerprintSeed); !matched {
		t.Fatalf("migrated seed = %q, want five-digit seed", migrated.FingerprintSeed)
	}
	if !migrated.UpdatedAt.After(updatedAt) {
		t.Fatalf("migrated UpdatedAt = %v, want after prior %v", migrated.UpdatedAt, updatedAt)
	}
	firstSeed := migrated.FingerprintSeed

	reopened, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(reopen) error = %v", err)
	}
	readAgain, err := reopened.Get(id)
	if err != nil {
		t.Fatalf("Get(migrated) error = %v", err)
	}
	if readAgain.FingerprintSeed != firstSeed {
		t.Fatalf("second Get() seed = %q, want one-time seed %q", readAgain.FingerprintSeed, firstSeed)
	}

	persisted, err := os.ReadFile(filepath.Join(profileDir, metadataFileName))
	if err != nil {
		t.Fatalf("ReadFile(migrated) error = %v", err)
	}
	var persistedFields map[string]any
	if err := json.Unmarshal(persisted, &persistedFields); err != nil {
		t.Fatalf("Unmarshal(migrated) error = %v", err)
	}
	if persistedFields["fingerprint_seed"] != firstSeed {
		t.Fatalf("persisted fingerprint_seed = %v, want %q", persistedFields["fingerprint_seed"], firstSeed)
	}
}

func TestCreatePersistsProfileAndBrowserData(t *testing.T) {
	store, root := newTestStore(t)

	created, err := store.Create("  Primary profile  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !profileIDPattern.MatchString(created.ID) {
		t.Fatalf("Create() ID = %q, want generated br_ ID", created.ID)
	}
	if created.Name != "Primary profile" {
		t.Errorf("Create() Name = %q, want trimmed name", created.Name)
	}
	if created.EngineID != DefaultEngineID {
		t.Errorf("Create() EngineID = %q, want %q", created.EngineID, DefaultEngineID)
	}
	wantRelativePath := filepath.ToSlash(filepath.Join(created.ID, browserDataDirName))
	if created.ProfilePath != wantRelativePath {
		t.Errorf("Create() ProfilePath = %q, want %q", created.ProfilePath, wantRelativePath)
	}
	if filepath.IsAbs(created.ProfilePath) {
		t.Errorf("Create() ProfilePath = %q, want relative path", created.ProfilePath)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Create() timestamps must be populated")
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("Create() CreatedAt = %v, UpdatedAt = %v; want equal", created.CreatedAt, created.UpdatedAt)
	}
	if created.LastLaunchedAt != nil {
		t.Errorf("Create() LastLaunchedAt = %v, want nil", created.LastLaunchedAt)
	}

	profileDir := filepath.Join(root, created.ID)
	if info, err := os.Stat(filepath.Join(profileDir, browserDataDirName)); err != nil || !info.IsDir() {
		t.Fatalf("browser-data directory: info = %v, err = %v", info, err)
	}

	data, err := os.ReadFile(filepath.Join(profileDir, metadataFileName))
	if err != nil {
		t.Fatalf("ReadFile(profile.json) error = %v", err)
	}
	var persisted Profile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("profile.json is invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(persisted, *created) {
		t.Errorf("persisted profile = %#v, want %#v", persisted, *created)
	}
}

func TestCreateGeneratesUniqueIDs(t *testing.T) {
	store, _ := newTestStore(t)

	first, err := store.Create("First")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := store.Create("Second")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	if first.ID == second.ID {
		t.Fatalf("Create() generated duplicate ID %q", first.ID)
	}
}

func TestListAndGet(t *testing.T) {
	store, _ := newTestStore(t)
	first, err := store.Create("First")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := store.Create("Second")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("List() returned %d profiles, want 2", len(profiles))
	}
	listed := map[string]Profile{}
	for _, profile := range profiles {
		listed[profile.ID] = profile
	}
	if !reflect.DeepEqual(listed[first.ID], *first) || !reflect.DeepEqual(listed[second.ID], *second) {
		t.Errorf("List() = %#v, want created profiles", profiles)
	}

	got, err := store.Get(first.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(*got, *first) {
		t.Errorf("Get() = %#v, want %#v", *got, *first)
	}
}

func TestRenamePersistsAfterStoreRecreation(t *testing.T) {
	store, root := newTestStore(t)
	created, err := store.Create("Original")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	renamed, err := store.Rename(created.ID, "  Renamed  ")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if renamed.Name != "Renamed" {
		t.Errorf("Rename() Name = %q, want %q", renamed.Name, "Renamed")
	}
	if !renamed.CreatedAt.Equal(created.CreatedAt) {
		t.Error("Rename() changed CreatedAt")
	}
	if renamed.UpdatedAt.Before(created.UpdatedAt) {
		t.Error("Rename() moved UpdatedAt backwards")
	}

	reopened, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(reopen) error = %v", err)
	}
	got, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() after recreation error = %v", err)
	}
	if !reflect.DeepEqual(*got, *renamed) {
		t.Errorf("Get() after recreation = %#v, want %#v", *got, *renamed)
	}
}

func TestDeleteRemovesOnlySelectedProfile(t *testing.T) {
	store, root := newTestStore(t)
	deleted, err := store.Create("Delete me")
	if err != nil {
		t.Fatalf("Create(delete) error = %v", err)
	}
	kept, err := store.Create("Keep me")
	if err != nil {
		t.Fatalf("Create(keep) error = %v", err)
	}

	if err := store.Delete(deleted.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, deleted.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted profile directory still exists or stat failed: %v", err)
	}
	if _, err := store.Get(deleted.ID); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrProfileNotFound", err)
	}
	if _, err := store.Get(kept.ID); err != nil {
		t.Fatalf("Get(kept) error = %v", err)
	}
}

func TestCreateRejectsInvalidNames(t *testing.T) {
	store, root := newTestStore(t)
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "whitespace", value: " \t\r\n "},
		{name: "over 80 characters", value: strings.Repeat("界", 81)},
		{name: "control character", value: "bad\u0000name"},
		{name: "invalid UTF-8", value: string([]byte{0xff})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Create(tt.value); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("Create(%q) error = %v, want ErrInvalidName", tt.value, err)
			}
		})
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid creates left %d entries in root", len(entries))
	}
}

func TestNamesAreUniqueCaseInsensitively(t *testing.T) {
	store, _ := newTestStore(t)
	first, err := store.Create("Alpha")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := store.Create("Beta")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	if _, err := store.Create("  alpha  "); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Create(duplicate) error = %v, want ErrDuplicateName", err)
	}
	if _, err := store.Rename(second.ID, "ALPHA"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Rename(duplicate) error = %v, want ErrDuplicateName", err)
	}
	if _, err := store.Rename(first.ID, "ALPHA"); err != nil {
		t.Fatalf("Rename(same profile casing) error = %v", err)
	}
}

func TestOperationsRejectInvalidAndTraversalIDs(t *testing.T) {
	store, root := newTestStore(t)
	outsidePath := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}

	invalidIDs := []string{
		"",
		"profile-1",
		"../outside.txt",
		"..\\outside.txt",
		filepath.Join(root, "br_0123456789abcdef0123456789abcdef"),
		"br_0123456789abcdef0123456789abcdeg",
	}
	for _, id := range invalidIDs {
		t.Run(strings.ReplaceAll(id, "\\", "_"), func(t *testing.T) {
			if _, err := store.Get(id); !errors.Is(err, ErrInvalidProfileID) {
				t.Errorf("Get(%q) error = %v, want ErrInvalidProfileID", id, err)
			}
			if _, err := store.Rename(id, "Valid"); !errors.Is(err, ErrInvalidProfileID) {
				t.Errorf("Rename(%q) error = %v, want ErrInvalidProfileID", id, err)
			}
			if err := store.Delete(id); !errors.Is(err, ErrInvalidProfileID) {
				t.Errorf("Delete(%q) error = %v, want ErrInvalidProfileID", id, err)
			}
		})
	}

	data, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatalf("outside file was removed: %v", err)
	}
	if string(data) != "keep" {
		t.Fatalf("outside file changed to %q", data)
	}
}

func TestGetRejectsTamperedProfilePath(t *testing.T) {
	store, root := newTestStore(t)
	created, err := store.Create("Tamper check")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	created.ProfilePath = "../outside"
	data, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, created.ID, metadataFileName), data, 0o600); err != nil {
		t.Fatalf("WriteFile(tampered metadata) error = %v", err)
	}

	if _, err := store.Get(created.ID); !errors.Is(err, ErrInvalidProfileMetadata) {
		t.Fatalf("Get(tampered metadata) error = %v, want ErrInvalidProfileMetadata", err)
	}
}

func TestAtomicMetadataWriteFailurePreservesExistingJSON(t *testing.T) {
	store, root := newTestStore(t)
	created, err := store.Create("Original")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	metadataPath := filepath.Join(root, created.ID, metadataFileName)
	before, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	store.renameFile = func(_, _ string) error {
		return errors.New("injected rename failure")
	}
	if _, err := store.Rename(created.ID, "Changed"); err == nil {
		t.Fatal("Rename() error = nil, want injected failure")
	}

	after, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("failed atomic rename changed existing profile.json")
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(root, created.ID, ".profile-*.tmp"))
	if err != nil {
		t.Fatalf("Glob(temp files) error = %v", err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("failed atomic rename left temporary files: %v", temporaryFiles)
	}
}
