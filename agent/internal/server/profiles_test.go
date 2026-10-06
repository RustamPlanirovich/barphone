package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

func putProfiles(t *testing.T, e *env, profiles []store.Profile) (int, []store.Profile, string) {
	t.Helper()
	resp, data := e.call("PUT", "/api/profiles", profiles)
	var out []store.Profile
	json.Unmarshal(data, &out)
	return resp.StatusCode, out, string(data)
}

// readResultAndState waits for a result and a matching state, in whichever order they
// arrive (the history broadcast can overtake the result).
func readResultAndState(t *testing.T, ws *websocket.Conn, pred func(map[string]any) bool) (result, state map[string]any) {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for result == nil || state == nil {
		var m map[string]any
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatalf("waiting for result and state: %v", err)
		}
		switch {
		case m["type"] == "result":
			result = m
		case m["type"] == "state" && pred(m):
			state = m
		}
	}
	return result, state
}

// noMessage asserts the phone got nothing meanwhile (no redundant broadcasts): after a
// pause, the next message must be the answer to a probe request. (A read timeout cannot
// be used: it leaves a gorilla websocket unusable.)
func noMessage(t *testing.T, ws *websocket.Conn, d time.Duration) {
	t.Helper()
	time.Sleep(d)
	ws.WriteJSON(map[string]any{"type": "launch", "req": "probe", "id": "no-such-button"})
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var m map[string]any
	if err := ws.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "result" || m["req"] != "probe" {
		t.Fatalf("unexpected message before the probe answer: %v", m)
	}
}

func TestProfilesValidation(t *testing.T) {
	e := newEnv(t)
	vs := store.Profile{Name: "VS Code", Apps: []store.AppRule{{Name: "Visual Studio Code", Keys: []string{"EXE:Code.exe"}}},
		Deck: store.Deck{Columns: 3, Buttons: []store.Button{{ID: "dup", Title: "Палитра", Kind: store.KindKeys, Target: "ctrl+shift+p"}}}}
	def := store.Profile{ID: store.DefaultProfileID, Deck: store.Deck{Columns: 4, Buttons: []store.Button{{ID: "dup", Kind: store.KindSystem, Target: "mute"}}}}

	if code, _, body := putProfiles(t, e, []store.Profile{vs}); code != 400 || !strings.Contains(body, "основной") {
		t.Fatalf("default profile is required: %d %s", code, body)
	}
	other := vs
	other.Name = "Ещё"
	other.Deck = store.Deck{Columns: 3}
	if code, _, body := putProfiles(t, e, []store.Profile{def, vs, other}); code != 400 || !strings.Contains(body, "уже привязано к профилю «VS Code»") {
		t.Fatalf("one app, two profiles: %d %s", code, body)
	}
	bad := vs
	bad.Apps = []store.AppRule{{Keys: []string{"cmd:rm -rf"}}}
	if code, _, _ := putProfiles(t, e, []store.Profile{def, bad}); code != 400 {
		t.Fatal("unknown key kind must be refused")
	}
	unnamed := vs
	unnamed.Name = " "
	if code, _, _ := putProfiles(t, e, []store.Profile{def, unnamed}); code != 400 {
		t.Fatal("profile needs a name")
	}

	// Default sent last still ends up first; duplicate button IDs are split; keys normalized.
	code, out, body := putProfiles(t, e, []store.Profile{vs, def})
	if code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	if len(out) != 2 || out[0].ID != store.DefaultProfileID || out[0].Name != store.DefaultProfileName || out[1].ID == "" {
		t.Fatalf("profiles: %+v", out)
	}
	if out[0].Deck.Buttons[0].ID == out[1].Deck.Buttons[0].ID {
		t.Fatal("button IDs must be unique across profiles")
	}
	if got := out[1].Apps[0].Keys; len(got) != 1 || got[0] != "exe:code.exe" {
		t.Fatalf("keys: %v", got)
	}
	if out[1].Deck.Buttons[0].Target != "Ctrl+Shift+P" {
		t.Fatal("decks are normalized like /api/deck")
	}

	// The old single-deck endpoint edits only the default profile.
	e.call("PUT", "/api/deck", store.Deck{Columns: 2, Buttons: []store.Button{{Kind: store.KindSystem, Target: "lock"}}})
	cfg := e.srv.Store.Snapshot()
	if len(cfg.Profiles) != 2 || cfg.Default().Deck.Columns != 2 || len(cfg.Profiles[1].Deck.Buttons) != 1 {
		t.Fatalf("PUT /api/deck must leave other profiles alone: %+v", cfg.Profiles)
	}

	if _, data := e.call("GET", "/api/appkeys?kind=app&target=Code", nil); !strings.Contains(string(data), "exe:code.exe") {
		t.Fatalf("appkeys: %s", data)
	}
}

func TestProfileFollowsForeground(t *testing.T) {
	e := newEnv(t)
	code, out, body := putProfiles(t, e, []store.Profile{
		{ID: store.DefaultProfileID, Deck: store.Deck{Columns: 3, Buttons: []store.Button{{Kind: store.KindSystem, Target: "mute"}}}},
		{Name: "VS Code", Apps: []store.AppRule{{Name: "Visual Studio Code", Keys: []string{"exe:code.exe"}}},
			Deck: store.Deck{Columns: 4, Buttons: []store.Button{{Kind: store.KindKeys, Target: "Ctrl+Shift+P"}}}},
	})
	if code != 200 {
		t.Fatal(body)
	}
	vsID, vsButton := out[1].ID, out[1].Deck.Buttons[0].ID

	status, pr := e.pair(e.startPairing(), "dev")
	if status != 200 {
		t.Fatal(status)
	}
	ws, _, err := e.dial(pr["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Icons are filled in asynchronously after a save; wait for them so that later
	// "nothing else is sent" checks are not disturbed by those legitimate updates.
	st := readStateWhere(t, ws, func(m map[string]any) bool {
		ps := m["profiles"].([]any)
		if len(ps) != 2 {
			return false
		}
		for _, p := range ps {
			for _, b := range p.(map[string]any)["buttons"].([]any) {
				if b.(map[string]any)["icon"] == nil {
					return false
				}
			}
		}
		return true
	})
	if st["activeProfile"] != store.DefaultProfileID || st["deck"].(map[string]any)["columns"].(float64) != 3 {
		t.Fatalf("starts on the default profile: %v", st["activeProfile"])
	}
	if _, leaked := st["profiles"].([]any)[1].(map[string]any)["apps"]; leaked {
		t.Error("app bindings are PC-side configuration, not for phones")
	}

	// VS Code comes to the front → the phone switches; old apps see its deck in "deck".
	e.srv.SetForeground(launch.ForegroundApp{Keys: []string{"exe:code.exe"}, Name: "Visual Studio Code"})
	st = readStateWhere(t, ws, func(m map[string]any) bool { return m["activeProfile"] == vsID })
	if st["deck"].(map[string]any)["columns"].(float64) != 4 {
		t.Fatalf("deck must be the active profile's: %v", st["deck"])
	}

	// Pressing a button of a profile that is not in front still works (pinned phone).
	e.srv.SetForeground(launch.ForegroundApp{Keys: []string{"exe:chrome.exe"}, Name: "Google Chrome"})
	readStateWhere(t, ws, func(m map[string]any) bool { return m["activeProfile"] == store.DefaultProfileID })
	ws.WriteJSON(map[string]any{"type": "launch", "req": "1", "id": vsButton})
	r, _ := readResultAndState(t, ws, func(m map[string]any) bool { return len(m["recent"].([]any)) == 1 })
	if r["ok"] != true {
		t.Fatalf("button of another profile: %v", r)
	}

	// Another unbound app in front changes nothing for phones: no broadcast at all.
	e.srv.SetForeground(launch.ForegroundApp{Keys: []string{"exe:notepad.exe"}, Name: "Блокнот"})
	noMessage(t, ws, 400*time.Millisecond)

	// The web UI sees the app in front and the recent ones (names only, no titles).
	_, data := e.call("GET", "/api/state", nil)
	var ui struct {
		Foreground struct{ Name string }   `json:"foreground"`
		RecentApps []struct{ Name string } `json:"recentApps"`
		Active     string                  `json:"activeProfile"`
	}
	json.Unmarshal(data, &ui)
	if ui.Foreground.Name != "Блокнот" || len(ui.RecentApps) != 3 || ui.RecentApps[1].Name != "Google Chrome" || ui.Active != store.DefaultProfileID {
		t.Fatalf("ui state: %s", data)
	}

	// Deleting the profile while VS Code is in front falls back to the default one.
	e.srv.SetForeground(launch.ForegroundApp{Keys: []string{"exe:code.exe"}})
	readStateWhere(t, ws, func(m map[string]any) bool { return m["activeProfile"] == vsID })
	if code, _, body := putProfiles(t, e, []store.Profile{e.srv.Store.Snapshot().Profiles[0]}); code != 200 {
		t.Fatal(body)
	}
	readStateWhere(t, ws, func(m map[string]any) bool {
		return m["activeProfile"] == store.DefaultProfileID && len(m["profiles"].([]any)) == 1
	})
}
