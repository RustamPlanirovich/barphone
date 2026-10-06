// Package server exposes the LAN API for phones (see docs/protocol.md) and the
// loopback-only configuration UI.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"barphone/agent/internal/favicon"
	"barphone/agent/internal/firewall"
	"barphone/agent/internal/icons"
	"barphone/agent/internal/launch"
	"barphone/agent/internal/netinfo"
	"barphone/agent/internal/pairing"
	"barphone/agent/internal/store"
	"barphone/agent/internal/sysstat"
)

const ProtocolVersion = 1

type Server struct {
	Store    *store.Store
	Icons    *icons.Store
	Launcher launch.Launcher
	Apps     *launch.AppCache
	Pairing  *pairing.Sessions
	Log      *log.Logger

	LANPort int
	UIPort  int
	// Addrs lists LAN addresses; replaceable in tests.
	Addrs func() []netinfo.Addr
	// Autostart is optional; nil hides the setting in the UI.
	Autostart Autostart
	// Firewall is optional; nil hides the firewall status in the UI.
	Firewall *firewall.Checker
	// SiteIcon fetches a website's icon for link buttons; nil means favicon.Fetch.
	SiteIcon func(ctx context.Context, url string) (image.Image, error)
	// Stats measures CPU and memory for live tiles; nil means this computer (sysstat).
	Stats interface {
		Read() (sysstat.Sample, error)
	}
	// Command prepares a command button's process; nil means launch.Command.
	Command func(store.Button) (*exec.Cmd, error)
	// TLSFingerprint is the pinned fingerprint of the certificate the LAN port also
	// speaks TLS with (see SniffTLS); "" when the agent has none.
	TLSFingerprint string

	hub    hub
	fwBusy atomic.Bool
	// macroBusy: a macro is running; runCtx stops it when the agent shuts down.
	macroBusy atomic.Bool
	runCtx    atomic.Pointer[context.Context]
	// forceBroadcast makes Run push state even if it looks unchanged (a phone connected
	// and may have seen an intermediate state).
	forceBroadcast atomic.Bool

	fgMu      sync.Mutex
	fg        launch.ForegroundApp   // app in front on the PC
	fgHistory []launch.ForegroundApp // recently in front, newest first (memory only)
	iconWake  chan struct{}

	iconMu    sync.Mutex
	iconTried map[string]bool   // kind|target -> extraction attempted
	appIcons  map[string]string // kind|target -> hash ("" = no icon), for the UI picker
}

type Autostart interface {
	Enabled() (bool, error)
	Set(enabled bool) error
}

func OSName() string {
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	return runtime.GOOS
}

// Run broadcasts state to phones on every change and fills in missing icons. Blocks until ctx is done.
func (s *Server) Run(ctx context.Context) {
	s.init()
	s.runCtx.Store(&ctx)
	changes, unsubscribe := s.Store.Subscribe()
	defer unsubscribe()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		s.iconWorker(ctx)
	}()
	go s.statsLoop(ctx)
	go s.desktopsLoop(ctx)
	go s.callLoop(ctx)
	s.wakeIcons()
	var last []byte
	for {
		select {
		case <-ctx.Done():
			s.hub.closeAll()
			<-workerDone // it may be writing icons/config; don't return until it stops
			return
		case <-changes:
			// Changes also fire for things phones don't see (the app in front, which
			// only the web UI shows): push only when the state really differs.
			msg := s.stateMessage()
			if s.forceBroadcast.Swap(false) || !bytes.Equal(msg, last) {
				s.hub.broadcast(msg)
				last = msg
			}
		}
	}
}

func (s *Server) init() {
	s.iconMu.Lock()
	defer s.iconMu.Unlock()
	if s.Stats == nil {
		s.Stats = sysstat.New()
	}
	if s.Command == nil {
		s.Command = launch.Command
	}
	if s.iconTried == nil {
		s.iconTried = map[string]bool{}
		s.appIcons = map[string]string{}
		s.iconWake = make(chan struct{}, 1)
	}
	if s.Addrs == nil {
		s.Addrs = netinfo.LANAddrs
	}
	if s.Log == nil {
		s.Log = log.Default()
	}
}

// --- phone-facing state --------------------------------------------------

type machineInfo struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	OS   string   `json:"os"`
	MACs []string `json:"macs,omitempty"`
	// Addrs: where phones can reach this agent now (LAN first, then Tailscale and the like).
	Addrs []string `json:"addrs,omitempty"`
}

type wireButton struct {
	ID    string           `json:"id"`
	Title string           `json:"title"`
	Kind  store.ButtonKind `json:"kind"`
	Icon  *string          `json:"icon"`
	// Glyph names a built-in picture for buttons without an icon: "keys", "text" or a
	// system action id ("media_play_pause", "volume", "lock", ...).
	Glyph string `json:"glyph,omitempty"`
	// Control "slider": hold and drag to set a level (the volume button).
	Control string `json:"control,omitempty"`
	// Confirm: ask before pressing; the agent refuses the press without "confirmed".
	Confirm bool `json:"confirm,omitempty"`
	// Buttons: a folder's buttons.
	Buttons []wireButton `json:"buttons,omitempty"`
	// Seconds: a timer's duration; the phone counts down by itself.
	Seconds int `json:"seconds,omitempty"`
	// Stat: what a live tile shows ("cpu", "ram"), from "stats" messages.
	Stat string `json:"stat,omitempty"`
}

type stateMsg struct {
	Type    string      `json:"type"`
	Machine machineInfo `json:"machine"`
	// Deck is the active profile's deck: apps that predate profiles show just this.
	Deck          wireDeck       `json:"deck"`
	Profiles      []wireProfile  `json:"profiles"`
	ActiveProfile string         `json:"activeProfile"`
	Recent        []store.Recent `json:"recent"`
	// TLS tells phones paired without the QR fingerprint what to pin.
	TLS *wireTLS `json:"tls,omitempty"`
}

type wireTLS struct {
	FP string `json:"fp"`
}

type wireDeck struct {
	Columns int          `json:"columns"`
	Buttons []wireButton `json:"buttons"`
}

type wireProfile struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Columns int          `json:"columns"`
	Buttons []wireButton `json:"buttons"`
}

func (s *Server) stateMessage() []byte {
	cfg := s.Store.Snapshot()
	active := s.activeProfile(&cfg)
	addrs := s.Addrs()
	msg := stateMsg{
		Type:          "state",
		Machine:       machineInfo{ID: cfg.MachineID, Name: cfg.Name, OS: OSName(), MACs: netinfo.MACs(addrs), Addrs: netinfo.IPs(addrs)},
		ActiveProfile: active.ID,
		Deck:          wireDeck{Columns: active.Deck.Columns, Buttons: wireButtons(active.Deck.Buttons)},
		Recent:        cfg.Recent,
	}
	if msg.Recent == nil {
		msg.Recent = []store.Recent{}
	}
	if s.TLSFingerprint != "" {
		msg.TLS = &wireTLS{FP: s.TLSFingerprint}
	}
	for _, p := range cfg.Profiles {
		msg.Profiles = append(msg.Profiles, wireProfile{ID: p.ID, Name: p.Name, Columns: p.Deck.Columns, Buttons: wireButtons(p.Deck.Buttons)})
	}
	data, _ := json.Marshal(msg)
	return data
}

// wireButtons is what the phone may know about buttons: no targets (paths, texts).
func wireButtons(buttons []store.Button) []wireButton {
	out := make([]wireButton, 0, len(buttons))
	for _, b := range buttons {
		wb := wireButton{ID: b.ID, Title: b.Title, Kind: b.Kind}
		switch b.Kind {
		case store.KindKeys, store.KindText:
			wb.Glyph = string(b.Kind)
		case store.KindFolder:
			wb.Glyph = string(b.Kind)
			wb.Buttons = wireButtons(b.Buttons)
		case store.KindMacro:
			wb.Glyph = string(b.Kind) // never the steps: they hold paths and texts
		case store.KindTimer:
			wb.Glyph = string(b.Kind)
			wb.Seconds, _ = strconv.Atoi(b.Target)
		case store.KindStat:
			wb.Glyph, wb.Stat = b.Target, b.Target
		case store.KindTrackpad:
			wb.Glyph = string(b.Kind)
		case store.KindCommand:
			wb.Glyph, wb.Confirm = string(b.Kind), b.Confirm
		case store.KindSystem:
			wb.Glyph = b.Target
			if a, ok := launch.LookupSystemAction(b.Target); ok {
				wb.Confirm = a.Confirm
				switch {
				case a.Slider:
					wb.Control = "slider"
				case a.Swipe:
					wb.Control = "desktops"
				}
			}
		}
		if b.Icon != "" {
			icon := b.Icon
			wb.Icon = &icon
		}
		out = append(out, wb)
	}
	return out
}

// --- the app in front on the PC -------------------------------------------

const maxRecentApps = 10

// SetForeground is fed by launch.WatchForeground. Only identity is kept, in memory.
func (s *Server) SetForeground(app launch.ForegroundApp) {
	if app.Name == "" {
		app.Name = s.appName(app.Keys)
	}
	s.fgMu.Lock()
	s.fg = app
	history := []launch.ForegroundApp{app}
	for _, a := range s.fgHistory {
		if a.ID() != app.ID() && len(history) < maxRecentApps {
			history = append(history, a)
		}
	}
	s.fgHistory = history
	s.fgMu.Unlock()
	s.Store.Notify() // the web UI shows it; phones only hear about a profile change
}

func (s *Server) foreground() (launch.ForegroundApp, []launch.ForegroundApp) {
	s.fgMu.Lock()
	defer s.fgMu.Unlock()
	return s.fg, append([]launch.ForegroundApp(nil), s.fgHistory...)
}

// activeProfile is the first profile bound to the app in front, else the default one.
func (s *Server) activeProfile(cfg *store.Config) *store.Profile {
	fg, _ := s.foreground()
	if len(fg.Keys) > 0 {
		for i := range cfg.Profiles {
			p := &cfg.Profiles[i]
			if p.ID == store.DefaultProfileID {
				continue
			}
			for _, rule := range p.Apps {
				if fg.MatchesAny(rule.Keys) {
					return p
				}
			}
		}
	}
	return cfg.Default()
}

// appName finds a friendly name for Store apps (whose windows carry only an AUMID) in
// the Start menu listing, if it has been loaded.
func (s *Server) appName(keys []string) string {
	if s.Apps == nil {
		return ""
	}
	apps, err := s.Apps.Get(context.Background(), false)
	if err != nil {
		return ""
	}
	for _, k := range keys {
		aumid, ok := strings.CutPrefix(k, "aumid:")
		if !ok {
			continue
		}
		for _, a := range apps {
			if strings.EqualFold(a.Target, aumid) {
				return a.Name
			}
		}
	}
	return ""
}

// --- icons ---------------------------------------------------------------

func (s *Server) siteIcon(_ store.ButtonKind, target string) (image.Image, error) {
	fetch := s.SiteIcon
	if fetch == nil {
		fetch = favicon.Fetch
	}
	return fetch(context.Background(), target)
}

func iconKey(kind store.ButtonKind, target string) string { return string(kind) + "|" + target }

func (s *Server) wakeIcons() {
	select {
	case s.iconWake <- struct{}{}:
	default:
	}
}

func (s *Server) forgetIcon(kind store.ButtonKind, target string) {
	s.iconMu.Lock()
	delete(s.iconTried, iconKey(kind, target))
	delete(s.appIcons, iconKey(kind, target))
	s.iconMu.Unlock()
}

// iconWorker extracts icons for buttons that have none, one at a time.
func (s *Server) iconWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.iconWake:
		}
		cfg := s.Store.Snapshot()
		for _, b := range cfg.AllButtons() {
			if b.Icon != "" || ctx.Err() != nil {
				continue
			}
			hash := s.iconFor(b.Kind, b.Target)
			if hash == "" {
				continue
			}
			err := s.Store.Update(func(c *store.Config) error {
				if cb := c.ButtonRef(b.ID); cb != nil && cb.Kind == b.Kind && cb.Target == b.Target && cb.Icon == "" {
					cb.Icon = hash
				}
				return nil
			})
			if err != nil {
				s.Log.Printf("icon: save: %v", err)
			}
		}
	}
}

// iconFor extracts (once per target) and stores a native icon; "" if there is none.
func (s *Server) iconFor(kind store.ButtonKind, target string) string {
	switch kind {
	case store.KindFolder, store.KindMacro, store.KindTimer, store.KindStat, store.KindTrackpad, store.KindCommand:
		return "" // a built-in glyph unless the user sets an icon
	}
	key := iconKey(kind, target)
	s.iconMu.Lock()
	if hash, ok := s.appIcons[key]; ok {
		s.iconMu.Unlock()
		return hash
	}
	if s.iconTried[key] {
		s.iconMu.Unlock()
		return ""
	}
	s.iconTried[key] = true
	s.iconMu.Unlock()

	hash := ""
	fetch := s.Launcher.Icon
	if kind == store.KindURL {
		fetch = s.siteIcon // links get the website's own icon
	}
	if img, err := fetch(kind, target); err == nil {
		if hash, err = s.Icons.PutImage(img); err != nil {
			s.Log.Printf("icon: store %q: %v", target, err)
		}
	}
	s.iconMu.Lock()
	s.appIcons[key] = hash
	s.iconMu.Unlock()
	return hash
}

// --- firewall ------------------------------------------------------------

// firewallStatus returns the cached status and refreshes it in the background when stale;
// the UI is notified when the fresh result lands.
func (s *Server) firewallStatus() firewall.Status {
	if s.Firewall == nil {
		return firewall.Status{}
	}
	st, fresh := s.Firewall.Cached()
	if !fresh && s.fwBusy.CompareAndSwap(false, true) {
		go func() {
			defer s.fwBusy.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			before := st
			if after := s.Firewall.Status(ctx); after != before {
				s.Store.Notify()
			}
		}()
	}
	st.Supported = true
	return st
}

// --- helpers -------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func readJSON(r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	return dec.Decode(v)
}
