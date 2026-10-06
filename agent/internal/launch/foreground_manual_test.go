//go:build windows && manual

// Manual check that a launched app comes to the front even though the agent is a
// background process. Opens small windows on the desktop, so it is excluded from normal
// test runs:  go test -tags manual -run Foreground -v ./internal/launch
package launch

import (
	"fmt"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procFindWindowW         = user32.NewProc("FindWindowW")
)

func windowText(hwnd uintptr) string {
	buf := make([]uint16, 256)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func foregroundTitle() string {
	h, _, _ := procGetForegroundWindow.Call()
	return windowText(h)
}

// launchProbe opens a WinForms window titled `title` and reports whether it became the
// foreground window.
func launchProbe(t *testing.T, th *comThread, title string, unlock bool) bool {
	t.Helper()
	args := fmt.Sprintf(`-NoProfile -WindowStyle Hidden -Command "Add-Type -AssemblyName System.Windows.Forms; $f=New-Object Windows.Forms.Form; $f.Text='%s'; $f.Width=360; $f.Height=120; [void]$f.ShowDialog()"`, title)
	err := th.do(func() error {
		if unlock {
			unlockForeground()
		}
		return shellExecute("powershell.exe", args, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	tp, _ := windows.UTF16PtrFromString(title)
	var hwnd uintptr
	front := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if hwnd == 0 {
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(tp)))
			continue
		}
		time.Sleep(700 * time.Millisecond) // give activation time to settle
		front = foregroundTitle() == title
		break
	}
	if hwnd == 0 {
		t.Fatalf("window %q never appeared", title)
	}
	t.Logf("unlock=%v: foreground=%q, probe in front=%v", unlock, foregroundTitle(), front)
	procPostMessageW.Call(hwnd, 0x0010 /* WM_CLOSE */, 0, 0)
	time.Sleep(500 * time.Millisecond)
	return front
}

func TestForegroundLaunch(t *testing.T) {
	th := newCOMThread()
	t.Logf("foreground before: %q", foregroundTitle())
	baseline := launchProbe(t, th, "barphone-fgtest-plain", false)
	withUnlock := launchProbe(t, th, "barphone-fgtest-unlock", true)
	t.Logf("baseline in front: %v, with unlock: %v", baseline, withUnlock)
	if !withUnlock {
		t.Error("launched window did not come to the foreground")
	}
}
