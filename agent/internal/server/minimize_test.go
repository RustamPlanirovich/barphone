package server

import (
	"encoding/json"
	"strings"
	"testing"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

func TestPressToggleMinimizes(t *testing.T) {
	e := newEnv(t)
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Title: "Term", Kind: store.KindApp, Target: "term"},
		{Title: "Code", Kind: store.KindApp, Target: "code"},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	term, code := deck.Buttons[0].ID, deck.Buttons[1].ID

	status, out := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}

	// The only window is behind: switch to it.
	e.fake.windows = map[string][]launch.Window{"Term": {{ID: "21", Title: "Terminal"}}}
	if r := send(map[string]any{"type": "launch", "req": "1", "id": term}); r["action"] != "focused" {
		t.Fatalf("behind → focus: %v", r)
	}
	// Now it is in front: the same press minimizes it.
	e.fake.windows["Term"] = []launch.Window{{ID: "21", Title: "Terminal", Active: true}}
	if r := send(map[string]any{"type": "launch", "req": "2", "id": term}); r["action"] != "minimized" || r["ok"] != true {
		t.Fatalf("in front → minimize: %v", r)
	}

	// Several windows: still a choice, and the one in front is marked.
	e.fake.windows["Code"] = []launch.Window{{ID: "11", Title: "a - VS Code", Active: true}, {ID: "12", Title: "b - VS Code"}}
	r := send(map[string]any{"type": "launch", "req": "3", "id": code})
	if r["action"] != "choose" || r["windows"].([]any)[0].(map[string]any)["active"] != true {
		t.Fatalf("choose with active flag: %v", r)
	}
	// From the chooser: minimize one, or all.
	if r := send(map[string]any{"type": "minimize", "req": "4", "id": code, "window": "11"}); r["action"] != "minimized" {
		t.Fatalf("minimize one: %v", r)
	}
	if r := send(map[string]any{"type": "minimize", "req": "5", "id": code}); r["action"] != "minimized" {
		t.Fatalf("minimize all: %v", r)
	}
	if got := strings.Join(e.fake.minimized, ","); got != "21,11,11,12" {
		t.Fatalf("minimized: %s", got)
	}
	if len(e.fake.launched) != 0 {
		t.Fatal("nothing should have been launched")
	}
}
