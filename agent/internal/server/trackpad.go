package server

import (
	"errors"
	"unicode/utf8"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

const maxPointerStep = 2000 // px in one "pointer" message

// remoteInput handles the trackpad screen: pointer moves, wheel, clicks, typing and
// keys. It is accepted only on behalf of a trackpad button: free control of the mouse
// and keyboard is something the PC's owner turns on by adding one. Moves and wheel are
// not answered (the phone sends them many times a second); the rest is, when asked
// (req set).
func (s *Server) remoteInput(msg inbound) (resultMsg, bool) {
	res := resultMsg{Type: "result", Req: msg.Req}
	answer := msg.Req != "" && msg.Type != "pointer" && msg.Type != "scroll"
	cfg := s.Store.Snapshot()
	if b, ok := cfg.Button(msg.ID); !ok || b.Kind != store.KindTrackpad {
		res.Error = "not_found"
		return res, answer
	}
	var err error
	switch msg.Type {
	case "pointer":
		err = s.Launcher.MovePointer(clamp(msg.DX, maxPointerStep), clamp(msg.DY, maxPointerStep))
	case "scroll":
		err = s.Launcher.Scroll(clamp(msg.DX, 120*20), clamp(msg.DY, 120*20))
	case "click":
		err = s.Launcher.Click(msg.Button, msg.Double)
	case "type":
		if msg.Text == "" || utf8.RuneCountInString(msg.Text) > launch.MaxTextLen {
			res.Error = "bad_request"
			return res, answer
		}
		err = s.Launcher.Launch(store.Button{Kind: store.KindText, Target: msg.Text})
	case "key":
		if _, perr := launch.ParseCombo(msg.Key); perr != nil {
			res.Error = "bad_request"
			return res, answer
		}
		err = s.Launcher.Launch(store.Button{Kind: store.KindKeys, Target: msg.Key})
	}
	switch {
	case errors.Is(err, launch.ErrUnsupported):
		res.Error = "unsupported"
	case err != nil:
		s.Log.Printf("trackpad %s: %v", msg.Type, err)
		res.Error = "launch_failed"
	default:
		res.OK, res.Action = true, actionDone
	}
	return res, answer
}

func clamp(v, limit int) int { return min(max(v, -limit), limit) }
