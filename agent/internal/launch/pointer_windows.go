//go:build windows

package launch

import "unsafe"

// Mouse input for the phone's trackpad, through SendInput like the keyboard: it goes
// to whatever is under the cursor or in front, as if the user moved the real mouse.

const (
	inputMouse = 0

	mouseeventfMove       = 0x0001
	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfRightDown  = 0x0008
	mouseeventfRightUp    = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp   = 0x0040
	mouseeventfWheel      = 0x0800
	mouseeventfHWheel     = 0x1000
)

type mouseInput struct {
	dx, dy      int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// mInput is INPUT with the MOUSEINPUT member of the union (same 40 bytes as input).
type mInput struct {
	typ uint32
	mi  mouseInput
}

func sendMouse(in ...mInput) error {
	if len(in) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(n) != len(in) {
		return err
	}
	return nil
}

func mouse(flags uint32, dx, dy int32, data int32) mInput {
	return mInput{typ: inputMouse, mi: mouseInput{dx: dx, dy: dy, mouseData: uint32(data), dwFlags: flags}}
}

func (w *winLauncher) MovePointer(dx, dy int) error {
	return sendMouse(mouse(mouseeventfMove, int32(dx), int32(dy), 0))
}

func (w *winLauncher) Click(button string, double bool) error {
	down, up, ok := clickFlags(button)
	if !ok {
		return ErrUnsupported
	}
	in := []mInput{mouse(down, 0, 0, 0), mouse(up, 0, 0, 0)}
	if double {
		in = append(in, in...)
	}
	return sendMouse(in...)
}

func clickFlags(button string) (down, up uint32, ok bool) {
	switch button {
	case "left", "":
		return mouseeventfLeftDown, mouseeventfLeftUp, true
	case "right":
		return mouseeventfRightDown, mouseeventfRightUp, true
	case "middle":
		return mouseeventfMiddleDown, mouseeventfMiddleUp, true
	}
	return 0, 0, false
}

func (w *winLauncher) Scroll(dx, dy int) error {
	var in []mInput
	if dy != 0 {
		in = append(in, mouse(mouseeventfWheel, 0, 0, int32(dy)))
	}
	if dx != 0 {
		in = append(in, mouse(mouseeventfHWheel, 0, 0, int32(dx)))
	}
	return sendMouse(in...)
}
