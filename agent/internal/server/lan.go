package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/pairing"
	"barphone/agent/internal/store"
)

// LANHandler serves the phone API on the local network. It can only reveal public
// machine info, pair with a one-time code, push deck state and launch configured buttons.
func (s *Server) LANHandler() http.Handler {
	s.init()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("POST /api/v1/pair", s.handlePair)
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)
	mux.HandleFunc("GET /api/v1/icon/{file}", s.handlePhoneIcon)
	return mux
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	cfg := s.Store.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"v": ProtocolVersion, "id": cfg.MachineID, "name": cfg.Name, "os": OSName()})
}

type pairRequest struct {
	Code       string `json:"code"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := readJSON(r, &req, 4<<10); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	if req.DeviceID == "" || len(req.DeviceID) > 64 || utf8.RuneCountInString(req.DeviceName) > 64 {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.DeviceName == "" {
		req.DeviceName = "Телефон"
	}
	switch err := s.Pairing.Verify(strings.TrimSpace(req.Code)); {
	case errors.Is(err, pairing.ErrNoSession):
		writeError(w, http.StatusGone, "no_session")
		return
	case err != nil:
		s.Store.Notify() // a burnt session must disappear from the UI
		writeError(w, http.StatusForbidden, "bad_code")
		return
	}

	token, hash := pairing.NewToken()
	err := s.Store.Update(func(c *store.Config) error {
		dev := store.Device{ID: req.DeviceID, Name: req.DeviceName, TokenHash: hash, PairedAt: time.Now().UTC()}
		for i := range c.Devices {
			if c.Devices[i].ID == req.DeviceID {
				c.Devices[i] = dev
				return nil
			}
		}
		c.Devices = append(c.Devices, dev)
		return nil
	})
	if err != nil {
		s.Log.Printf("pair: save: %v", err)
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	s.hub.kick(req.DeviceID) // sessions with the replaced token
	s.Log.Printf("paired device %q (%s) from %s", req.DeviceName, req.DeviceID, r.RemoteAddr)
	cfg := s.Store.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"token":   token,
		"machine": machineInfo{ID: cfg.MachineID, Name: cfg.Name, OS: OSName()},
	})
}

// authDevice resolves the bearer token to a paired device.
func (s *Server) authDevice(r *http.Request) (store.Device, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return store.Device{}, false
	}
	hash := []byte(pairing.HashToken(token))
	for _, d := range s.Store.Snapshot().Devices {
		if subtle.ConstantTimeCompare(hash, []byte(d.TokenHash)) == 1 {
			return d, true
		}
	}
	return store.Device{}, false
}

func (s *Server) handlePhoneIcon(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authDevice(r); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	s.serveIcon(w, r, r.PathValue("file"))
}

func (s *Server) serveIcon(w http.ResponseWriter, r *http.Request, file string) {
	hash, ok := strings.CutSuffix(file, ".png")
	path, found := s.Icons.Path(hash)
	if !ok || !found {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, path)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4 << 10,
	WriteBufferSize: 16 << 10,
	// The phone app sends no Origin; browsers always do. Refuse browser pages outright.
	CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" },
}

type inbound struct {
	Type      string   `json:"type"`
	Req       string   `json:"req"`
	ID        string   `json:"id"`
	New       bool     `json:"new"`       // launch: start another instance even if windows are open
	Window    string   `json:"window"`    // focus: which window
	Confirmed bool     `json:"confirmed"` // launch: the user confirmed a dangerous action
	Value     *float64 `json:"value"`     // volume: level to set (0..1); absent = just read

	// Trackpad input (see remoteInput).
	DX     int    `json:"dx"`
	DY     int    `json:"dy"`
	Button string `json:"button"`
	Double bool   `json:"double"`
	Text   string `json:"text"`
	Key    string `json:"key"`
}

// Result actions, see docs/protocol.md.
const (
	actionLaunched  = "launched"
	actionFocused   = "focused"
	actionChoose    = "choose"    // several windows are open: the phone asks the user
	actionWindows   = "windows"   // answer to an explicit "windows" request
	actionDone      = "done"      // keys/text/system button performed
	actionVolume    = "volume"    // answer to a "volume" request
	actionMinimized = "minimized" // the app's window was in front and got minimized
)

type resultMsg struct {
	Type    string          `json:"type"`
	Req     string          `json:"req"`
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Action  string          `json:"action,omitempty"`
	Windows []launch.Window `json:"windows,omitzero"`
	Value   *float64        `json:"value,omitempty"` // volume level 0..1
	Muted   *bool           `json:"muted,omitempty"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authDevice(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already replied
	}
	conn.SetReadLimit(16 << 10)
	// Phones that can show a window chooser say so; older apps get the old behaviour
	// (several windows open → just launch) instead of a "choose" they would ignore.
	canChoose := slices.Contains(strings.Split(r.URL.Query().Get("features"), ","), "windows")
	c := newClient(dev.ID, conn)
	s.hub.add(c)
	s.forceBroadcast.Store(true)
	go c.writeLoop()
	defer func() {
		s.hub.remove(c)
		c.close()
		s.Store.Notify() // online status changed
	}()

	s.Log.Printf("ws: %q connected from %s", dev.Name, r.RemoteAddr)
	if err := s.Store.Update(func(cfg *store.Config) error {
		for i := range cfg.Devices {
			if cfg.Devices[i].ID == dev.ID {
				cfg.Devices[i].LastSeen = time.Now().UTC()
			}
		}
		return nil
	}); err != nil {
		s.Log.Printf("ws: lastSeen: %v", err)
	}
	c.queue(s.stateMessage())

	extend := func() { conn.SetReadDeadline(time.Now().Add(readWait)) }
	extend()
	conn.SetPongHandler(func(string) error { extend(); return nil })
	conn.SetPingHandler(func(data string) error {
		extend()
		err := conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(writeWait))
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil
		}
		return err
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			s.Log.Printf("ws: %q disconnected: %v", dev.Name, err)
			return
		}
		extend()
		var msg inbound
		if err := json.Unmarshal(data, &msg); err != nil {
			c.queue(mustJSON(resultMsg{Type: "result", Error: "bad_request"}))
			continue
		}
		switch msg.Type {
		case "launch", "focus", "windows", "volume", "minimize":
			c.queue(mustJSON(s.pressButton(msg, canChoose)))
		case "pointer", "scroll", "click", "type", "key":
			if res, answer := s.remoteInput(msg); answer {
				c.queue(mustJSON(res))
			}
		default:
			// Unknown types are ignored so newer phones can talk to older agents.
		}
	}
}

// pressButton handles everything a phone can do with a button:
//
//	launch            no window open → start it; one → bring it up; several → ask
//	launch + new      always start another instance
//	focus + window    bring up the chosen window
//	windows           just list the open windows (long press)
func (s *Server) pressButton(msg inbound, canChoose bool) resultMsg {
	res := resultMsg{Type: "result", Req: msg.Req}
	b, ok := func() (store.Button, bool) { cfg := s.Store.Snapshot(); return cfg.Button(msg.ID) }()
	if !ok {
		res.Error = "not_found"
		return res
	}
	if !agentHandles(b.Kind) {
		res.Error = "unsupported" // an app too old to open folders or run timers itself
		return res
	}
	if b.Kind == store.KindStat { // a live tile: a tap opens the task manager
		if msg.Type != "launch" {
			res.Error = "bad_request"
		} else if err := s.Launcher.Launch(monitorApp()); err != nil {
			s.Log.Printf("task manager: %v", err)
			res.Error = "launch_failed"
		} else {
			res.OK, res.Action = true, actionDone
		}
		return res
	}
	if b.Kind == store.KindCommand {
		switch {
		case msg.Type != "launch":
			res.Error = "bad_request"
		case b.Confirm && !msg.Confirmed:
			res.Error = "confirm_required"
		default:
			if err := s.runCommand(b); err != nil {
				s.Log.Printf("command %q: %v", b.Title, err)
				res.Error = "launch_failed"
				return res
			}
			s.pushRecent(b.ID)
			res.OK, res.Action = true, actionDone
		}
		return res
	}
	if b.Kind == store.KindMacro {
		switch {
		case msg.Type != "launch":
			res.Error = "bad_request"
		case !s.startMacro(b):
			res.Error = "busy"
		default:
			s.pushRecent(b.ID)
			res.OK, res.Action = true, actionDone // the steps go on in the background
		}
		return res
	}
	listWindows := func() []launch.Window {
		ws, err := s.Launcher.Windows(b)
		if err != nil {
			s.Log.Printf("windows %q: %v", b.Title, err)
		}
		launch.ShortTitles(ws)
		return ws
	}

	launchErr := func(err error) string {
		s.Log.Printf("press %q: %v", b.Title, err)
		if errors.Is(err, launch.ErrUnsupported) {
			return "unsupported"
		}
		return "launch_failed"
	}
	action, isSystem := launch.LookupSystemAction(b.Target)
	isSystem = isSystem && b.Kind == store.KindSystem
	setVolume := func(st launch.VolumeState) {
		level, muted := st.Level, st.Muted
		res.Value, res.Muted = &level, &muted
	}

	switch msg.Type {
	case "volume":
		if !isSystem || !action.Slider {
			res.Error = "bad_request"
			return res
		}
		if b.Target == "brightness" {
			if msg.Value != nil {
				if err := s.Launcher.SetBrightness(*msg.Value); err != nil {
					res.Error = launchErr(err)
					return res
				}
			}
			level, err := s.Launcher.Brightness()
			if err != nil {
				res.Error = launchErr(err)
				return res
			}
			res.OK, res.Action, res.Value = true, actionVolume, &level
			return res
		}
		if msg.Value != nil {
			if err := s.Launcher.SetVolume(*msg.Value); err != nil {
				res.Error = launchErr(err)
				return res
			}
		}
		st, err := s.Launcher.Volume()
		if err != nil {
			res.Error = launchErr(err)
			return res
		}
		res.OK, res.Action = true, actionVolume
		setVolume(st)
		return res // adjusting a slider is not a "launch" worth a history entry
	case "windows":
		res.OK, res.Action, res.Windows = true, actionWindows, listWindows()
		if res.Windows == nil {
			res.Windows = []launch.Window{}
		}
		return res
	case "minimize": // a window from the chooser, or all of them ("window" empty)
		if err := s.Launcher.Minimize(b, msg.Window); err != nil {
			res.Error = "window_gone"
			if errors.Is(err, launch.ErrUnsupported) {
				res.Error = "unsupported"
			}
			return res
		}
		res.OK, res.Action = true, actionMinimized
		return res // putting an app away is not a launch worth a history entry
	case "focus":
		if err := s.Launcher.Focus(b, msg.Window); err != nil {
			res.Error = "window_gone"
			return res
		}
		res.Action = actionFocused
	default: // launch
		if !b.Kind.Launches() {
			// Older phone apps cannot confirm, so they can never trigger shutdown/restart.
			if isSystem && action.Confirm && !msg.Confirmed {
				res.Error = "confirm_required"
				return res
			}
			if err := s.Launcher.Launch(b); err != nil {
				res.Error = launchErr(err)
				return res
			}
			res.Action = actionDone
			switch {
			case isSystem && b.Target == "brightness": // a tap only tells the level
				if level, err := s.Launcher.Brightness(); err == nil {
					res.Value = &level
				}
			case isSystem && action.Slider: // the tap toggled mute: tell the phone the new state
				if st, err := s.Launcher.Volume(); err == nil {
					setVolume(st)
				}
			}
			break
		}
		var ws []launch.Window
		if !msg.New && b.OnRunning != store.OnRunningNew {
			ws = listWindows()
		}
		switch {
		case len(ws) > 1 && canChoose:
			res.OK, res.Action, res.Windows = true, actionChoose, ws
			return res // nothing happened yet: no history entry
		case len(ws) == 1 && ws[0].Active && s.Launcher.Minimize(b, ws[0].ID) == nil:
			// Pressing the button of the app that is already in front puts it away.
			res.OK, res.Action = true, actionMinimized
			return res
		case len(ws) == 1 && s.Launcher.Focus(b, ws[0].ID) == nil:
			res.Action = actionFocused
		default:
			if err := s.Launcher.Launch(b); err != nil {
				res.Error = launchErr(err)
				return res
			}
			res.Action = actionLaunched
		}
	}
	s.pushRecent(b.ID)
	res.OK = true
	return res
}

func (s *Server) pushRecent(id string) {
	if err := s.Store.Update(func(c *store.Config) error { c.PushRecent(id, time.Now()); return nil }); err != nil {
		s.Log.Printf("press: history: %v", err)
	}
}

// agentHandles reports whether pressing a kind is the agent's business: a folder, a timer
// and the trackpad screen open on the phone without asking the agent.
func agentHandles(k store.ButtonKind) bool {
	return k != store.KindFolder && k != store.KindTimer && k != store.KindTrackpad
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
