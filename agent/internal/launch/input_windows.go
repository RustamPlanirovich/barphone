//go:build windows

package launch

import (
	"fmt"
	"unicode/utf16"
	"unsafe"
)

// Keyboard input goes to whatever window is in front on the PC, exactly as if the user
// pressed the keys. Each button press is sent as one SendInput batch, so it cannot
// interleave with the user's own typing.

var procMapVirtualKeyW = user32.NewProc("MapVirtualKeyW")

const (
	keyeventfExtended = 0x0001
	keyeventfUnicode  = 0x0004

	vkShift, vkControl, vkMenu, vkLWin = 0x10, 0x11, 0x12, 0x5B
	vkReturn, vkTab                    = 0x0D, 0x09
)

type vkey struct {
	code     uint16
	extended bool
}

var vkByName = map[string]vkey{
	"Enter": {0x0D, false}, "Escape": {0x1B, false}, "Tab": {0x09, false}, "Space": {0x20, false},
	"Backspace": {0x08, false}, "Delete": {0x2E, true}, "Insert": {0x2D, true},
	"Home": {0x24, true}, "End": {0x23, true}, "PageUp": {0x21, true}, "PageDown": {0x22, true},
	"Left": {0x25, true}, "Up": {0x26, true}, "Right": {0x27, true}, "Down": {0x28, true},
	"PrintScreen": {0x2C, true}, "Pause": {0x13, false}, "CapsLock": {0x14, false},
	"NumLock": {0x90, true}, "ScrollLock": {0x91, false}, "Menu": {0x5D, true},
	";": {0xBA, false}, "=": {0xBB, false}, ",": {0xBC, false}, "-": {0xBD, false}, ".": {0xBE, false},
	"/": {0xBF, false}, "`": {0xC0, false}, "[": {0xDB, false}, "\\": {0xDC, false}, "]": {0xDD, false},
	"'": {0xDE, false},
}

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		vkByName[string(c)] = vkey{uint16(c), false}
	}
	for c := '0'; c <= '9'; c++ {
		vkByName[string(c)] = vkey{uint16(c), false}
	}
	for i := 1; i <= 24; i++ {
		vkByName[fmt.Sprintf("F%d", i)] = vkey{uint16(0x70 + i - 1), false}
	}
}

func keyInput(k vkey, up bool) input {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(k.code), 0) // MAPVK_VK_TO_VSC
	var flags uint32
	if k.extended {
		flags |= keyeventfExtended
	}
	if up {
		flags |= keyeventfKeyUp
	}
	return input{typ: inputKeyboard, ki: keybdInput{wVk: k.code, wScan: uint16(scan), dwFlags: flags}}
}

func sendInputs(in []input) error {
	if len(in) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(n) != len(in) {
		return fmt.Errorf("SendInput: %d of %d events: %v", n, len(in), err)
	}
	return nil
}

// tap presses and releases one key, holding the given modifiers.
func comboInputs(c Combo) ([]input, error) {
	var mods []vkey
	if c.Ctrl {
		mods = append(mods, vkey{vkControl, false})
	}
	if c.Shift {
		mods = append(mods, vkey{vkShift, false})
	}
	if c.Alt {
		mods = append(mods, vkey{vkMenu, false})
	}
	if c.Win {
		mods = append(mods, vkey{vkLWin, true})
	}
	var in []input
	for _, m := range mods {
		in = append(in, keyInput(m, false))
	}
	if c.Key != "" {
		k, ok := vkByName[c.Key]
		if !ok {
			return nil, fmt.Errorf("клавиша %q не поддерживается", c.Key)
		}
		in = append(in, keyInput(k, false), keyInput(k, true))
	}
	for i := len(mods) - 1; i >= 0; i-- {
		in = append(in, keyInput(mods[i], true))
	}
	return in, nil
}

func sendCombo(c Combo) error {
	in, err := comboInputs(c)
	if err != nil {
		return err
	}
	return sendInputs(in)
}

// textInputs types text as Unicode characters, independent of the keyboard layout.
// Newlines and tabs become real Enter/Tab presses so forms and terminals react to them.
func textInputs(text string) []input {
	var in []input
	for _, r := range text {
		switch r {
		case '\r':
			continue
		case '\n':
			in = append(in, keyInput(vkey{vkReturn, false}, false), keyInput(vkey{vkReturn, false}, true))
			continue
		case '\t':
			in = append(in, keyInput(vkey{vkTab, false}, false), keyInput(vkey{vkTab, false}, true))
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			in = append(in,
				input{typ: inputKeyboard, ki: keybdInput{wScan: u, dwFlags: keyeventfUnicode}},
				input{typ: inputKeyboard, ki: keybdInput{wScan: u, dwFlags: keyeventfUnicode | keyeventfKeyUp}})
		}
	}
	return in
}

func pressVK(code uint16) error {
	k := vkey{code, true}
	return sendInputs([]input{keyInput(k, false), keyInput(k, true)})
}
