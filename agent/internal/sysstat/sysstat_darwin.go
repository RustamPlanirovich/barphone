package sysstat

import (
	"context"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// Sampler asks top and vm_stat (no cgo needed); a Read takes about a second.
type Sampler struct{}

func New() *Sampler { return &Sampler{} }

func (s *Sampler) Read() (Sample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out Sample
	// Two samples a second apart: the first one of `top -l` is an average since boot.
	if b, err := exec.CommandContext(ctx, "/usr/bin/top", "-l", "2", "-n", "0", "-s", "1").Output(); err == nil {
		out.CPU = parseTopCPU(string(b))
	}
	total, err := unix.SysctlUint64("hw.memsize")
	if err == nil {
		if b, err := exec.CommandContext(ctx, "/usr/bin/vm_stat").Output(); err == nil {
			if used, ok := parseVMStat(string(b)); ok {
				out.setRAM(min(used, total), total)
			}
		}
	}
	return out, nil
}
