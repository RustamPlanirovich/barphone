// Package sysstat measures what the deck's live tiles show: CPU load and memory in use.
package sysstat

import (
	"bufio"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Sample is one measurement; a nil field means the OS did not tell.
type Sample struct {
	CPU      *float64 `json:"cpu,omitempty"` // 0..1
	RAM      *float64 `json:"ram,omitempty"` // 0..1
	RAMUsed  uint64   `json:"ramUsed,omitempty"`
	RAMTotal uint64   `json:"ramTotal,omitempty"`
}

var ErrUnsupported = errors.New("system stats are not supported on this OS")

func fraction(v float64) *float64 {
	v = min(max(v, 0), 1)
	return &v
}

func (s *Sample) setRAM(used, total uint64) {
	if total == 0 || used > total {
		return
	}
	s.RAMUsed, s.RAMTotal, s.RAM = used, total, fraction(float64(used)/float64(total))
}

var cpuIdleRe = regexp.MustCompile(`CPU usage:.*?([\d.]+)% idle`)

// parseTopCPU reads the load from macOS `top -l 2 -n 0` output: the last "CPU usage"
// line (the first one of `top -l` is an average since boot).
func parseTopCPU(out string) *float64 {
	m := cpuIdleRe.FindAllStringSubmatch(out, -1)
	if len(m) == 0 {
		return nil
	}
	idle, err := strconv.ParseFloat(m[len(m)-1][1], 64)
	if err != nil {
		return nil
	}
	return fraction(1 - idle/100)
}

var pageSizeRe = regexp.MustCompile(`page size of (\d+) bytes`)

// parseVMStat turns macOS `vm_stat` output into the "Memory Used" of Activity Monitor:
// app memory (anonymous, not purgeable) + wired + compressed, in bytes.
func parseVMStat(out string) (uint64, bool) {
	m := pageSizeRe.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	page, _ := strconv.ParseUint(m[1], 10, 64)
	pages := map[string]uint64{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		name, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64)
		if err == nil {
			pages[strings.TrimSpace(name)] = n
		}
	}
	anon, ok1 := pages["Anonymous pages"]
	wired, ok2 := pages["Pages wired down"]
	compressed, ok3 := pages["Pages occupied by compressor"]
	if !ok1 || !ok2 || !ok3 || page == 0 {
		return 0, false
	}
	app := anon - min(anon, pages["Pages purgeable"])
	return (app + wired + compressed) * page, true
}
