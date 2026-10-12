//go:build windows

package processmanager

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Private, ephemeral observations only. Never format these into errors, logs,
// DTOs or diagnostics: command lines can contain unrelated private material.
type browserObservation struct {
	pid, parentPID uint32
	created        uint64
	commandLine    string
	unreadable     bool
}

func canonicalWindowsPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrOccupancyUnknown
	}
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
		short, convertErr := windows.UTF16PtrFromString(path)
		if convertErr != nil {
			return "", ErrOccupancyUnknown
		}
		long := make([]uint16, 32768)
		length, convertErr := windows.GetLongPathName(short, &long[0], uint32(len(long)))
		if convertErr != nil || length == 0 || length >= uint32(len(long)) {
			return "", ErrOccupancyUnknown
		}
		path = windows.UTF16ToString(long[:length])
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrOccupancyUnknown
	}
	path = strings.TrimPrefix(path, `\\?\`)
	return filepath.Clean(path), nil
}

func browserDataArgument(commandLine string) (string, bool, error) {
	if commandLine == "" {
		return "", false, ErrOccupancyUnknown
	}
	args, err := windows.DecomposeCommandLine(commandLine)
	if err != nil || len(args) == 0 {
		return "", false, ErrOccupancyUnknown
	}
	path := ""
	seenPath := false
	child := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--type=") {
			child = true
		}
		if arg == "--user-data-dir" {
			if i+1 >= len(args) || seenPath {
				return "", child, ErrOccupancyUnknown
			}
			seenPath = true
			i++
			path = args[i]
		} else if strings.HasPrefix(arg, "--user-data-dir=") {
			if seenPath {
				return "", child, ErrOccupancyUnknown
			}
			seenPath = true
			path = strings.TrimPrefix(arg, "--user-data-dir=")
		}
	}
	if path == "" {
		return "", child, nil
	}
	path, err = canonicalWindowsPath(path)
	return path, child, err
}

func classifyBrowserOccupancy(expected string, observations []browserObservation) Status {
	expected, err := canonicalWindowsPath(expected)
	if err != nil {
		return StatusUnknown
	}
	byPID := make(map[uint32]browserObservation, len(observations))
	for _, observation := range observations {
		byPID[observation.pid] = observation
	}
	unknown, found := false, false
	for _, observation := range observations {
		current := observation
		resolved := false
		for depth := 0; depth < 32; depth++ {
			if current.unreadable {
				break
			}
			path, child, err := browserDataArgument(current.commandLine)
			if err != nil {
				break
			}
			if path != "" {
				found = found || strings.EqualFold(path, expected)
				resolved = true
				break
			}
			if !child {
				break
			}
			parent, exists := byPID[current.parentPID]
			// A parent created later is a reused PID, not this child's owner.
			if !exists || parent.pid == current.pid || parent.created == 0 || current.created < parent.created {
				break
			}
			current = parent
		}
		unknown = unknown || !resolved
	}
	if found {
		return StatusExternalRunning
	}
	if unknown {
		return StatusUnknown
	}
	return StatusStopped
}

func scanWindowsBrowsers(executable string) ([]browserObservation, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, ErrOccupancyUnknown
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, ErrOccupancyUnknown
	}
	observations := make([]browserObservation, 0)
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(executable)) {
			observation, candidate, err := inspectWindowsBrowser(entry.ProcessID, entry.ParentProcessID, executable)
			if err != nil {
				observation.unreadable = true
				candidate = true
			}
			if candidate {
				observations = append(observations, observation)
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, ErrOccupancyUnknown
		}
	}
	return observations, nil
}

func inspectWindowsBrowser(pid, parent uint32, executable string) (browserObservation, bool, error) {
	observation := browserObservation{pid: pid, parentPID: parent}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return observation, false, nil
		} // exited before OpenProcess
		return observation, true, ErrOccupancyUnknown
	}
	defer windows.CloseHandle(handle)
	if wait, err := windows.WaitForSingleObject(handle, 0); err != nil {
		return observation, true, ErrOccupancyUnknown
	} else if wait == windows.WAIT_OBJECT_0 {
		return observation, false, nil
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if windows.QueryFullProcessImageName(handle, 0, &image[0], &size) != nil {
		return observation, true, ErrOccupancyUnknown
	}
	imagePath, err := canonicalWindowsPath(windows.UTF16ToString(image[:size]))
	if err != nil {
		return observation, true, ErrOccupancyUnknown
	}
	// Recognize the configured binary and installed versions in its Cloak cache.
	// A stock Chrome with the same filename is not assigned to any Profile.
	cache := filepath.Dir(filepath.Dir(executable))
	sameCache := strings.EqualFold(filepath.Base(cache), ".cloakbrowser") && strings.EqualFold(filepath.Dir(filepath.Dir(imagePath)), cache)
	if !strings.EqualFold(imagePath, executable) && !sameCache {
		return observation, false, nil
	}
	var creation, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user) != nil {
		return observation, true, ErrOccupancyUnknown
	}
	observation.created = uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	observation.commandLine, err = readWindowsCommandLine(handle)
	if err != nil {
		return observation, true, ErrOccupancyUnknown
	}
	// The same handle is used throughout; no cached PID is trusted as identity.
	if wait, err := windows.WaitForSingleObject(handle, 0); err != nil {
		return observation, true, ErrOccupancyUnknown
	} else if wait == windows.WAIT_OBJECT_0 {
		return observation, false, nil
	}
	return observation, true, nil
}

// Read only command-line structures, never environment or browser storage.
// PEB/RTL layouts are the pinned x/sys Windows definitions. Unsupported bitness
// or any read/layout failure conservatively blocks rather than guessing.
func readWindowsCommandLine(handle windows.Handle) (string, error) {
	if runtime.GOARCH != "amd64" {
		return "", ErrOccupancyUnknown
	}
	var wow64 bool
	if windows.IsWow64Process(handle, &wow64) != nil || wow64 {
		return "", ErrOccupancyUnknown
	}
	var basic windows.PROCESS_BASIC_INFORMATION
	if windows.NtQueryInformationProcess(handle, windows.ProcessBasicInformation, unsafe.Pointer(&basic), uint32(unsafe.Sizeof(basic)), nil) != nil || basic.PebBaseAddress == nil {
		return "", ErrOccupancyUnknown
	}
	var peb windows.PEB
	if err := readRemote(handle, uintptr(unsafe.Pointer(basic.PebBaseAddress)), unsafe.Pointer(&peb), unsafe.Offsetof(peb.ProcessParameters)+unsafe.Sizeof(peb.ProcessParameters)); err != nil || peb.ProcessParameters == nil {
		return "", ErrOccupancyUnknown
	}
	var parameters windows.RTL_USER_PROCESS_PARAMETERS
	if err := readRemote(handle, uintptr(unsafe.Pointer(peb.ProcessParameters)), unsafe.Pointer(&parameters), unsafe.Offsetof(parameters.CommandLine)+unsafe.Sizeof(parameters.CommandLine)); err != nil {
		return "", ErrOccupancyUnknown
	}
	line := parameters.CommandLine
	if line.Length == 0 || line.Length%2 != 0 || line.Length > line.MaximumLength || line.Buffer == nil {
		return "", ErrOccupancyUnknown
	}
	buffer := make([]uint16, int(line.Length)/2)
	if err := readRemote(handle, uintptr(unsafe.Pointer(line.Buffer)), unsafe.Pointer(&buffer[0]), uintptr(line.Length)); err != nil {
		return "", ErrOccupancyUnknown
	}
	return windows.UTF16ToString(buffer), nil
}

func readRemote(handle windows.Handle, address uintptr, destination unsafe.Pointer, length uintptr) error {
	var read uintptr
	if windows.ReadProcessMemory(handle, address, (*byte)(destination), length, &read) != nil || read != length {
		return ErrOccupancyUnknown
	}
	return nil
}
