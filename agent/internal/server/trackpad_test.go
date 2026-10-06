package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"barphone/agent/internal/store"
)

func TestTrackpad(t *testing.T) {
	e := newEnv(t)
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindTrackpad, Target: "ignored"},
		{Kind: store.KindURL, Target: "https://example.com"},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	pad, other := deck.Buttons[0], deck.Buttons[1]
	if pad.Title != "Трекпад" || pad.Target != "" {
		t.Fatalf("trackpad: %s", data)
	}
	if resp, _ := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindTrackpad}}},
	}}); resp.StatusCode != 400 {
		t.Fatal("a trackpad is not a macro step")
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
	state := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 2 })
	if w := buttons(state)[0].(map[string]any); w["kind"] != "trackpad" || w["glyph"] != "trackpad" {
		t.Fatalf("trackpad on the wire: %v", w)
	}
	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}

	// Only on behalf of a trackpad button.
	ws.WriteJSON(map[string]any{"type": "pointer", "id": other.ID, "dx": 5, "dy": 5})
	if r := send(map[string]any{"type": "type", "req": "1", "id": other.ID, "text": "x"}); r["error"] != "not_found" {
		t.Fatalf("typing through a link button: %v", r)
	}

	ws.WriteJSON(map[string]any{"type": "pointer", "id": pad.ID, "dx": 3, "dy": -2})
	ws.WriteJSON(map[string]any{"type": "pointer", "id": pad.ID, "dx": 99999, "dy": 0})
	ws.WriteJSON(map[string]any{"type": "scroll", "id": pad.ID, "dx": 0, "dy": 120})
	if r := send(map[string]any{"type": "click", "req": "2", "id": pad.ID, "button": "right"}); r["action"] != "done" {
		t.Fatalf("click: %v", r)
	}
	if r := send(map[string]any{"type": "type", "req": "3", "id": pad.ID, "text": "привет\n"}); r["action"] != "done" {
		t.Fatalf("type: %v", r)
	}
	if r := send(map[string]any{"type": "key", "req": "4", "id": pad.ID, "key": "ctrl+z"}); r["action"] != "done" {
		t.Fatalf("key: %v", r)
	}
	if r := send(map[string]any{"type": "key", "req": "5", "id": pad.ID, "key": "Ctrl+Foo"}); r["error"] != "bad_request" {
		t.Fatalf("bad key: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "6", "id": pad.ID}); r["error"] != "unsupported" {
		t.Fatalf("pressing the trackpad (old phone app): %v", r)
	}
	time.Sleep(20 * time.Millisecond)
	e.fake.mu.Lock()
	input, launched := strings.Join(e.fake.input, "; "), strings.Join(e.fake.launched, "; ")
	e.fake.mu.Unlock()
	if input != "move 3,-2; move 2000,0; scroll 0,120; click right false" {
		t.Fatalf("input: %s", input)
	}
	if launched != "step:; step:" { // typing and the key, as unnamed text/keys actions
		t.Fatalf("launched: %s", launched)
	}
}
