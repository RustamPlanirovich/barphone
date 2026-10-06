//go:build !windows && !darwin

// Package autostart toggles starting the agent at user login.
package autostart

import "errors"

type Login struct{}

func (Login) Enabled() (bool, error) { return false, errors.ErrUnsupported }
func (Login) Set(bool) error         { return errors.ErrUnsupported }
