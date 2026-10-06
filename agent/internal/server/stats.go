package server

import (
	"context"
	"runtime"
	"time"

	"barphone/agent/internal/store"
	"barphone/agent/internal/sysstat"
)

// statsEvery is how often live tiles get fresh numbers.
var statsEvery = 2 * time.Second

type statsMsg struct {
	Type string `json:"type"`
	sysstat.Sample
}

// statsLoop measures the computer only while a phone is connected and some deck has a
// live tile, and sends the numbers to the phones.
func (s *Server) statsLoop(ctx context.Context) {
	t := time.NewTicker(statsEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if len(s.hub.online()) == 0 || !s.hasStatTiles() {
			continue
		}
		sample, err := s.Stats.Read()
		if err != nil {
			continue // not measurable on this OS: the tiles keep a dash
		}
		s.hub.broadcast(mustJSON(statsMsg{Type: "stats", Sample: sample}))
	}
}

func (s *Server) hasStatTiles() bool {
	cfg := s.Store.Snapshot()
	for _, b := range cfg.AllButtons() {
		if b.Kind == store.KindStat {
			return true
		}
	}
	return false
}

// monitorApp is what a tap on a live tile opens: the task manager of this OS.
func monitorApp() store.Button {
	if runtime.GOOS == "darwin" {
		return store.Button{Kind: store.KindApp, Target: "/System/Applications/Utilities/Activity Monitor.app", Title: "Мониторинг системы"}
	}
	return store.Button{Kind: store.KindPath, Target: "taskmgr.exe", Title: "Диспетчер задач"}
}
