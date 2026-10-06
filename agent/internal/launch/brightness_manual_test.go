//go:build manual && windows

package launch

import "testing"

// Reads the screen brightness and sets it to the same value, so nothing visibly changes:
// go test -tags manual -run Brightness -v ./internal/launch
func TestBrightnessRoundTrip(t *testing.T) {
	l := New()
	before, err := l.Brightness()
	if err != nil {
		t.Skipf("no adjustable screen here: %v", err)
	}
	t.Logf("brightness %.0f%%", before*100)
	if err := l.SetBrightness(before); err != nil {
		t.Fatal(err)
	}
	after, err := l.Brightness()
	if err != nil || after != before {
		t.Fatalf("after setting the same level: %v %v", after, err)
	}
}
