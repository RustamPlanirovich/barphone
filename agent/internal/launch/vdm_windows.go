//go:build windows

package launch

import (
	"bytes"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IVirtualDesktopManager (public since Windows 10) tells which virtual desktop a window
// is on. Windows on other desktops are cloaked like suspended Store apps; this is how
// the two are told apart, so the app's windows on every desktop count as its windows.

var (
	clsidVirtualDesktopManager = windows.GUID{Data1: 0xAA509086, Data2: 0x5CA9, Data3: 0x4C25, Data4: [8]byte{0x8F, 0x95, 0x58, 0x9D, 0x3C, 0x07, 0xB4, 0x8A}}
	iidVirtualDesktopManager   = windows.GUID{Data1: 0xA5CD92FF, Data2: 0x29BE, Data3: 0x454C, Data4: [8]byte{0x8D, 0x04, 0xD8, 0x28, 0x79, 0xFB, 0x3F, 0x1B}}
)

type desktopManager struct {
	p  unsafe.Pointer
	vt *[6]uintptr
}

// newDesktopManager needs a COM-initialized thread; nil when unavailable.
func newDesktopManager() *desktopManager {
	var p unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidVirtualDesktopManager)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidVirtualDesktopManager)), uintptr(unsafe.Pointer(&p)))
	if hr != 0 || p == nil {
		return nil
	}
	return &desktopManager{p: p, vt: *(**[6]uintptr)(p)}
}

func (m *desktopManager) release() {
	if m != nil {
		syscall.SyscallN(m.vt[2], uintptr(m.p))
	}
}

// desktopOf is the virtual desktop a window is on; false when it is on none.
func (m *desktopManager) desktopOf(h windows.HWND) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	var id [16]byte
	hr, _, _ := syscall.SyscallN(m.vt[4], uintptr(m.p), uintptr(h), uintptr(unsafe.Pointer(&id))) // GetWindowDesktopId
	if hr != 0 || id == [16]byte{} {
		return nil, false
	}
	return id[:], true
}

// desktopIndex finds a desktop ID in the registry's list (-1 if absent).
func desktopIndex(ids [][]byte, id []byte) int {
	for i, x := range ids {
		if bytes.Equal(x, id) {
			return i
		}
	}
	return -1
}

// otherDesktop tells, for a cloaked window, the 1-based number of the virtual desktop it
// is on when that is not the current one; 0 means "not a window on another desktop".
func otherDesktop(m *desktopManager, ids [][]byte, cur []byte, h windows.HWND) int {
	id, ok := m.desktopOf(h)
	if !ok || bytes.Equal(id, cur) {
		return 0
	}
	return desktopIndex(ids, id) + 1
}

// goToDesktop switches to the desktop with this 1-based number (Win+Ctrl+arrows) and
// waits for the switch; false when it cannot tell where it is now.
func (w *winLauncher) goToDesktop(n int) bool {
	ids, cur := desktopList()
	from := desktopIndex(ids, cur)
	if from < 0 || n < 1 || n > len(ids) {
		return false
	}
	if steps := n - 1 - from; steps != 0 {
		if w.MoveDesktop(steps) != nil {
			return false
		}
		sleepForSwitch()
	}
	return true
}
