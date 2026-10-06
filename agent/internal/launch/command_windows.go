//go:build windows

package launch

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"barphone/agent/internal/store"
)

// Command prepares a "command" button: cmd.exe in a console window of its own, so the
// user sees what runs (never a hidden shell). KeepOpen leaves the window for reading.
func Command(b store.Button) (*exec.Cmd, error) {
	shell := os.Getenv("ComSpec")
	if shell == "" {
		shell = `C:\Windows\System32\cmd.exe`
	}
	mode := "/c"
	if b.KeepOpen {
		mode = "/k"
	}
	cmd := exec.Command(shell)
	// The command line goes to cmd.exe as typed: Go's argument quoting would break
	// cmd's own rules for quotes, & and |.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `"` + shell + `" ` + mode + ` ` + b.Target,
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	cmd.Dir = expandDir(b.Dir)
	return cmd, nil
}

// commandLine is what cmd.exe gets (for tests).
func commandLine(cmd *exec.Cmd) string { return cmd.SysProcAttr.CmdLine }
