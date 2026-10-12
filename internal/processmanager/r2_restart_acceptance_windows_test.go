//go:build windows

package processmanager

import (
	"testing"
	"time"
)

// A real Windows profile mutex and an existing fake process establish an
// independently owned live profile without starting or killing any browser.
func TestR2UntrackedLiveProfileMustNotReportStopped(t *testing.T) {
	process := newFakeProcess(6002, false)
	first := newTestProcessManager(&fakeLauncher{processes: []*fakeProcess{process}}, windowsMutexLeaseProvider{}, time.Second)
	if err := first.Launch(validTestProfileID, validSpec()); err != nil {
		t.Fatal("simulated browser launch failed")
	}
	defer func() {
		process.exit()
		waitForStatus(t, first, StatusStopped)
	}()
	second := NewProcessManager()
	status, err := second.Status(validTestProfileID)
	if err != nil || status != StatusInUse {
		t.Fatal("BLOCKER: live OS-leased profile is incorrectly reported as STOPPED by a new manager")
	}
}
