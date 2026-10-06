//go:build manual && windows

package launch

import (
	"testing"

	"barphone/agent/internal/store"
)

// Lists, read only, the windows of a few apps with the desktop they are on, and whether
// a Meet call is going on: go test -tags manual -run AllDesktops -v ./internal/launch
func TestWindowsOnAllDesktops(t *testing.T) {
	l := New().(*winLauncher)
	for _, b := range []store.Button{
		{Kind: store.KindApp, Target: "Microsoft.VisualStudioCode"},
		{Kind: store.KindPath, Target: "chrome.exe"},
		{Kind: store.KindApp, Target: "Microsoft.Windows.Explorer"},
	} {
		ws, err := l.Windows(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range ws {
			t.Logf("%-30s desktop=%d active=%v %q", b.Target, w.Desktop, w.Active, w.Title)
		}
	}
	call, err := l.Call()
	t.Logf("call: %+v %v", call, err)
}
