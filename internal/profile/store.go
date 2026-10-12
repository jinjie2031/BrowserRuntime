package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultEngineID    = "cloak"
	metadataFileName   = "profile.json"
	browserDataDirName = "browser-data"

	profileIDRandomBytes = 16
	maxNameCharacters    = 80
	maxMetadataBytes     = 1 << 20
	fingerprintSeedFloor = 10_000
	fingerprintSeedRange = 90_000
)

var (
	ErrInvalidName            = errors.New("invalid profile name")
	ErrDuplicateName          = errors.New("profile name already exists")
	ErrInvalidProfileID       = errors.New("invalid profile ID")
	ErrProfileNotFound        = errors.New("profile not found")
	ErrInvalidProfileMetadata = errors.New("invalid profile metadata")

	validProfileID       = regexp.MustCompile(`^br_[0-9a-f]{32}$`)
	validFingerprintSeed = regexp.MustCompile(`^[1-9][0-9]{4}$`)
)

// Store persists BrowserRuntime profiles beneath one injected profiles root.
type Store struct {
	root       string
	mu         sync.RWMutex
	renameFile func(string, string) error
	now        func() time.Time
}

// NewStore creates or opens a filesystem-backed profile store.
func NewStore(profilesRoot string) (*Store, error) {
	if strings.TrimSpace(profilesRoot) == "" {
		return nil, errors.New("profiles root is required")
	}

	absoluteRoot, err := filepath.Abs(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve profiles root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create profiles root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical profiles root: %w", err)
	}

	return &Store{
		root:       filepath.Clean(canonicalRoot),
		renameFile: os.Rename,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// Create creates a profile and its browser-data directory.
func (s *Store) Create(name string) (*Profile, error) {
	normalizedName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.listUnlocked()
	if err != nil {
		return nil, err
	}
	if nameExists(profiles, normalizedName, "") {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateName, normalizedName)
	}

	for attempts := 0; attempts < 16; attempts++ {
		id, err := generateProfileID()
		if err != nil {
			return nil, err
		}
		profileDir, err := s.profileDir(id)
		if err != nil {
			return nil, err
		}
		if err := os.Mkdir(profileDir, 0o700); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("create profile directory: %w", err)
		}

		created := false
		defer func() {
			if !created {
				_ = os.RemoveAll(profileDir)
			}
		}()

		if err := os.Mkdir(filepath.Join(profileDir, browserDataDirName), 0o700); err != nil {
			return nil, fmt.Errorf("create browser-data directory: %w", err)
		}
		now := s.now().UTC()
		fingerprintSeed, err := generateFingerprintSeed()
		if err != nil {
			return nil, err
		}
		profile := &Profile{
			ID:              id,
			Name:            normalizedName,
			EngineID:        DefaultEngineID,
			ProfilePath:     relativeBrowserDataPath(id),
			FingerprintSeed: fingerprintSeed,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.writeProfileAtomic(profileDir, profile); err != nil {
			return nil, fmt.Errorf("persist profile metadata: %w", err)
		}

		created = true
		return profile, nil
	}

	return nil, errors.New("generate unique profile ID: collision limit reached")
}

// List returns every valid persisted profile in deterministic ID order.
func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listUnlocked()
}

func (s *Store) listUnlocked() ([]Profile, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list profiles root: %w", err)
	}

	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validProfileID.MatchString(entry.Name()) {
			continue
		}
		profile, err := s.readProfileUnlocked(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("list profile %s: %w", entry.Name(), err)
		}
		profiles = append(profiles, *profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

// Get reads one profile by its BrowserRuntime-generated ID.
func (s *Store) Get(id string) (*Profile, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readProfileUnlocked(id)
}

// Rename updates a profile name while preserving its identity and browser data.
func (s *Store) Rename(id string, newName string) (*Profile, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	normalizedName, err := normalizeName(newName)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	profile, err := s.readProfileUnlocked(id)
	if err != nil {
		return nil, err
	}
	profiles, err := s.listUnlocked()
	if err != nil {
		return nil, err
	}
	if nameExists(profiles, normalizedName, id) {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateName, normalizedName)
	}

	updated := *profile
	updated.Name = normalizedName
	updated.UpdatedAt = s.now().UTC()
	if !updated.UpdatedAt.After(profile.UpdatedAt) {
		updated.UpdatedAt = profile.UpdatedAt.Add(time.Nanosecond)
	}
	profileDir, err := s.profileDir(id)
	if err != nil {
		return nil, err
	}
	if err := s.writeProfileAtomic(profileDir, &updated); err != nil {
		return nil, fmt.Errorf("persist renamed profile metadata: %w", err)
	}
	return &updated, nil
}

// Delete removes one validated, BrowserRuntime-owned profile directory.
func (s *Store) Delete(id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	profileDir, err := s.profileDir(id)
	if err != nil {
		return err
	}
	info, err := os.Lstat(profileDir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("inspect profile directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: profile directory is not an owned directory", ErrInvalidProfileMetadata)
	}

	canonicalDir, err := filepath.EvalSymlinks(profileDir)
	if err != nil {
		return fmt.Errorf("resolve profile directory: %w", err)
	}
	if !pathWithinRoot(s.root, canonicalDir) || !samePath(profileDir, canonicalDir) {
		return fmt.Errorf("%w: profile directory escapes profiles root", ErrInvalidProfileMetadata)
	}
	if _, err := s.readProfileUnlocked(id); err != nil {
		return err
	}
	if err := os.RemoveAll(profileDir); err != nil {
		return fmt.Errorf("delete profile directory: %w", err)
	}
	return nil
}

func (s *Store) readProfileUnlocked(id string) (*Profile, error) {
	metadataPath, err := s.metadataPath(id)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(metadataPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrProfileNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("open profile metadata: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(file, maxMetadataBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read profile metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close profile metadata: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("%w: profile.json exceeds size limit", ErrInvalidProfileMetadata)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("%w: decode profile.json: %v", ErrInvalidProfileMetadata, err)
	}
	if profile.FingerprintSeed == "" {
		if err := validatePersistedProfile(id, &profile, true); err != nil {
			return nil, err
		}
		seed, err := generateFingerprintSeed()
		if err != nil {
			return nil, err
		}
		profile.FingerprintSeed = seed
		previousUpdatedAt := profile.UpdatedAt
		profile.UpdatedAt = s.now().UTC()
		if !profile.UpdatedAt.After(previousUpdatedAt) {
			profile.UpdatedAt = previousUpdatedAt.Add(time.Nanosecond)
		}
		profileDir, err := s.profileDir(id)
		if err != nil {
			return nil, err
		}
		if err := s.writeProfileAtomic(profileDir, &profile); err != nil {
			return nil, fmt.Errorf("persist fingerprint seed migration: %w", err)
		}
	}
	if err := validatePersistedProfile(id, &profile, false); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *Store) writeProfileAtomic(profileDir string, profile *Profile) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile metadata: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(profileDir, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary metadata: %w", err)
	}
	temporaryPath := temporary.Name()
	renamed := false
	defer func() {
		_ = temporary.Close()
		if !renamed {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary metadata permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary metadata: %w", err)
	}
	if err := s.renameFile(temporaryPath, filepath.Join(profileDir, metadataFileName)); err != nil {
		return fmt.Errorf("atomic rename metadata: %w", err)
	}
	renamed = true
	return nil
}

func (s *Store) profileDir(id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	directory := filepath.Join(s.root, id)
	if !pathWithinRoot(s.root, directory) {
		return "", fmt.Errorf("%w: %q", ErrInvalidProfileID, id)
	}
	return directory, nil
}

func (s *Store) metadataPath(id string) (string, error) {
	directory, err := s.profileDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, metadataFileName), nil
}

func normalizeName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: invalid UTF-8", ErrInvalidName)
	}
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if utf8.RuneCountInString(normalized) > maxNameCharacters {
		return "", fmt.Errorf("%w: name exceeds %d characters", ErrInvalidName, maxNameCharacters)
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: name contains a control character", ErrInvalidName)
		}
	}
	return normalized, nil
}

func nameExists(profiles []Profile, name string, exceptID string) bool {
	for _, profile := range profiles {
		if profile.ID != exceptID && strings.EqualFold(profile.Name, name) {
			return true
		}
	}
	return false
}

func generateProfileID() (string, error) {
	random := make([]byte, profileIDRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate secure profile ID: %w", err)
	}
	return "br_" + hex.EncodeToString(random), nil
}

func generateFingerprintSeed() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(fingerprintSeedRange))
	if err != nil {
		return "", fmt.Errorf("generate secure fingerprint seed: %w", err)
	}
	return strconv.FormatInt(value.Int64()+fingerprintSeedFloor, 10), nil
}

// ValidateID rejects every ID not generated by BrowserRuntime.
func ValidateID(id string) error {
	if !validProfileID.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidProfileID, id)
	}
	return nil
}

// ValidateFingerprintSeed checks the persisted CloakBrowser seed format.
func ValidateFingerprintSeed(seed string) error {
	if !validFingerprintSeed.MatchString(seed) {
		return fmt.Errorf("%w: invalid fingerprint seed", ErrInvalidProfileMetadata)
	}
	return nil
}

func relativeBrowserDataPath(id string) string {
	return filepath.ToSlash(filepath.Join(id, browserDataDirName))
}

func validatePersistedProfile(expectedID string, profile *Profile, allowMissingSeed bool) error {
	if !validProfileID.MatchString(profile.ID) || profile.ID != expectedID {
		return fmt.Errorf("%w: profile ID does not match directory", ErrInvalidProfileMetadata)
	}
	normalizedName, err := normalizeName(profile.Name)
	if err != nil || normalizedName != profile.Name {
		return fmt.Errorf("%w: invalid persisted name", ErrInvalidProfileMetadata)
	}
	if strings.TrimSpace(profile.EngineID) == "" {
		return fmt.Errorf("%w: engine ID is empty", ErrInvalidProfileMetadata)
	}
	if profile.ProfilePath != relativeBrowserDataPath(expectedID) || filepath.IsAbs(profile.ProfilePath) {
		return fmt.Errorf("%w: profile path is not the generated relative path", ErrInvalidProfileMetadata)
	}
	if profile.FingerprintSeed == "" && !allowMissingSeed {
		return fmt.Errorf("%w: fingerprint seed is missing", ErrInvalidProfileMetadata)
	}
	if profile.FingerprintSeed != "" {
		if err := ValidateFingerprintSeed(profile.FingerprintSeed); err != nil {
			return err
		}
	}
	if profile.CreatedAt.IsZero() || profile.UpdatedAt.IsZero() || profile.UpdatedAt.Before(profile.CreatedAt) {
		return fmt.Errorf("%w: invalid timestamps", ErrInvalidProfileMetadata)
	}
	return nil
}

func pathWithinRoot(root string, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) || relative == "." {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(first string, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}
