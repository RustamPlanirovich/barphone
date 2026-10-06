package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

func TestMeetCallPanel(t *testing.T) {
	defer func(old time.Duration) { callEvery = old }(callEvery)
	callEvery = 20 * time.Millisecond
	e := newEnv(t)
	e.fake.mu.Lock()
	e.fake.call = launch.CallInfo{Active: true, App: "meet", Camera: true}
	e.fake.windows = map[string][]launch.Window{"Code": {{ID: "1", Title: "barphone"}, {ID: "2", Title: "ecobox", Desktop: 3}}}
	e.fake.mu.Unlock()
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{{Kind: store.KindPath, Target: `C:\code.exe`, Title: "Code"}}})
	var deck store.Deck
	json.Unmarshal(data, &deck)

	status, out := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if c := readMsg(t, ws, "call"); c["active"] != true || c["app"] != "meet" || c["camera"] != true {
		t.Fatalf("call: %v", c)
	}
	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	if r := send(map[string]any{"type": "call", "req": "1", "action": "camera"}); r["ok"] != true || r["action"] != "call" {
		t.Fatalf("camera: %v", r)
	}
	for c := readMsg(t, ws, "call"); c["camera"] != false; c = readMsg(t, ws, "call") {
	}
	if r := send(map[string]any{"type": "call", "req": "2", "action": "explode"}); r["error"] != "bad_request" {
		t.Fatalf("unknown action: %v", r)
	}

	// Windows on another virtual desktop come with its number.
	r := send(map[string]any{"type": "windows", "req": "3", "id": deck.Buttons[0].ID})
	wins := r["windows"].([]any)
	if len(wins) != 2 || wins[1].(map[string]any)["desktop"] != float64(3) || wins[0].(map[string]any)["desktop"] != nil {
		t.Fatalf("windows: %v", r)
	}

	// The owner turns the panel off: the phones hear "no call", controls refuse.
	resp, body := e.call("PUT", "/api/meet", map[string]bool{"enabled": false})
	if resp.StatusCode != 200 {
		t.Fatal(string(body))
	}
	for c := readMsg(t, ws, "call"); c["active"] != false; c = readMsg(t, ws, "call") {
	}
	if r := send(map[string]any{"type": "call", "req": "4", "action": "mic"}); r["error"] != "unsupported" {
		t.Fatalf("controls off: %v", r)
	}
	if _, st := e.call("GET", "/api/state", nil); !strings.Contains(string(st), `"meetControls":false`) {
		t.Fatal("UI state must show the setting")
	}
	e.call("PUT", "/api/meet", map[string]bool{"enabled": true})
	e.fake.mu.Lock()
	e.fake.call = launch.CallInfo{}
	calls := strings.Join(e.fake.calls, ",")
	e.fake.mu.Unlock()
	if r := send(map[string]any{"type": "call", "req": "5", "action": "mic"}); r["error"] != "not_found" {
		t.Fatalf("no call: %v", r)
	}
	if calls != "camera" {
		t.Fatalf("calls: %s", calls)
	}
}
