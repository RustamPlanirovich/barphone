//go:build windows && manual

package winutil

import (
	"strings"
	"testing"
)

// Exercises the ShellExecuteEx + wait + exit-code path used for elevation, with the
// "open" verb so no UAC prompt appears:  go test -tags manual -run ShellExecute -v ./internal/winutil
func TestShellExecuteWait(t *testing.T) {
	if err := shellExecuteWait("open", "cmd.exe", "/c exit 0"); err != nil {
		t.Fatalf("exit 0: %v", err)
	}
	if err := shellExecuteWait("open", "cmd.exe", "/c exit 3"); err == nil || !strings.Contains(err.Error(), "code 3") {
		t.Fatalf("exit 3 should be reported, got %v", err)
	}
}
