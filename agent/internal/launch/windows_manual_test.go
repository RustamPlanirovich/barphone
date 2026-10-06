//go:build windows && manual

package launch

import (
	"testing"

	"barphone/agent/internal/store"
)

// Read-only: shows which open windows would be offered for a few common buttons.
//
//	go test -tags manual -run ListAppWindows -v ./internal/launch
func TestListAppWindows(t *testing.T) {
	l := New()
	for _, b := range []store.Button{
		{Kind: store.KindApp, Target: "Microsoft.VisualStudioCode", Title: "VS Code"},
		{Kind: store.KindApp, Target: "Microsoft.Windows.Explorer", Title: "Проводник"},
		{Kind: store.KindApp, Target: "MSEdge", Title: "Edge"},
		{Kind: store.KindApp, Target: "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", Title: "Калькулятор"},
		{Kind: store.KindURL, Target: "https://example.com", Title: "URL"},
	} {
		ws, err := l.Windows(b)
		if err != nil {
			t.Fatal(err)
		}
		var m appMatch
		l.(*winLauncher).launchThread.do(func() error { m = resolveMatch(b); return nil })
		ShortTitles(ws)
		t.Logf("%-12s exes=%v aumids=%v -> %d windows", b.Title, keys(m.exes), keys(m.aumids), len(ws))
		for _, w := range ws {
			t.Logf("      %s  %q", w.ID, w.Title)
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
