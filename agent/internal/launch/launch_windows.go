//go:build windows

package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"barphone/agent/internal/store"
	"barphone/agent/internal/winutil"
)

type winLauncher struct {
	launchThread *comThread // ShellExecute must run on a COM-initialized STA thread
	iconThread   *comThread // separate, so slow icon extraction never delays a button press
	brightThread *comThread // WMI and DDC/CI are slow: keep them off the launch thread too
}

func New() Launcher {
	return &winLauncher{launchThread: newCOMThread(), iconThread: newCOMThread(), brightThread: newCOMThread()}
}

func (w *winLauncher) Launch(b store.Button) error {
	switch b.Kind {
	case store.KindKeys:
		c, err := ParseCombo(b.Target)
		if err != nil {
			return err
		}
		return sendCombo(c)
	case store.KindText:
		return sendInputs(textInputs(b.Target))
	case store.KindSystem:
		return w.runSystem(b.Target)
	}
	file, args, dir := resolve(b)
	return w.launchThread.do(func() error {
		unlockForeground()
		return shellExecute(file, args, dir)
	})
}

func (w *winLauncher) OpenURL(u string) error {
	return w.launchThread.do(func() error { return shellExecute(u, "", "") })
}

func resolve(b store.Button) (file, args, dir string) {
	switch b.Kind {
	case store.KindApp:
		return `shell:AppsFolder\` + b.Target, "", ""
	case store.KindPath:
		file = expandEnv(strings.Trim(strings.TrimSpace(b.Target), `"`))
		if st, err := os.Stat(file); err == nil && !st.IsDir() {
			dir = filepath.Dir(file)
		}
		return file, b.Args, dir
	default:
		return strings.TrimSpace(b.Target), "", ""
	}
}

func expandEnv(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if out, err := registry.ExpandString(s); err == nil {
		return out
	}
	return s
}

func shellExecute(file, args, dir string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	var a, d *uint16
	if args != "" {
		if a, err = windows.UTF16PtrFromString(args); err != nil {
			return err
		}
	}
	if dir != "" {
		if d, err = windows.UTF16PtrFromString(dir); err != nil {
			return err
		}
	}
	if err := windows.ShellExecute(0, verb, f, a, d, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("ShellExecute %q: %w", file, err)
	}
	return nil
}

// --- foreground handling -------------------------------------------------
//
// The agent is a background process, so Windows' foreground lock would normally make a
// freshly launched app open *behind* the active window (with a blinking taskbar button).
// A process that received the last input event may set the foreground window, so we
// inject a no-op input event and then delegate that right to whatever we launch.

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSendInput                = user32.NewProc("SendInput")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

const (
	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
	vkF24          = 0x87 // a key no application binds; avoids Alt's menu-bar side effect
	asfwAny        = ^uintptr(0)
)

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input mirrors the Win32 INPUT struct; the union is padded to MOUSEINPUT's size.
type input struct {
	typ uint32
	ki  keybdInput
	_   [8]byte
}

func unlockForeground() {
	in := [2]input{
		{typ: inputKeyboard, ki: keybdInput{wVk: vkF24}},
		{typ: inputKeyboard, ki: keybdInput{wVk: vkF24, dwFlags: keyeventfKeyUp}},
	}
	procSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	procAllowSetForegroundWindow.Call(asfwAny)
}

// --- installed apps ------------------------------------------------------

var uninstallRe = regexp.MustCompile(`(?i)uninstall|deinstall|удалить|удаление|деинсталл`)

func (w *winLauncher) Apps(ctx context.Context) ([]App, error) {
	out, err := winutil.PowerShell(ctx, `Get-StartApps | Select-Object Name,AppID | ConvertTo-Json -Compress`)
	if err != nil {
		return nil, fmt.Errorf("Get-StartApps: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	type row struct{ Name, AppID string }
	var rows []row
	if strings.HasPrefix(out, "{") {
		var r row
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			return nil, err
		}
		rows = []row{r}
	} else if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	apps := make([]App, 0, len(rows))
	for _, r := range rows {
		if r.AppID == "" || r.Name == "" || seen[r.AppID] || uninstallRe.MatchString(r.Name) {
			continue
		}
		seen[r.AppID] = true
		apps = append(apps, App{Name: r.Name, Kind: store.KindApp, Target: r.AppID})
	}
	return apps, nil
}

// PickFile shows the native open-file dialog on a fresh STA thread, so a user taking
// minutes to choose never blocks button launches.
func (w *winLauncher) PickFile(ctx context.Context) (string, error) {
	type result struct {
		path string
		err  error
	}
	res := make(chan result, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread exits with the goroutine
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
			defer windows.CoUninitialize()
		}
		unlockForeground() // the browser is in front; let the dialog take focus
		p, err := openFileDialog(pickTitle)
		res <- result{p, err}
	}()
	r := <-res
	return r.path, r.err
}

// --- icons ---------------------------------------------------------------

func (w *winLauncher) Icon(kind store.ButtonKind, target string) (image.Image, error) {
	var name string
	switch kind {
	case store.KindApp:
		name = `shell:AppsFolder\` + target
	case store.KindPath:
		name, _, _ = resolve(store.Button{Kind: kind, Target: target})
		if !filepath.IsAbs(name) {
			if p, err := exec.LookPath(name); err == nil {
				name = p
			}
		}
	default:
		return nil, ErrUnsupported
	}
	var img image.Image
	err := w.iconThread.do(func() (err error) {
		img, err = shellItemImage(name, 256)
		return err
	})
	return img, err
}

// --- COM thread ----------------------------------------------------------

type comThread struct{ ch chan func() }

func newCOMThread() *comThread {
	t := &comThread{ch: make(chan func())}
	go func() {
		runtime.LockOSThread()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil {
			// S_FALSE (already initialized) surfaces as an error too; COM is usable either way.
			_ = err
		}
		for f := range t.ch {
			f()
		}
	}()
	return t
}

func (t *comThread) do(f func() error) error {
	res := make(chan error, 1)
	t.ch <- func() {
		defer func() {
			if r := recover(); r != nil {
				res <- fmt.Errorf("panic: %v", r)
			}
		}()
		res <- f()
	}
	return <-res
}
