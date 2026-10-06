//go:build windows

package launch

import (
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procLockWorkStation  = user32.NewProc("LockWorkStation")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procExitWindowsEx    = user32.NewProc("ExitWindowsEx")
	procSetSuspendState  = windows.NewLazySystemDLL("powrprof.dll").NewProc("SetSuspendState")
	procCoCreateInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")

	clsidMMDeviceEnumerator = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = windows.GUID{Data1: 0x5CDF2C82, Data2: 0x841E, Data3: 0x4546, Data4: [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}
)

const (
	vkMediaNext, vkMediaPrev, vkMediaStop, vkMediaPlayPause = 0xB0, 0xB1, 0xB2, 0xB3
	vkVolumeMute, vkVolumeDown, vkVolumeUp                  = 0xAD, 0xAE, 0xAF

	clsctxAll         = 0x17
	eRender, eConsole = 0, 0

	hwndBroadcast  = 0xFFFF
	wmSysCommand   = 0x0112
	scMonitorPower = 0xF170

	ewxReboot, ewxShutdown, ewxPowerOff = 0x02, 0x01, 0x08
	shtdnReasonPlanned                  = 0x80000000
)

func (w *winLauncher) runSystem(id string) error {
	a, ok := LookupSystemAction(id)
	if !ok {
		return fmt.Errorf("unknown system action %q", id)
	}
	do := func() error {
		switch id {
		case "media_play_pause":
			return pressVK(vkMediaPlayPause)
		case "media_next":
			return pressVK(vkMediaNext)
		case "media_prev":
			return pressVK(vkMediaPrev)
		case "media_stop":
			return pressVK(vkMediaStop)
		case "volume_up":
			return pressVK(vkVolumeUp) // also shows Windows' volume flyout
		case "volume_down":
			return pressVK(vkVolumeDown)
		case "mute":
			return pressVK(vkVolumeMute)
		case "brightness", "brightness_up", "brightness_down":
			err, _ := brightnessAction(w, id)
			return err
		case "desktop_next":
			return w.MoveDesktop(1)
		case "desktop_prev":
			return w.MoveDesktop(-1)
		case "task_view", "desktops": // a tap on the desktops tile shows them all
			return sendCombo(Combo{Win: true, Key: "Tab"})
		case "volume": // tap on the slider button toggles mute
			return w.launchThread.do(func() error {
				return withVolume(func(v unsafe.Pointer, vt *[16]uintptr) error {
					var muted int32
					if hr, _, _ := syscall.SyscallN(vt[15], uintptr(v), uintptr(unsafe.Pointer(&muted))); int32(hr) < 0 {
						return fmt.Errorf("GetMute: 0x%x", hr)
					}
					return hresult(syscall.SyscallN(vt[14], uintptr(v), uintptr(1-muted), 0)) // SetMute
				})
			})
		case "lock":
			if r, _, err := procLockWorkStation.Call(); r == 0 {
				return err
			}
			return nil
		case "display_off":
			procPostMessageW.Call(hwndBroadcast, wmSysCommand, scMonitorPower, 2)
			return nil
		case "sleep":
			if err := enableShutdownPrivilege(); err != nil {
				return err
			}
			if r, _, err := procSetSuspendState.Call(0, 0, 0); r == 0 {
				return err
			}
			return nil
		case "shutdown", "restart":
			if err := enableShutdownPrivilege(); err != nil {
				return err
			}
			flags := uintptr(ewxShutdown | ewxPowerOff)
			if id == "restart" {
				flags = ewxReboot
			}
			// No EWX_FORCE: apps with unsaved work may still object, as with the Start menu.
			if r, _, err := procExitWindowsEx.Call(flags, shtdnReasonPlanned); r == 0 {
				return err
			}
			return nil
		}
		return ErrUnsupported
	}
	if a.Deferred {
		go func() {
			time.Sleep(deferDelay)
			do()
		}()
		return nil
	}
	return do()
}

func enableShutdownPrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
}

// hresult treats S_FALSE (e.g. "already muted") as success, like the SUCCEEDED macro.
func hresult(hr, _ uintptr, _ syscall.Errno) error {
	if int32(hr) < 0 {
		return fmt.Errorf("HRESULT 0x%08x", uint32(hr))
	}
	return nil
}

// withVolume hands f the default render device's IAudioEndpointVolume. COM thread only.
func withVolume(f func(v unsafe.Pointer, vtbl *[16]uintptr) error) error {
	var enum unsafe.Pointer
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum))); hr != 0 || enum == nil {
		return fmt.Errorf("MMDeviceEnumerator: 0x%08x", uint32(hr))
	}
	ev := *(**[8]uintptr)(enum)
	defer syscall.SyscallN(ev[2], uintptr(enum))

	var dev unsafe.Pointer
	if hr, _, _ := syscall.SyscallN(ev[4], uintptr(enum), eRender, eConsole, uintptr(unsafe.Pointer(&dev))); hr != 0 || dev == nil {
		return fmt.Errorf("no default audio output (0x%08x)", uint32(hr))
	}
	dv := *(**[7]uintptr)(dev)
	defer syscall.SyscallN(dv[2], uintptr(dev))

	var vol unsafe.Pointer
	if hr, _, _ := syscall.SyscallN(dv[3], uintptr(dev), uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)), clsctxAll, 0,
		uintptr(unsafe.Pointer(&vol))); hr != 0 || vol == nil {
		return fmt.Errorf("IAudioEndpointVolume: 0x%08x", uint32(hr))
	}
	vv := *(**[16]uintptr)(vol)
	defer syscall.SyscallN(vv[2], uintptr(vol))
	return f(vol, vv)
}

func (w *winLauncher) Volume() (VolumeState, error) {
	var st VolumeState
	err := w.launchThread.do(func() error {
		return withVolume(func(v unsafe.Pointer, vt *[16]uintptr) error {
			var level float32
			var muted int32
			if err := hresult(syscall.SyscallN(vt[9], uintptr(v), uintptr(unsafe.Pointer(&level)))); err != nil {
				return err
			}
			if err := hresult(syscall.SyscallN(vt[15], uintptr(v), uintptr(unsafe.Pointer(&muted)))); err != nil {
				return err
			}
			st = VolumeState{Level: float64(level), Muted: muted != 0}
			return nil
		})
	})
	return st, err
}

func (w *winLauncher) SetVolume(level float64) error {
	level = math.Max(0, math.Min(1, level))
	return w.launchThread.do(func() error {
		return withVolume(func(v unsafe.Pointer, vt *[16]uintptr) error {
			// SetMasterVolumeLevelScalar(float, guid): the float travels in XMM1, which
			// Go's Windows syscall path fills from the same argument slot.
			if err := hresult(syscall.SyscallN(vt[7], uintptr(v), uintptr(math.Float32bits(float32(level))), 0)); err != nil {
				return err
			}
			return hresult(syscall.SyscallN(vt[14], uintptr(v), 0, 0)) // SetMute(false)
		})
	})
}
