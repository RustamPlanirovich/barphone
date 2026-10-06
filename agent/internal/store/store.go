// Package store keeps the agent configuration (deck, paired devices, launch history)
// in a single JSON file and notifies subscribers about every change.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ButtonKind string

const (
	KindApp  ButtonKind = "app"  // Windows AppUserModelID / macOS .app bundle path
	KindPath ButtonKind = "path" // file, program or folder
	KindURL  ButtonKind = "url"  // any URL, including custom schemes (steam://, tg://)

	KindKeys   ButtonKind = "keys"   // key combination, e.g. "Ctrl+Shift+P"
	KindText   ButtonKind = "text"   // text typed into the focused window; a newline presses Enter
	KindSystem ButtonKind = "system" // media/volume/power action, see launch.SystemActions

	KindFolder ButtonKind = "folder" // opens its own Buttons on the phone; one level deep
	KindMacro  ButtonKind = "macro"  // runs its Steps one after another
	KindWait   ButtonKind = "wait"   // a macro step only: pause for Target milliseconds
	KindTimer  ButtonKind = "timer"  // counts down Target seconds on the phone
	KindStat   ButtonKind = "stat"   // live tile: Target "cpu" or "ram"
	// KindTrackpad opens the phone's trackpad and keyboard; the agent accepts pointer and
	// typing messages only on behalf of such a button.
	KindTrackpad ButtonKind = "trackpad"
	// KindCommand runs Target as a command line in Dir (a console window on Windows).
	KindCommand ButtonKind = "command"
)

func (k ButtonKind) Valid() bool {
	switch k {
	case KindApp, KindPath, KindURL, KindKeys, KindText, KindSystem, KindFolder, KindMacro, KindWait, KindTimer, KindStat, KindTrackpad, KindCommand:
		return true
	}
	return false
}

// Launches reports whether the button starts a program (and so can have open windows).
func (k ButtonKind) Launches() bool { return k == KindApp || k == KindPath }

type Button struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Kind   ButtonKind `json:"kind"`
	Target string     `json:"target"`
	Args   string     `json:"args,omitempty"`
	Icon   string     `json:"icon,omitempty"` // icon hash, see package icons
	// OnRunning: "" (default) switches to the app's open window, asking the phone to
	// choose when there are several; "new" always starts another instance.
	OnRunning string `json:"onRunning,omitempty"`

	// Command buttons: working folder, ask on the phone first, keep the console open.
	Dir      string `json:"dir,omitempty"`
	Confirm  bool   `json:"confirm,omitempty"`
	KeepOpen bool   `json:"keepOpen,omitempty"`

	Buttons []Button `json:"buttons,omitempty"` // a folder's buttons
	// Steps of a macro: actions (no folders, macros or confirmed actions) and waits. They
	// are not buttons of their own: no ID, never pressed or shown on the phone.
	Steps []Button `json:"steps,omitempty"`
}

const OnRunningNew = "new"

type Deck struct {
	Columns int      `json:"columns"`
	Buttons []Button `json:"buttons"`
}

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash"`
	PairedAt  time.Time `json:"pairedAt"`
	LastSeen  time.Time `json:"lastSeen,omitzero"`
}

type Recent struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

// AppRule binds a profile to an application on the PC. Keys come from the agent
// ("exe:code.exe", "aumid:…", "class:cabinetwclass", "bundle:…"); a foreground window
// matches when it produces any of them. Name is only for display.
type AppRule struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
}

// Profile is one deck. The phone shows the profile whose apps are in front on the PC,
// and the default profile otherwise.
type Profile struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Apps []AppRule `json:"apps"`
	Deck Deck      `json:"deck"`
}

const (
	DefaultProfileID   = "default"
	DefaultProfileName = "Основной"
)

type Config struct {
	MachineID string    `json:"machineId"`
	Name      string    `json:"name"`
	Profiles  []Profile `json:"profiles"`
	Devices   []Device  `json:"devices"`
	Recent    []Recent  `json:"recent"`

	// LegacyDeck is the single deck of config files written before profiles existed;
	// Open moves it into the default profile.
	LegacyDeck *Deck `json:"deck,omitempty"`
}

const (
	MaxRecent      = 12
	DefaultColumns = 3
	MinColumns     = 2
	MaxColumns     = 8
	MaxProfiles    = 30
)

// Default returns the default profile (always present after Open).
func (c *Config) Default() *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].ID == DefaultProfileID {
			return &c.Profiles[i]
		}
	}
	c.Profiles = append([]Profile{{ID: DefaultProfileID, Name: DefaultProfileName, Deck: Deck{Columns: DefaultColumns}}}, c.Profiles...)
	return &c.Profiles[0]
}

func (c *Config) Profile(id string) (*Profile, bool) {
	for i := range c.Profiles {
		if c.Profiles[i].ID == id {
			return &c.Profiles[i], true
		}
	}
	return nil, false
}

// Button finds a button in any profile: a phone may press buttons of a profile that
// is not the one in front (pinned, or it just switched).
func (c *Config) Button(id string) (Button, bool) {
	if b := c.ButtonRef(id); b != nil {
		return *b, true
	}
	return Button{}, false
}

// ButtonRef returns a pointer into the config for in-place edits, or nil.
func (c *Config) ButtonRef(id string) *Button {
	var found *Button
	c.walk(func(b *Button) bool {
		if b.ID == id {
			found = b
		}
		return found == nil
	})
	return found
}

// AllButtons lists the pressable buttons of every profile, folder contents included.
func (c *Config) AllButtons() []Button {
	var out []Button
	c.walk(func(b *Button) bool { out = append(out, *b); return true })
	return out
}

// walk visits every pressable button (folders and what is inside them) until fn
// returns false.
func (c *Config) walk(fn func(*Button) bool) {
	for p := range c.Profiles {
		if !WalkButtons(c.Profiles[p].Deck.Buttons, fn) {
			return
		}
	}
}

// WalkButtons visits buttons and the contents of folders among them until fn returns
// false; it reports whether the walk went to the end.
func WalkButtons(buttons []Button, fn func(*Button) bool) bool {
	for i := range buttons {
		if !fn(&buttons[i]) || !WalkButtons(buttons[i].Buttons, fn) {
			return false
		}
	}
	return true
}

// PushRecent moves id to the head of the history, dropping duplicates and overflow.
func (c *Config) PushRecent(id string, at time.Time) {
	out := []Recent{{ID: id, At: at.UTC()}}
	for _, r := range c.Recent {
		if r.ID != id && len(out) < MaxRecent {
			out = append(out, r)
		}
	}
	c.Recent = out
}

// PruneRecent drops history entries whose buttons no longer exist.
func (c *Config) PruneRecent() {
	out := c.Recent[:0]
	for _, r := range c.Recent {
		if _, ok := c.Button(r.ID); ok {
			out = append(out, r)
		}
	}
	c.Recent = out
}

func (c Config) clone() Config {
	profiles := make([]Profile, len(c.Profiles))
	for i, p := range c.Profiles {
		p.Deck.Buttons = cloneButtons(p.Deck.Buttons)
		apps := make([]AppRule, len(p.Apps))
		for j, a := range p.Apps {
			a.Keys = append([]string(nil), a.Keys...)
			apps[j] = a
		}
		if p.Apps == nil {
			apps = nil
		}
		p.Apps = apps
		profiles[i] = p
	}
	c.Profiles = profiles
	c.LegacyDeck = nil
	c.Devices = append([]Device(nil), c.Devices...)
	c.Recent = append([]Recent(nil), c.Recent...)
	return c
}

// cloneButtons copies nested lists too: edits through ButtonRef must never reach a
// snapshot or the config Update rolls back to.
func cloneButtons(in []Button) []Button {
	if len(in) == 0 {
		return nil
	}
	out := make([]Button, len(in))
	for i, b := range in {
		b.Buttons = cloneButtons(b.Buttons)
		b.Steps = cloneButtons(b.Steps)
		out[i] = b
	}
	return out
}

type Store struct {
	path string

	mu   sync.RWMutex
	cfg  Config
	subs map[chan struct{}]struct{}
}

// Open loads <dir>/config.json, creating it with defaults on first run.
func Open(dir, defaultName string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "config.json"), subs: map[chan struct{}]struct{}{}}
	data, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", s.path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, err
	}

	dirty := false
	if s.cfg.MachineID == "" {
		s.cfg.MachineID = RandomHex(16)
		dirty = true
	}
	if s.cfg.Name == "" {
		s.cfg.Name = defaultName
		dirty = true
	}
	if s.cfg.LegacyDeck != nil {
		// Pre-profiles config: keep a copy of the original file, then move the deck into
		// the default profile. Buttons, icons, onRunning and history are kept as they are.
		if data != nil {
			bak := filepath.Join(dir, "config.v1.bak.json")
			if _, err := os.Stat(bak); errors.Is(err, os.ErrNotExist) {
				if err := os.WriteFile(bak, data, 0o600); err != nil {
					return nil, fmt.Errorf("backup before migration: %w", err)
				}
			}
		}
		if _, ok := s.cfg.Profile(DefaultProfileID); !ok {
			s.cfg.Profiles = append([]Profile{{ID: DefaultProfileID, Name: DefaultProfileName, Deck: *s.cfg.LegacyDeck}}, s.cfg.Profiles...)
		}
		s.cfg.LegacyDeck = nil
		dirty = true
	}
	if _, ok := s.cfg.Profile(DefaultProfileID); !ok {
		s.cfg.Default()
		dirty = true
	}
	for i := range s.cfg.Profiles {
		if c := s.cfg.Profiles[i].Deck.Columns; c < MinColumns || c > MaxColumns {
			s.cfg.Profiles[i].Deck.Columns = DefaultColumns
			dirty = true
		}
	}
	if dirty {
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.clone()
}

// Update applies fn to a copy of the config; if fn succeeds the copy is persisted and
// subscribers are notified. Returning an error from fn leaves the config untouched.
func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	next := s.cfg.clone()
	if err := fn(&next); err != nil {
		s.mu.Unlock()
		return err
	}
	prev := s.cfg
	s.cfg = next
	if err := s.save(); err != nil {
		s.cfg = prev
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.Notify()
	return nil
}

// Subscribe returns a channel that receives a (coalesced) signal after every change.
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// Notify wakes all subscribers. Also used for changes that live outside the config
// (e.g. a phone connecting) but still affect what the UI shows.
func (s *Store) Notify() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// save writes atomically; caller holds s.mu.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func RandomHex(nbytes int) string {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
