//go:build darwin

package launch

import (
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// macOS: keystrokes go through System Events (the agent needs Accessibility permission),
// volume and power through AppleScript/pmset. Media keys have no scriptable equivalent.

func osascript(script string) error {
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

var macKeyCodes = map[string]int{
	"Enter": 36, "Escape": 53, "Tab": 48, "Space": 49, "Backspace": 51, "Delete": 117,
	"Home": 115, "End": 119, "PageUp": 116, "PageDown": 121,
	"Left": 123, "Right": 124, "Down": 125, "Up": 126,
	"F1": 122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97, "F7": 98, "F8": 100,
	"F9": 101, "F10": 109, "F11": 103, "F12": 111,
}

// comboScript maps Win to ⌘ Command, the key Mac users reach for where Windows uses Win/Ctrl.
func comboScript(c Combo) string {
	var mods []string
	if c.Win {
		mods = append(mods, "command down")
	}
	if c.Ctrl {
		mods = append(mods, "control down")
	}
	if c.Alt {
		mods = append(mods, "option down")
	}
	if c.Shift {
		mods = append(mods, "shift down")
	}
	press := "keystroke " + appleString(strings.ToLower(c.Key))
	if code, ok := macKeyCodes[c.Key]; ok {
		press = "key code " + strconv.Itoa(code)
	}
	if len(mods) > 0 {
		press += " using {" + strings.Join(mods, ", ") + "}"
	}
	return `tell application "System Events" to ` + press
}

func (m macLauncher) runSystem(id string) error {
	a, ok := LookupSystemAction(id)
	if !ok {
		return fmt.Errorf("unknown system action %q", id)
	}
	do := func() error {
		switch id {
		case "volume", "mute":
			return osascript(`set volume output muted (not (output muted of (get volume settings)))`)
		case "volume_up", "volume_down":
			st, err := m.Volume()
			if err != nil {
				return err
			}
			step := 0.0625
			if id == "volume_down" {
				step = -step
			}
			return m.SetVolume(st.Level + step)
		case "lock":
			return osascript(`tell application "System Events" to keystroke "q" using {control down, command down}`)
		case "sleep":
			return exec.Command("/usr/bin/pmset", "sleepnow").Run()
		case "display_off":
			return exec.Command("/usr/bin/pmset", "displaysleepnow").Run()
		case "shutdown":
			return osascript(`tell application "System Events" to shut down`)
		case "restart":
			return osascript(`tell application "System Events" to restart`)
		}
		return ErrUnsupported
	}
	if a.Deferred {
		go func() {
			time.Sleep(deferDelay)
			do()
		}()
		return nil
	}
	return do()
}

// The mouse needs CGEvent (cgo); typing goes through System Events as for buttons.
func (macLauncher) MovePointer(int, int) error { return ErrUnsupported }
func (macLauncher) Click(string, bool) error   { return ErrUnsupported }
func (macLauncher) Scroll(int, int) error      { return ErrUnsupported }

// Brightness has no public API on macOS.
func (macLauncher) Brightness() (float64, error) { return 0, ErrUnsupported }
func (macLauncher) SetBrightness(float64) error  { return ErrUnsupported }

func (macLauncher) Volume() (VolumeState, error) {
	out, err := exec.Command("/usr/bin/osascript", "-e",
		`set s to get volume settings
return ((output volume of s) as text) & "," & ((output muted of s) as text)`).Output()
	if err != nil {
		return VolumeState{}, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return VolumeState{}, fmt.Errorf("unexpected volume settings %q", out)
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return VolumeState{}, err
	}
	return VolumeState{Level: float64(v) / 100, Muted: parts[1] == "true"}, nil
}

func (macLauncher) SetVolume(level float64) error {
	v := int(math.Round(math.Max(0, math.Min(1, level)) * 100))
	return osascript(fmt.Sprintf("set volume output volume %d without output muted", v))
}
