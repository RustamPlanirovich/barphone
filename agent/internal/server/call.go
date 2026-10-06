package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

const actionCall = "call"

// callEvery is how often the agent looks whether a Meet call is going on.
var callEvery = 1500 * time.Millisecond

type callMsg struct {
	Type string `json:"type"`
	launch.CallInfo
}

// callLoop tells the phones when a call starts, ends or its camera changes, while one
// is connected and the owner has not turned the call controls off.
func (s *Server) callLoop(ctx context.Context) {
	t := time.NewTicker(callEvery)
	defer t.Stop()
	var last *launch.CallInfo
	lastPhones := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		phones := len(s.hub.online())
		if phones == 0 {
			last, lastPhones = nil, 0
			continue
		}
		var info launch.CallInfo
		if cfg := s.Store.Snapshot(); !cfg.NoMeetControls {
			var err error
			if info, err = s.Launcher.Call(); err != nil {
				continue // not on this OS
			}
		}
		if last != nil && phones <= lastPhones && *last == info {
			continue
		}
		s.hub.broadcast(mustJSON(callMsg{Type: "call", CallInfo: info}))
		last, lastPhones = &info, phones
	}
}

// callAction is the phone's call panel: microphone, camera, raise hand, show Meet.
func (s *Server) callAction(msg inbound) resultMsg {
	res := resultMsg{Type: "result", Req: msg.Req}
	if cfg := s.Store.Snapshot(); cfg.NoMeetControls {
		res.Error = "unsupported"
		return res
	}
	switch msg.Action {
	case "mic", "camera", "hand", "show":
	default:
		res.Error = "bad_request"
		return res
	}
	switch err := s.Launcher.CallAction(msg.Action); {
	case errors.Is(err, launch.ErrWindowGone):
		res.Error = "not_found"
	case errors.Is(err, launch.ErrUnsupported):
		res.Error = "unsupported"
	case err != nil:
		s.Log.Printf("call %s: %v", msg.Action, err)
		res.Error = "launch_failed"
	default:
		res.OK, res.Action = true, actionCall
	}
	return res
}

// uiSetMeet turns the phone's Meet call controls on or off.
func (s *Server) uiSetMeet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if readJSON(r, &in, 1<<10) != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.Store.Update(func(c *store.Config) error { c.NoMeetControls = !in.Enabled; return nil }); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": in.Enabled})
}
