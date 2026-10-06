package launch

import (
	"fmt"
	"strings"
)

// Combo is a parsed key combination such as "Ctrl+Shift+P". Key is a canonical key
// name (see keyAliases) or "" for a lone modifier ("Win" opens the Start menu).
type Combo struct {
	Ctrl, Shift, Alt, Win bool
	Key                   string
}

// String spells the combo the way Windows documents shortcuts: Win, Ctrl, Alt, Shift.
func (c Combo) String() string {
	var parts []string
	if c.Win {
		parts = append(parts, "Win")
	}
	if c.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if c.Alt {
		parts = append(parts, "Alt")
	}
	if c.Shift {
		parts = append(parts, "Shift")
	}
	if c.Key != "" {
		parts = append(parts, c.Key)
	}
	return strings.Join(parts, "+")
}

// Canonical key names. Letters, digits and F1–F24 are added in init.
var keyAliases = map[string]string{
	"enter": "Enter", "return": "Enter",
	"esc": "Escape", "escape": "Escape",
	"tab": "Tab", "space": "Space", "spacebar": "Space",
	"backspace": "Backspace", "bksp": "Backspace",
	"delete": "Delete", "del": "Delete",
	"insert": "Insert", "ins": "Insert",
	"home": "Home", "end": "End",
	"pageup": "PageUp", "pgup": "PageUp", "pagedown": "PageDown", "pgdn": "PageDown",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"arrowup": "Up", "arrowdown": "Down", "arrowleft": "Left", "arrowright": "Right",
	"printscreen": "PrintScreen", "prtsc": "PrintScreen", "prtscr": "PrintScreen",
	"pause": "Pause", "capslock": "CapsLock", "numlock": "NumLock", "scrolllock": "ScrollLock",
	"menu": "Menu", "apps": "Menu", "contextmenu": "Menu",
	";": ";", "=": "=", ",": ",", "-": "-", ".": ".", "/": "/", "`": "`",
	"[": "[", "\\": "\\", "]": "]", "'": "'",
	"plus": "=", "minus": "-", "comma": ",", "period": ".", "slash": "/",
}

var modifierAliases = map[string]string{
	"ctrl": "Ctrl", "control": "Ctrl", "ctl": "Ctrl",
	"shift": "Shift",
	"alt":   "Alt", "option": "Alt", "opt": "Alt",
	"win": "Win", "windows": "Win", "super": "Win", "meta": "Win", "cmd": "Win", "command": "Win",
}

func init() {
	for c := 'a'; c <= 'z'; c++ {
		keyAliases[string(c)] = strings.ToUpper(string(c))
	}
	for c := '0'; c <= '9'; c++ {
		keyAliases[string(c)] = string(c)
	}
	for i := 1; i <= 24; i++ {
		keyAliases[fmt.Sprintf("f%d", i)] = fmt.Sprintf("F%d", i)
	}
}

// ParseCombo accepts "Ctrl+Shift+P", "ctrl + alt + del", "Win+D", "F5", "Win"…
func ParseCombo(s string) (Combo, error) {
	var c Combo
	s = strings.TrimSpace(s)
	if s == "" {
		return c, fmt.Errorf("пустое сочетание")
	}
	parts := strings.Split(s, "+")
	for i, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			return c, fmt.Errorf("лишний «+» в %q", s)
		}
		if m, ok := modifierAliases[p]; ok {
			switch m {
			case "Ctrl":
				c.Ctrl = true
			case "Shift":
				c.Shift = true
			case "Alt":
				c.Alt = true
			case "Win":
				c.Win = true
			}
			continue
		}
		key, ok := keyAliases[p]
		if !ok {
			return c, fmt.Errorf("неизвестная клавиша %q", strings.TrimSpace(parts[i]))
		}
		if c.Key != "" {
			return c, fmt.Errorf("в сочетании две обычные клавиши: %s и %s", c.Key, key)
		}
		c.Key = key
	}
	if c.Key == "" && !c.Win {
		return c, fmt.Errorf("нужна клавиша, а не только модификаторы")
	}
	return c, nil
}
