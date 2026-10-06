//go:build windows

package launch

import (
	"testing"
	"unsafe"
)

func TestMouseInputIsAnINPUT(t *testing.T) {
	if unsafe.Sizeof(mInput{}) != unsafe.Sizeof(input{}) {
		t.Fatalf("INPUT with MOUSEINPUT: %d bytes, with KEYBDINPUT: %d", unsafe.Sizeof(mInput{}), unsafe.Sizeof(input{}))
	}
	// A zero move: Windows checks the structure size and accepts it, the cursor stays put.
	if err := (&winLauncher{}).MovePointer(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := (&winLauncher{}).Click("fourth", false); err != ErrUnsupported {
		t.Fatalf("unknown button: %v", err)
	}
}
