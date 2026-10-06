//go:build windows

package launch

import (
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// A Google Meet call is a Meet window (a browser tab "Meet – abc-defg-hij" in front of
// its window, or the Meet app) whose process is using the microphone or the camera right
// now. Windows tracks that for the tray icon; it is read from the registry, never changed.

// Meet puts a no-break space (U+00A0) after "Meet" in its title: RE2's \s is ASCII
// only, so spaces are matched as any Unicode space.
var meetTitle = regexp.MustCompile(`(?i)^(google[\s\p{Zs}]+)?meet[\s\p{Zs}]*[-–—][\s\p{Zs}]*\S`)

const consentStore = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\`

// deviceInUse: is this exe using the "microphone" or "webcam" right now?
func deviceInUse(device, exe string) bool {
	if exe == "" {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, consentStore+device+`\NonPackaged\`+strings.ReplaceAll(exe, `\`, "#"), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	start, _, err1 := k.GetIntegerValue("LastUsedTimeStart")
	stop, _, err2 := k.GetIntegerValue("LastUsedTimeStop")
	return err1 == nil && err2 == nil && start != 0 && stop == 0
}

// findMeetWindow returns the Meet window (on any virtual desktop) and its process' exe.
func findMeetWindow() (windows.HWND, string) {
	self := uint32(os.Getpid())
	exes := map[uint32]string{}
	for _, h := range topWindows() {
		title, _, ok := switchableAnyDesktop(h)
		if !ok || !meetTitle.MatchString(title) {
			continue
		}
		var pid uint32
		windows.GetWindowThreadProcessId(h, &pid)
		if pid == self {
			continue
		}
		return h, processExe(pid, exes)
	}
	return 0, ""
}

func (w *winLauncher) Call() (CallInfo, error) {
	var info CallInfo
	err := w.launchThread.do(func() error {
		h, exe := findMeetWindow()
		if h == 0 {
			return nil
		}
		mic, cam := deviceInUse("microphone", exe), deviceInUse("webcam", exe)
		if mic || cam { // a Meet tab outside a call holds neither
			info = CallInfo{Active: true, App: "meet", Camera: cam}
		}
		return nil
	})
	return info, err
}

// Meet's own shortcuts; they only work in its window.
var meetKeys = map[string]Combo{
	"mic":    {Ctrl: true, Key: "D"},
	"camera": {Ctrl: true, Key: "E"},
	"hand":   {Ctrl: true, Alt: true, Key: "H"},
	// Meet has no shortcut to leave: closing its tab (the one in front in that window, as
	// the title says) ends the call.
	"leave": {Ctrl: true, Key: "W"},
}

// CallAction brings the Meet window up for a moment, presses the shortcut and puts the
// previous window (and desktop) back; "show" just brings Meet up and stays.
func (w *winLauncher) CallAction(action string) error {
	combo, isKey := meetKeys[action]
	if !isKey && action != "show" {
		return ErrUnsupported
	}
	return w.launchThread.do(func() error {
		h, _ := findMeetWindow()
		if h == 0 {
			return ErrWindowGone
		}
		prev := frontWindow()
		vdm := newDesktopManager()
		ids, cur := desktopList()
		desktop := otherDesktop(vdm, ids, cur, h)
		vdm.release()
		back := desktopIndex(ids, cur) + 1
		if desktop > 0 {
			w.goToDesktop(desktop)
		}
		minimized, _, _ := procIsIconic.Call(uintptr(h))
		if minimized != 0 {
			procShowWindow.Call(uintptr(h), swRestore)
		}
		unlockForeground()
		procSetForegroundWindow.Call(uintptr(h))
		if !isKey {
			return nil
		}
		time.Sleep(150 * time.Millisecond) // let the page take the keyboard
		err := sendCombo(combo)
		time.Sleep(100 * time.Millisecond)
		if minimized != 0 {
			procShowWindow.Call(uintptr(h), swMinimize)
		}
		if desktop > 0 && back > 0 {
			w.goToDesktop(back)
		}
		if prev != 0 && prev != h {
			unlockForeground()
			procSetForegroundWindow.Call(uintptr(prev))
		}
		return err
	})
}
