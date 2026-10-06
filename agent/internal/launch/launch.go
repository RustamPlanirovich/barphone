// Package launch starts configured buttons, lists installed applications and
// extracts their icons using the native facilities of each OS.
package launch

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"barphone/agent/internal/store"
)

// App is an installed application offered in the config UI picker.
type App struct {
	Name   string           `json:"name"`
	Kind   store.ButtonKind `json:"kind"`
	Target string           `json:"target"`
}

type Launcher interface {
	// Launch starts the button's target. Must not block for long.
	Launch(b store.Button) error
	// Apps lists installed applications (uncached; may take seconds).
	Apps(ctx context.Context) ([]App, error)
	// Icon returns the native icon for a target, or an error if there is none.
	Icon(kind store.ButtonKind, target string) (image.Image, error)
	// PickFile shows a native "open file" dialog; returns "" if cancelled.
	PickFile(ctx context.Context) (string, error)
	// OpenURL opens a URL in the default browser (used for the config UI).
	OpenURL(u string) error
	// Windows lists the open top-level windows that belong to the button's app, most
	// recently active first. Nil when there are none or the OS cannot tell.
	Windows(b store.Button) ([]Window, error)
	// Focus brings one of those windows to the front.
	Focus(b store.Button, windowID string) error
	// Volume reads the master output volume; SetVolume sets it (0..1) and unmutes.
	Volume() (VolumeState, error)
	SetVolume(level float64) error
	// AppKeys lists the foreground keys (see ForegroundApp) an app/path button stands
	// for, so a profile can be bound to that app. Nil if it cannot tell.
	AppKeys(b store.Button) []string
}

// Window is an open window of a button's app, as offered to the phone.
type Window struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

var (
	ErrUnsupported = errors.New("not supported on this OS")
	ErrWindowGone  = errors.New("window is gone")
)

// ShortTitles drops the suffix all titles share, e.g. " - Visual Studio Code", so the
// phone shows "main.go - barphone" instead of a column of identical endings.
func ShortTitles(ws []Window) {
	if len(ws) < 2 {
		return
	}
	for _, sep := range []string{" - ", " — ", " – "} {
		i := strings.LastIndex(ws[0].Title, sep)
		if i <= 0 {
			continue
		}
		suffix := ws[0].Title[i:]
		for _, w := range ws {
			if !strings.HasSuffix(w.Title, suffix) || len(w.Title) == len(suffix) {
				suffix = ""
				break
			}
		}
		if suffix == "" {
			continue
		}
		for i := range ws {
			ws[i].Title = strings.TrimSuffix(ws[i].Title, suffix)
		}
		return
	}
}

// Validate checks a button before it is saved into the deck.
func Validate(b store.Button) error {
	if !b.Kind.Valid() {
		return errors.New("неизвестный тип кнопки")
	}
	switch b.Kind {
	case store.KindKeys:
		_, err := ParseCombo(b.Target)
		return err
	case store.KindText:
		if b.Target == "" || len([]rune(b.Target)) > MaxTextLen {
			return fmt.Errorf("текст должен быть от 1 до %d символов", MaxTextLen)
		}
		return nil
	case store.KindSystem:
		if _, ok := LookupSystemAction(b.Target); !ok {
			return fmt.Errorf("неизвестное действие %q", b.Target)
		}
		return nil
	}
	t := strings.TrimSpace(b.Target)
	if t == "" || len(t) > 2048 {
		return errors.New("пустая или слишком длинная цель")
	}
	if b.Kind == store.KindURL {
		u, err := url.Parse(t)
		if err != nil || u.Scheme == "" || len(u.Scheme) < 2 {
			return errors.New("ссылка должна быть вида https://… или steam://…")
		}
		if s := strings.ToLower(u.Scheme); s == "file" || s == "javascript" {
			return errors.New("для файлов используйте тип «Файл или программа»")
		}
	}
	return nil
}

// AppCache memoizes the (slow) installed apps listing.
type AppCache struct {
	L   Launcher
	TTL time.Duration

	mu   sync.Mutex
	apps []App
	at   time.Time
}

func (c *AppCache) Get(ctx context.Context, refresh bool) ([]App, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !refresh && c.apps != nil && time.Since(c.at) < c.TTL {
		return c.apps, nil
	}
	apps, err := c.L.Apps(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	c.apps, c.at = apps, time.Now()
	return apps, nil
}
