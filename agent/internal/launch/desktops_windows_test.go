//go:build windows

package launch

import "testing"

// Reads this PC's virtual desktops (never switches: that would move the user).
func TestDesktopsAreReadable(t *testing.T) {
	info, err := (&winLauncher{}).Desktops()
	if err != nil {
		t.Fatal(err)
	}
	if info.Count < 1 || info.Current < 0 || info.Current >= info.Count || len(info.Names) != info.Count {
		t.Fatalf("desktops: %+v", info)
	}
	t.Logf("%d desktops, current #%d, names %q", info.Count, info.Current+1, info.Names)
	if guidString([]byte{0x12, 0x62, 0xa9, 0x79, 0x2b, 0x3c, 0x27, 0x47, 0x8e, 0x14, 0x7b, 0x50, 0x03, 0x68, 0xf9, 0xab}) != "{79A96212-3C2B-4727-8E14-7B500368F9AB}" {
		t.Fatal("GUID bytes are little-endian in the first three groups")
	}
}
