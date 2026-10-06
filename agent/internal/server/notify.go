package server

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"barphone/agent/internal/store"
)

// Notice is a message for the phones: a command from the deck finished, or a script on
// the PC reported something (POST /api/notify, barphone-agent -notify).
type Notice struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Level string `json:"level"` // info | ok | error
}

type notifyMsg struct {
	Type string `json:"type"`
	Notice
}

const (
	maxNoticeTitle = 100
	maxNoticeText  = 500
)

func (n Notice) normalized() (Notice, error) {
	n.Title, n.Text = strings.TrimSpace(n.Title), strings.TrimSpace(n.Text)
	if n.Title == "" && n.Text == "" {
		return n, fmt.Errorf("пустое уведомление")
	}
	if utf8.RuneCountInString(n.Title) > maxNoticeTitle || utf8.RuneCountInString(n.Text) > maxNoticeText {
		return n, fmt.Errorf("заголовок до %d и текст до %d символов", maxNoticeTitle, maxNoticeText)
	}
	switch n.Level {
	case "info", "ok", "error":
	default:
		n.Level = "info"
	}
	return n, nil
}

// Notify sends a notice to every connected phone and tells how many got it.
func (s *Server) Notify(n Notice) int {
	s.hub.broadcast(mustJSON(notifyMsg{Type: "notify", Notice: n}))
	return len(s.hub.online())
}

// runCommand starts a command button and, once it ends, tells the phones how it went
// (unless its console stays open: then the user reads the result there).
func (s *Server) runCommand(b store.Button) error {
	cmd, err := s.Command(b)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		if b.KeepOpen {
			return
		}
		n := Notice{Title: b.Title, Text: "Готово", Level: "ok"}
		switch {
		case err == nil:
		case cmd.ProcessState != nil:
			n.Text, n.Level = fmt.Sprintf("Ошибка (код %d)", cmd.ProcessState.ExitCode()), "error"
		default:
			n.Text, n.Level = "Ошибка: "+err.Error(), "error"
		}
		s.Notify(n)
	}()
	return nil
}

// uiNotify lets scripts on this PC notify the phones (see docs/protocol.md).
func (s *Server) uiNotify(w http.ResponseWriter, r *http.Request) {
	var in Notice
	if err := readJSON(r, &in, 16<<10); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	n, err := in.normalized()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"phones": s.Notify(n)})
}
