//go:build windows

package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"barphone/agent/internal/store"
)

// Finding "the windows of this button's app": a window belongs to the app when its
// process runs the app's exe, or when the window carries the app's AppUserModelID
// (Store apps, whose windows live in ApplicationFrameHost.exe).

var (
	procGetWindowTextW              = user32.NewProc("GetWindowTextW")
	procGetWindow                   = user32.NewProc("GetWindow")
	procGetWindowLongPtrW           = user32.NewProc("GetWindowLongPtrW")
	procIsIconic                    = user32.NewProc("IsIconic")
	procShowWindow                  = user32.NewProc("ShowWindow")
	procSetForegroundWindow         = user32.NewProc("SetForegroundWindow")
	procGetAncestor                 = user32.NewProc("GetAncestor")
	procGetClassNameW               = user32.NewProc("GetClassNameW")
	procDwmGetWindowAttribute       = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	procSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	procPropVariantClear            = windows.NewLazySystemDLL("ole32.dll").NewProc("PropVariantClear")

	iidPropertyStore   = windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	iidShellItem2      = windows.GUID{Data1: 0x7e9fb0d3, Data2: 0x919f, Data3: 0x4307, Data4: [8]byte{0xab, 0x2e, 0x9b, 0x18, 0x60, 0x31, 0x0c, 0x93}}
	pkeyAppUserModelID = propertyKey{windows.GUID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}, 5}
	pkeyLinkTargetPath = propertyKey{windows.GUID{Data1: 0xB9B4B3FC, Data2: 0x2B51, Data3: 0x4A42, Data4: [8]byte{0xB5, 0xD8, 0x32, 0x41, 0x46, 0xAF, 0xCF, 0x25}}, 2}

	knownFolderAppID = regexp.MustCompile(`^(\{[0-9A-Fa-f-]{36}\})\\(.+)$`)
)

const (
	gwOwner          = 4
	gwlExStyle       = ^uintptr(19) // -20
	wsExToolWindow   = 0x00000080
	dwmwaCloaked     = 14
	swRestore        = 9
	swMinimize       = 6
	gaRootOwner      = 3
	vtLPWSTR         = 31
	processQueryInfo = 0x1000 // PROCESS_QUERY_LIMITED_INFORMATION
)

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

// propVariant mirrors PROPVARIANT (24 bytes on 64-bit).
type propVariant struct {
	vt      uint16
	_, _, _ uint16
	val     *uint16
	_       uintptr
}

type appMatch struct {
	exes    map[string]bool // lower-cased absolute exe paths
	aumids  map[string]bool // lower-cased AppUserModelIDs
	classes map[string]bool // lower-cased window classes
}

func (m appMatch) empty() bool { return len(m.exes) == 0 && len(m.aumids) == 0 && len(m.classes) == 0 }

// resolveMatch works out which exe/AUMID a button launches. Runs on a COM thread.
func resolveMatch(b store.Button) appMatch {
	m := appMatch{exes: map[string]bool{}, aumids: map[string]bool{}, classes: map[string]bool{}}
	addExe := func(p string) {
		if strings.EqualFold(filepath.Ext(p), ".exe") {
			m.exes[strings.ToLower(filepath.Clean(p))] = true
		}
	}
	switch b.Kind {
	case store.KindApp:
		m.aumids[strings.ToLower(b.Target)] = true
		if strings.EqualFold(b.Target, "Microsoft.Windows.Explorer") {
			// File Explorer shares explorer.exe with the taskbar and desktop.
			m.classes["cabinetwclass"] = true
			return m
		}
		if sm := knownFolderAppID.FindStringSubmatch(b.Target); sm != nil {
			// Desktop apps without an explicit AUMID: "{KNOWNFOLDERID}\rel\path.exe".
			if g, err := windows.GUIDFromString(sm[1]); err == nil {
				if dir, err := windows.KnownFolderPath((*windows.KNOWNFOLDERID)(&g), 0); err == nil {
					addExe(filepath.Join(dir, sm[2]))
				}
			}
		} else if p := shellItemString(`shell:AppsFolder\`+b.Target, &pkeyLinkTargetPath); p != "" {
			addExe(p) // Start-menu shortcut with an AUMID, e.g. VS Code
		}
	case store.KindPath:
		p, _, _ := resolve(b)
		if strings.EqualFold(filepath.Ext(p), ".lnk") {
			p = shellItemString(p, &pkeyLinkTargetPath)
		} else if !filepath.IsAbs(p) {
			if lp, err := exec.LookPath(p); err == nil {
				p = lp
			}
		}
		addExe(p)
	}
	return m
}

// shellItemString reads a string property of a shell item ("" if unavailable).
func shellItemString(parsingName string, key *propertyKey) string {
	name, err := windows.UTF16PtrFromString(parsingName)
	if err != nil {
		return ""
	}
	var item unsafe.Pointer
	hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&iidShellItem2)), uintptr(unsafe.Pointer(&item)))
	if hr != 0 || item == nil {
		return ""
	}
	vtbl := *(**[18]uintptr)(item)
	defer syscall.SyscallN(vtbl[2], uintptr(item)) // Release
	var out *uint16
	if hr, _, _ := syscall.SyscallN(vtbl[17], uintptr(item), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&out))); hr != 0 || out == nil {
		return "" // IShellItem2::GetString
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(out))
	return windows.UTF16PtrToString(out)
}

// windowAUMID reads the AppUserModelID stamped on a window ("" if none).
func windowAUMID(h windows.HWND) string {
	var ps unsafe.Pointer
	if hr, _, _ := procSHGetPropertyStoreForWindow.Call(uintptr(h), uintptr(unsafe.Pointer(&iidPropertyStore)), uintptr(unsafe.Pointer(&ps))); hr != 0 || ps == nil {
		return ""
	}
	vtbl := *(**[8]uintptr)(ps)
	defer syscall.SyscallN(vtbl[2], uintptr(ps)) // Release
	var pv propVariant
	if hr, _, _ := syscall.SyscallN(vtbl[5], uintptr(ps), uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&pv))); hr != 0 {
		return "" // IPropertyStore::GetValue
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	if pv.vt != vtLPWSTR || pv.val == nil {
		return ""
	}
	return windows.UTF16PtrToString(pv.val)
}

var (
	enumMu  sync.Mutex
	enumOut []windows.HWND
	enumCB  = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		enumOut = append(enumOut, h)
		return 1
	})
)

// topWindows returns top-level windows in z-order (front first).
func topWindows() []windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumOut = enumOut[:0]
	windows.EnumWindows(enumCB, nil)
	return append([]windows.HWND(nil), enumOut...)
}

// switchable applies the Alt+Tab rules: visible, unowned, not a tool window, not cloaked
// (suspended Store apps and windows on other virtual desktops are cloaked), has a title.
func switchable(h windows.HWND) (title string, ok bool) {
	title, cloaked, ok := switchableAnyDesktop(h)
	return title, ok && !cloaked
}

// switchableAnyDesktop is switchable, but tells a cloaked window instead of skipping it:
// the caller decides whether it is a window on another virtual desktop.
func switchableAnyDesktop(h windows.HWND) (title string, cloaked, ok bool) {
	if !windows.IsWindowVisible(h) {
		return "", false, false
	}
	if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner != 0 {
		return "", false, false
	}
	if ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle); ex&wsExToolWindow != 0 {
		return "", false, false
	}
	var c uint32
	if hr, _, _ := procDwmGetWindowAttribute.Call(uintptr(h), dwmwaCloaked, uintptr(unsafe.Pointer(&c)), 4); hr == 0 && c != 0 {
		cloaked = true
	}
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return "", false, false
	}
	return windows.UTF16ToString(buf[:n]), cloaked, true
}

func windowClass(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func processExe(pid uint32, cache map[uint32]string) string {
	if exe, ok := cache[pid]; ok {
		return exe
	}
	exe := ""
	if h, err := windows.OpenProcess(processQueryInfo, false, pid); err == nil {
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil {
			exe = strings.ToLower(filepath.Clean(windows.UTF16ToString(buf[:size])))
		}
		windows.CloseHandle(h)
	}
	cache[pid] = exe
	return exe
}

// matchingWindows lists the switchable windows of the button's app on every virtual
// desktop, as if there were one. COM thread only.
func matchingWindows(b store.Button) []Window {
	if b.Kind == store.KindURL {
		return nil
	}
	m := resolveMatch(b)
	if m.empty() {
		return nil
	}
	self := uint32(os.Getpid())
	exes := map[uint32]string{}
	front := frontWindow()
	vdm := newDesktopManager()
	defer vdm.release()
	ids, cur := desktopList()
	var out []Window
	for _, h := range topWindows() {
		title, cloaked, ok := switchableAnyDesktop(h)
		if !ok {
			continue
		}
		desktop := 0
		if cloaked {
			if desktop = otherDesktop(vdm, ids, cur, h); desktop == 0 {
				continue // cloaked for another reason: a suspended Store app and the like
			}
		}
		var pid uint32
		windows.GetWindowThreadProcessId(h, &pid)
		if pid == self {
			continue
		}
		hit := m.classes[strings.ToLower(windowClass(h))] || m.exes[processExe(pid, exes)]
		if !hit && len(m.aumids) > 0 {
			hit = m.aumids[strings.ToLower(windowAUMID(h))]
		}
		if hit {
			minimized, _, _ := procIsIconic.Call(uintptr(h))
			out = append(out, Window{ID: hwndID(h), Title: title, Active: h == front && minimized == 0, Desktop: desktop})
		}
	}
	return out
}

// frontWindow is the top-level window in front: a dialog or child that has focus counts
// as its owner window.
func frontWindow() windows.HWND {
	fg := windows.GetForegroundWindow()
	if fg == 0 {
		return 0
	}
	if root, _, _ := procGetAncestor.Call(uintptr(fg), gaRootOwner); root != 0 {
		return windows.HWND(root)
	}
	return fg
}

// Minimize minimizes a window of the button's app (all of them for windowID ""). As with
// Focus, only windows that belong to the app are accepted.
func (w *winLauncher) Minimize(b store.Button, windowID string) error {
	return w.launchThread.do(func() error {
		done := false
		for _, win := range matchingWindows(b) {
			if windowID != "" && win.ID != windowID {
				continue
			}
			v, _ := strconv.ParseUint(win.ID, 10, 64)
			procShowWindow.Call(uintptr(v), swMinimize) // the next window in z-order gets focus
			done = true
		}
		if !done {
			return ErrWindowGone
		}
		return nil
	})
}

func (w *winLauncher) Windows(b store.Button) ([]Window, error) {
	var out []Window
	err := w.launchThread.do(func() error { out = matchingWindows(b); return nil })
	return out, err
}

// Focus only accepts a window that still belongs to the button's app, so the phone can
// never be used to poke at arbitrary windows.
func (w *winLauncher) Focus(b store.Button, windowID string) error {
	return w.launchThread.do(func() error {
		for _, win := range matchingWindows(b) {
			if win.ID != windowID {
				continue
			}
			v, _ := strconv.ParseUint(windowID, 10, 64)
			h := windows.HWND(uintptr(v))
			if win.Desktop > 0 {
				w.goToDesktop(win.Desktop) // "as if one desktop": go where the window is
			}
			if minimized, _, _ := procIsIconic.Call(uintptr(h)); minimized != 0 {
				procShowWindow.Call(uintptr(h), swRestore)
			}
			unlockForeground()
			procSetForegroundWindow.Call(uintptr(h))
			return nil
		}
		return ErrWindowGone
	})
}

func hwndID(h windows.HWND) string { return strconv.FormatUint(uint64(h), 10) }
