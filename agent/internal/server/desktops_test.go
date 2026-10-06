package server

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

func TestVirtualDesktops(t *testing.T) {
	defer func(old time.Duration) { desktopsEvery = old }(desktopsEvery)
	desktopsEvery = 20 * time.Millisecond
	e := newEnv(t)
	e.fake.mu.Lock()
	e.fake.desktops = launch.DesktopInfo{Count: 4, Current: 1, Names: []string{"", "Работа", "", ""}}
	e.fake.mu.Unlock()
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindSystem, Target: "desktops"},
		{Kind: store.KindURL, Target: "https://example.com"},
		{Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindSystem, Target: "desktop_next"}}},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	tile, link := deck.Buttons[0], deck.Buttons[1]
	if tile.Title != "Рабочие столы" {
		t.Fatalf("deck: %s", data)
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
	state := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 3 })
	if w := buttons(state)[0].(map[string]any); w["control"] != "desktops" || w["glyph"] != "desktops" {
		t.Fatalf("desktops tile on the wire: %v", w)
	}
	// The phone hears the desktops without asking.
	d := readMsg(t, ws, "desktops")
	if d["count"] != float64(4) || d["current"] != float64(1) || fmt.Sprint(d["names"]) != "[ Работа  ]" {
		t.Fatalf("desktops message: %v", d)
	}

	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	r := send(map[string]any{"type": "desktop", "req": "1", "id": tile.ID, "move": 1})
	if got := r["desktops"].(map[string]any); r["action"] != "desktop" || got["current"] != float64(2) {
		t.Fatalf("move right: %v", r)
	}
	r = send(map[string]any{"type": "desktop", "req": "2", "id": tile.ID, "move": 10})
	if got := r["desktops"].(map[string]any); got["current"] != float64(3) {
		t.Fatalf("no wrapping past the last desktop: %v", r)
	}
	r = send(map[string]any{"type": "desktop", "req": "3", "id": tile.ID, "to": 0})
	if got := r["desktops"].(map[string]any); got["current"] != float64(0) {
		t.Fatalf("jump to the first: %v", r)
	}
	if r := send(map[string]any{"type": "desktop", "req": "4", "id": tile.ID, "overview": true}); r["ok"] != true {
		t.Fatalf("overview: %v", r)
	}
	if r := send(map[string]any{"type": "desktop", "req": "5", "id": link.ID, "move": 1}); r["error"] != "not_found" {
		t.Fatalf("only the desktops tile or the trackpad may switch desktops: %v", r)
	}
	// A change made on the PC itself reaches the phone.
	e.fake.mu.Lock()
	e.fake.desktops.Current = 2
	moves := fmt.Sprint(e.fake.moves)
	e.fake.mu.Unlock()
	if moves != "[1 10 -3]" {
		t.Fatalf("moves: %s", moves)
	}
	for {
		d := readMsg(t, ws, "desktops")
		if d["current"] == float64(2) {
			break
		}
	}
	// A tap on the tile (an older phone, or the tap gesture) shows all desktops.
	if r := send(map[string]any{"type": "launch", "req": "6", "id": tile.ID}); r["action"] != "done" {
		t.Fatalf("tap: %v", r)
	}
}
