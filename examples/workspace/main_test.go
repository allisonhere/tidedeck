package main

import (
	"os"
	"strings"
	"testing"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
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

// A dashboard started with live data configured must be live on the first
// frame. The deck's settings and mode used to be applied only when settings
// were saved, so a configured panel sat in demo mode until you opened the
// settings screen and pressed ctrl+s.
func TestStartupAppliesTheSavedConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.Live = true
	cfg.doc = dash.NewValues()
	cfg.doc.Set("weather.latitude", 52.52)
	cfg.doc.Set("weather.longitude", 13.405)
	cfg.doc.Set("weather.location", "Berlin")
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	if m.deck.Mode() != dash.ModeLive {
		t.Fatal("the deck was left in demo mode by a live configuration")
	}
	if m.state.live == nil {
		t.Fatal("the live source was not built at startup")
	}
	// Panels that hold their own data are filled before the first frame,
	// rather than a second later when the first tick arrives.
	clock, ok := m.ws.Lookup("clock")
	if !ok {
		t.Fatal("no clock panel")
	}
	body := ansi.Strip(clock.Render(tideui.PanelContext{
		ID: "clock", Width: 40, Renderer: viewRenderer(m.state),
	}))
	if strings.Contains(body, "Jan 1") {
		t.Fatalf("the clock rendered a zero time on the first frame:\n%s", body)
	}
	// A configured weather panel is told where it is, so it says it is
	// loading rather than that it has nowhere to look.
	weather, _ := m.ws.Lookup("weather")
	if got := ansi.Strip(weather.Render(tideui.PanelContext{
		ID: "weather", Width: 40, Renderer: viewRenderer(m.state),
	})); !strings.Contains(got, "Loading") {
		t.Fatalf("weather was not configured at startup: %q", got)
	}
}
