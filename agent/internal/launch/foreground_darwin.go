//go:build darwin

package launch

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"barphone/agent/internal/store"
)

// macOS: lsappinfo reports the frontmost app without any permission prompt (System
// Events would ask for Automation access). Polled once a second.

var (
	asnRe    = regexp.MustCompile(`ASN:[0-9a-fA-Fx-]+:`)
	bundleRe = regexp.MustCompile(`"CFBundleIdentifier"="([^"]+)"`)
	nameRe   = regexp.MustCompile(`"LSDisplayName"="([^"]+)"`)
)

func WatchForeground(ctx context.Context, onChange func(ForegroundApp)) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var reported string
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		front, err := exec.Command("/usr/bin/lsappinfo", "front").Output()
		if err != nil {
			continue
		}
		asn := asnRe.Find(front)
		if asn == nil {
			continue
		}
		info, err := exec.Command("/usr/bin/lsappinfo", "info", "-only", "bundleid", string(asn)).Output()
		if err != nil {
			continue
		}
		m := bundleRe.FindSubmatch(info)
		if m == nil || strings.EqualFold(string(m[1]), "com.apple.loginwindow") {
			continue
		}
		app := ForegroundApp{Keys: []string{"bundle:" + strings.ToLower(string(m[1]))}}
		if app.ID() == reported {
			continue
		}
		if out, err := exec.Command("/usr/bin/lsappinfo", "info", "-only", "name", string(asn)).Output(); err == nil {
			if n := nameRe.FindSubmatch(out); n != nil {
				app.Name = string(n[1])
			}
		}
		reported = app.ID()
		onChange(app)
	}
}

func (macLauncher) AppKeys(b store.Button) []string {
	if b.Kind != store.KindApp && !(b.Kind == store.KindPath && strings.HasSuffix(b.Target, ".app")) {
		return nil
	}
	plist := filepath.Join(expandHome(strings.TrimSpace(b.Target)), "Contents", "Info")
	out, err := exec.Command("/usr/bin/defaults", "read", plist, "CFBundleIdentifier").Output()
	if err != nil {
		return nil
	}
	if id := strings.TrimSpace(string(out)); id != "" {
		return []string{"bundle:" + strings.ToLower(id)}
	}
	return nil
}
