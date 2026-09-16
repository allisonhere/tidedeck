package main

import (
	"os"
	"strings"
	"testing"
)

// TestMain points the demo's persistence at a throwaway config directory so
// tests are hermetic and never read or write the developer's real layout.
func TestMain(m *testing.M) {
	if dir, err := os.MkdirTemp("", "tidedeck-test"); err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", dir)
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// The new panels must be placed in the presets that show them and named in the
// hidden list of every preset that does not. A panel that is neither placed nor
// hidden counts as visible-but-unplaced, and Show() drops it into that layout's
// root split the moment it is toggled.
func TestNewPanelsArePlacedOrHiddenInEveryPreset(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60

	expected := map[string]map[string]bool{ // preset -> panel -> should be visible
		"Overview":     {"gpu": true, "updates": false},
		"System":       {"gpu": true, "updates": true},
		"Productivity": {"gpu": false, "updates": false},
		"Developer":    {"gpu": false, "updates": false},
		"Minimal":      {"gpu": false, "updates": false},
	}
	for preset, panels := range expected {
		m.ws.ApplyPreset(preset)
		for id, want := range panels {
			if _, ok := m.ws.Lookup(id); !ok {
				t.Fatalf("panel %q is not registered", id)
			}
			if got := !m.ws.Hidden(id); got != want {
				t.Fatalf("preset %s: panel %q visible = %v, want %v", preset, id, got, want)
			}
		}
	}
}

// Both panels must actually draw in the layout they belong to.
func TestNewPanelsRenderInTheirPreset(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	m.ws.ApplyPreset("System")
	out := m.View()
	for _, want := range []string{"GPU", "Updates"} {
		if !strings.Contains(out, want) {
			t.Fatalf("System preset does not render %q", want)
		}
	}
}
