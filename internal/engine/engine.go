package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidLaunchSpec       = errors.New("invalid native launch specification")
	ErrForbiddenLaunchArgument = errors.New("forbidden automation or CDP launch argument")
)

var forbiddenLaunchFlags = []string{
	"--remote-debugging-port",
	"--remote-debugging-pipe",
	"--remote-debugging-address",
	"--enable-automation",
}

// LaunchSpec is the complete input for a direct native browser process launch.
type LaunchSpec struct {
	ExecutablePath  string
	Arguments       []string
	ProfilePath     string
	FingerprintSeed string
}

// ForbiddenLaunchFlags returns a copy of the flags that native adapters must reject.
func ForbiddenLaunchFlags() []string {
	flags := make([]string, len(forbiddenLaunchFlags))
	copy(flags, forbiddenLaunchFlags)
	return flags
}

// ValidateLaunchSpec verifies a specification without launching a process.
func ValidateLaunchSpec(spec LaunchSpec) error {
	if strings.TrimSpace(spec.ExecutablePath) == "" || !filepath.IsAbs(spec.ExecutablePath) {
		return fmt.Errorf("%w: executable path must be absolute", ErrInvalidLaunchSpec)
	}
	for _, argument := range spec.Arguments {
		flag := strings.ToLower(argument)
		if equalsOrAssignsForbiddenFlag(flag) {
			return fmt.Errorf("%w: %s", ErrForbiddenLaunchArgument, strings.SplitN(flag, "=", 2)[0])
		}
	}
	return nil
}

func equalsOrAssignsForbiddenFlag(argument string) bool {
	for _, forbidden := range forbiddenLaunchFlags {
		if argument == forbidden || strings.HasPrefix(argument, forbidden+"=") {
			return true
		}
	}
	return false
}
