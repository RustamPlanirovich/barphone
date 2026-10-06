package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"barphone/agent/internal/launch"
	"barphone/agent/internal/store"
)

const maxButtonsPerDeck = 200

// deckNormalizer validates decks coming from the UI. Button IDs stay unique across all
// profiles (a phone presses buttons by ID, whatever profile they are in) and icons stay
// attached unless the button's target changed.
type deckNormalizer struct {
	s    *Server
	prev map[string]store.Button // every button before the edit, by ID
	seen map[string]bool         // IDs already used in the result
}

func (s *Server) newNormalizer(c *store.Config) *deckNormalizer {
	n := &deckNormalizer{s: s, prev: map[string]store.Button{}, seen: map[string]bool{}}
	for _, b := range c.AllButtons() {
		n.prev[b.ID] = b
	}
	return n
}

func (n *deckNormalizer) deck(in store.Deck) (store.Deck, error) {
	total := 0
	store.WalkButtons(in.Buttons, func(*store.Button) bool { total++; return true })
	if total > maxButtonsPerDeck {
		return store.Deck{}, fmt.Errorf("не больше %d кнопок в профиле (вместе с папками)", maxButtonsPerDeck)
	}
	buttons, err := n.buttons(in.Buttons, false)
	if err != nil {
		return store.Deck{}, err
	}
	return store.Deck{Columns: min(max(in.Columns, store.MinColumns), store.MaxColumns), Buttons: buttons}, nil
}

// buttons normalizes one level of a deck: the top or, with inFolder, a folder's contents.
func (n *deckNormalizer) buttons(in []store.Button, inFolder bool) ([]store.Button, error) {
	out := []store.Button{}
	for _, b := range in {
		if b.Kind == store.KindFolder && inFolder {
			return nil, errors.New("папку нельзя положить в другую папку")
		}
		if b.Kind != store.KindText { // text keeps its spaces and newlines
			b.Target = strings.TrimSpace(b.Target)
		}
		b.Args = strings.TrimSpace(b.Args)
		if b.OnRunning != store.OnRunningNew {
			b.OnRunning = ""
		}
		if err := launch.Validate(b); err != nil {
			if strings.TrimSpace(b.Title) == "" {
				return nil, err
			}
			return nil, fmt.Errorf("«%s»: %w", strings.TrimSpace(b.Title), err)
		}
		if b.Kind == store.KindKeys {
			c, _ := launch.ParseCombo(b.Target)
			b.Target = c.String() // store the canonical spelling
		}
		if !b.Kind.Launches() {
			b.OnRunning, b.Args = "", ""
		}
		if b.Kind == store.KindFolder {
			b.Target = ""
			children, err := n.buttons(b.Buttons, true)
			if err != nil {
				return nil, fmt.Errorf("папка «%s»: %w", strings.TrimSpace(b.Title), err)
			}
			b.Buttons = children
		} else {
			b.Buttons = nil
		}
		b.Title = strings.TrimSpace(b.Title)
		if b.Title == "" {
			b.Title = defaultTitle(b)
		}
		if utf8.RuneCountInString(b.Title) > 64 {
			b.Title = string([]rune(b.Title)[:64])
		}
		old, existed := n.prev[b.ID]
		if !existed || n.seen[b.ID] {
			b.ID = "b_" + store.RandomHex(4)
		}
		n.seen[b.ID] = true
		// Keep a known icon unless the target changed underneath it.
		if b.Icon != "" && !n.s.Icons.Has(b.Icon) {
			b.Icon = ""
		}
		if existed && (old.Kind != b.Kind || old.Target != b.Target) && b.Icon == old.Icon {
			b.Icon = ""
		}
		out = append(out, b)
	}
	return out, nil
}

// uiPutDeck edits the default profile's deck (kept for scripts and older UIs).
func (s *Server) uiPutDeck(w http.ResponseWriter, r *http.Request) {
	var in store.Deck
	if err := readJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	var saved store.Deck
	err := s.Store.Update(func(c *store.Config) error {
		n := s.newNormalizer(c)
		for _, p := range c.Profiles {
			if p.ID != store.DefaultProfileID {
				store.WalkButtons(p.Deck.Buttons, func(b *store.Button) bool { n.seen[b.ID] = true; return true })
			}
		}
		deck, err := n.deck(in)
		if err != nil {
			return err
		}
		c.Default().Deck = deck
		c.PruneRecent()
		saved = deck
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "message": err.Error()})
		return
	}
	s.wakeIcons()
	writeJSON(w, http.StatusOK, saved)
}

var appKeyRe = regexp.MustCompile(`^(exe|aumid|class|bundle):\S.{0,255}$`)

// uiPutProfiles replaces all profiles at once: names, app bindings and decks.
func (s *Server) uiPutProfiles(w http.ResponseWriter, r *http.Request) {
	var in []store.Profile
	if err := readJSON(r, &in, 4<<20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	var saved []store.Profile
	err := s.Store.Update(func(c *store.Config) error {
		profiles, err := s.normalizeProfiles(c, in)
		if err != nil {
			return err
		}
		c.Profiles = profiles
		c.PruneRecent()
		saved = profiles
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "message": err.Error()})
		return
	}
	s.wakeIcons()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) normalizeProfiles(c *store.Config, in []store.Profile) ([]store.Profile, error) {
	if len(in) > store.MaxProfiles {
		return nil, fmt.Errorf("не больше %d профилей", store.MaxProfiles)
	}
	n := s.newNormalizer(c)
	ids := map[string]bool{}
	keyOwner := map[string]int{} // app key → index of the profile that binds it
	var out []store.Profile
	hasDefault := false
	for _, p := range in {
		p.Name = strings.TrimSpace(p.Name)
		if p.ID == store.DefaultProfileID && !ids[p.ID] {
			hasDefault = true
			if p.Name == "" {
				p.Name = store.DefaultProfileName
			}
			p.Apps = nil // the default profile is what shows when nothing else matches
		} else {
			if _, known := c.Profile(p.ID); !known || ids[p.ID] || p.ID == store.DefaultProfileID {
				p.ID = "p_" + store.RandomHex(4)
			}
			if p.Name == "" {
				return nil, errors.New("у профиля должно быть название")
			}
		}
		if utf8.RuneCountInString(p.Name) > 32 {
			return nil, fmt.Errorf("название «%s» длиннее 32 символов", p.Name)
		}
		ids[p.ID] = true

		var apps []store.AppRule
		for _, rule := range p.Apps {
			var keys []string
			for _, k := range rule.Keys {
				k = strings.ToLower(strings.TrimSpace(k))
				if !appKeyRe.MatchString(k) {
					return nil, fmt.Errorf("непонятная привязка %q", k)
				}
				if owner, taken := keyOwner[k]; taken && owner != len(out) {
					return nil, fmt.Errorf("«%s» уже привязано к профилю «%s»", ruleName(rule, k), out[owner].Name)
				}
				if !containsString(keys, k) {
					keys = append(keys, k)
				}
			}
			if len(keys) == 0 {
				continue
			}
			for _, k := range keys {
				keyOwner[k] = len(out)
			}
			rule.Name = strings.TrimSpace(rule.Name)
			if r := []rune(rule.Name); len(r) > 64 {
				rule.Name = string(r[:64])
			}
			if rule.Name == "" {
				rule.Name = ruleName(rule, keys[0])
			}
			rule.Keys = keys
			apps = append(apps, rule)
		}
		if apps == nil {
			apps = []store.AppRule{} // always a list for the UI
		}
		p.Apps = apps

		deck, err := n.deck(p.Deck)
		if err != nil {
			return nil, fmt.Errorf("профиль «%s»: %w", p.Name, err)
		}
		p.Deck = deck
		out = append(out, p)
	}
	if !hasDefault {
		return nil, errors.New("основной профиль нельзя удалить")
	}
	// The default profile always comes first.
	for i, p := range out {
		if p.ID == store.DefaultProfileID && i > 0 {
			out = append([]store.Profile{p}, append(out[:i:i], out[i+1:]...)...)
			break
		}
	}
	return out, nil
}

func ruleName(rule store.AppRule, key string) string {
	if strings.TrimSpace(rule.Name) != "" {
		return strings.TrimSpace(rule.Name)
	}
	_, v, _ := strings.Cut(key, ":")
	return v
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// uiAppKeys tells the UI what a profile binding to an installed app should match.
func (s *Server) uiAppKeys(w http.ResponseWriter, r *http.Request) {
	b := store.Button{Kind: store.ButtonKind(r.URL.Query().Get("kind")), Target: r.URL.Query().Get("target")}
	if !b.Kind.Launches() || b.Target == "" {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	keys := s.Launcher.AppKeys(b)
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}
