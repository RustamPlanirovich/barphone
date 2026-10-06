package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"barphone/agent/internal/store"
)

func TestCommandsAndNotices(t *testing.T) {
	e := newEnv(t)
	for name, bad := range map[string]store.Button{
		"empty":     {Kind: store.KindCommand, Target: "  "},
		"two lines": {Kind: store.KindCommand, Target: "a\nb"},
		"confirmed step": {Kind: store.KindMacro, Steps: []store.Button{
			{Kind: store.KindCommand, Target: "deploy", Confirm: true},
		}},
	} {
		if resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{bad}}); resp.StatusCode != 400 {
			t.Errorf("%s must be rejected: %s", name, data)
		}
	}
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindCommand, Target: "make build", Dir: " ~/proj ", Confirm: true, Title: "Сборка"},
		{Kind: store.KindCommand, Target: "exit 3"},
		{Kind: store.KindCommand, Target: "dir", KeepOpen: true, Title: "Список"},
		{Kind: store.KindURL, Target: "https://example.com", Dir: "x", Confirm: true, KeepOpen: true},
		{Kind: store.KindMacro, Title: "Утро", Steps: []store.Button{{Kind: store.KindCommand, Target: "git pull", Dir: "~/proj"}}},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 5 {
		t.Fatalf("deck: %s", data)
	}
	build, fail, keep, link, macro := deck.Buttons[0], deck.Buttons[1], deck.Buttons[2], deck.Buttons[3], deck.Buttons[4]
	if build.Dir != "~/proj" || !build.Confirm || fail.Title != "exit 3" {
		t.Fatalf("commands normalized: %+v %+v", build, fail)
	}
	if link.Dir != "" || link.Confirm || link.KeepOpen {
		t.Fatalf("command options only for commands: %+v", link)
	}

	status, out := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	state := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 5 })
	if w := buttons(state)[0].(map[string]any); w["glyph"] != "command" || w["confirm"] != true {
		t.Fatalf("command on the wire: %v", w)
	}
	if raw, _ := json.Marshal(state); strings.Contains(string(raw), "make build") || strings.Contains(string(raw), "proj") {
		t.Fatal("command lines and folders must not reach the phone")
	}

	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	if r := send(map[string]any{"type": "launch", "req": "1", "id": build.ID}); r["error"] != "confirm_required" {
		t.Fatalf("a command marked «confirm» without confirmation: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "2", "id": build.ID, "confirmed": true}); r["action"] != "done" {
		t.Fatalf("confirmed command: %v", r)
	}
	if n := readMsg(t, ws, "notify"); n["title"] != "Сборка" || n["text"] != "Готово" || n["level"] != "ok" {
		t.Fatalf("finished command: %v", n)
	}
	if r := send(map[string]any{"type": "launch", "req": "3", "id": fail.ID}); r["action"] != "done" {
		t.Fatalf("failing command still starts: %v", r)
	}
	if n := readMsg(t, ws, "notify"); n["text"] != "Ошибка (код 3)" || n["level"] != "error" {
		t.Fatalf("failed command: %v", n)
	}

	// A console left open: the user reads the result there, no notice.
	send(map[string]any{"type": "launch", "req": "4", "id": keep.ID})
	time.Sleep(500 * time.Millisecond)
	ws.WriteJSON(map[string]any{"type": "launch", "req": "probe", "id": "no-such-button"})
	for {
		var m map[string]any
		ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m["type"] == "notify" {
			t.Fatalf("notice for a command whose console stays open: %v", m)
		}
		if m["req"] == "probe" {
			break
		}
	}

	// A macro step can be a command too.
	send(map[string]any{"type": "launch", "req": "5", "id": macro.ID})
	if n := readMsg(t, ws, "notify"); n["title"] != "git pull" {
		t.Fatalf("command step: %v", n)
	}
	e.fake.mu.Lock()
	ran := strings.Join(e.fake.commands, "; ")
	e.fake.mu.Unlock()
	if ran != "make build @ ~/proj; exit 3 @ ; dir @ ; git pull @ ~/proj" {
		t.Fatalf("commands: %s", ran)
	}

	// Scripts on the PC notify the phones.
	resp, body := e.call("POST", "/api/notify", map[string]string{"title": "Деплой", "text": "Прошёл", "level": "ok"})
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"phones":1`) {
		t.Fatalf("notify: %d %s", resp.StatusCode, body)
	}
	if n := readMsg(t, ws, "notify"); n["title"] != "Деплой" || n["text"] != "Прошёл" {
		t.Fatalf("script notice: %v", n)
	}
	if resp, _ := e.call("POST", "/api/notify", map[string]string{"text": "  "}); resp.StatusCode != 400 {
		t.Fatal("empty notice")
	}
	if resp, _ := e.call("POST", "/api/notify", map[string]string{"text": strings.Repeat("я", 501)}); resp.StatusCode != 400 {
		t.Fatal("too long notice")
	}
	// Web pages cannot do it: no custom header without CORS, which the agent never allows.
	raw, _ := http.NewRequest("POST", e.ui.URL+"/api/notify", bytes.NewReader([]byte(`{"text":"x"}`)))
	raw.Header.Set("Content-Type", "application/json")
	if r, err := http.DefaultClient.Do(raw); err != nil || r.StatusCode != 403 {
		t.Fatalf("notify without the UI header: %v %v", r, err)
	}
}
