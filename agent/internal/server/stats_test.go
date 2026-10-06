package server

import (
	"encoding/json"
	"testing"
	"time"

	"barphone/agent/internal/store"
)

func TestLiveTiles(t *testing.T) {
	defer func(old time.Duration) { statsEvery = old }(statsEvery)
	statsEvery = 20 * time.Millisecond
	e := newEnv(t)
	if resp, _ := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{{Kind: store.KindStat, Target: "gpu"}}}); resp.StatusCode != 400 {
		t.Fatal("unknown stat must be rejected")
	}
	if resp, _ := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindStat, Target: "cpu"}}},
	}}); resp.StatusCode != 400 {
		t.Fatal("a live tile is not a macro step")
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
	readMsg(t, ws, "state")
	// No live tiles yet: nothing is measured or sent.
	time.Sleep(100 * time.Millisecond)
	ws.WriteJSON(map[string]any{"type": "launch", "req": "probe", "id": "no-such-button"})
	for {
		var m map[string]any
		ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m["type"] == "stats" {
			t.Fatal("stats without live tiles")
		}
		if m["req"] == "probe" {
			break
		}
	}

	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindStat, Target: "cpu"},
		{Kind: store.KindStat, Target: "ram"},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if deck.Buttons[0].Title != "Процессор" || deck.Buttons[1].Title != "Память" {
		t.Fatalf("titles: %s", data)
	}
	state := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 2 })
	cpu := buttons(state)[0].(map[string]any)
	if cpu["kind"] != "stat" || cpu["stat"] != "cpu" || cpu["glyph"] != "cpu" {
		t.Fatalf("live tile on the wire: %v", cpu)
	}
	stats := readMsg(t, ws, "stats")
	if stats["cpu"] != 0.25 || stats["ram"] != 0.5 || stats["ramTotal"] != float64(16<<30) {
		t.Fatalf("stats: %v", stats)
	}

	// A tap opens the task manager.
	ws.WriteJSON(map[string]any{"type": "launch", "req": "1", "id": deck.Buttons[0].ID})
	if r := readMsg(t, ws, "result"); r["ok"] != true || r["action"] != "done" {
		t.Fatalf("tap on a live tile: %v", r)
	}
	e.fake.mu.Lock()
	launched := append([]string(nil), e.fake.launched...)
	e.fake.mu.Unlock()
	if len(launched) != 1 || launched[0] != "step:Диспетчер задач" {
		t.Fatalf("launched: %v", launched)
	}
}

func TestBrightnessSlider(t *testing.T) {
	e := newEnv(t)
	e.fake.bright = 0.4
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindSystem, Target: "brightness"},
		{Kind: store.KindSystem, Target: "brightness_up"},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	slider := deck.Buttons[0]
	if slider.Title != "Яркость" || deck.Buttons[1].Title != "Ярче" {
		t.Fatalf("titles: %s", data)
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
	if w := buttons(state)[0].(map[string]any); w["control"] != "slider" || w["glyph"] != "brightness" {
		t.Fatalf("slider on the wire: %v", w)
	}
	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	if r := send(map[string]any{"type": "volume", "req": "1", "id": slider.ID}); r["value"] != 0.4 || r["muted"] != nil {
		t.Fatalf("read brightness: %v", r)
	}
	if r := send(map[string]any{"type": "volume", "req": "2", "id": slider.ID, "value": 0.8}); r["value"] != 0.8 {
		t.Fatalf("set brightness: %v", r)
	}
	// A tap tells the level (it is the launcher's job not to change it).
	if r := send(map[string]any{"type": "launch", "req": "3", "id": slider.ID}); r["action"] != "done" || r["value"] != 0.8 || r["muted"] != nil {
		t.Fatalf("tap on the brightness slider: %v", r)
	}
}
