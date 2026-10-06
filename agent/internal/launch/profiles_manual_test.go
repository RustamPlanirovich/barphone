//go:build windows && manual

package launch

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"barphone/agent/internal/store"
)

// Read-only, nothing is focused: for apps that have windows open right now, the keys a
// profile binding stores (AppKeys of the Start-menu entry) must intersect the keys the
// foreground poller computes for those windows.
//
//	go test -tags manual -run ProfileBindingMatchesWindows -v ./internal/launch
func TestProfileBindingMatchesWindows(t *testing.T) {
	l := New().(*winLauncher)
	self := uint32(os.Getpid())
	for _, b := range []store.Button{
		{Kind: store.KindApp, Target: "Microsoft.VisualStudioCode", Title: "VS Code"},
		{Kind: store.KindApp, Target: "Microsoft.Windows.Explorer", Title: "Проводник"},
		{Kind: store.KindApp, Target: "MSEdge", Title: "Edge"},
	} {
		binding := l.AppKeys(b)
		ws, _ := l.Windows(b)
		if len(ws) == 0 {
			t.Logf("%-10s binding %v — no open windows, skipped", b.Title, binding)
			continue
		}
		exes := map[uint32]string{}
		for _, w := range ws {
			var h windows.HWND
			for _, c := range topWindows() {
				if w.ID == hwndID(c) {
					h = c
				}
			}
			var keys []string
			l.launchThread.do(func() error { keys, _, _ = windowKeys(h, self, exes); return nil })
			if !(ForegroundApp{Keys: keys}).MatchesAny(binding) {
				t.Errorf("%s: window keys %v do not match binding %v", b.Title, keys, binding)
			}
		}
		t.Logf("%-10s binding %v matches its %d open window(s)", b.Title, binding, len(ws))
	}
	var fg ForegroundApp
	l.launchThread.do(func() error {
		var ok bool
		fg, ok = foregroundApp(self, map[uint32]string{}, map[string]string{})
		if !ok {
			t.Log("foreground: ignored (shell surface or nothing)")
		}
		return nil
	})
	t.Logf("in front now: %q %v", fg.Name, fg.Keys)
}
