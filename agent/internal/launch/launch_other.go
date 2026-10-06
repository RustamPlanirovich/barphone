//go:build !windows && !darwin

package launch

import (
	"context"
	"image"
	"os/exec"
	"strings"

	"barphone/agent/internal/store"
)

// Minimal fallback (Linux etc.): xdg-open for everything, no app listing or icons.
type xdgLauncher struct{}

func New() Launcher { return xdgLauncher{} }

func (xdgLauncher) Launch(b store.Button) error {
	switch b.Kind {
	case store.KindKeys, store.KindText, store.KindSystem:
		return ErrUnsupported
	}
	if b.Kind == store.KindPath && b.Args != "" {
		return exec.Command(b.Target, strings.Fields(b.Args)...).Start()
	}
	return exec.Command("xdg-open", b.Target).Start()
}

func (xdgLauncher) OpenURL(u string) error { return exec.Command("xdg-open", u).Start() }

func (xdgLauncher) Apps(context.Context) ([]App, error) { return nil, nil }

func (xdgLauncher) Icon(store.ButtonKind, string) (image.Image, error) { return nil, ErrUnsupported }

func (xdgLauncher) PickFile(context.Context) (string, error) { return "", ErrUnsupported }

func (xdgLauncher) Windows(store.Button) ([]Window, error) { return nil, nil }

func (xdgLauncher) Focus(store.Button, string) error { return ErrUnsupported }

func (xdgLauncher) Volume() (VolumeState, error) { return VolumeState{}, ErrUnsupported }

func (xdgLauncher) SetVolume(float64) error { return ErrUnsupported }

func (xdgLauncher) Brightness() (float64, error) { return 0, ErrUnsupported }

func (xdgLauncher) MovePointer(int, int) error { return ErrUnsupported }

func (xdgLauncher) Click(string, bool) error { return ErrUnsupported }

func (xdgLauncher) Scroll(int, int) error { return ErrUnsupported }

func (xdgLauncher) Desktops() (DesktopInfo, error) { return DesktopInfo{}, ErrUnsupported }

func (xdgLauncher) MoveDesktop(int) error { return ErrUnsupported }

func (xdgLauncher) Call() (CallInfo, error) { return CallInfo{}, ErrUnsupported }

func (xdgLauncher) CallAction(string) error { return ErrUnsupported }

func (xdgLauncher) SetBrightness(float64) error { return ErrUnsupported }

func WatchForeground(ctx context.Context, onChange func(ForegroundApp)) { <-ctx.Done() }

func (xdgLauncher) AppKeys(store.Button) []string { return nil }

func (xdgLauncher) Minimize(store.Button, string) error { return ErrUnsupported }
