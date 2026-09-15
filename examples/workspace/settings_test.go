package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWeatherWiredFromConfig(t *testing.T) {
	cfg := config{Weather: weatherConfig{
		Enabled: true, Latitude: 52.52, Longitude: 13.405,
		Location: "Berlin", Fahrenheit: true, WindMPH: true,
	}}
	source := newLiveSource(cfg)
	if source.dashboard.Weather == nil {
		t.Fatal("weather fetcher was not wired from config")
	}
}

func TestWeatherSkippedWithoutCoordinates(t *testing.T) {
	source := newLiveSource(config{Weather: weatherConfig{Enabled: true}})
	if source.dashboard.Weather != nil {
		t.Fatal("weather should be unset when no coordinates are configured")
	}
}

func TestSettingsFormEditsAndSaves(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})

	// Cursor starts on "Live data"; space toggles it.
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !form.state.live {
		t.Fatal("space did not toggle live data")
	}
	// Move to latitude, clear it, and type a value.
	form.Update(tea.KeyMsg{Type: tea.KeyDown})
	form.Update(tea.KeyMsg{Type: tea.KeyDown})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	form.state.latitude = ""
	for _, r := range "52.52" {
		form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v, want settingsSaved", action)
	}
	cfg := form.SavedConfig()
	if !cfg.Live || cfg.Weather.Latitude != 52.52 {
		t.Fatalf("saved config = %+v", cfg)
	}
}

func TestSettingsFormRejectsBadNumber(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	form.state.latitude = "not-a-number"
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsNone {
		t.Fatalf("save should be rejected, got %v", action)
	}
	if form.problem == "" {
		t.Fatal("expected a validation message")
	}
}

// TestSavingSettingsAppliesLiveSource drives the model: s opens settings, space
// enables live data, ctrl+s saves, and the dashboard switches to providers.
func TestSavingSettingsAppliesLiveSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	if m.cfg.Live {
		t.Fatal("fresh model should start in demo mode")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.settings.Opened() {
		t.Fatal("s did not open settings")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.cfg.Live {
		t.Fatal("save did not persist live mode")
	}
	if m.state.live == nil {
		t.Fatal("live source was not applied after save")
	}
	if m.settings.Opened() {
		t.Fatal("settings should close after save")
	}
}

func TestSettingsLookupValidatesInput(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	form.state.place = ""
	if action := form.lookupCoordinates(); action != settingsNone {
		t.Fatalf("blank lookup action = %v", action)
	}
	if form.problem == "" {
		t.Fatal("blank lookup should report a problem")
	}
}
