package netinfo

import (
	"net"
	"testing"
)

func TestTailscaleRangeCounts(t *testing.T) {
	for ip, want := range map[string]bool{
		"100.101.12.7":  true,  // Tailscale
		"100.64.0.1":    true,
		"100.127.255.1": true,
		"100.128.0.1":   false, // public
		"8.8.8.8":       false,
	} {
		if got := cgnat.Contains(net.ParseIP(ip).To4()); got != want {
			t.Errorf("%s: %v", ip, got)
		}
	}
}
