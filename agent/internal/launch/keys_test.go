package launch

import (
	"testing"

	"barphone/agent/internal/store"
)

func TestParseCombo(t *testing.T) {
	for in, want := range map[string]string{
		"Ctrl+Shift+P":      "Ctrl+Shift+P",
		" ctrl + shift + p": "Ctrl+Shift+P",
		"shift+ctrl+p":      "Ctrl+Shift+P", // canonical modifier order
		"Win+D":             "Win+D",
		"cmd+space":         "Win+Space",
		"Alt+F4":            "Alt+F4",
		"f5":                "F5",
		"Ctrl+Alt+Del":      "Ctrl+Alt+Delete",
		"win":               "Win",
		"Ctrl+plus":         "Ctrl+=",
		"Ctrl+[":            "Ctrl+[",
		"Win+Shift+S":       "Win+Shift+S",
		"PgDn":              "PageDown",
		"Ctrl+F24":          "Ctrl+F24",
	} {
		c, err := ParseCombo(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if c.String() != want {
			t.Errorf("%q -> %q, want %q", in, c.String(), want)
		}
	}
	for _, bad := range []string{"", "Ctrl", "Ctrl+Shift", "Ctrl+Foo", "A+B", "Ctrl++", "F25"} {
		if _, err := ParseCombo(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestValidateActionKinds(t *testing.T) {
	ok := []store.Button{
		{Kind: store.KindKeys, Target: "Ctrl+C"},
		{Kind: store.KindText, Target: "  отступы и\nперенос  "},
		{Kind: store.KindSystem, Target: "volume"},
	}
	for _, b := range ok {
		if err := Validate(b); err != nil {
			t.Errorf("%+v: %v", b, err)
		}
	}
	long := make([]rune, MaxTextLen+1)
	for i := range long {
		long[i] = 'я'
	}
	for _, b := range []store.Button{
		{Kind: store.KindText, Target: string(long)},
		{Kind: store.KindSystem, Target: "rm_rf"},
	} {
		if Validate(b) == nil {
			t.Errorf("%s should be rejected", b.Kind)
		}
	}
}

func TestSystemActions(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range SystemActions {
		if a.ID == "" || a.Title == "" || seen[a.ID] {
			t.Errorf("bad or duplicate action %+v", a)
		}
		seen[a.ID] = true
	}
	for _, id := range []string{"shutdown", "restart"} {
		if a, _ := LookupSystemAction(id); !a.Confirm || !a.Deferred {
			t.Errorf("%s must require confirmation and run deferred", id)
		}
	}
	if a, _ := LookupSystemAction("volume"); !a.Slider {
		t.Error("volume is the slider")
	}
}
