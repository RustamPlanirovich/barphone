package server

import (
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/netinfo"
	"barphone/agent/internal/store"
)

//go:embed web
var webFS embed.FS

// UIHandler serves the configuration UI. It must only be bound to loopback.
func (s *Server) UIHandler() http.Handler {
	s.init()
	static, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/state", s.uiState)
	mux.HandleFunc("GET /api/events", s.uiEvents)
	mux.HandleFunc("PUT /api/deck", s.uiPutDeck)
	mux.HandleFunc("PUT /api/profiles", s.uiPutProfiles)
	mux.HandleFunc("GET /api/appkeys", s.uiAppKeys)
	mux.HandleFunc("PUT /api/machine", s.uiPutMachine)
	mux.HandleFunc("GET /api/apps", s.uiApps)
	mux.HandleFunc("GET /api/appicon", s.uiAppIcon)
	mux.HandleFunc("GET /api/icon/{file}", func(w http.ResponseWriter, r *http.Request) { s.serveIcon(w, r, r.PathValue("file")) })
	mux.HandleFunc("PUT /api/buttons/{id}/icon", s.uiSetIcon)
	mux.HandleFunc("DELETE /api/buttons/{id}/icon", s.uiResetIcon)
	mux.HandleFunc("POST /api/buttons/{id}/launch", s.uiLaunch)
	mux.HandleFunc("POST /api/pairing", s.uiStartPairing)
	mux.HandleFunc("DELETE /api/pairing", s.uiCancelPairing)
	mux.HandleFunc("DELETE /api/devices/{id}", s.uiRemoveDevice)
	mux.HandleFunc("POST /api/pickfile", s.uiPickFile)
	mux.HandleFunc("PUT /api/autostart", s.uiSetAutostart)
	mux.HandleFunc("POST /api/firewall/allow", s.uiFirewallAllow)
	return s.uiGuard(mux)
}

// uiGuard stops other websites open in the user's browser from driving the UI API:
// the Host check defeats DNS rebinding, and the custom header on writes forces a CORS
// preflight that we never approve.
func (s *Server) uiGuard(next http.Handler) http.Handler {
	port := strconv.Itoa(s.UIPort)
	allowedHost := map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true}
	allowedOrigin := map[string]bool{"http://127.0.0.1:" + port: true, "http://localhost:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !allowedOrigin[o] {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Barphone-UI") != "1" {
			http.Error(w, "missing X-Barphone-UI header", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

type uiDevice struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	PairedAt time.Time `json:"pairedAt"`
	LastSeen time.Time `json:"lastSeen,omitzero"`
	Online   bool      `json:"online"`
}

type uiPairing struct {
	Active    bool      `json:"active"`
	Code      string    `json:"code,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
	URI       string    `json:"uri,omitempty"`
	QR        string    `json:"qr,omitempty"` // data: URL
}

func (s *Server) uiState(w http.ResponseWriter, r *http.Request) {
	cfg := s.Store.Snapshot()
	addrs := s.Addrs()
	online := s.hub.online()
	devices := make([]uiDevice, 0, len(cfg.Devices))
	for _, d := range cfg.Devices {
		devices = append(devices, uiDevice{ID: d.ID, Name: d.Name, PairedAt: d.PairedAt, LastSeen: d.LastSeen, Online: online[d.ID]})
	}
	if addrs == nil {
		addrs = []netinfo.Addr{}
	}
	recent := cfg.Recent
	if recent == nil {
		recent = []store.Recent{}
	}
	for i := range cfg.Profiles { // the page expects arrays, never null
		if cfg.Profiles[i].Deck.Buttons == nil {
			cfg.Profiles[i].Deck.Buttons = []store.Button{}
		}
		if cfg.Profiles[i].Apps == nil {
			cfg.Profiles[i].Apps = []store.AppRule{}
		}
	}
	fg, history := s.foreground()
	var fgOut any
	if len(fg.Keys) > 0 {
		fgOut = fg
	}
	if history == nil {
		history = []launch.ForegroundApp{}
	}
	def := cfg.Default()
	resp := map[string]any{
		"machine":       machineInfo{ID: cfg.MachineID, Name: cfg.Name, OS: OSName()},
		"lan":           map[string]any{"port": s.LANPort, "addrs": addrs},
		"deck":          map[string]any{"columns": def.Deck.Columns, "buttons": def.Deck.Buttons},
		"profiles":      cfg.Profiles,
		"activeProfile": s.activeProfile(&cfg).ID,
		"foreground":    fgOut,   // app in front on the PC right now (identity only)
		"recentApps":    history, // recently in front, for binding profiles
		"devices":       devices,
		"recent":        recent,
		"pairing":       s.pairingInfo(cfg, addrs),
		"systemActions": launch.SystemActions,
		"firewall":      s.firewallStatus(),
	}
	if s.Autostart != nil {
		on, err := s.Autostart.Enabled()
		resp["autostart"] = map[string]any{"supported": err == nil, "enabled": on}
	} else {
		resp["autostart"] = map[string]any{"supported": false}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) pairingInfo(cfg store.Config, addrs []netinfo.Addr) uiPairing {
	code, expires, ok := s.Pairing.Active()
	if !ok {
		return uiPairing{}
	}
	q := url.Values{}
	q.Set("v", strconv.Itoa(ProtocolVersion))
	q.Set("id", cfg.MachineID)
	q.Set("name", cfg.Name)
	q.Set("os", OSName())
	q.Set("port", strconv.Itoa(s.LANPort))
	q.Set("code", code)
	q.Set("ip", strings.Join(netinfo.IPs(addrs), ","))
	uri := "barphone://pair?" + q.Encode()
	p := uiPairing{Active: true, Code: code, ExpiresAt: expires, URI: uri}
	if png, err := qrcode.Encode(uri, qrcode.Medium, 360); err == nil {
		p.QR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	return p
}

// uiEvents is a Server-Sent Events stream; the UI refetches /api/state on each event.
func (s *Server) uiEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	changes, unsubscribe := s.Store.Subscribe()
	defer unsubscribe()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	fmt.Fprint(w, "event: change\ndata: {}\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes:
			fmt.Fprint(w, "event: change\ndata: {}\n\n")
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		flusher.Flush()
	}
}

func defaultTitle(b store.Button) string {
	switch b.Kind {
	case store.KindFolder:
		return "Папка"
	case store.KindMacro:
		return "Макрос"
	case store.KindWait:
		ms, _ := strconv.Atoi(b.Target)
		return "Пауза " + strings.Replace(strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64), ".", ",", 1) + " с"
	case store.KindStat:
		if b.Target == "ram" {
			return "Память"
		}
		return "Процессор"
	case store.KindTimer:
		sec, _ := strconv.Atoi(b.Target)
		if sec%60 == 0 {
			return fmt.Sprintf("Таймер %d мин", sec/60)
		}
		return fmt.Sprintf("Таймер %d:%02d", sec/60, sec%60)
	case store.KindKeys:
		return b.Target
	case store.KindSystem:
		if a, ok := launch.LookupSystemAction(b.Target); ok {
			return a.Title
		}
	case store.KindText:
		line := strings.TrimSpace(strings.SplitN(b.Target, "\n", 2)[0])
		if r := []rune(line); len(r) > 24 {
			line = string(r[:24]) + "…"
		}
		if line == "" {
			return "Текст"
		}
		return line
	}
	t := b.Target
	if b.Kind == store.KindURL {
		if u, err := url.Parse(t); err == nil && u.Host != "" {
			return u.Host
		}
		return t
	}
	if i := strings.LastIndexAny(t, `\/!`); i >= 0 && i < len(t)-1 {
		t = t[i+1:]
	}
	for _, ext := range []string{".exe", ".lnk", ".app", ".bat", ".cmd"} {
		t = strings.TrimSuffix(t, ext)
	}
	return t
}

func (s *Server) uiPutMachine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in, 4<<10); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 48 {
		writeError(w, http.StatusBadRequest, "bad_name")
		return
	}
	if err := s.Store.Update(func(c *store.Config) error { c.Name = name; return nil }); err != nil {
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (s *Server) uiApps(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	apps, err := s.Apps.Get(ctx, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		s.Log.Printf("apps: %v", err)
		writeError(w, http.StatusInternalServerError, "apps_failed")
		return
	}
	if apps == nil {
		apps = []launch.App{}
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) uiAppIcon(w http.ResponseWriter, r *http.Request) {
	kind := store.ButtonKind(r.URL.Query().Get("kind"))
	target := r.URL.Query().Get("target")
	if !kind.Valid() || target == "" {
		http.NotFound(w, r)
		return
	}
	hash := s.iconFor(kind, target)
	if hash == "" {
		http.NotFound(w, r)
		return
	}
	s.serveIcon(w, r, hash+".png")
}

func (s *Server) uiSetIcon(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "too_large")
		return
	}
	hash, err := s.Icons.PutEncoded(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_image")
		return
	}
	s.setButtonIcon(w, r.PathValue("id"), hash)
}

func (s *Server) uiResetIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if b, ok := func() (store.Button, bool) { c := s.Store.Snapshot(); return c.Button(id) }(); ok {
		s.forgetIcon(b.Kind, b.Target)
	}
	s.setButtonIcon(w, id, "")
	s.wakeIcons()
}

func (s *Server) setButtonIcon(w http.ResponseWriter, id, hash string) {
	err := s.Store.Update(func(c *store.Config) error {
		if b := c.ButtonRef(id); b != nil {
			b.Icon = hash
			return nil
		}
		return errNotFound
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"icon": hash})
}

var errNotFound = errors.New("not found")

// uiLaunch is the "Проверить" button: launches without touching the deck history.
func (s *Server) uiLaunch(w http.ResponseWriter, r *http.Request) {
	cfg := s.Store.Snapshot()
	b, ok := cfg.Button(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if a, ok := launch.LookupSystemAction(b.Target); ok && b.Kind == store.KindSystem && a.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm_required", "message": "выключение и перезагрузка — только с телефона, с подтверждением"})
		return
	}
	if !agentHandles(b.Kind) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported", "message": "это открывается на телефоне"})
		return
	}
	if b.Kind == store.KindStat {
		b = monitorApp()
	}
	if b.Kind == store.KindMacro {
		if !s.startMacro(b) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "busy", "message": "ещё выполняется другой макрос"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := s.Launcher.Launch(b); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "launch_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) uiStartPairing(w http.ResponseWriter, r *http.Request) {
	s.Pairing.Start()
	s.Store.Notify()
	writeJSON(w, http.StatusOK, s.pairingInfo(s.Store.Snapshot(), s.Addrs()))
}

func (s *Server) uiCancelPairing(w http.ResponseWriter, r *http.Request) {
	s.Pairing.Cancel()
	s.Store.Notify()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uiRemoveDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.Store.Update(func(c *store.Config) error {
		for i, d := range c.Devices {
			if d.ID == id {
				c.Devices = append(c.Devices[:i], c.Devices[i+1:]...)
				return nil
			}
		}
		return errNotFound
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.hub.kick(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uiPickFile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	path, err := s.Launcher.PickFile(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pick_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (s *Server) uiSetAutostart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if s.Autostart == nil || readJSON(r, &in, 1<<10) != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.Autostart.Set(in.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "autostart_failed", "message": err.Error()})
		return
	}
	s.Store.Notify()
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": in.Enabled})
}

func (s *Server) uiFirewallAllow(w http.ResponseWriter, r *http.Request) {
	if s.Firewall == nil {
		writeError(w, http.StatusBadRequest, "unsupported")
		return
	}
	if err := s.Firewall.Allow(); err != nil {
		s.Log.Printf("firewall: allow: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "firewall_failed", "message": err.Error()})
		return
	}
	st := s.Firewall.Status(r.Context())
	s.Store.Notify()
	writeJSON(w, http.StatusOK, st)
}

// LoopbackOnly reports whether addr binds to a loopback interface.
func LoopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
