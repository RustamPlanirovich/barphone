//go:build windows && manual

package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"barphone/agent/internal/store"
)

// Opens two probe windows (tools/probewin), checks they are found as the windows of a
// path button and that Focus really brings the chosen one to the front.
//
//	go test -tags manual -run FocusChosenWindow -v ./internal/launch
func TestFocusChosenWindow(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "probewin.exe")
	if out, err := exec.Command("go", "build", "-o", exe, "../../tools/probewin").CombinedOutput(); err != nil {
		t.Fatalf("build probewin: %v\n%s", err, out)
	}
	probe := exec.Command(exe, "Probe A - barphone probe", "Probe B - barphone probe")
	if err := probe.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { probe.Process.Kill(); probe.Wait(); os.Remove(exe) }()

	l := New()
	b := store.Button{Kind: store.KindPath, Target: exe, Title: "probe"}
	var ws []Window
	for i := 0; i < 50 && len(ws) < 2; i++ {
		time.Sleep(100 * time.Millisecond)
		ws, _ = l.Windows(b)
	}
	ShortTitles(ws)
	t.Logf("windows: %+v", ws)
	if len(ws) != 2 {
		t.Fatalf("want 2 probe windows, got %d", len(ws))
	}
	for _, target := range []Window{ws[1], ws[0]} {
		if err := l.Focus(b, target.ID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(400 * time.Millisecond)
		fg := windows.GetForegroundWindow()
		if strconv.FormatUint(uint64(fg), 10) != target.ID {
			t.Errorf("focus %q: foreground is %d", target.Title, fg)
		} else {
			t.Logf("focused %q", target.Title)
		}
	}
	if err := l.Focus(b, "12345"); err != ErrWindowGone {
		t.Errorf("foreign window id must be refused, got %v", err)
	}
	// A window of another app must not be focusable through this button.
	other := store.Button{Kind: store.KindPath, Target: `C:\Windows\notepad.exe`}
	if err := l.Focus(other, ws[0].ID); err != ErrWindowGone {
		t.Errorf("window of another app must be refused, got %v", err)
	}
}
