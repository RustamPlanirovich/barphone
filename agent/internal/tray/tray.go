//go:build windows || (darwin && cgo)

// Package tray shows the agent's menu-bar / notification-area icon.
package tray

import (
	"runtime"

	"fyne.io/systray"
)

type Menu struct {
	OpenUI func()
	Pair   func()
	Quit   func()
}

// Run blocks (it must own the main thread on macOS) until Quit is called.
func Run(tooltip string, m Menu) {
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(GlyphICO(16, 32, 48))
		} else {
			systray.SetIcon(GlyphPNG(44))
		}
		systray.SetTooltip(tooltip)
		systray.SetOnTapped(m.OpenUI) // left click on Windows

		open := systray.AddMenuItem("Открыть настройки", "")
		pair := systray.AddMenuItem("Подключить телефон…", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Выход", "")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					m.OpenUI()
				case <-pair.ClickedCh:
					m.Pair()
				case <-quit.ClickedCh:
					m.Quit()
					return
				}
			}
		}()
	}, nil)
}

func Quit() { systray.Quit() }
