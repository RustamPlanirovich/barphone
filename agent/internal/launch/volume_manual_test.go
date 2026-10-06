//go:build windows && manual

package launch

import (
	"math"
	"testing"
)

// Reads the master volume and writes the same level back, so nothing audible changes.
//
//	go test -tags manual -run VolumeRoundTrip -v ./internal/launch
func TestVolumeRoundTrip(t *testing.T) {
	l := New()
	before, err := l.Volume()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("volume %.0f%%, muted=%v", before.Level*100, before.Muted)
	if before.Muted {
		t.Skip("output is muted; SetVolume would unmute it, so leave it alone")
	}
	if err := l.SetVolume(before.Level); err != nil {
		t.Fatal(err)
	}
	after, err := l.Volume()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(after.Level-before.Level) > 0.005 || after.Muted {
		t.Fatalf("round trip changed volume: %+v -> %+v", before, after)
	}
}
