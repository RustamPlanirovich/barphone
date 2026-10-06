//go:build windows

package launch

import "testing"

// Builds input batches without sending them.

func TestComboInputsOrder(t *testing.T) {
	c, _ := ParseCombo("Ctrl+Shift+Left")
	in, err := comboInputs(c)
	if err != nil {
		t.Fatal(err)
	}
	type ev struct {
		vk   uint16
		up   bool
		ext  bool
		scan bool
	}
	var got []ev
	for _, i := range in {
		got = append(got, ev{i.ki.wVk, i.ki.dwFlags&keyeventfKeyUp != 0, i.ki.dwFlags&keyeventfExtended != 0, i.ki.wScan != 0})
	}
	want := []ev{
		{vkControl, false, false, true}, {vkShift, false, false, true},
		{0x25, false, true, true}, {0x25, true, true, true}, // Left is an extended key
		{vkShift, true, false, true}, {vkControl, true, false, true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTextInputs(t *testing.T) {
	in := textInputs("Я😀\r\n")
	// Я: down/up; 😀: surrogate pair → 2×(down/up); \r skipped; \n → Enter down/up.
	if len(in) != 2+4+2 {
		t.Fatalf("got %d events", len(in))
	}
	if in[0].ki.wScan != 'Я' || in[0].ki.dwFlags != keyeventfUnicode || in[1].ki.dwFlags != keyeventfUnicode|keyeventfKeyUp {
		t.Errorf("first char: %+v %+v", in[0].ki, in[1].ki)
	}
	if in[6].ki.wVk != vkReturn || in[7].ki.dwFlags&keyeventfKeyUp == 0 {
		t.Errorf("newline must press Enter: %+v %+v", in[6].ki, in[7].ki)
	}
}
