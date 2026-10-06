// Package firewall reports whether the OS firewall lets phones reach the agent and,
// on Windows, can add an inbound rule for it (after a UAC prompt).
package firewall

import (
	"context"
	"sync"
	"time"
)

type Status struct {
	Supported bool `json:"supported"`
	Checked   bool `json:"checked"`
	// Allowed: inbound traffic to the agent is allowed on the profile of the main network.
	Allowed bool `json:"allowed"`
	// Network / Category describe the main LAN adapter, e.g. "Wi-Fi" / "Public".
	Network  string `json:"network,omitempty"`
	Category string `json:"category,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Checker caches the (slow, PowerShell-based) status check.
type Checker struct {
	// Iface returns the name of the adapter phones are expected to come from.
	Iface func() string

	mu     sync.Mutex
	status Status
	at     time.Time
}

const cacheTTL = 30 * time.Second

func (c *Checker) Status(ctx context.Context) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < cacheTTL {
		return c.status
	}
	iface := ""
	if c.Iface != nil {
		iface = c.Iface()
	}
	c.status = check(ctx, iface)
	c.at = time.Now()
	return c.status
}

// Cached returns the last known status without blocking, and whether it is still fresh.
func (c *Checker) Cached() (Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status, !c.at.IsZero() && time.Since(c.at) < cacheTTL
}

func (c *Checker) Invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// Allow adds an inbound rule for the agent executable (prompts for elevation).
func (c *Checker) Allow() error {
	defer c.Invalidate()
	return allow()
}
