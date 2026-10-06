package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A config.json written before profiles existed (single top-level "deck").
const v1Config = `{
  "machineId": "abc",
  "name": "Рабочий ПК",
  "deck": {
    "columns": 4,
    "buttons": [
      {"id": "b_1", "title": "VS Code", "kind": "app", "target": "Microsoft.VisualStudioCode", "icon": "0123456789abcdef", "onRunning": "new"},
      {"id": "b_2", "title": "Пауза", "kind": "system", "target": "media_play_pause"}
    ]
  },
  "devices": [{"id": "d1", "name": "Pixel 8", "tokenHash": "h", "pairedAt": "2026-10-06T08:00:00Z"}],
  "recent": [{"id": "b_2", "at": "2026-10-06T09:00:00Z"}]
}`

func TestMigratesV1Deck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(v1Config), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, "host")
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Snapshot()
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].ID != DefaultProfileID || cfg.Profiles[0].Name != DefaultProfileName {
		t.Fatalf("profiles: %+v", cfg.Profiles)
	}
	d := cfg.Profiles[0].Deck
	if d.Columns != 4 || len(d.Buttons) != 2 || d.Buttons[0].Icon != "0123456789abcdef" || d.Buttons[0].OnRunning != "new" {
		t.Fatalf("deck not carried over: %+v", d)
	}
	if len(cfg.Recent) != 1 || len(cfg.Devices) != 1 || cfg.Name != "Рабочий ПК" {
		t.Fatalf("recent/devices/name lost: %+v", cfg)
	}
	if b, ok := cfg.Button("b_2"); !ok || b.Title != "Пауза" {
		t.Fatal("button lookup across profiles")
	}

	bak, err := os.ReadFile(filepath.Join(dir, "config.v1.bak.json"))
	if err != nil || string(bak) != v1Config {
		t.Fatalf("backup must hold the original file: %v", err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var raw map[string]json.RawMessage
	json.Unmarshal(saved, &raw)
	if _, legacy := raw["deck"]; legacy {
		t.Error("migrated config must not keep the top-level deck")
	}
	if _, ok := raw["profiles"]; !ok {
		t.Error("migrated config must have profiles")
	}

	// Opening again is a no-op and keeps the first backup.
	os.WriteFile(filepath.Join(dir, "config.v1.bak.json"), []byte("keep me"), 0o600)
	if _, err := Open(dir, "host"); err != nil {
		t.Fatal(err)
	}
	if bak, _ := os.ReadFile(filepath.Join(dir, "config.v1.bak.json")); string(bak) != "keep me" {
		t.Error("backup must be written only once")
	}
}

func TestFreshConfigHasDefaultProfile(t *testing.T) {
	s, err := Open(t.TempDir(), "host")
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Snapshot()
	if len(cfg.Profiles) != 1 || cfg.Default().Deck.Columns != DefaultColumns {
		t.Fatalf("%+v", cfg.Profiles)
	}
}

func TestSnapshotIsDeep(t *testing.T) {
	s, _ := Open(t.TempDir(), "host")
	s.Update(func(c *Config) error {
		c.Profiles = append(c.Profiles, Profile{ID: "p", Name: "VS Code", Apps: []AppRule{{Name: "Code", Keys: []string{"exe:code.exe"}}},
			Deck: Deck{Columns: 3, Buttons: []Button{{ID: "x", Title: "a"}}}})
		return nil
	})
	snap := s.Snapshot()
	snap.Profiles[1].Apps[0].Keys[0] = "mutated"
	snap.Profiles[1].Deck.Buttons[0].Title = "mutated"
	again := s.Snapshot()
	if again.Profiles[1].Apps[0].Keys[0] != "exe:code.exe" || again.Profiles[1].Deck.Buttons[0].Title != "a" {
		t.Fatal("snapshot shares memory with the store")
	}
	if !strings.HasPrefix(again.Profiles[0].ID, DefaultProfileID) {
		t.Fatal("default first")
	}
}
