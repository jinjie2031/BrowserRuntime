//go:build windows

// Package portable anchors all instance data to the executable, never the CWD,
// environment overrides, AppData, or another application's cache.
package portable

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

var ErrUnavailable = errors.New("Portable folder is unavailable or not writable. Move the entire BrowserRuntime folder to a writable local folder.")

type Layout struct{ Root, Data, Engines, Profiles, Settings, WebView string }

func Current() (Layout, error) {
	exe, err := os.Executable()
	if err != nil {
		return Layout{}, ErrUnavailable
	}
	return FromExecutable(exe)
}

func FromExecutable(exe string) (Layout, error) {
	if !filepath.IsAbs(exe) {
		return Layout{}, ErrUnavailable
	}
	root := filepath.Dir(filepath.Clean(exe))
	data := filepath.Join(root, "data")
	settings := filepath.Join(data, "settings")
	return Layout{root, data, filepath.Join(data, "engines"), filepath.Join(data, "profiles"), settings, filepath.Join(settings, "webview2")}, nil
}

func (l Layout) Prepare() error {
	// Probe the root too: never use an existing writable data directory to hide
	// a read-only portable root. No ACL changes or alternative locations.
	for _, p := range []string{l.Root, l.Data, l.Engines, l.Profiles, l.Settings, l.WebView} {
		if checkAncestors(p) != nil || os.MkdirAll(p, 0700) != nil {
			return ErrUnavailable
		}
		f, err := os.CreateTemp(p, ".writable-")
		if err != nil {
			return ErrUnavailable
		}
		closeErr := f.Close()
		removeErr := os.Remove(f.Name())
		if closeErr != nil || removeErr != nil {
			return ErrUnavailable
		}
	}
	return nil
}

func checkAncestors(path string) error {
	if !filepath.IsAbs(path) {
		return ErrUnavailable
	}
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		ptr, err := windows.UTF16PtrFromString(p)
		if err != nil {
			return ErrUnavailable
		}
		attrs, err := windows.GetFileAttributes(ptr)
		if err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return ErrUnavailable
		}
		if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return ErrUnavailable
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

// CheckTree is used only for engine content, rejecting redirects both above
// and inside the engine tree. It never changes filesystem permissions.
func CheckTree(path string) error {
	if checkAncestors(path) != nil {
		return ErrUnavailable
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return ErrUnavailable
		}
		ptr, e := windows.UTF16PtrFromString(p)
		if e != nil {
			return ErrUnavailable
		}
		attrs, e := windows.GetFileAttributes(ptr)
		if e != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return ErrUnavailable
		}
		return nil
	})
}
