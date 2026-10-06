package sysstat

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type cpuTimes struct{ idle, kernel, user uint64 }

func readCPUTimes() (cpuTimes, bool) {
	var idle, kernel, user windows.Filetime
	r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return cpuTimes{}, false
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return cpuTimes{ft(idle), ft(kernel), ft(user)}, true
}

// Sampler measures the CPU load since its previous Read. Not safe for concurrent use.
type Sampler struct {
	prev cpuTimes
	have bool
}

func New() *Sampler { return &Sampler{} }

func (s *Sampler) Read() (Sample, error) {
	if !s.have { // the load is a difference: take a first point
		s.prev, s.have = readCPUTimes()
		time.Sleep(300 * time.Millisecond)
	}
	var out Sample
	if t, ok := readCPUTimes(); ok {
		idle := t.idle - s.prev.idle
		busy := t.kernel - s.prev.kernel + t.user - s.prev.user // kernel time includes idle
		if busy > 0 {
			out.CPU = fraction(1 - float64(idle)/float64(busy))
		}
		s.prev = t
	}
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		out.setRAM(m.TotalPhys-m.AvailPhys, m.TotalPhys)
	}
	return out, nil
}
