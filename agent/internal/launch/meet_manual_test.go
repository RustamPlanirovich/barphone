//go:build manual && windows

package launch

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Read only: windows that look like Meet, their process and whether it holds the mic or
// camera, and what Call() concludes: go test -tags manual -run MeetDiag -v ./internal/launch
func TestMeetDiag(t *testing.T) {
	l := New().(*winLauncher)
	l.launchThread.do(func() error {
		exes := map[uint32]string{}
		self := uint32(os.Getpid())
		for _, h := range topWindows() {
			title, cloaked, ok := switchableAnyDesktop(h)
			if !ok || !strings.Contains(strings.ToLower(title), "meet") {
				continue
			}
			var pid uint32
			windows.GetWindowThreadProcessId(h, &pid)
			exe := processExe(pid, exes)
			t.Logf("window %q cloaked=%v self=%v regex=%v exe=%s mic=%v cam=%v", title, cloaked, pid == self,
				meetTitle.MatchString(title), exe, deviceInUse("microphone", exe), deviceInUse("webcam", exe))
		}
		return nil
	})
	info, err := l.Call()
	t.Logf("Call(): %+v %v", info, err)
}
