//go:build windows

package processmanager

import (
	"errors"
	"fmt"
	"os/exec"
	goruntime "runtime"
	"syscall"
	"time"
	"unsafe"

	"browserruntime/internal/engine"
)

const (
	windowRestore      = 9
	windowMessageClose = 0x0010
	getWindowOwner     = 4
	attachInput        = 1
	detachInput        = 0
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindow                = user32.NewProc("GetWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetActiveWindow          = user32.NewProc("SetActiveWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procSwitchToThisWindow       = user32.NewProc("SwitchToThisWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
)

var errBrowserWindowNotFound = errors.New("browser top-level window not found")

type windowsNativeLauncher struct{}

type windowsNativeProcess struct {
	command *exec.Cmd
	pid     uint32
}

func (windowsNativeLauncher) Start(spec engine.LaunchSpec) (managedProcess, error) {
	if err := engine.ValidateLaunchSpec(spec); err != nil {
		return nil, err
	}
	command := exec.Command(spec.ExecutablePath, spec.Arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &windowsNativeProcess{command: command, pid: uint32(command.Process.Pid)}, nil
}

func (p *windowsNativeProcess) PID() int { return int(p.pid) }

func (p *windowsNativeProcess) Wait() error { return p.command.Wait() }

func (p *windowsNativeProcess) OpenWindow() error {
	windows, err := topLevelWindowsForPID(p.pid)
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		return errBrowserWindowNotFound
	}
	if !activateWindow(windows[0], p.pid) {
		return errors.New("Windows denied browser foreground activation")
	}
	return nil
}

func activateWindow(window syscall.Handle, pid uint32) bool {
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	currentThread, _, _ := procGetCurrentThreadID.Call()
	foregroundWindow, _, _ := procGetForegroundWindow.Call()
	foregroundThread, _, _ := procGetWindowThreadProcessID.Call(foregroundWindow, 0)
	targetThread, _, _ := procGetWindowThreadProcessID.Call(uintptr(window), 0)

	attachedForeground := foregroundThread != 0 && foregroundThread != currentThread
	attachedTarget := targetThread != 0 && targetThread != currentThread && targetThread != foregroundThread
	if attachedForeground {
		_, _, _ = procAttachThreadInput.Call(currentThread, foregroundThread, attachInput)
	}
	if attachedTarget {
		_, _, _ = procAttachThreadInput.Call(currentThread, targetThread, attachInput)
	}
	defer func() {
		if attachedTarget {
			_, _, _ = procAttachThreadInput.Call(currentThread, targetThread, detachInput)
		}
		if attachedForeground {
			_, _, _ = procAttachThreadInput.Call(currentThread, foregroundThread, detachInput)
		}
	}()

	iconic, _, _ := procIsIconic.Call(uintptr(window))
	if iconic != 0 {
		_, _, _ = procShowWindow.Call(uintptr(window), windowRestore)
	}
	_, _, _ = procBringWindowToTop.Call(uintptr(window))
	_, _, _ = procSetActiveWindow.Call(uintptr(window))
	_, _, _ = procSetFocus.Call(uintptr(window))
	_, _, _ = procSetForegroundWindow.Call(uintptr(window))
	time.Sleep(50 * time.Millisecond)
	if foregroundBelongsToPID(pid) {
		return true
	}

	_, _, _ = procSwitchToThisWindow.Call(uintptr(window), 1)
	time.Sleep(50 * time.Millisecond)
	return foregroundBelongsToPID(pid)
}

func foregroundBelongsToPID(pid uint32) bool {
	foreground, _, _ := procGetForegroundWindow.Call()
	if foreground == 0 {
		return false
	}
	var foregroundPID uint32
	_, _, _ = procGetWindowThreadProcessID.Call(foreground, uintptr(unsafe.Pointer(&foregroundPID)))
	return foregroundPID == pid
}

func (p *windowsNativeProcess) CloseWindow() error {
	windows, err := topLevelWindowsForPID(p.pid)
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		return errBrowserWindowNotFound
	}

	posted := false
	for _, window := range windows {
		result, _, _ := procPostMessageW.Call(uintptr(window), windowMessageClose, 0, 0)
		posted = posted || result != 0
	}
	if !posted {
		return errors.New("failed to post graceful close to browser window")
	}
	return nil
}

func topLevelWindowsForPID(pid uint32) ([]syscall.Handle, error) {
	windows := make([]syscall.Handle, 0, 1)
	callback := syscall.NewCallback(func(window uintptr, _ uintptr) uintptr {
		var windowPID uint32
		_, _, _ = procGetWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID != pid {
			return 1
		}
		visible, _, _ := procIsWindowVisible.Call(window)
		owner, _, _ := procGetWindow.Call(window, getWindowOwner)
		if visible != 0 && owner == 0 {
			windows = append(windows, syscall.Handle(window))
		}
		return 1
	})
	result, _, callErr := procEnumWindows.Call(callback, 0)
	if result == 0 {
		return nil, fmt.Errorf("enumerate browser windows: %w", callErr)
	}
	return windows, nil
}
