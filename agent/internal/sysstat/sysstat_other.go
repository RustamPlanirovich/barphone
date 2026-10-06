//go:build !windows && !darwin

package sysstat

type Sampler struct{}

func New() *Sampler { return &Sampler{} }

func (s *Sampler) Read() (Sample, error) { return Sample{}, ErrUnsupported }
