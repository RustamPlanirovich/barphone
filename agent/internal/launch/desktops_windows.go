//go:build windows

package launch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows keeps its virtual desktops in the registry (read only here) and has no public
// API to switch them: switching is Win+Ctrl+Left/Right, as the user would press it.

const vdKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\VirtualDesktops`

func guidString(b []byte) string {
	if len(b) < 16 {
		return ""
	}
	g := windows.GUID{
		Data1: binary.LittleEndian.Uint32(b[0:4]),
		Data2: binary.LittleEndian.Uint16(b[4:6]),
		Data3: binary.LittleEndian.Uint16(b[6:8]),
	}
	copy(g.Data4[:], b[8:16])
	return g.String()
}

// currentDesktopID: Windows 11 keeps it next to the list, Windows 10 per logon session.
func currentDesktopID(k registry.Key) []byte {
	if cur, _, err := k.GetBinaryValue("CurrentVirtualDesktop"); err == nil && len(cur) == 16 {
		return cur
	}
	var session uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) != nil {
		return nil
	}
	path := fmt.Sprintf(`Software\Microsoft\Windows\CurrentVersion\Explorer\SessionInfo\%d\VirtualDesktops`, session)
	sk, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer sk.Close()
	cur, _, err := sk.GetBinaryValue("CurrentVirtualDesktop")
	if err != nil || len(cur) != 16 {
		return nil
	}
	return cur
}

// desktopList reads the virtual desktops (IDs in order) and the current one; nil IDs
// when there has never been a second desktop.
func desktopList() (ids [][]byte, cur []byte) {
	k, err := registry.OpenKey(registry.CURRENT_USER, vdKey, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	raw, _, err := k.GetBinaryValue("VirtualDesktopIDs")
	if err != nil {
		return nil, nil
	}
	for i := 0; i+16 <= len(raw); i += 16 {
		ids = append(ids, raw[i:i+16])
	}
	return ids, currentDesktopID(k)
}

func (w *winLauncher) Desktops() (DesktopInfo, error) {
	ids, cur := desktopList()
	if len(ids) == 0 {
		return DesktopInfo{Count: 1, Names: []string{""}}, nil // never had a second desktop
	}
	info := DesktopInfo{Count: len(ids)}
	for i, id := range ids {
		if cur != nil && bytes.Equal(id, cur) {
			info.Current = i
		}
		name := ""
		if dk, err := registry.OpenKey(registry.CURRENT_USER, vdKey+`\Desktops\`+guidString(id), registry.QUERY_VALUE); err == nil {
			name, _, _ = dk.GetStringValue("Name")
			dk.Close()
		}
		info.Names = append(info.Names, name)
	}
	return info, nil
}

// sleepForSwitch waits for Windows' desktop switch animation.
func sleepForSwitch() { time.Sleep(350 * time.Millisecond) }

func (w *winLauncher) MoveDesktop(steps int) error {
	key := "Right"
	if steps < 0 {
		key, steps = "Left", -steps
	}
	for i := 0; i < steps; i++ {
		if i > 0 {
			time.Sleep(80 * time.Millisecond) // let the switch start before the next one
		}
		if err := sendCombo(Combo{Ctrl: true, Win: true, Key: key}); err != nil {
			return err
		}
	}
	return nil
}
