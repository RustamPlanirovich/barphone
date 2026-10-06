package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

func TestMacroValidation(t *testing.T) {
	e := newEnv(t)
	wait := func(ms string) store.Button { return store.Button{Kind: store.KindWait, Target: ms} }
	many := make([]store.Button, maxMacroSteps+1)
	for i := range many {
		many[i] = wait("100")
	}
	for name, bad := range map[string]store.Button{
		"folder step":     {Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindFolder}}},
		"macro step":      {Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindMacro}}},
		"shutdown step":   {Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindSystem, Target: "shutdown"}}},
		"long wait":       {Kind: store.KindMacro, Steps: []store.Button{wait("61000")}},
		"bad wait":        {Kind: store.KindMacro, Steps: []store.Button{wait("abc")}},
		"bad step":        {Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindKeys, Target: "Ctrl+Foo"}}},
		"too many steps":  {Kind: store.KindMacro, Steps: many},
		"wait on a deck":  wait("100"),
		"wait in folder":  {Kind: store.KindFolder, Buttons: []store.Button{wait("100")}},
		"macro in folder": {Kind: store.KindFolder, Buttons: []store.Button{{Kind: store.KindMacro, Steps: []store.Button{{Kind: store.KindFolder}}}}},
	} {
		if resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{bad}}); resp.StatusCode != 400 {
			t.Errorf("%s must be rejected: %s", name, data)
		}
	}
}

func TestMacroRuns(t *testing.T) {
	defer func(old time.Duration) { macroGap = old }(macroGap)
	macroGap = 5 * time.Millisecond
	e := newEnv(t)

	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindMacro, Title: "Начать работу", Target: "ignored", Steps: []store.Button{
			{Kind: store.KindApp, Target: "Code", Title: "VS Code"},
			{Kind: store.KindWait, Target: " 150 "},
			{Kind: store.KindText, Target: "секретный текст", Title: "Логин"},
			{ID: "b_pretend", Icon: "0123456789abcdef", Kind: store.KindSystem, Target: "media_play_pause"},
		}},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 1 || len(deck.Buttons[0].Steps) != 4 {
		t.Fatalf("deck: %s", data)
	}
	macro := deck.Buttons[0]
	wait, play := macro.Steps[1], macro.Steps[3]
	if macro.Target != "" || wait.Target != "150" || wait.Title != "Пауза 0,15 с" {
		t.Errorf("normalized: %+v / %+v", macro, wait)
	}
	for _, st := range macro.Steps {
		if st.ID != "" || st.Icon != "" {
			t.Errorf("steps are not buttons: %+v", st)
		}
	}
	if a, _ := launch.LookupSystemAction("media_play_pause"); play.Title != a.Title {
		t.Errorf("step title: %q", play.Title)
	}
	e.fake.windows = map[string][]launch.Window{"VS Code": {{ID: "w1", Title: "barphone"}}}

	status, out := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	state := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 1 })
	wm := buttons(state)[0].(map[string]any)
	if wm["kind"] != "macro" || wm["glyph"] != "macro" || wm["steps"] != nil {
		t.Errorf("macro on the wire: %v", wm)
	}
	if raw, _ := json.Marshal(state); strings.Contains(string(raw), "секретный") || strings.Contains(string(raw), "Code") {
		t.Fatal("macro steps must not reach the phone")
	}

	started := time.Now()
	ws.WriteJSON(map[string]any{"type": "launch", "req": "1", "id": macro.ID})
	r, _ := readResultAndState(t, ws, func(m map[string]any) bool {
		rec, _ := m["recent"].([]any)
		return len(rec) == 1 && rec[0].(map[string]any)["id"] == macro.ID
	})
	if r["ok"] != true || r["action"] != "done" {
		t.Fatalf("macro press: %v", r)
	}
	ws.WriteJSON(map[string]any{"type": "launch", "req": "2", "id": macro.ID})
	if r := readMsg(t, ws, "result"); r["error"] != "busy" {
		t.Fatalf("a second macro while one runs: %v", r)
	}
	if resp, _ := e.call("POST", "/api/buttons/"+macro.ID+"/launch", nil); resp.StatusCode != 409 {
		t.Fatalf("«Проверить» while a macro runs: %d", resp.StatusCode)
	}

	done := func() bool {
		e.fake.mu.Lock()
		defer e.fake.mu.Unlock()
		return len(e.fake.launched) == 2
	}
	for !done() {
		if time.Since(started) > 3*time.Second {
			t.Fatalf("steps did not run: %v", e.fake.launched)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if elapsed := time.Since(started); elapsed < 150*time.Millisecond {
		t.Errorf("the pause was skipped: %v", elapsed)
	}
	e.fake.mu.Lock()
	launched, focused := strings.Join(e.fake.launched, ","), strings.Join(e.fake.focused, ",")
	e.fake.mu.Unlock()
	if launched != "step:Логин,step:"+play.Title || focused != "w1" {
		t.Fatalf("steps in order, the open app brought up (not launched): launched=%s focused=%s", launched, focused)
	}

	// Finished: it can run again, from the phone or from «Проверить».
	time.Sleep(20 * time.Millisecond)
	if resp, data := e.call("POST", "/api/buttons/"+macro.ID+"/launch", nil); resp.StatusCode != 200 {
		t.Fatalf("«Проверить» runs the macro: %d %s", resp.StatusCode, data)
	}
}
