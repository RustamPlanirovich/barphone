package server

import (
	"context"
	"errors"
	"reflect"
	"time"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

const actionDesktop = "desktop"

// desktopsEvery is how often the agent looks whether the virtual desktops changed.
var desktopsEvery = time.Second

type desktopsMsg struct {
	Type string `json:"type"`
	launch.DesktopInfo
}

// desktopMsg switches virtual desktops for the "desktops" tile and the trackpad.
func (s *Server) desktopMsg(msg inbound) resultMsg {
	res := resultMsg{Type: "result", Req: msg.Req}
	cfg := s.Store.Snapshot()
	b, ok := cfg.Button(msg.ID)
	if !ok || !(b.Kind == store.KindTrackpad || b.Kind == store.KindSystem && b.Target == "desktops") {
		res.Error = "not_found"
		return res
	}
	info, infoErr := s.Launcher.Desktops()
	var err error
	switch {
	case msg.Overview:
		err = s.Launcher.Launch(store.Button{Kind: store.KindSystem, Target: "task_view"})
	case msg.To != nil:
		if infoErr != nil {
			err = infoErr
			break
		}
		to := min(max(*msg.To, 0), max(info.Count-1, 0))
		err = s.Launcher.MoveDesktop(to - info.Current)
		info.Current = to
	case msg.Move != 0:
		steps := clamp(msg.Move, 20)
		err = s.Launcher.MoveDesktop(steps)
		// Windows does not wrap around; the registry follows a moment later.
		info.Current = min(max(info.Current+steps, 0), max(info.Count-1, 0))
	}
	switch {
	case errors.Is(err, launch.ErrUnsupported):
		res.Error = "unsupported"
		return res
	case err != nil:
		s.Log.Printf("desktop: %v", err)
		res.Error = "launch_failed"
		return res
	}
	res.OK, res.Action = true, actionDesktop
	if infoErr == nil {
		res.Desktops = &info
	}
	return res
}

// desktopsLoop tells the phones when the virtual desktops change (switched on the PC,
// added, renamed), while one is connected and some deck has the desktops tile.
func (s *Server) desktopsLoop(ctx context.Context) {
	t := time.NewTicker(desktopsEvery)
	defer t.Stop()
	var last *launch.DesktopInfo
	lastPhones := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		phones := len(s.hub.online())
		if phones == 0 || !s.hasDesktopsTile() {
			last, lastPhones = nil, 0
			continue
		}
		info, err := s.Launcher.Desktops()
		if err != nil {
			continue
		}
		if last != nil && phones <= lastPhones && reflect.DeepEqual(*last, info) {
			continue // nothing new, and no phone joined that has not heard it
		}
		s.hub.broadcast(mustJSON(desktopsMsg{Type: "desktops", DesktopInfo: info}))
		last, lastPhones = &info, phones
	}
}

func (s *Server) hasDesktopsTile() bool {
	cfg := s.Store.Snapshot()
	for _, b := range cfg.AllButtons() {
		if b.Kind == store.KindSystem && b.Target == "desktops" {
			return true
		}
	}
	return false
}
