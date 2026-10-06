//go:build darwin

package launch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"barphone/agent/internal/store"
)

type macLauncher struct{}

func New() Launcher { return macLauncher{} }

// Launch uses `open`, which also brings an already running app to the front.
func (m macLauncher) Launch(b store.Button) error {
	switch b.Kind {
	case store.KindKeys:
		c, err := ParseCombo(b.Target)
		if err != nil {
			return err
		}
		return osascript(comboScript(c))
	case store.KindText:
		return osascript(`tell application "System Events" to keystroke ` + appleString(b.Target))
	case store.KindSystem:
		return m.runSystem(b.Target)
	}
	target := strings.TrimSpace(b.Target)
	var args []string
	switch b.Kind {
	case store.KindApp:
		args = []string{"-a", target}
	case store.KindPath:
		target = expandHome(target)
		if b.Args != "" {
			args = append([]string{"-a", target, "--args"}, strings.Fields(b.Args)...)
		} else {
			args = []string{target}
		}
	default:
		args = []string{target}
	}
	return runOpen(args...)
}

func (macLauncher) OpenURL(u string) error { return runOpen(u) }

func runOpen(args ...string) error {
	out, err := exec.Command("/usr/bin/open", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("open %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func (macLauncher) Apps(ctx context.Context) ([]App, error) {
	home, _ := os.UserHomeDir()
	dirs := []string{
		"/Applications", "/Applications/Utilities",
		"/System/Applications", "/System/Applications/Utilities",
		filepath.Join(home, "Applications"),
	}
	seen := map[string]bool{}
	var apps []App
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".app")
			if seen[name] {
				continue
			}
			seen[name] = true
			apps = append(apps, App{Name: name, Kind: store.KindApp, Target: filepath.Join(d, e.Name())})
		}
	}
	return apps, nil
}

// iconScript renders any file's Finder icon into a 256x256 PNG via AppKit (JXA).
const iconScript = `
ObjC.import('AppKit');
function run(argv) {
  var img = $.NSWorkspace.sharedWorkspace.iconForFile(argv[0]);
  var rep = $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
    null, 256, 256, 8, 4, true, false, $.NSCalibratedRGBColorSpace, 0, 0);
  $.NSGraphicsContext.saveGraphicsState;
  $.NSGraphicsContext.setCurrentContext($.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep));
  img.drawInRectFromRectOperationFraction($.NSMakeRect(0, 0, 256, 256), $.NSZeroRect, $.NSCompositingOperationCopy, 1.0);
  $.NSGraphicsContext.restoreGraphicsState;
  var data = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $.NSDictionary.dictionary);
  data.writeToFileAtomically(argv[1], true);
}`

func (macLauncher) Icon(kind store.ButtonKind, target string) (image.Image, error) {
	if kind == store.KindURL {
		return nil, ErrUnsupported
	}
	path := expandHome(strings.TrimSpace(target))
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "barphone-icon-*.png")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", iconScript, path, tmp.Name()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("osascript icon: %w: %s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty icon")
	}
	return png.Decode(bytes.NewReader(data))
}

func (macLauncher) PickFile(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e",
		`try
	POSIX path of (choose file with prompt "barphone: выберите программу или файл")
on error number -128
	return ""
end try`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSpace(string(out)), "/"), nil
}

// Listing a specific app's windows needs Accessibility permission on macOS; `open -a`
// already brings a running app to the front, so the phone simply launches.
func (macLauncher) Windows(store.Button) ([]Window, error) { return nil, nil }

func (macLauncher) Focus(store.Button, string) error { return ErrUnsupported }

// Windows() reports none on macOS, so a press never asks to minimize.
func (macLauncher) Minimize(store.Button, string) error { return ErrUnsupported }
