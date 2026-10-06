package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"barphone/agent/internal/icons"
	"barphone/agent/internal/launch"
	"barphone/agent/internal/netinfo"
	"barphone/agent/internal/pairing"
	"barphone/agent/internal/store"
)

type fakeLauncher struct {
	mu        sync.Mutex
	launched  []string
	focused   []string
	minimized []string
	fail      bool
	windows   map[string][]launch.Window // by button title
	volume    launch.VolumeState
}

func (f *fakeLauncher) Launch(b store.Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	if b.Kind == store.KindSystem && b.Target == "media_stop" {
		return launch.ErrUnsupported
	}
	if b.Kind == store.KindSystem && b.Target == "volume" {
		f.volume.Muted = !f.volume.Muted
	}
	f.launched = append(f.launched, b.ID)
	return nil
}
func (f *fakeLauncher) Apps(context.Context) ([]launch.App, error) {
	return []launch.App{{Name: "Калькулятор", Kind: store.KindApp, Target: "calc!App"}}, nil
}
func (f *fakeLauncher) Icon(store.ButtonKind, string) (image.Image, error) {
	return image.NewNRGBA(image.Rect(0, 0, 32, 32)), nil
}
func (f *fakeLauncher) PickFile(context.Context) (string, error) { return "", nil }
func (f *fakeLauncher) Windows(b store.Button) ([]launch.Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]launch.Window(nil), f.windows[b.Title]...), nil
}
func (f *fakeLauncher) Volume() (launch.VolumeState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.volume, nil
}
func (f *fakeLauncher) AppKeys(b store.Button) []string {
	return []string{"exe:" + strings.ToLower(b.Target) + ".exe", "aumid:" + strings.ToLower(b.Target)}
}
func (f *fakeLauncher) SetVolume(level float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volume = launch.VolumeState{Level: level}
	return nil
}
func (f *fakeLauncher) Focus(b store.Button, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.windows[b.Title] {
		if w.ID == id {
			f.focused = append(f.focused, id)
			return nil
		}
	}
	return launch.ErrWindowGone
}
func (f *fakeLauncher) Minimize(b store.Button, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.windows[b.Title] {
		if id == "" || w.ID == id {
			f.minimized = append(f.minimized, w.ID)
		}
	}
	if len(f.windows[b.Title]) == 0 {
		return launch.ErrWindowGone
	}
	return nil
}
func (f *fakeLauncher) OpenURL(string) error { return nil }

type env struct {
	t    *testing.T
	srv  *Server
	lan  *httptest.Server
	ui   *httptest.Server
	fake *fakeLauncher
}

func newEnv(t *testing.T) *env {
	t.Helper()
	// Not t.TempDir(): on Windows the antivirus briefly holds freshly written icons, and
	// a failed cleanup would fail an otherwise green test. Best-effort removal instead.
	dir, err := os.MkdirTemp("", "barphone-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	st, err := store.Open(dir, "Тестовый ПК")
	if err != nil {
		t.Fatal(err)
	}
	ic, err := icons.Open(dir + "/icons")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeLauncher{}
	uiLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		Store: st, Icons: ic, Launcher: fake, Apps: &launch.AppCache{L: fake, TTL: time.Minute},
		Pairing: &pairing.Sessions{}, Log: log.New(io.Discard, "", 0),
		LANPort: 47800, UIPort: uiLn.Addr().(*net.TCPAddr).Port,
		// Tests never go to the internet for link icons.
		SiteIcon: func(context.Context, string) (image.Image, error) { return nil, errors.New("offline in tests") },
		Addrs: func() []netinfo.Addr {
			return []netinfo.Addr{{IP: "192.168.1.10", Iface: "Wi-Fi", MAC: "aa:bb:cc:dd:ee:ff"}}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() { srv.Run(ctx); close(runDone) }()
	e := &env{t: t, srv: srv, fake: fake, lan: httptest.NewServer(srv.LANHandler())}
	e.ui = &httptest.Server{Listener: uiLn, Config: &http.Server{Handler: srv.UIHandler()}}
	e.ui.Start()
	t.Cleanup(func() { e.lan.Close(); e.ui.Close(); cancel(); <-runDone })
	return e
}

// ui performs a UI API call the way the page does.
func (e *env) call(method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, e.ui.URL+path, r)
	req.Header.Set("X-Barphone-UI", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (e *env) pair(code, deviceID string) (int, map[string]any) {
	e.t.Helper()
	body, _ := json.Marshal(map[string]string{"code": code, "deviceId": deviceID, "deviceName": "Pixel"})
	resp, err := http.Post(e.lan.URL+"/api/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *env) startPairing() string {
	e.t.Helper()
	resp, data := e.call("POST", "/api/pairing", nil)
	if resp.StatusCode != 200 {
		e.t.Fatalf("start pairing: %d %s", resp.StatusCode, data)
	}
	var p uiPairing
	json.Unmarshal(data, &p)
	if !p.Active || len(p.Code) != 6 || !strings.HasPrefix(p.QR, "data:image/png;base64,") {
		e.t.Fatalf("bad pairing info: %+v", p)
	}
	if !strings.Contains(p.URI, "ip=192.168.1.10") || !strings.Contains(p.URI, "port=47800") {
		e.t.Fatalf("bad pairing uri: %s", p.URI)
	}
	return p.Code
}

func (e *env) dial(token string) (*websocket.Conn, *http.Response, error) {
	return e.dialPath(token, "/api/v1/ws?features=windows")
}

func (e *env) dialPath(token, path string) (*websocket.Conn, *http.Response, error) {
	h := http.Header{}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	return websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(e.lan.URL, "http")+path, h)
}

func readMsg(t *testing.T, c *websocket.Conn, wantType string) map[string]any {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			t.Fatalf("waiting for %q: %v", wantType, err)
		}
		if m["type"] == wantType {
			return m
		}
	}
}

// readStateWhere skips state messages until pred holds.
func readStateWhere(t *testing.T, c *websocket.Conn, pred func(map[string]any) bool) map[string]any {
	t.Helper()
	for {
		m := readMsg(t, c, "state")
		if pred(m) {
			return m
		}
	}
}

func buttons(m map[string]any) []any { return m["deck"].(map[string]any)["buttons"].([]any) }

func TestPhoneFlow(t *testing.T) {
	e := newEnv(t)

	// Configure a deck through the UI API.
	resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 4, Buttons: []store.Button{
		{Title: "Калькулятор", Kind: store.KindApp, Target: "calc!App"},
		{Title: "", Kind: store.KindURL, Target: "https://example.com/x"},
	}})
	if resp.StatusCode != 200 {
		t.Fatalf("put deck: %d %s", resp.StatusCode, data)
	}
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 2 || deck.Buttons[0].ID == "" || deck.Buttons[1].Title != "example.com" {
		t.Fatalf("unexpected saved deck: %+v", deck)
	}
	calcID := deck.Buttons[0].ID

	// Public info needs no auth.
	r, err := http.Get(e.lan.URL + "/api/v1/info")
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("info: %v %v", err, r)
	}

	// Pairing: no session -> 410; wrong code -> 403; right code -> token.
	if code, _ := e.pair("123456", "dev-1"); code != http.StatusGone {
		t.Fatalf("pair without session: %d", code)
	}
	code := e.startPairing()
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if status, _ := e.pair(wrong, "dev-1"); status != http.StatusForbidden {
		t.Fatalf("wrong code: %d", status)
	}
	status, out := e.pair(code, "dev-1")
	if status != 200 {
		t.Fatalf("pair: %d %v", status, out)
	}
	token := out["token"].(string)
	if out["machine"].(map[string]any)["name"] != "Тестовый ПК" {
		t.Fatalf("machine in pair response: %v", out)
	}
	if status, _ := e.pair(code, "dev-2"); status != http.StatusGone {
		t.Fatalf("code must be single-use, got %d", status)
	}

	// WebSocket requires the token.
	if _, resp, err := e.dial("bogus"); err == nil || resp.StatusCode != 401 {
		t.Fatalf("ws with bad token should be 401: %v", err)
	}
	ws, _, err := e.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	st := readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 2 })
	if st["deck"].(map[string]any)["columns"].(float64) != 4 {
		t.Fatalf("columns: %v", st["deck"])
	}
	mach := st["machine"].(map[string]any)
	if mach["macs"].([]any)[0] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("macs: %v", mach)
	}
	if _, ok := buttons(st)[0].(map[string]any)["target"]; ok {
		t.Fatal("targets must not be sent to the phone")
	}

	// Icons get filled in asynchronously and are fetchable with the token only.
	st = readStateWhere(t, ws, func(m map[string]any) bool { return buttons(m)[0].(map[string]any)["icon"] != nil })
	hash := buttons(st)[0].(map[string]any)["icon"].(string)
	req, _ := http.NewRequest("GET", e.lan.URL+"/api/v1/icon/"+hash+".png", nil)
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 401 {
		t.Fatalf("icon without token: %d", r.StatusCode)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("icon with token: %d", r.StatusCode)
	}

	// Launch a button -> result + history pushed in state.
	// The history broadcast may overtake the result, so accept them in either order.
	ws.WriteJSON(map[string]string{"type": "launch", "req": "r1", "id": calcID})
	var res map[string]any
	st = nil
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for res == nil || st == nil {
		var m map[string]any
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatalf("waiting for result and state: %v", err)
		}
		switch {
		case m["type"] == "result":
			res = m
		case m["type"] == "state" && len(m["recent"].([]any)) == 1:
			st = m
		}
	}
	if res["req"] != "r1" || res["ok"] != true {
		t.Fatalf("launch result: %v", res)
	}
	if st["recent"].([]any)[0].(map[string]any)["id"] != calcID {
		t.Fatalf("recent: %v", st["recent"])
	}
	if len(e.fake.launched) != 1 || e.fake.launched[0] != calcID {
		t.Fatalf("launcher calls: %v", e.fake.launched)
	}

	ws.WriteJSON(map[string]string{"type": "launch", "req": "r2", "id": "nope"})
	if res := readMsg(t, ws, "result"); res["ok"] != false || res["error"] != "not_found" {
		t.Fatalf("unknown button: %v", res)
	}
	e.fake.fail = true
	ws.WriteJSON(map[string]string{"type": "launch", "req": "r3", "id": calcID})
	if res := readMsg(t, ws, "result"); res["error"] != "launch_failed" {
		t.Fatalf("failing launch: %v", res)
	}
	e.fake.fail = false

	// Editing the deck on the PC is pushed live; deleting a button prunes history.
	e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{deck.Buttons[1]}})
	st = readStateWhere(t, ws, func(m map[string]any) bool { return len(buttons(m)) == 1 })
	if len(st["recent"].([]any)) != 0 {
		t.Fatalf("recent should be pruned: %v", st["recent"])
	}

	// UI shows the phone as online; unpairing kicks it.
	resp, data = e.call("GET", "/api/state", nil)
	if !strings.Contains(string(data), `"online":true`) {
		t.Fatalf("device should be online: %s", data)
	}
	resp, _ = e.call("DELETE", "/api/devices/dev-1", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("remove device: %d", resp.StatusCode)
	}
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break // closed by the agent
		}
	}
	if _, resp, err := e.dial(token); err == nil || resp.StatusCode != 401 {
		t.Fatal("revoked token must not connect")
	}
}

func TestPairingBurnsAfterTooManyAttempts(t *testing.T) {
	e := newEnv(t)
	code := e.startPairing()
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < pairing.MaxAttempts; i++ {
		if status, _ := e.pair(wrong, "x"); status != http.StatusForbidden {
			t.Fatalf("attempt %d: %d", i, status)
		}
	}
	if status, _ := e.pair(code, "x"); status != http.StatusGone {
		t.Fatalf("session should be burnt, got %d", status)
	}
}

func TestUIGuard(t *testing.T) {
	e := newEnv(t)
	do := func(method, host, origin string, header bool) int {
		req, _ := http.NewRequest(method, e.ui.URL+"/api/pairing", nil)
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if header {
			req.Header.Set("X-Barphone-UI", "1")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if s := do("POST", "", "", false); s != 403 {
		t.Errorf("write without header: %d", s)
	}
	if s := do("POST", "evil.example:47801", "", true); s != 403 {
		t.Errorf("rebinding host: %d", s)
	}
	if s := do("POST", "", "http://evil.example", true); s != 403 {
		t.Errorf("foreign origin: %d", s)
	}
	if s := do("POST", "", "", true); s != 200 {
		t.Errorf("legit request: %d", s)
	}
	// Browsers cannot open the phone WebSocket even with a stolen token.
	h := http.Header{"Origin": {"http://evil.example"}}
	if _, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(e.lan.URL, "http")+"/api/v1/ws", h); err == nil || resp.StatusCode == 101 {
		t.Error("ws from a browser origin must be refused")
	}
	// Static UI is served.
	r, err := http.Get(e.ui.URL + "/")
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("index: %v %v", err, r)
	}
}

func TestDeckValidation(t *testing.T) {
	e := newEnv(t)
	for _, b := range []store.Button{
		{Kind: "nope", Target: "x"},
		{Kind: store.KindPath, Target: "  "},
		{Kind: store.KindURL, Target: "example.com"},
		{Kind: store.KindURL, Target: "javascript:alert(1)"},
	} {
		resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{b}})
		if resp.StatusCode != 400 {
			t.Errorf("%+v should be rejected, got %d %s", b, resp.StatusCode, data)
		}
	}
	resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 99, Buttons: []store.Button{{Kind: store.KindPath, Target: `C:\Tools\app.exe`}}})
	var d store.Deck
	json.Unmarshal(data, &d)
	if resp.StatusCode != 200 || d.Columns != store.MaxColumns || d.Buttons[0].Title != "app" {
		t.Fatalf("clamp/default title: %d %+v", resp.StatusCode, d)
	}
}

func TestWindowChoice(t *testing.T) {
	e := newEnv(t)
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Title: "Code", Kind: store.KindApp, Target: "code"},
		{Title: "Term", Kind: store.KindApp, Target: "term"},
		{Title: "Always new", Kind: store.KindApp, Target: "new", OnRunning: store.OnRunningNew},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	code, term, always := deck.Buttons[0].ID, deck.Buttons[1].ID, deck.Buttons[2].ID
	if deck.Buttons[2].OnRunning != store.OnRunningNew {
		t.Fatalf("onRunning not saved: %+v", deck.Buttons[2])
	}
	e.fake.windows = map[string][]launch.Window{
		"Code":       {{ID: "11", Title: "main.go - barphone - Visual Studio Code"}, {ID: "12", Title: "AOv5 - Visual Studio Code"}},
		"Term":       {{ID: "21", Title: "Terminal"}},
		"Always new": {{ID: "31", Title: "x"}, {ID: "32", Title: "y"}},
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
	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}

	// Several windows: nothing is launched, the phone gets the list with short titles.
	r := send(map[string]any{"type": "launch", "req": "1", "id": code})
	if r["action"] != "choose" || len(r["windows"].([]any)) != 2 {
		t.Fatalf("choose: %v", r)
	}
	if title := r["windows"].([]any)[0].(map[string]any)["title"]; title != "main.go - barphone" {
		t.Fatalf("short title: %v", title)
	}
	if len(e.fake.launched) != 0 {
		t.Fatal("must not launch while asking")
	}
	// The user picks a window.
	if r := send(map[string]any{"type": "focus", "req": "2", "id": code, "window": "12"}); r["action"] != "focused" || r["ok"] != true {
		t.Fatalf("focus: %v", r)
	}
	// ...or a window that has closed meanwhile.
	if r := send(map[string]any{"type": "focus", "req": "3", "id": code, "window": "999"}); r["error"] != "window_gone" {
		t.Fatalf("gone: %v", r)
	}
	// ...or asks for a new instance.
	if r := send(map[string]any{"type": "launch", "req": "4", "id": code, "new": true}); r["action"] != "launched" {
		t.Fatalf("new: %v", r)
	}
	// One window: switch to it right away.
	if r := send(map[string]any{"type": "launch", "req": "5", "id": term}); r["action"] != "focused" {
		t.Fatalf("single window: %v", r)
	}
	// Button configured to always start a new instance.
	if r := send(map[string]any{"type": "launch", "req": "6", "id": always}); r["action"] != "launched" {
		t.Fatalf("always new: %v", r)
	}
	// Long press: list only, always an array.
	if r := send(map[string]any{"type": "windows", "req": "7", "id": term}); r["action"] != "windows" || len(r["windows"].([]any)) != 1 {
		t.Fatalf("windows: %v", r)
	}
	e.fake.windows["Term"] = nil
	if r := send(map[string]any{"type": "windows", "req": "8", "id": term}); r["windows"] == nil {
		t.Fatalf("empty windows must be [], got %v", r)
	}
	if got := strings.Join(e.fake.focused, ","); got != "12,21" {
		t.Fatalf("focused: %s", got)
	}
	if got := strings.Join(e.fake.launched, ","); got != code+","+always {
		t.Fatalf("launched: %s", got)
	}

	// An older phone app (no features) is never asked to choose: it gets a plain launch.
	old, _, err := e.dialPath(out["token"].(string), "/api/v1/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	old.WriteJSON(map[string]any{"type": "launch", "req": "o1", "id": code})
	if r := readMsg(t, old, "result"); r["action"] != "launched" {
		t.Fatalf("old client: %v", r)
	}
}

func TestActionButtons(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []store.Button{
		{Kind: store.KindKeys, Target: "Ctrl+Foo"},
		{Kind: store.KindKeys, Target: "Ctrl+Shift"},
		{Kind: store.KindSystem, Target: "format_c"},
		{Kind: store.KindText, Target: ""},
	} {
		if resp, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{bad}}); resp.StatusCode != 400 {
			t.Errorf("%+v should be rejected: %s", bad, data)
		}
	}
	_, data := e.call("PUT", "/api/deck", store.Deck{Columns: 3, Buttons: []store.Button{
		{Kind: store.KindKeys, Target: " ctrl + shift + p "},
		{Kind: store.KindText, Target: "Привет\nмир\n"},
		{Kind: store.KindSystem, Target: "volume"},
		{Kind: store.KindSystem, Target: "shutdown"},
		{Kind: store.KindSystem, Target: "media_stop"},
	}})
	var deck store.Deck
	json.Unmarshal(data, &deck)
	if len(deck.Buttons) != 5 {
		t.Fatalf("deck: %s", data)
	}
	keys, text, volume, shutdown, stop := deck.Buttons[0], deck.Buttons[1], deck.Buttons[2], deck.Buttons[3], deck.Buttons[4]
	if keys.Target != "Ctrl+Shift+P" || keys.Title != "Ctrl+Shift+P" {
		t.Errorf("keys canonical: %+v", keys)
	}
	if text.Target != "Привет\nмир\n" || text.Title != "Привет" {
		t.Errorf("text kept verbatim, titled by first line: %+v", text)
	}
	if volume.Title != "Громкость" || shutdown.Title != "Выключить" {
		t.Errorf("system titles: %q %q", volume.Title, shutdown.Title)
	}
	resp, st := e.call("GET", "/api/state", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(st), `"systemActions"`) {
		t.Fatal("UI state must list system actions")
	}

	e.fake.volume = launch.VolumeState{Level: 0.3}
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
	wire := func(i int) map[string]any { return buttons(state)[i].(map[string]any) }
	if wire(0)["glyph"] != "keys" || wire(1)["glyph"] != "text" || wire(2)["glyph"] != "volume" || wire(2)["control"] != "slider" {
		t.Errorf("glyphs/control: %v %v %v", wire(0), wire(1), wire(2))
	}
	if wire(3)["confirm"] != true || wire(0)["confirm"] != nil {
		t.Errorf("confirm flags: %v %v", wire(3), wire(0))
	}
	if _, leaked := wire(1)["target"]; leaked {
		t.Error("text content must not be sent to the phone")
	}

	send := func(m map[string]any) map[string]any {
		t.Helper()
		ws.WriteJSON(m)
		return readMsg(t, ws, "result")
	}
	if r := send(map[string]any{"type": "launch", "req": "1", "id": keys.ID}); r["action"] != "done" {
		t.Errorf("keys: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "2", "id": shutdown.ID}); r["error"] != "confirm_required" {
		t.Errorf("shutdown without confirmation must be refused: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "3", "id": shutdown.ID, "confirmed": true}); r["action"] != "done" {
		t.Errorf("confirmed shutdown: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "4", "id": stop.ID}); r["error"] != "unsupported" {
		t.Errorf("unsupported action: %v", r)
	}
	// Slider: read, set, and tap = mute toggle that reports the new state.
	if r := send(map[string]any{"type": "volume", "req": "5", "id": volume.ID}); r["value"] != 0.3 || r["muted"] != false {
		t.Errorf("volume read: %v", r)
	}
	if r := send(map[string]any{"type": "volume", "req": "6", "id": volume.ID, "value": 0.75}); r["value"] != 0.75 {
		t.Errorf("volume set: %v", r)
	}
	if r := send(map[string]any{"type": "launch", "req": "7", "id": volume.ID}); r["action"] != "done" || r["muted"] != true {
		t.Errorf("volume tap toggles mute: %v", r)
	}
	if r := send(map[string]any{"type": "volume", "req": "8", "id": keys.ID, "value": 1}); r["error"] != "bad_request" {
		t.Errorf("volume on a non-slider button: %v", r)
	}
	if got := strings.Join(e.fake.launched, ","); got != keys.ID+","+shutdown.ID+","+volume.ID {
		t.Errorf("launched: %s", got)
	}
}
