package sysstat

import (
	"math"
	"runtime"
	"testing"
)

const topOut = `Processes: 512 total, 3 running, 509 sleeping, 2345 threads
2026/10/06 12:00:00
Load Avg: 1.92, 2.11, 2.20
CPU usage: 5.26% user, 10.52% sys, 84.21% idle
SharedLibs: 500M resident, 80M data, 40M linkedit.
PhysMem: 15G used (2050M wired, 1500M compressor), 1024M unused.

Processes: 512 total, 2 running, 510 sleeping, 2340 threads
2026/10/06 12:00:01
Load Avg: 1.92, 2.11, 2.20
CPU usage: 20.0% user, 5.0% sys, 75.0% idle
PhysMem: 15G used (2050M wired, 1500M compressor), 1024M unused.
`

const vmStatOut = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               12000.
Pages active:                            300000.
Pages inactive:                          290000.
Pages speculative:                         5000.
Pages throttled:                              0.
Pages wired down:                        100000.
Pages purgeable:                          10000.
"Translation faults":                 123456789.
File-backed pages:                       250000.
Anonymous pages:                         340000.
Pages stored in compressor:              200000.
Pages occupied by compressor:             50000.
`

func TestParseTopTakesTheLastSample(t *testing.T) {
	cpu := parseTopCPU(topOut)
	if cpu == nil || math.Abs(*cpu-0.25) > 1e-9 {
		t.Fatalf("cpu: %v", cpu)
	}
	if parseTopCPU("garbage") != nil {
		t.Fatal("no CPU line, no value")
	}
}

func TestParseVMStatLikeActivityMonitor(t *testing.T) {
	used, ok := parseVMStat(vmStatOut)
	// app (340000 - 10000) + wired 100000 + compressed 50000 pages of 16 KiB
	if want := uint64(330000+100000+50000) * 16384; !ok || used != want {
		t.Fatalf("used %d, want %d", used, want)
	}
	if _, ok := parseVMStat("Pages free: 1."); ok {
		t.Fatal("no page size, no value")
	}
}

func TestReadOnThisMachine(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("covered by the parsers on macOS")
	}
	s := New()
	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.CPU == nil || *got.CPU < 0 || *got.CPU > 1 {
		t.Fatalf("cpu: %v", got.CPU)
	}
	if got.RAM == nil || got.RAMTotal < 1<<28 || got.RAMUsed == 0 || got.RAMUsed > got.RAMTotal {
		t.Fatalf("ram: %+v", got)
	}
	t.Logf("cpu %.1f%%, ram %.1f%% of %d MiB", *got.CPU*100, *got.RAM*100, got.RAMTotal>>20)
}
