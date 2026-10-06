//go:build windows

// Command probewin opens a few small top-level windows and waits; a stand-in app for
// testing window switching without touching real programs:
//
//	go run ./tools/probewin "Probe A" "Probe B"
//	go run ./tools/probewin -fg      # print the foreground window's title and exit
package main

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procRegisterClass = user32.NewProc("RegisterClassExW")
	procCreateWindow  = user32.NewProc("CreateWindowExW")
	procDefWindowProc = user32.NewProc("DefWindowProcW")
	procGetMessage    = user32.NewProc("GetMessageW")
	procTranslate     = user32.NewProc("TranslateMessage")
	procDispatch      = user32.NewProc("DispatchMessageW")
	procGetWindowText = user32.NewProc("GetWindowTextW")
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-fg" {
		buf := make([]uint16, 512)
		procGetWindowText.Call(uintptr(windows.GetForegroundWindow()), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		os.Stdout.WriteString(windows.UTF16ToString(buf) + "\n")
		return
	}
	runtime.LockOSThread()
	titles := os.Args[1:]
	if len(titles) == 0 {
		titles = []string{"Probe A - barphone probe", "Probe B - barphone probe"}
	}
	var inst windows.Handle
	windows.GetModuleHandleEx(0, nil, &inst)
	class, _ := windows.UTF16PtrFromString("barphoneProbe")
	wc := wndClassEx{
		lpfnWndProc: windows.NewCallback(func(h, m, w, l uintptr) uintptr {
			r, _, _ := procDefWindowProc.Call(h, m, w, l)
			return r
		}),
		hInstance:     inst,
		hbrBackground: windows.Handle(6), // COLOR_WINDOW+1
		lpszClassName: class,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))
	const wsOverlappedWindow, wsVisible = 0x00CF0000, 0x10000000
	for i, t := range titles {
		title, _ := windows.UTF16PtrFromString(t)
		procCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
			wsOverlappedWindow|wsVisible, uintptr(80+40*i), uintptr(80+40*i), 420, 140, 0, 0, uintptr(inst), 0)
	}
	var m msg
	for {
		if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); r == 0 || int32(r) == -1 {
			return
		}
		procTranslate.Call(uintptr(unsafe.Pointer(&m)))
		procDispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}
