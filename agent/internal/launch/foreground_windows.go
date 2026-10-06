//go:build windows

package launch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"barphone/agent/internal/store"
)

const (
	foregroundPoll   = 200 * time.Millisecond
	foregroundSettle = 300 * time.Millisecond // an app must stay in front this long
)

// WatchForeground reports the application in front whenever it changes (after it has
// stayed in front for a moment). Shell surfaces and the agent itself are ignored, so
// opening the Start menu or Alt+Tab does not count as a switch. Blocks until ctx ends.
func WatchForeground(ctx context.Context, onChange func(ForegroundApp)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}
	self := uint32(os.Getpid())
	exes := map[uint32]string{}
	names := map[string]string{}
	var candidate, reported string
	var candidateApp ForegroundApp
	var since time.Time

	tick := time.NewTicker(foregroundPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if len(exes) > 512 { // pids get reused; keep the cache small and fresh
			exes = map[uint32]string{}
		}
		app, ok := foregroundApp(self, exes, names)
		if !ok {
			continue
		}
		id := app.ID()
		if id != candidate {
			candidate, candidateApp, since = id, app, time.Now()
			continue
		}
		if id != reported && time.Since(since) >= foregroundSettle {
			reported = id
			onChange(candidateApp)
		}
	}
}

// windowKeys computes the match keys of one window; the same function serves the
// foreground poller and the read-only binding check in tests.
func windowKeys(h windows.HWND, self uint32, exes map[uint32]string) (keys []string, exe string, ok bool) {
	var pid uint32
	windows.GetWindowThreadProcessId(h, &pid)
	exe = processExe(pid, exes)
	keys, ok = classifyWindow(filepath.Base(exe), windowClass(h), windowAUMID(h), pid == self)
	return keys, exe, ok
}

func foregroundApp(self uint32, exes map[uint32]string, names map[string]string) (ForegroundApp, bool) {
	h := windows.GetForegroundWindow()
	if h == 0 { // secure desktop (UAC, lock screen) or nothing in front
		return ForegroundApp{}, false
	}
	keys, exe, ok := windowKeys(h, self, exes)
	if !ok {
		return ForegroundApp{}, false
	}
	app := ForegroundApp{Keys: keys}
	switch {
	case keys[0] == "class:"+ExplorerClass:
		app.Name = "Проводник"
	case strings.HasPrefix(keys[0], "exe:"):
		name, cached := names[exe]
		if !cached {
			name = fileDescription(exe)
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
			}
			names[exe] = name
		}
		app.Name = name
	}
	// Store apps get their name from the Start menu list (see server.appName).
	return app, true
}

// fileDescription reads the product's FileDescription ("Visual Studio Code") from the
// exe's version resource.
func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var trans *[2]uint16
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&trans), &n); err != nil || n < 4 {
		return ""
	}
	var desc *uint16
	sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, trans[0], trans[1])
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), sub, unsafe.Pointer(&desc), &n); err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString(desc))
}

// AppKeys says which foreground keys an app/path button stands for, so a profile can be
// bound to it. It uses the same resolution as the window matcher.
func (w *winLauncher) AppKeys(b store.Button) []string {
	var keys []string
	w.launchThread.do(func() error {
		m := resolveMatch(b)
		for exe := range m.exes {
			keys = append(keys, "exe:"+strings.ToLower(filepath.Base(exe)))
		}
		for aumid := range m.aumids {
			keys = append(keys, "aumid:"+aumid)
		}
		for class := range m.classes {
			keys = append(keys, "class:"+class)
		}
		return nil
	})
	return keys
}
