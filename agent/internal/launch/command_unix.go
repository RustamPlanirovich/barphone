//go:build !windows

package launch

import (
	"os/exec"
	"runtime"

	"barphone/agent/internal/store"
)

// Command prepares a "command" button: a login shell, so the user's PATH (Homebrew and
// the like) is there. It runs in the background; the phone hears when it ends.
func Command(b store.Button) (*exec.Cmd, error) {
	shell := "/bin/sh"
	if runtime.GOOS == "darwin" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-lc", b.Target)
	cmd.Dir = expandDir(b.Dir)
	return cmd, nil
}

func commandLine(cmd *exec.Cmd) string { return cmd.String() }
