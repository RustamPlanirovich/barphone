//go:build !windows && !(darwin && cgo)

// Package tray shows the agent's menu-bar / notification-area icon.
// This build has no tray (e.g. macOS without cgo, Linux): Run just blocks.
package tray

import "sync"

type Menu struct {
	OpenUI func()
	Pair   func()
	Quit   func()
}

var (
	done     = make(chan struct{})
	quitOnce sync.Once
)

func Run(string, Menu) { <-done }

func Quit() { quitOnce.Do(func() { close(done) }) }
