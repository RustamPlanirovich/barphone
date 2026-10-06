package server

import (
	"encoding/json"
	"strings"
	"testing"

	"barphone/agent/internal/store"
)

func TestFolders(t *testing.T) {
	e := newEnv(t)
	if resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindFolder, Title: "A", Buttons: []store.Button{{Kind: store.KindFolder, Title: "B"}}},
	}}); resp.StatusCode != 400 || !strings.Contains(string(data), "другую папку") {
		t.Fatalf("a folder in a folder must be rejected: %d %s", resp.StatusCode, data)
	}

	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindURL, Target: "https://mail.example", Title: "Почта"},
		{Kind: store.KindFolder, Target: "ignored", Buttons: []store.Button{
			{Kind: store.KindText, Target: "секрет", Title: "Пароль"},
			{Kind: store.KindKeys, Target: "ctrl+c"},
		}},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 2 {
		t.Fatalf("deck: %s", data)
	}
	mail, folder := deck.Buttons[0], deck.Buttons[1]
	if folder.Title != "Папка" || folder.Target != "" || len(folder.Buttons) != 2 || folder.Buttons[1].Target != "Ctrl+C" {
		t.Fatalf("folder normalized: %+v", folder)
	}
	secret := folder.Buttons[0]
	if secret.ID == "" || secret.ID == mail.ID || secret.ID == folder.ID {
		t.Fatalf("buttons inside folders get their own IDs: %+v", folder)
	}

	// Moving a button out of the folder keeps its ID (and so its icon and history).
	moved := []store.Button{mail, secret, {ID: folder.ID, Kind: store.KindFolder, Title: folder.Title, Buttons: folder.Buttons[1:]}}
	_, data = e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: moved})
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 3 || deck.Buttons[1].ID != secret.ID || deck.Buttons[2].ID != folder.ID || len(deck.Buttons[2].Buttons) != 1 {
		t.Fatalf("move out of a folder: %s", data)
	}
	// ...and a duplicate ID (same button pasted twice) gets a fresh one.
	_, data = e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		secret, {ID: folder.ID, Kind: store.KindFolder, Buttons: []store.Button{secret}},
	}})
	json.Unmarshal(data, &deck)
	if deck.Buttons[0].ID != secret.ID || deck.Buttons[1].Buttons[0].ID == secret.ID {
		t.Fatalf("duplicate IDs across levels: %s", data)
	}
	inside := deck.Buttons[1].Buttons[0]

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
	wf := buttons(state)[1].(map[string]any)
	kids, _ := wf["buttons"].([]any)
	if wf["kind"] != "folder" || wf["glyph"] != "folder" || len(kids) != 1 || kids[0].(map[string]any)["id"] != inside.ID {
		t.Fatalf("folder on the wire: %v", wf)
	}
	if raw, _ := json.Marshal(state); strings.Contains(string(raw), "секрет") {
		t.Fatal("button targets inside folders must not reach the phone")
	}

	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	ws.WriteJSON(map[string]any{"type": "launch", "req": "1", "id": inside.ID})
	r, _ := readResultAndState(t, ws, func(m map[string]any) bool {
		rec, _ := m["recent"].([]any)
		return len(rec) == 1 && rec[0].(map[string]any)["id"] == inside.ID
	})
	if r["ok"] != true || r["action"] != "done" {
		t.Fatalf("a button inside a folder is pressed by ID: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "2", "id": folder.ID}); r["error"] != "unsupported" {
		t.Fatalf("pressing a folder (old phone app): %v", r)
	}
	if resp, _ := e.call("POST", "/api/buttons/"+folder.ID+"/launch", nil); resp.StatusCode != 400 {
		t.Fatalf("«Проверить» on a folder: %d", resp.StatusCode)
	}
}

func TestTimers(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []string{"0", "86401", "abc", ""} {
		if resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{{Kind: store.KindTimer, Target: bad}}}); resp.StatusCode != 400 {
			t.Errorf("timer %q must be rejected: %s", bad, data)
		}
	}
	if resp, _ := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindTimer, Target: "60"}}},
	}}); resp.StatusCode != 400 {
		t.Error("a timer is not a macro step")
	}
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindTimer, Target: " 1500 "},
		{Kind: store.KindTimer, Target: "90", Title: "Чай"},
		{Kind: store.KindFolder, Buttons: []store.Button{{Kind: store.KindTimer, Target: "30"}}},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 3 || deck.Buttons[0].Title != "Таймер 25 мин" || deck.Buttons[0].Target != "1500" || deck.Buttons[2].Buttons[0].Title != "Таймер 0:30" {
		t.Fatalf("timers: %s", data)
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
	tea := buttons(state)[1].(map[string]any)
	if tea["kind"] != "timer" || tea["glyph"] != "timer" || tea["seconds"] != float64(90) || tea["title"] != "Чай" {
		t.Fatalf("timer on the wire: %v", tea)
	}
	ws.WriteJSON(map[string]any{"type": "launch", "req": "1", "id": deck.Buttons[1].ID})
	if r := readMsg(t, ws, "result"); r["error"] != "unsupported" {
		t.Fatalf("a timer runs on the phone: %v", r)
	}
	if resp, _ := e.call("POST", "/api/buttons/"+deck.Buttons[1].ID+"/launch", nil); resp.StatusCode != 400 {
		t.Fatal("«Проверить» on a timer")
	}
}

func TestStateListsAddresses(t *testing.T) {
	e := newEnv(t)
	status, out := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	m := readMsg(t, ws, "state")["machine"].(map[string]any)
	if addrs, _ := m["addrs"].([]any); len(addrs) != 1 || addrs[0] != "192.168.1.10" {
		t.Fatalf("machine.addrs: %v", m)
	}
}
