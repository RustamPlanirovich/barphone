package server

import (
	"context"
	"strconv"
	"time"

	"barphone/agent/internal/store"
)

// macroGap separates macro steps: a program that was just started or brought up gets a
// moment to take the focus before the next step types into it.
var macroGap = 250 * time.Millisecond

// startMacro runs a macro's steps in the background, one macro at a time (keystrokes of
// two macros must never interleave); false means another one is still running.
func (s *Server) startMacro(m store.Button) bool {
	if !s.macroBusy.CompareAndSwap(false, true) {
		return false
	}
	ctx := context.Background()
	if p := s.runCtx.Load(); p != nil {
		ctx = *p
	}
	go func() {
		defer s.macroBusy.Store(false)
		for i, st := range m.Steps {
			if i > 0 && !pause(ctx, macroGap) {
				return
			}
			if st.Kind == store.KindWait {
				ms, _ := strconv.Atoi(st.Target)
				if !pause(ctx, time.Duration(ms)*time.Millisecond) {
					return
				}
				continue
			}
			if err := s.runStep(st); err != nil {
				s.Log.Printf("macro %q, step %d %q: %v", m.Title, i+1, st.Title, err)
			}
		}
	}()
	return true
}

// runStep does one action. Unlike a button press, an app that is already open is simply
// brought up: nobody is there to pick a window, and putting it away would be surprising.
func (s *Server) runStep(st store.Button) error {
	if st.Kind == store.KindCommand {
		return s.runCommand(st)
	}
	if st.Kind.Launches() && st.OnRunning != store.OnRunningNew {
		if ws, _ := s.Launcher.Windows(st); len(ws) > 0 && s.Launcher.Focus(st, ws[0].ID) == nil {
			return nil
		}
	}
	return s.Launcher.Launch(st)
}

// pause sleeps unless the agent is shutting down; false means stop.
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
