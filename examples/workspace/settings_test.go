package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
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
	fields := form.currentFields()
	for i := 0; i < len(fields) && form.currentField().label != "latitude"; i++ {
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if got := form.currentField().label; got != "latitude" {
		t.Fatalf("could not navigate to latitude, landed on %q", got)
	}
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

func TestSettingsPanelToggles(t *testing.T) {
	ws := tideui.NewWorkspace()
	ws.Panel("weather", nil).Title("Weather")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	// Weather's own settings page opens with an enable/disable toggle.
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // General -> Weather
	if got := form.categories[form.category].name; got != "Weather" {
		t.Fatalf("category = %q, want Weather", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	field := form.currentField()
	if field == nil || field.kind != fieldPanel || field.panel != "weather" {
		t.Fatalf("first Weather field = %+v, want the panel toggle", field)
	}
	if !form.panelVisible("weather") {
		t.Fatal("panel should start visible")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // hide
	if !ws.Hidden("weather") {
		t.Fatal("enter did not hide the panel")
	}
	if form.panelVisible("weather") {
		t.Fatal("tick should be off after hiding")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // show
	if ws.Hidden("weather") {
		t.Fatal("second enter did not show the panel")
	}
	if !form.panelVisible("weather") {
		t.Fatal("tick should be on after showing")
	}
}

func TestSettingsClockHasHourFormat(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{Clock24: true})
	for i, category := range form.categories {
		if category.name != "Clock" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open Clock
		field := form.currentField()
		if field == nil || field.label != "24-hour" || field.flag == nil {
			t.Fatalf("first Clock field = %+v, want the 24-hour toggle", field)
		}
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // toggle off
		if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
			t.Fatalf("save action = %v", action)
		}
		if form.SavedConfig().Clock24 {
			t.Fatal("24-hour toggle did not save as off")
		}
		return
	}
	t.Fatal("no Clock category")
}

func TestSettingsGaugeStyleChoice(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{GaugeStyle: "solid"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // Live data -> gauge style
	field := form.currentField()
	if field == nil || field.kind != fieldChoice || field.choice == nil {
		t.Fatalf("field = %+v, want gauge style choice", field)
	}
	if got := form.value(*field); got != "solid" {
		t.Fatalf("gauge value = %q, want solid", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cycle
	if form.state.gauge == "solid" {
		t.Fatal("gauge style did not cycle")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if form.SavedConfig().GaugeStyle == "solid" {
		t.Fatal("gauge style did not save")
	}
}

func TestSettingsChoiceArrowKeys(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{GaugeStyle: "solid"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // gauge style
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := form.value(*form.currentField()); got != "blocks" {
		t.Fatalf("right arrow -> %q, want blocks", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := form.value(*form.currentField()); got != "solid" {
		t.Fatalf("left arrow -> %q, want solid", got)
	}
}

func TestSettingsSparkStyleChoice(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{SparkStyle: "blocks"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // gauge style
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // spark style
	field := form.currentField()
	if field == nil || field.kind != fieldChoice || !field.sparkPreview {
		t.Fatalf("field = %+v, want spark style choice", field)
	}
	if got := form.value(*field); got != "blocks" {
		t.Fatalf("spark value = %q, want blocks", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := form.value(*form.currentField()); got == "blocks" {
		t.Fatal("spark style did not cycle")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if form.SavedConfig().SparkStyle == "blocks" {
		t.Fatal("spark style did not save")
	}
}

func TestSettingsPanelSparkChoice(t *testing.T) {
	ws := tideui.NewWorkspace()
	ws.Panel("network", nil).Title("Network")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	for i, category := range form.categories {
		if category.name != "Network" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open Network
		form.Update(tea.KeyMsg{Type: tea.KeyDown})  // enabled -> gauge
		form.Update(tea.KeyMsg{Type: tea.KeyDown})  // gauge -> spark
		field := form.currentField()
		if field == nil || field.kind != fieldChoice || !field.sparkPreview {
			t.Fatalf("field = %+v, want the spark choice", field)
		}
		if got := form.value(*field); got != "default" {
			t.Fatalf("initial panel spark = %q, want default", got)
		}
		form.Update(tea.KeyMsg{Type: tea.KeyRight})
		if got := form.value(*form.currentField()); got == "default" {
			t.Fatal("panel spark did not cycle")
		}
		if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
			t.Fatalf("save action = %v", action)
		}
		if form.SavedConfig().PanelSparks["network"] == "" {
			t.Fatalf("panel spark not saved: %+v", form.SavedConfig().PanelSparks)
		}
		return
	}
	t.Fatal("no Network category")
}

func TestSettingsClockFontChoice(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{ClockFont: "dash"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	for i := 0; i < 3; i++ {
		form.Update(tea.KeyMsg{Type: tea.KeyDown}) // Live data -> gauge -> spark -> clock font
	}
	field := form.currentField()
	if field == nil || field.kind != fieldChoice || field.choice == nil {
		t.Fatalf("field = %+v, want the clock font choice", field)
	}
	if got := form.value(*field); got != "dash" {
		t.Fatalf("clock font = %q, want dash", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := form.value(*form.currentField()); got == "dash" {
		t.Fatal("clock font did not cycle")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if form.SavedConfig().ClockFont == "dash" {
		t.Fatal("clock font did not save")
	}
}

func TestSettingsPanelGaugeChoice(t *testing.T) {
	ws := tideui.NewWorkspace()
	ws.Panel("system", nil).Title("System")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	for i, category := range form.categories {
		if category.name != "System" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open System
		form.Update(tea.KeyMsg{Type: tea.KeyDown})  // enabled -> gauge style
		field := form.currentField()
		if field == nil || field.kind != fieldChoice {
			t.Fatalf("field = %+v, want the gauge choice", field)
		}
		if got := form.value(*field); got != "default" {
			t.Fatalf("initial panel gauge = %q, want default", got)
		}
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cycle
		if got := form.value(*form.currentField()); got == "default" {
			t.Fatal("panel gauge choice did not cycle")
		}
		if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
			t.Fatalf("save action = %v", action)
		}
		if form.SavedConfig().PanelGauges["system"] == "" {
			t.Fatalf("panel gauge not saved: %+v", form.SavedConfig().PanelGauges)
		}
		return
	}
	t.Fatal("no System category")
}

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
