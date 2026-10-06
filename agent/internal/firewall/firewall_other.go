//go:build !windows

package firewall

import (
	"context"
	"errors"
)

// The macOS application firewall prompts on its own and is off by default; nothing to manage.
func check(context.Context, string) Status { return Status{Supported: false} }

func allow() error { return errors.ErrUnsupported }

const ElevatedFlag = "-firewall-allow"

func ApplyAllowRule() error { return errors.ErrUnsupported }
