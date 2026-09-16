package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/dash/panels"
	"github.com/allisonhere/tideui/provider"
)

// weatherForm opens a settings form with the weather panel registered, which
// is where the weather settings live now.
func weatherForm(t *testing.T) *settingsForm {
	t.Helper()
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.Weather())
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)
	return form
}

func TestSettingsFormEditsAndSaves(t *testing.T) {
	form := weatherForm(t)

	// The panel opens on the category list; General holds "Live data".
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // open General
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle Live data
	if !form.state.live {
		t.Fatal("space did not toggle live data")
	}
	// Back out, open Weather, and type a latitude into the panel's own row.
	form.Update(tea.KeyMsg{Type: tea.KeyEsc}) // back to categories
	openCategory(t, form, "Weather")
	latitude := selectNewsField(t, form, "latitude")
	*latitude.text = ""
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, r := range "52.52" {
		form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v, want settingsSaved", action)
	}
	cfg := form.SavedConfig()
	if !cfg.Live {
		t.Fatalf("saved config = %+v", cfg)
	}
	saved, err := cfg.document()
	if err != nil {
		t.Fatal(err)
	}
	// A coordinate stays a JSON number: moving it onto a panel must not
	// change the shape of a key that has always been numeric.
	if saved.Float("weather.latitude") != 52.52 {
		t.Fatalf("saved latitude = %v", saved.Float("weather.latitude"))
	}
}

func TestSettingsFormRejectsBadNumber(t *testing.T) {
	form := weatherForm(t)
	*form.state.panelText["weather.latitude"] = "not-a-number"
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
	form := weatherForm(t)
	form.state.place = ""
	if action := form.lookupCoordinates(); action != settingsNone {
		t.Fatalf("blank lookup action = %v", action)
	}
	if form.problem == "" {
		t.Fatal("blank lookup should report a problem")
	}
}

func TestLookupAppliesPlaceAndEnablesLive(t *testing.T) {
	form := weatherForm(t)
	if form.state.live {
		t.Fatal("live should start off")
	}
	form.applyPlace(provider.Place{Name: "Berlin", Latitude: 52.52, Longitude: 13.405, Country: "Germany"})
	if !form.state.live {
		t.Fatal("a lookup should enable live data")
	}
	// The lookup fills in the panel's own rows, so what it writes is exactly
	// what you could have typed there yourself.
	if !*form.state.panelFlag["weather.enabled"] {
		t.Fatal("lookup did not enable the weather panel")
	}
	if got := *form.state.panelText["weather.latitude"]; got != "52.52" {
		t.Fatalf("latitude = %q", got)
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
	form := weatherForm(t)
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
	form := weatherForm(t)
	form.ApplyLookup(provider.Place{Name: "Berlin", Latitude: 52.52, Longitude: 13.405, Country: "Germany"}, nil)
	if form.lookingUp {
		t.Fatal("lookingUp should be cleared")
	}
	if !form.state.live || !*form.state.panelFlag["weather.enabled"] {
		t.Fatal("lookup should enable live weather")
	}
	if *form.state.panelText["weather.latitude"] != "52.52" ||
		*form.state.panelText["weather.location"] != "Berlin" {
		t.Fatalf("place not applied: %+v", form.state.panelText)
	}
}

func TestApplyLookupError(t *testing.T) {
	form := weatherForm(t)
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

// The clock's settings are declared by the panel now rather than listed by
// hand, and they still write the keys that are already in the config file.
func TestSettingsClockFieldsComeFromThePanel(t *testing.T) {
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.Clock())
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	cfg.doc.Set("clock_24", true)
	cfg.doc.Set("zones", "Europe/London")
	form.Open(cfg)

	openCategory(t, form, "Clock")
	hour := selectNewsField(t, form, "24-hour")
	if hour.flag == nil || !*hour.flag {
		t.Fatalf("24-hour row = %+v, want a tick that is on", hour)
	}
	if got := *selectNewsField(t, form, "zones").text; got != "Europe/London" {
		t.Fatalf("zones = %q", got)
	}

	*hour.flag = false
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	saved, err := form.SavedConfig().document()
	if err != nil {
		t.Fatal(err)
	}
	// The keys keep the spelling they already had: moving a setting onto a
	// panel must not rewrite anyone's config.
	if saved.Bool("clock_24") {
		t.Fatal("24-hour toggle did not save as off")
	}
	if got := saved.String("zones"); got != "Europe/London" {
		t.Fatalf("zones = %q", got)
	}
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
	ws.Panel("gpu", nil).Title("GPU")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	for i, category := range form.categories {
		if category.name != "GPU" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open GPU
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
		if form.SavedConfig().PanelSparks["gpu"] == "" {
			t.Fatalf("panel spark not saved: %+v", form.SavedConfig().PanelSparks)
		}
		return
	}
	t.Fatal("no GPU category")
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
	ws.Panel("gpu", nil).Title("GPU")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	for i, category := range form.categories {
		if category.name != "GPU" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open GPU
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
		if form.SavedConfig().PanelGauges["gpu"] == "" {
			t.Fatalf("panel gauge not saved: %+v", form.SavedConfig().PanelGauges)
		}
		return
	}
	t.Fatal("no GPU category")
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

// openNews moves the form into the News category and returns its fields.
func openNews(t *testing.T, form *settingsForm) []formField {
	t.Helper()
	for i, category := range form.categories {
		if category.name != "News" {
			continue
		}
		form.category = i
		form.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return form.currentFields()
	}
	t.Fatal("no News category")
	return nil
}

// selectNewsField walks down to the row with the given label.
func selectNewsField(t *testing.T, form *settingsForm, label string) *formField {
	t.Helper()
	for i := 0; i < 60; i++ {
		if field := form.currentField(); field != nil && field.label == label {
			return field
		}
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	t.Fatalf("no field labelled %q", label)
	return nil
}

func TestSettingsNewsPresetToggle(t *testing.T) {
	ws := tideui.NewWorkspace()
	ws.Panel("news", nil).Title("News")
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.Open(config{})

	catalogue := provider.NewsSources()
	fields := openNews(t, form)
	if len(fields) < len(catalogue) {
		t.Fatalf("News has %d fields, want a row per source plus the text row", len(fields))
	}
	field := selectNewsField(t, form, catalogue[0].Name)
	if field.kind != fieldBool {
		t.Fatalf("%s row kind = %v, want a tick", catalogue[0].Name, field.kind)
	}
	// Starts off, because Open was given an empty config.
	if *field.flag {
		t.Fatalf("%s should start unticked", catalogue[0].Name)
	}
	form.Update(tea.KeyMsg{Type: tea.KeySpace})
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if !strings.Contains(form.SavedConfig().Feeds, catalogue[0].URL) {
		t.Fatalf("ticked source not saved: %q", form.SavedConfig().Feeds)
	}
}

func TestSettingsNewsSplitsCustomURLs(t *testing.T) {
	const custom = "https://example.com/custom.xml"
	catalogue := provider.NewsSources()
	form := newSettingsForm()
	form.Open(config{Feeds: catalogue[0].URL + "," + custom})

	openNews(t, form)
	if field := selectNewsField(t, form, catalogue[0].Name); !*field.flag {
		t.Fatalf("%s should be ticked from the saved config", catalogue[0].Name)
	}
	// The catalogue URL is not also left sitting in the text row.
	if got := *selectNewsField(t, form, "other feed urls").text; got != custom {
		t.Fatalf("other feed urls = %q, want %q", got, custom)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	saved := form.SavedConfig().Feeds
	if !strings.Contains(saved, catalogue[0].URL) || !strings.Contains(saved, custom) {
		t.Fatalf("saving dropped a feed: %q", saved)
	}
}

// The tick rows hold pointers into a slice on the form state. If anything
// re-allocates that slice after the rows are built, ticks land in an orphaned
// array and vanish on save without any error.
func TestSettingsNewsTicksSurviveTogether(t *testing.T) {
	catalogue := provider.NewsSources()
	form := newSettingsForm()
	form.Open(config{})

	openNews(t, form)
	form.Update(tea.KeyMsg{Type: tea.KeySpace}) // tick whichever row is first
	first := form.currentField().label
	second := catalogue[len(catalogue)-1].Name
	selectNewsField(t, form, second)
	form.Update(tea.KeyMsg{Type: tea.KeySpace})
	form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	saved := form.SavedConfig().Feeds
	for _, name := range []string{first, second} {
		var want string
		for _, source := range catalogue {
			if source.Name == name {
				want = source.URL
			}
		}
		if !strings.Contains(saved, want) {
			t.Fatalf("%s (%s) missing from saved feeds %q", name, want, saved)
		}
	}
}

// Both panels need a settings category, or their enabled/gauge/spark rows
// never appear and they cannot be toggled from the UI. Both now declare
// themselves through the deck rather than being listed by hand.
func TestSettingsHasPanelDeclaredCategories(t *testing.T) {
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.GPU(), panels.Updates())
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)

	// The setting lives in the document, which is where a panel-owned key
	// belongs now that the typed config has no field for it.
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	cfg.doc.Set("aur_helper", "paru")
	form.Open(cfg)

	found := map[string]bool{}
	for _, category := range form.categories {
		found[category.name] = true
	}
	for _, want := range []string{"GPU", "Updates"} {
		if !found[want] {
			t.Fatalf("no %s category in settings", want)
		}
	}

	// The panel's declared field is editable and round-trips through the
	// document rather than through a struct field.
	openCategory(t, form, "Updates")
	field := selectNewsField(t, form, "aur helper")
	if got := *field.text; got != "paru" {
		t.Fatalf("aur helper = %q, want paru", got)
	}
	*field.text = "yay"
	form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	saved, err := form.SavedConfig().document()
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.String("aur_helper"); got != "yay" {
		t.Fatalf("saved aur helper = %q, want yay", got)
	}
	// Saving a panel-owned key must not disturb the keys the struct owns.
	if saved.String("gauge_style") != defaultConfig().GaugeStyle {
		t.Fatalf("gauge_style changed to %q", saved.String("gauge_style"))
	}
}

// A panel that declares a setting is what puts it in the document, so the
// panel and the settings screen cannot disagree about where it lives.
func TestPanelOwnedSettingReachesThePanel(t *testing.T) {
	deck := dash.New()
	deck.Register(panels.Updates())

	values := dash.NewValues()
	values.Set("aur_helper", "paru")
	if errs := deck.Configure(values); len(errs) != 0 {
		t.Fatalf("configure errors: %v", errs)
	}
	schema := deck.Schema()
	if len(schema) != 1 || len(schema[0].Fields) != 1 {
		t.Fatalf("schema = %#v", schema)
	}
	if got := schema[0].Fields[0].Key; got != "aur_helper" {
		t.Fatalf("declared key = %q, want the key already in config.json", got)
	}
}

// openCategory moves the form into a named category.
func openCategory(t *testing.T, form *settingsForm, name string) {
	t.Helper()
	for i, category := range form.categories {
		if category.name == name {
			form.category = i
			form.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return
		}
	}
	t.Fatalf("no %s category", name)
}

func TestEditingViewFollowsCaret(t *testing.T) {
	const value = "~/Projects/tidedeck, ~/Projects/tideui, ~/Projects/tidegit, ~/Projects/tidemail"
	runes := []rune(value)

	// A value that fits is shown whole, with the caret where it belongs.
	if got := editingView("main", 2, 46); got != "ma"+caretMark+"in" {
		t.Fatalf("short value = %q", got)
	}
	// The caret marker used to be pinned to the end of the value whatever the
	// caret position, so typing after Home inserted text away from the cursor.
	for _, caret := range []int{0, 20, 42, len(runes)} {
		got := editingView(value, caret, 46)
		if lipgloss.Width(got) != 46 {
			t.Fatalf("caret %d: width = %d, want 46 (%q)", caret, lipgloss.Width(got), got)
		}
		if !strings.Contains(got, caretMark) {
			t.Fatalf("caret %d: no caret drawn in %q", caret, got)
		}
		// Text before the caret marker must be text that precedes it in the
		// value, which is what makes the cursor position believable.
		before := strings.TrimPrefix(strings.Split(got, caretMark)[0], "…")
		if before != "" && !strings.Contains(value, before) {
			t.Fatalf("caret %d: %q is not part of the value", caret, before)
		}
	}
	// Clipping is marked at whichever end is cut.
	if got := editingView(value, 0, 46); !strings.HasSuffix(got, "…") || strings.HasPrefix(got, "…") {
		t.Fatalf("caret at start should clip only the tail: %q", got)
	}
	if got := editingView(value, len(runes), 46); !strings.HasPrefix(got, "…") || strings.HasSuffix(got, "…") {
		t.Fatalf("caret at end should clip only the head: %q", got)
	}
	// Degenerate inputs must not panic or overflow.
	for _, c := range []struct {
		value        string
		caret, width int
	}{{"", 0, 46}, {value, 999, 46}, {value, -5, 46}, {value, 40, 2}, {"ünïcödé-brånch", 6, 10}} {
		got := editingView(c.value, c.caret, c.width)
		if lipgloss.Width(got) > max(6, c.width) {
			t.Fatalf("editingView(%q, %d, %d) = %q overflows", c.value, c.caret, c.width, got)
		}
	}
}

// A long path list used to render raw: it truncated mid-path and pushed the
// field's own label off the row entirely.
func TestSettingsRepoFieldSummarizes(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.Git())
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)

	// The setting lives in the document, where a panel-owned key belongs.
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	cfg.doc.Set("repos", real+","+filepath.Join(real, "missing"))
	form.Open(cfg)

	openCategory(t, form, "Git Activity")
	field := selectNewsField(t, form, "repositories")
	summary := form.value(*field)
	if !strings.Contains(summary, "2 repos") || !strings.Contains(summary, "1 not found") {
		t.Fatalf("summary = %q, want a count and the missing one", summary)
	}
	if strings.Contains(summary, real) {
		t.Fatalf("the row should summarize, not print the raw paths: %q", summary)
	}
	// An empty list says so rather than rendering as a blank row.
	empty := defaultConfig()
	empty.doc = dash.NewValues()
	form.Open(empty)
	openCategory(t, form, "Git Activity")
	if got := form.value(*selectNewsField(t, form, "repositories")); got != "none set" {
		t.Fatalf("empty repos = %q, want none set", got)
	}
	// Saving normalizes what was typed.
	dup := defaultConfig()
	dup.doc = dash.NewValues()
	form.Open(dup)
	openCategory(t, form, "Git Activity")
	*selectNewsField(t, form, "repositories").text = " " + real + " ," + real
	form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	saved, err := form.SavedConfig().document()
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.String("repos"); strings.Count(got, real) != 1 {
		t.Fatalf("saved repos = %q, want the duplicate collapsed", got)
	}
}

func TestSettingsIconStyleChoice(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{Icons: "emoji"})
	openCategory(t, form, "General")
	field := selectNewsField(t, form, "icons")
	if field.kind != fieldChoice {
		t.Fatalf("icons row = %+v, want a choice", field)
	}
	if got := form.value(*field); got != "emoji" {
		t.Fatalf("icons = %q, want emoji", got)
	}
	if form.Icons() != "emoji" {
		t.Fatal("the live preview accessor should follow the form state")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if form.Icons() == "emoji" {
		t.Fatal("the choice did not cycle")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if saved := form.SavedConfig().Icons; saved == "emoji" || saved == "" {
		t.Fatalf("saved icons = %q, want the cycled value", saved)
	}
	// An unknown value falls back rather than rendering nothing.
	if got := iconStyleOrDefault("sparkles"); got != string(tideui.IconEmoji) {
		t.Fatalf("iconStyleOrDefault(sparkles) = %q", got)
	}
}

// The settings pages must follow the panels, so the list does not reshuffle
// as panels move onto the registry - a page declared by a panel is appended
// to the hand-written list, and only the ordering puts it back in place.
func TestSettingsCategoriesFollowPanelOrder(t *testing.T) {
	m := newModel()
	m.settings.Open(m.cfg)

	rank := map[string]int{}
	for i, id := range m.ws.PanelIDs() {
		rank[id] = i
	}
	previous := -1
	seen := 0
	for _, category := range m.settings.categories {
		if category.panelID == "" {
			if seen > 0 {
				t.Fatalf("the non-panel page %q is not first", category.name)
			}
			continue
		}
		index, ok := rank[category.panelID]
		if !ok {
			t.Fatalf("category %q names an unregistered panel %q", category.name, category.panelID)
		}
		if index < previous {
			t.Fatalf("category %q (panel %d) comes after panel %d", category.name, index, previous)
		}
		previous = index
		seen++
	}
	if seen != len(m.ws.PanelIDs()) {
		t.Fatalf("%d panel pages for %d panels", seen, len(m.ws.PanelIDs()))
	}
}

// The Weather page is assembled from two places: the geocoder the form owns,
// and the fields the panel declares. This pins the order they appear in, so
// the lookup button still sits under the query it uses.
func TestWeatherCategoryMergesFormAndPanelFields(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")

	var labels []string
	for _, field := range form.currentFields() {
		labels = append(labels, field.label)
	}
	want := []string{
		"enabled", "gauge style", "spark style", // added to every panel's page
		"city or ZIP", "Look up coordinates", // the form's geocoder
		"live weather", "latitude", "longitude", "location", "fahrenheit", "wind mph",
	}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("Weather page rows =\n%v\nwant\n%v", labels, want)
	}
	// The booleans that default to true show ticked on a fresh config, where
	// the keys are absent: an unset key means the panel's default applies.
	for _, label := range []string{"live weather", "fahrenheit", "wind mph"} {
		if field := selectNewsField(t, form, label); field.flag == nil || !*field.flag {
			t.Fatalf("%q defaulted to off", label)
		}
	}
}
