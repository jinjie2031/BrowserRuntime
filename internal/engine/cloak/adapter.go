package cloak

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"browserruntime/internal/engine"
	brnetwork "browserruntime/internal/network"
	"browserruntime/internal/profile"
)

// Adapter builds direct native CloakBrowser launch specifications.
type Adapter struct {
	executablePath string
	profilesRoot   string
}

// NewAdapter configures a CloakBrowser native adapter.
func NewAdapter(executablePath string, profilesRoot string) (*Adapter, error) {
	if strings.TrimSpace(executablePath) == "" {
		return nil, errors.New("CloakBrowser executable path is required")
	}
	absoluteExecutable, err := filepath.Abs(executablePath)
	if err != nil {
		return nil, fmt.Errorf("resolve CloakBrowser executable: %w", err)
	}
	info, err := os.Stat(absoluteExecutable)
	if err != nil {
		return nil, fmt.Errorf("inspect CloakBrowser executable: %w", err)
	}
	if info.IsDir() {
		return nil, errors.New("CloakBrowser executable path is a directory")
	}

	if strings.TrimSpace(profilesRoot) == "" {
		return nil, errors.New("profiles root is required")
	}
	absoluteRoot, err := filepath.Abs(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve profiles root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical profiles root: %w", err)
	}

	return &Adapter{
		executablePath: filepath.Clean(absoluteExecutable),
		profilesRoot:   filepath.Clean(canonicalRoot),
	}, nil
}

// LaunchSpec returns a headed, direct-process specification for one profile.
func (a *Adapter) LaunchSpec(p profile.Profile) (engine.LaunchSpec, error) {
	return a.LaunchSpecWithNetwork(p, brnetwork.EngineProxy{Mode: brnetwork.ModeDirect})
}

// LaunchSpecWithNetwork returns a native specification using only a
// credential-free proxy endpoint. Authenticated upstream credentials must stay
// behind a loopback network bridge and never enter the browser command line.
func (a *Adapter) LaunchSpecWithNetwork(p profile.Profile, network brnetwork.EngineProxy) (engine.LaunchSpec, error) {
	if err := network.ValidateForLaunch(); err != nil {
		return engine.LaunchSpec{}, err
	}
	if err := profile.ValidateID(p.ID); err != nil {
		return engine.LaunchSpec{}, err
	}
	if p.EngineID != profile.DefaultEngineID {
		return engine.LaunchSpec{}, fmt.Errorf("profile engine %q is not supported by Cloak adapter", p.EngineID)
	}
	if err := profile.ValidateFingerprintSeed(p.FingerprintSeed); err != nil {
		return engine.LaunchSpec{}, err
	}

	expectedRelative := filepath.ToSlash(filepath.Join(p.ID, "browser-data"))
	if p.ProfilePath != expectedRelative || filepath.IsAbs(p.ProfilePath) {
		return engine.LaunchSpec{}, errors.New("profile path is not the generated relative browser-data path")
	}
	browserDataPath := filepath.Join(a.profilesRoot, filepath.FromSlash(p.ProfilePath))
	if !pathWithinRoot(a.profilesRoot, browserDataPath) {
		return engine.LaunchSpec{}, errors.New("profile path escapes profiles root")
	}
	info, err := os.Stat(browserDataPath)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.LaunchSpec{}, errors.New("profile browser-data directory does not exist")
	}
	if err != nil {
		return engine.LaunchSpec{}, fmt.Errorf("inspect profile browser-data directory: %w", err)
	}
	if !info.IsDir() {
		return engine.LaunchSpec{}, errors.New("profile browser-data path is not a directory")
	}
	canonicalBrowserData, err := filepath.EvalSymlinks(browserDataPath)
	if err != nil {
		return engine.LaunchSpec{}, fmt.Errorf("resolve profile browser-data directory: %w", err)
	}
	if !pathWithinRoot(a.profilesRoot, canonicalBrowserData) || !samePath(browserDataPath, canonicalBrowserData) {
		return engine.LaunchSpec{}, errors.New("profile browser-data directory is redirected")
	}

	arguments := []string{
		"--user-data-dir=" + browserDataPath,
		"--fingerprint=" + p.FingerprintSeed,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--new-window",
	}
	if network.Mode == brnetwork.ModeProxy {
		arguments = append(arguments,
			"--proxy-server="+network.ServerURL(),
			"--fingerprint-timezone="+network.Locality.Timezone,
			"--fingerprint-locale="+network.Locality.Locale,
			"--lang="+network.Locality.Locale,
		)
	}
	spec := engine.LaunchSpec{
		ExecutablePath:  a.executablePath,
		Arguments:       arguments,
		ProfilePath:     browserDataPath,
		FingerprintSeed: p.FingerprintSeed,
	}
	if err := engine.ValidateLaunchSpec(spec); err != nil {
		return engine.LaunchSpec{}, err
	}
	return spec, nil
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
