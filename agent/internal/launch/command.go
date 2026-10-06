package launch

import (
	"os"
	"path/filepath"
	"strings"
)

// expandDir resolves a command's working folder: "~" and environment variables
// (%USERPROFILE%, $HOME) are allowed; "" means the user's home.
func expandDir(dir string) string {
	dir = strings.TrimSpace(dir)
	home, _ := os.UserHomeDir()
	if dir == "" {
		return home
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		dir = filepath.Join(home, dir[1:])
	}
	return os.Expand(expandPercent(dir), os.Getenv)
}

// expandPercent turns Windows-style %VAR% into $VAR for os.Expand.
func expandPercent(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j <= 0 {
			break
		}
		b.WriteString(s[:i])
		b.WriteString("${" + s[i+1:i+1+j] + "}")
		s = s[i+2+j:]
	}
	b.WriteString(s)
	return b.String()
}
