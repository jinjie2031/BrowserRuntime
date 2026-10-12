//go:build windows

package processmanager

import (
	"fmt"
	goruntime "runtime"
	"sync"
	"syscall"
	"unsafe"

	"browserruntime/internal/profile"
)

const (
	waitObject0   = 0x00000000
	waitAbandoned = 0x00000080
	waitTimeout   = 0x00000102
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW  = kernel32.NewProc("CreateMutexW")
	procWaitForSingle = kernel32.NewProc("WaitForSingleObject")
	procReleaseMutex  = kernel32.NewProc("ReleaseMutex")
)

type windowsMutexLeaseProvider struct{}

type windowsMutexLease struct {
	release chan struct{}
	done    chan struct{}
	once    sync.Once
}

type leaseAcquisition struct {
	lease profileLease
	err   error
}

func (windowsMutexLeaseProvider) Acquire(profileID string) (profileLease, error) {
	if err := profile.ValidateID(profileID); err != nil {
		return nil, err
	}

	mutexName, err := syscall.UTF16PtrFromString("Local\\BrowserRuntime.Profile." + profileID)
	if err != nil {
		return nil, fmt.Errorf("encode profile mutex name: %w", err)
	}
	result := make(chan leaseAcquisition, 1)
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		goruntime.LockOSThread()
		defer goruntime.UnlockOSThread()
		defer close(done)

		handleValue, _, createErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
		if handleValue == 0 {
			result <- leaseAcquisition{err: fmt.Errorf("create profile mutex: %w", createErr)}
			return
		}
		handle := syscall.Handle(handleValue)
		defer syscall.CloseHandle(handle)

		waitResult, _, waitErr := procWaitForSingle.Call(handleValue, 0)
		switch waitResult {
		case waitObject0, waitAbandoned:
			lease := &windowsMutexLease{release: release, done: done}
			result <- leaseAcquisition{lease: lease}
			<-release
			_, _, _ = procReleaseMutex.Call(handleValue)
		case waitTimeout:
			result <- leaseAcquisition{err: ErrProfileAlreadyRunning}
		default:
			result <- leaseAcquisition{err: fmt.Errorf("acquire profile mutex: %w", waitErr)}
		}
	}()

	acquired := <-result
	return acquired.lease, acquired.err
}

func (l *windowsMutexLease) Release() {
	l.once.Do(func() { close(l.release) })
	<-l.done
}
