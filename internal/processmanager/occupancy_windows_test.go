//go:build windows

package processmanager

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestOccupancyExactWindowsPathsAndUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Profile With Spaces", "browser-data")
	for _, tc := range []struct {
		name string
		args []string
		want Status
	}{
		{"spaces", []string{"chrome.exe", "--user-data-dir=" + path}, StatusExternalRunning},
		{"case", []string{"chrome.exe", "--user-data-dir", strings.ToUpper(path)}, StatusExternalRunning},
		{"different", []string{"chrome.exe", "--user-data-dir=" + path + "-other"}, StatusStopped},
		{"missing", []string{"chrome.exe"}, StatusUnknown},
		{"duplicate", []string{"chrome.exe", "--user-data-dir=" + path, "--user-data-dir=" + path}, StatusUnknown},
		{"empty-duplicate", []string{"chrome.exe", "--user-data-dir=", "--user-data-dir=" + path + "-other"}, StatusUnknown},
		{"relative", []string{"chrome.exe", "--user-data-dir=relative"}, StatusUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := []browserObservation{{pid: 10, created: 100, commandLine: windows.ComposeCommandLine(tc.args)}}
			if got := classifyBrowserOccupancy(path, observations); got != tc.want {
				t.Fatal("incorrect occupancy classification; command line intentionally withheld")
			}
		})
	}
	if got := classifyBrowserOccupancy(path, []browserObservation{{unreadable: true}}); got != StatusUnknown {
		t.Fatal("unreadable candidate must fail closed")
	}
}

func TestOccupancyChildOwnershipAndPIDReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "browser-data")
	root := browserObservation{pid: 10, created: 100, commandLine: windows.ComposeCommandLine([]string{"chrome.exe", "--user-data-dir=" + path})}
	child := browserObservation{pid: 11, parentPID: 10, created: 200, commandLine: "chrome.exe --type=renderer"}
	if classifyBrowserOccupancy(path, []browserObservation{root, child}) != StatusExternalRunning {
		t.Fatal("child ownership lost")
	}
	root.created = 300 // reused parent PID must not establish ownership
	root.commandLine = windows.ComposeCommandLine([]string{"chrome.exe", "--user-data-dir=" + path + "-other"})
	if classifyBrowserOccupancy(path, []browserObservation{root, child}) != StatusUnknown {
		t.Fatal("reused parent PID was trusted")
	}
}

func TestOccupancyFailureAndGuardedAction(t *testing.T) {
	m := newProcessManager(&fakeLauncher{}, newFakeLeaseProvider(), time.Second)
	m.externalOccupancy = func(string) (Status, error) { return StatusStopped, errors.New("synthetic private process metadata") }
	if status, err := m.Status(validTestProfileID); err != nil || status != StatusUnknown {
		t.Fatal("scan failure did not become safe unknown")
	}
	called := false
	if m.WithStoppedProfiles([]string{validTestProfileID}, func() error { called = true; return nil }) == nil || called {
		t.Fatal("unknown occupancy allowed action")
	}
}

func TestOccupancyLeaseHeldAcrossCheckAndAction(t *testing.T) {
	le := windowsMutexLeaseProvider{}
	m := newProcessManager(&fakeLauncher{}, le, time.Second)
	m.externalOccupancy = func(string) (Status, error) { return StatusStopped, nil }
	if err := m.WithStoppedProfiles([]string{validTestProfileID}, func() error {
		if lease, err := le.Acquire(validTestProfileID); err == nil {
			lease.Release()
			t.Fatal("another manager acquired lease during action")
		}
		return nil
	}); err != nil {
		t.Fatal("safe stopped operation rejected")
	}
	lease, err := le.Acquire(validTestProfileID)
	if err != nil {
		t.Fatal("guard leaked lease")
	}
	lease.Release()
}

func TestOccupancyNativeCommandLineRead(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("executable lookup failed")
	}
	executable, err = canonicalWindowsPath(executable)
	if err != nil {
		t.Fatal("executable normalization failed")
	}
	observation, candidate, err := inspectWindowsBrowser(uint32(os.Getpid()), 0, executable)
	t.Logf("candidate=%t read_error=%t creation_observed=%t command_line_observed=%t", candidate, err != nil, observation.created != 0, observation.commandLine != "")
	if err != nil || !candidate || observation.created == 0 || observation.commandLine == "" {
		t.Fatal("native read failed; raw command line withheld")
	}
}

func TestR3LeaseHelper(t *testing.T) {
	if os.Getenv("BR_R3_LEASE_HELPER") != "1" {
		return
	}
	lease, err := (windowsMutexLeaseProvider{}).Acquire(validTestProfileID)
	if err != nil {
		fmt.Println("BUSY")
		os.Exit(0)
	}
	fmt.Println("READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
	lease.Release()
	os.Exit(0)
}

func startLeaseHelper(t *testing.T) (string, func()) {
	t.Helper()
	executable, _ := os.Executable()
	command := exec.Command(executable, "-test.run=^TestR3LeaseHelper$")
	command.Env = append(os.Environ(), "BR_R3_LEASE_HELPER=1")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdin, _ := command.StdinPipe()
	stdout, _ := command.StdoutPipe()
	if command.Start() != nil {
		t.Fatal("isolated lease helper start failed")
	}
	finished := false
	stop := func() {
		if !finished {
			finished = true
			_ = stdin.Close()
			if command.Wait() != nil {
				t.Error("isolated lease helper failed")
			}
		}
	}
	t.Cleanup(stop)
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- strings.TrimSpace(line) }()
	select {
	case result := <-ready:
		return result, stop
	case <-time.After(5 * time.Second):
		t.Fatal("isolated lease helper timeout")
	}
	return "", stop
}

func TestOccupancyCrossProcessLease(t *testing.T) {
	m := newTestProcessManager(&fakeLauncher{}, windowsMutexLeaseProvider{}, time.Second)
	if m.WithStoppedProfiles([]string{validTestProfileID}, func() error {
		result, stop := startLeaseHelper(t)
		stop()
		if result != "BUSY" {
			t.Fatal("cross-process check-to-action lease gap")
		}
		return nil
	}) != nil {
		t.Fatal("guarded operation failed")
	}
	result, stop := startLeaseHelper(t)
	if result != "READY" {
		t.Fatal("external manager could not acquire idle lease")
	}
	if status, err := m.Status(validTestProfileID); err != nil || status != StatusInUse {
		t.Fatal("external manager occupancy missed")
	}
	if m.WithStoppedProfiles([]string{validTestProfileID}, func() error { t.Fatal("external lease allowed mutation"); return nil }) == nil {
		t.Fatal("external lease accepted")
	}
	stop()
	if status, err := m.Status(validTestProfileID); err != nil || status != StatusStopped {
		t.Fatal("released lease did not restore verified stopped")
	}
}
