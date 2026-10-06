//go:build windows && manual

package firewall

import (
	"testing"

	ole "github.com/go-ole/go-ole"
)

// Read-only: enumerates the real firewall rules through COM (no admin needed).
//
//	go test -tags manual -run ListRules -v ./internal/firewall
func TestListRules(t *testing.T) {
	var all []fwRule
	err := withRules(func(rules *ole.IDispatch) (err error) {
		all, err = listRules(rules)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	withApp := 0
	for _, r := range all {
		if r.App != "" {
			withApp++
		}
	}
	t.Logf("%d rules, %d bound to a program", len(all), withApp)
	if len(all) == 0 || withApp == 0 {
		t.Fatal("expected to see rules")
	}
	for _, r := range all {
		if r.App != "" && validExe(r.App) == nil {
			remove, skipped := rulesToRemove(all, r.App)
			t.Logf("sample %q: would remove %v, skip %v", r.App, remove, skipped)
			break
		}
	}
	if remove, _ := rulesToRemove(all, `C:\nope\barphone-agent.exe`); len(remove) != 0 {
		t.Fatalf("nonexistent exe matched %v", remove)
	}
}
