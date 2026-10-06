//go:build manual

package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Migrates a COPY of a real pre-profiles config and checks nothing is lost:
//
//	BARPHONE_CONFIG_COPY=<dir with config.json and icons/> go test -tags manual -run MigrateRealCopy -v ./internal/store
func TestMigrateRealCopy(t *testing.T) {
	dir := os.Getenv("BARPHONE_CONFIG_COPY")
	if dir == "" {
		t.Skip("set BARPHONE_CONFIG_COPY")
	}
	original, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before struct {
		Deck    *Deck    `json:"deck"`
		Devices []Device `json:"devices"`
		Recent  []Recent `json:"recent"`
	}
	if err := json.Unmarshal(original, &before); err != nil {
		t.Fatal(err)
	}
	if before.Deck == nil {
		t.Skip("config already has profiles")
	}

	s, err := Open(dir, "host")
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Snapshot()
	def := cfg.Default()
	if len(cfg.Profiles) != 1 || def.Deck.Columns != before.Deck.Columns || len(def.Deck.Buttons) != len(before.Deck.Buttons) {
		t.Fatalf("deck changed: %+v vs %+v", def.Deck, before.Deck)
	}
	for i, b := range before.Deck.Buttons {
		got := def.Deck.Buttons[i]
		if got != b {
			t.Errorf("button %d: %+v -> %+v", i, b, got)
		}
		if b.Icon != "" {
			if _, err := os.Stat(filepath.Join(dir, "icons", b.Icon+".png")); err != nil {
				t.Errorf("icon of %q missing: %v", b.Title, err)
			}
		}
		t.Logf("kept: %-20q kind=%-6s icon=%v onRunning=%q", b.Title, b.Kind, b.Icon != "", b.OnRunning)
	}
	if len(cfg.Devices) != len(before.Devices) || len(cfg.Recent) != len(before.Recent) {
		t.Errorf("devices %d->%d, recent %d->%d", len(before.Devices), len(cfg.Devices), len(before.Recent), len(cfg.Recent))
	}
	for _, d := range cfg.Devices {
		t.Logf("device kept: %q", d.Name)
	}
	bak, err := os.ReadFile(filepath.Join(dir, "config.v1.bak.json"))
	if err != nil || !bytes.Equal(bak, original) {
		t.Fatalf("backup must equal the original file: %v", err)
	}
}
