//go:build windows

package launch

import (
	"errors"
	"math"
	"sync"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

// Screen brightness: the built-in panel of a laptop through WMI, external monitors
// through DDC/CI. Setting changes them all; reading takes the first one that answers.

var (
	dxva2                                       = windows.NewLazySystemDLL("dxva2.dll")
	procGetNumberOfPhysicalMonitorsFromHMONITOR = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procGetMonitorBrightness                    = dxva2.NewProc("GetMonitorBrightness")
	procSetMonitorBrightness                    = dxva2.NewProc("SetMonitorBrightness")
	procDestroyPhysicalMonitor                  = dxva2.NewProc("DestroyPhysicalMonitor")
	procEnumDisplayMonitors                     = user32.NewProc("EnumDisplayMonitors")

	// EnumDisplayMonitors calls back with each monitor; the callback is created once
	// (Windows callbacks are a limited resource) and collects into monFound.
	monMu    sync.Mutex
	monFound []uintptr
	monCB    = windows.NewCallback(func(hmon, hdc, rect, data uintptr) uintptr {
		monFound = append(monFound, hmon)
		return 1
	})
)

// PHYSICAL_MONITOR (packed: a handle and a 128-character description).
type physicalMonitor struct {
	handle windows.Handle
	desc   [128]uint16
}

func physicalMonitors() []physicalMonitor {
	monMu.Lock()
	monFound = nil
	procEnumDisplayMonitors.Call(0, 0, monCB, 0)
	hmons := monFound
	monMu.Unlock()
	var out []physicalMonitor
	for _, hm := range hmons {
		var n uint32
		if r, _, _ := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(hm, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
			continue
		}
		ms := make([]physicalMonitor, n)
		if r, _, _ := procGetPhysicalMonitorsFromHMONITOR.Call(hm, uintptr(n), uintptr(unsafe.Pointer(&ms[0]))); r == 0 {
			continue
		}
		out = append(out, ms...)
	}
	return out
}

func releaseMonitors(ms []physicalMonitor) {
	for _, m := range ms {
		procDestroyPhysicalMonitor.Call(uintptr(m.handle))
	}
}

// ddcRange reads a monitor's brightness scale over DDC/CI; ok is false when the
// monitor does not support it (many TVs, some docks and adapters).
func ddcRange(m physicalMonitor) (lo, cur, hi uint32, ok bool) {
	r, _, _ := procGetMonitorBrightness.Call(uintptr(m.handle),
		uintptr(unsafe.Pointer(&lo)), uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&hi)))
	return lo, cur, hi, r != 0 && hi > lo
}

// withWMI runs fn with a root\wmi connection; call on a COM-initialized thread.
func withWMI(fn func(svc *ole.IDispatch) error) error {
	unk, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return err
	}
	defer unk.Release()
	loc, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer loc.Release()
	v, err := oleutil.CallMethod(loc, "ConnectServer", ".", `root\wmi`)
	if err != nil {
		return err
	}
	svc := v.ToIDispatch()
	defer svc.Release()
	return fn(svc)
}

// wmiEach runs fn for every object a WMI query returns; it reports how many there were.
func wmiEach(svc *ole.IDispatch, query string, fn func(item *ole.IDispatch) error) (int, error) {
	v, err := oleutil.CallMethod(svc, "ExecQuery", query)
	if err != nil {
		return 0, err
	}
	set := v.ToIDispatch()
	defer set.Release()
	n := 0
	err = oleutil.ForEach(set, func(item *ole.VARIANT) error {
		d := item.ToIDispatch()
		defer d.Release()
		n++
		return fn(d)
	})
	return n, err
}

var errNoBrightness = errors.New("no screen with adjustable brightness (laptop panel or DDC/CI monitor)")

func (w *winLauncher) Brightness() (float64, error) {
	level := -1.0
	err := w.brightThread.do(func() error {
		// A laptop's own screen.
		withWMI(func(svc *ole.IDispatch) error {
			_, err := wmiEach(svc, "SELECT CurrentBrightness FROM WmiMonitorBrightness WHERE Active=TRUE", func(item *ole.IDispatch) error {
				if p, err := oleutil.GetProperty(item, "CurrentBrightness"); err == nil && level < 0 {
					if v, ok := p.Value().(uint8); ok {
						level = float64(v) / 100
					}
				}
				return nil
			})
			return err
		})
		if level >= 0 {
			return nil
		}
		ms := physicalMonitors()
		defer releaseMonitors(ms)
		for _, m := range ms {
			if lo, cur, hi, ok := ddcRange(m); ok {
				level = float64(cur-lo) / float64(hi-lo)
				return nil
			}
		}
		return errNoBrightness
	})
	return math.Min(math.Max(level, 0), 1), err
}

func (w *winLauncher) SetBrightness(level float64) error {
	level = math.Min(math.Max(level, 0), 1)
	return w.brightThread.do(func() error {
		done := 0
		withWMI(func(svc *ole.IDispatch) error {
			n, err := wmiEach(svc, "SELECT * FROM WmiMonitorBrightnessMethods WHERE Active=TRUE", func(item *ole.IDispatch) error {
				_, err := oleutil.CallMethod(item, "WmiSetBrightness", int32(0), uint8(math.Round(level*100)))
				return err
			})
			if err == nil {
				done += n
			}
			return err
		})
		ms := physicalMonitors()
		defer releaseMonitors(ms)
		for _, m := range ms {
			if lo, _, hi, ok := ddcRange(m); ok {
				v := lo + uint32(math.Round(level*float64(hi-lo)))
				if r, _, _ := procSetMonitorBrightness.Call(uintptr(m.handle), uintptr(v)); r != 0 {
					done++
				}
			}
		}
		if done == 0 {
			return errNoBrightness
		}
		return nil
	})
}
