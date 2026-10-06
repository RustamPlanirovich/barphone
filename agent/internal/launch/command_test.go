package launch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"barphone/agent/internal/store"
)

func TestCommandFolder(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("BARPHONE_TEST_DIR", "proj")
	for in, want := range map[string]string{
		"":                             home,
		"~":                            home,
		"~/code":                       filepath.Join(home, "code"),
		"%BARPHONE_TEST_DIR%/x":        "proj/x",
		"$BARPHONE_TEST_DIR/x":         "proj/x",
		"C:/no/vars":                   "C:/no/vars",
		"50%":                          "50%",
		"%NO_SUCH_BARPHONE_VAR%/files": "/files",
	} {
		if got := expandDir(in); got != want {
			t.Errorf("expandDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandLine(t *testing.T) {
	cmd, err := Command(store.Button{Kind: store.KindCommand, Target: `git pull && echo "done"`, Dir: "~", KeepOpen: true})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if cmd.Dir != home {
		t.Fatalf("dir: %q", cmd.Dir)
	}
	if runtime.GOOS != "windows" {
		if strings.Join(cmd.Args[1:], " ") != `-lc git pull && echo "done"` {
			t.Fatalf("args: %q", cmd.Args)
		}
		return
	}
	// Built, never started: the console window would pop up on the desktop.
	line := commandLine(cmd)
	if !strings.HasSuffix(line, `" /k git pull && echo "done"`) || !strings.Contains(strings.ToLower(line), "cmd.exe") {
		t.Fatalf("command line: %s", line)
	}
}
