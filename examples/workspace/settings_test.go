package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui/provider"
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

	// The panel opens on the category list; General holds "Live data".
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // open General
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle Live data
	if !form.state.live {
		t.Fatal("space did not toggle live data")
	}
	// Back out, open Weather, move to latitude, clear it, and type a value.
	form.Update(tea.KeyMsg{Type: tea.KeyEsc})  // back to categories
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // Weather
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // enabled -> latitude
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
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})                     // open General
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle Live data
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

func TestLookupAppliesPlaceAndEnablesLive(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	if form.state.live {
		t.Fatal("live should start off")
	}
	form.applyPlace(provider.Place{Name: "Berlin", Latitude: 52.52, Longitude: 13.405, Country: "Germany"})
	if !form.state.live {
		t.Fatal("a lookup should enable live data")
	}
	if !form.state.weatherEnabled || form.state.latitude != "52.52" {
		t.Fatalf("place not applied: %+v", form.state)
	}
	if !form.dirty {
		t.Fatal("a lookup should mark the form dirty")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if !form.SavedConfig().Live {
		t.Fatal("saved config should have live enabled")
	}
}

func TestLookupQueuesQuery(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	form.state.place = "Berlin"
	if action := form.lookupCoordinates(); action != settingsNone {
		t.Fatalf("lookup action = %v", action)
	}
	if query := form.TakeLookup(); query != "Berlin" {
		t.Fatalf("queued query = %q, want Berlin", query)
	}
	if !form.lookingUp {
		t.Fatal("form should be marked as looking up")
	}
	if form.TakeLookup() != "" {
		t.Fatal("TakeLookup should clear the pending query")
	}
}

func TestApplyLookupFillsPlace(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	form.ApplyLookup(provider.Place{Name: "Berlin", Latitude: 52.52, Longitude: 13.405, Country: "Germany"}, nil)
	if form.lookingUp {
		t.Fatal("lookingUp should be cleared")
	}
	if !form.state.live || !form.state.weatherEnabled {
		t.Fatal("lookup should enable live weather")
	}
	if form.state.latitude != "52.52" || form.state.location != "Berlin" {
		t.Fatalf("place not applied: %+v", form.state)
	}
}

func TestApplyLookupError(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	form.ApplyLookup(provider.Place{}, errLookup)
	if form.problem == "" {
		t.Fatal("expected the error to be shown")
	}
	if form.state.live {
		t.Fatal("a failed lookup should not enable live data")
	}
}

var errLookup = &lookupError{}

type lookupError struct{}

func (*lookupError) Error() string { return "no match" }

func TestSettingsCategoryNavigation(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{})
	if form.view != viewCategories {
		t.Fatal("should open on the category list")
	}
	if form.categories[0].name != "General" {
		t.Fatalf("first category = %q", form.categories[0].name)
	}

	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if form.view != viewFields {
		t.Fatal("enter did not open a category")
	}
	if len(form.currentFields()) == 0 || form.currentFields()[0].label != "Live data" {
		t.Fatalf("General fields = %+v", form.currentFields())
	}

	form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if form.view != viewCategories {
		t.Fatal("esc did not return to the categories")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyDown})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := form.categories[form.category].name; got != "Weather" {
		t.Fatalf("category = %q, want Weather", got)
	}
	found := false
	for _, field := range form.currentFields() {
		if field.label == "Look up coordinates" {
			found = true
		}
	}
	if !found {
		t.Fatal("Weather category is missing the coordinate lookup")
	}
}
