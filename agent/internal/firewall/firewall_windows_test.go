//go:build windows

package firewall

import (
	"sort"
	"testing"
)

func TestRulesToRemoveOnlyTouchesThisExe(t *testing.T) {
	exe := `C:\Apps\barphone\barphone-agent.exe`
	all := []fwRule{
		{Name: "barphone-agent.exe", App: exe},                                   // TCP rule from the Windows prompt
		{Name: "barphone-agent.exe", App: `c:\apps\BARPHONE\barphone-agent.EXE`}, // UDP twin, other case
		{Name: "barphone", App: exe},
		{Name: "Shared name", App: exe},
		{Name: "Shared name", App: `C:\Other\tool.exe`}, // same name, other program
		{Name: "Core Networking", App: "System"},
		{Name: "No program", App: ""},
	}
	remove, skipped := rulesToRemove(all, exe)
	if len(remove) != 2 || remove["barphone-agent.exe"] != 2 || remove["barphone"] != 1 {
		t.Fatalf("remove = %v", remove)
	}
	sort.Strings(skipped)
	if len(skipped) != 1 || skipped[0] != "Shared name" {
		t.Fatalf("skipped = %v", skipped)
	}
	if r, _ := rulesToRemove(all, `C:\nope\nope.exe`); len(r) != 0 {
		t.Fatalf("unknown exe must match nothing, got %v", r)
	}
}

func TestValidExe(t *testing.T) {
	for _, bad := range []string{"", "barphone-agent.exe", `C:\x\agent`} {
		if validExe(bad) == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if err := validExe(`C:\x\barphone-agent.exe`); err != nil {
		t.Error(err)
	}
}
