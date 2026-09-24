package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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

// gpuForm opens a settings form with the GPU panel registered, which draws both
// a gauge and a sparkline, so both metric-style rows appear on its page.
func gpuForm(t *testing.T) *settingsForm {
	t.Helper()
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.GPU())
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

func TestSettingsWorkspaceUsesTwoPanesAndCancelsInlineEdit(t *testing.T) {
	form := weatherForm(t)
	renderer := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{Density: tideui.Dense})
	wide := form.RenderWorkspace(renderer, 120, 30)
	if !strings.Contains(wide, "TideDeck › Settings") || !strings.Contains(wide, "Editor") {
		t.Fatalf("settings workspace lost pane chrome: %q", wide)
	}

	openCategory(t, form, "Weather")
	field := selectNewsField(t, form, "location")
	original := *field.text
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("changed")})
	if !form.dirty {
		t.Fatal("inline edit did not mark settings dirty")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if *field.text != original {
		t.Fatalf("escape kept cancelled edit: got %q want %q", *field.text, original)
	}

	narrow := form.RenderWorkspace(renderer, 70, 18)
	if !strings.Contains(narrow, "Editor") {
		t.Fatalf("narrow settings view did not expose editor tab: %q", narrow)
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
	openCategory(t, form, "Weather")
	field := form.currentField()
	if field == nil || field.kind != fieldPanel || field.panel != "weather" {
		t.Fatalf("first Weather field = %+v, want the panel toggle", field)
	}
	if !form.panelVisible("weather") {
		t.Fatal("panel should start visible")
	}
	// Hiding is a pending edit: the tick flips at once, but the workspace is
	// left alone until the settings are saved. Toggling used to commit
	// immediately, so cancelling the screen said "settings unchanged" and left
	// the panel hidden anyway.
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // hide
	if form.panelVisible("weather") {
		t.Fatal("tick should be off after hiding")
	}
	if ws.Hidden("weather") {
		t.Fatal("the workspace changed before the settings were saved")
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // show
	if !form.panelVisible("weather") {
		t.Fatal("second enter did not turn the tick back on")
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
	form.Open(config{GaugeStyle: "segment"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // Live data -> gauge style
	field := form.currentField()
	if field == nil || field.kind != fieldChoice || field.choice == nil {
		t.Fatalf("field = %+v, want gauge style choice", field)
	}
	if got := form.value(*field); got != "segment" {
		t.Fatalf("gauge value = %q, want segment", got)
	}
	// There are more gauge styles than are worth cycling blind, so enter opens
	// a picker. The arrows still step the value in place.
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if form.state.gauge == "segment" {
		t.Fatal("gauge style did not step")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if form.SavedConfig().GaugeStyle == "segment" {
		t.Fatal("gauge style did not save")
	}
}

func TestSettingsChoiceArrowKeys(t *testing.T) {
	form := newSettingsForm()
	form.Open(config{GaugeStyle: "segment"})
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open General
	form.Update(tea.KeyMsg{Type: tea.KeyDown})  // gauge style
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := form.value(*form.currentField()); got != "smooth" {
		t.Fatalf("right arrow -> %q, want smooth", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := form.value(*form.currentField()); got != "segment" {
		t.Fatalf("left arrow -> %q, want segment", got)
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
	form := gpuForm(t)
	openCategory(t, form, "GPU")
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // enabled -> gauge
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // gauge -> spark
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
	form := gpuForm(t)
	openCategory(t, form, "GPU")
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // enabled -> gauge style
	field := form.currentField()
	if field == nil || field.kind != fieldChoice {
		t.Fatalf("field = %+v, want the gauge choice", field)
	}
	if got := form.value(*field); got != "default" {
		t.Fatalf("initial panel gauge = %q, want default", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyRight}) // step
	if got := form.value(*form.currentField()); got == "default" {
		t.Fatal("panel gauge choice did not step")
	}
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save action = %v", action)
	}
	if form.SavedConfig().PanelGauges["gpu"] == "" {
		t.Fatalf("panel gauge not saved: %+v", form.SavedConfig().PanelGauges)
	}
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
	openCategory(t, form, "Weather")
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

func TestGlyphModeAndPanelOverride(t *testing.T) {
	if glyphModeOrDefault("unknown") != glyphModeOn {
		t.Fatal("unknown glyph mode did not fall back to on")
	}
	if !glyphsShown(config{GlyphMode: glyphModeOn}, "weather") {
		t.Fatal("on mode hid a glyph")
	}
	if glyphsShown(config{GlyphMode: glyphModeOff}, "weather") {
		t.Fatal("off mode showed a glyph")
	}
	if glyphsShown(config{GlyphMode: glyphModePerPane, PanelGlyphs: map[string]bool{"weather": false}}, "weather") {
		t.Fatal("per-panel override did not hide weather glyph")
	}
	if !glyphsShown(config{GlyphMode: glyphModePerPane}, "weather") {
		t.Fatal("per-panel default should show glyph")
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
	// The weather panel draws neither a gauge nor a sparkline, so its page
	// shows only the visibility toggle the screen adds to every panel.
	want := []string{
		"enabled",
		"city or ZIP", "Look up coordinates", // the form's geocoder
		"live weather", "latitude", "longitude", "location", "fahrenheit", "wind mph",
		"show glyph",
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

func TestPluginInstallFieldLooksLikeAnInput(t *testing.T) {
	if got := inputView("", "git URL or local path", 40); got != "[ git URL or local path ]" {
		t.Fatalf("empty input = %q, want the placeholder boxed", got)
	}
	if got := inputView("~/Projects/thing", "x", 40); got != "[ ~/Projects/thing ]" {
		t.Fatalf("filled input = %q", got)
	}
	// A long value is truncated so the label keeps its room.
	if got := inputView("https://github.com/you/tidedeck-plugins#my-panel", "x", 20); ansi.StringWidth(got) > 20 {
		t.Fatalf("long input = %q, exceeds its budget", got)
	}

	// The Plugins page shows that box rather than a bare blank value.
	form := newSettingsForm()
	form.Open(config{})
	openCategory(t, form, "Plugins")
	renderer := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{Density: tideui.Dense})
	rendered := ansi.Strip(strings.Join(form.renderFields(renderer, 80, 20), "\n"))
	if !strings.Contains(rendered, "[ git URL[#dir] or local path ]") {
		t.Fatalf("Plugins page has no input box:\n%s", rendered)
	}
}

// A panel's page offers only the metric styles it actually draws: no gauge for
// a sparkline panel, no spark for a gauge panel, and neither for a list.
func TestMetricStylesOnlyForPanelsThatUseThem(t *testing.T) {
	cases := []struct {
		panel        dash.Panel
		category     string
		gauge, spark bool
	}{
		{panels.Clock(), "Clock", true, false},
		{panels.Network(), "Network", false, true},
		{panels.Storage(), "Storage", true, false},
		{panels.News(), "News", false, false},
	}
	for _, c := range cases {
		ws := tideui.NewWorkspace()
		deck := dash.New()
		deck.Register(c.panel)
		deck.Attach(ws)

		form := newSettingsForm()
		form.SetWorkspace(ws)
		form.SetDeck(deck)
		form.Open(defaultConfig())
		openCategory(t, form, c.category)

		labels := map[string]bool{}
		for _, field := range form.currentFields() {
			labels[field.label] = true
		}
		if labels["gauge style"] != c.gauge || labels["spark style"] != c.spark {
			t.Fatalf("%s: gauge=%v spark=%v, got %v", c.category, c.gauge, c.spark, labels)
		}
	}
}

// Cancelling must leave the workspace exactly as it was. Toggling a panel used
// to call TogglePanel straight away, which commits, so esc reported "settings
// unchanged" and left the panel hidden regardless.
func TestSettingsCancelRestoresPanelVisibility(t *testing.T) {
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

	openCategory(t, form, "Weather")
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // hide the panel
	if form.panelVisible("weather") {
		t.Fatal("the tick did not turn off")
	}

	// esc from the field list goes back to the categories; a second closes.
	form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if action := form.Update(tea.KeyMsg{Type: tea.KeyEsc}); action != settingsCancelled {
		t.Fatalf("esc = %v, want settingsCancelled", action)
	}
	if ws.Hidden("weather") {
		t.Fatal("cancelling left the panel hidden")
	}
}

func TestSettingsSaveAppliesPanelVisibility(t *testing.T) {
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

	openCategory(t, form, "Weather")
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // hide
	if action := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != settingsSaved {
		t.Fatalf("save = %v", action)
	}
	if !ws.Hidden("weather") {
		t.Fatal("saving did not hide the panel")
	}
}

// Icons is called from the style preview on every keystroke, including before
// the form has ever been opened, so it has to tolerate a nil state like every
// other accessor here.
func TestSettingsAccessorsSurviveNilState(t *testing.T) {
	var form settingsForm
	if form.Icons() == "" {
		t.Fatal("Icons returned nothing for a nil state")
	}
	_ = form.GlyphMode()
	_ = form.ClockFont()
	_ = form.GaugeStyle()
	_ = form.SparkStyle()
}

// A message that is not a failure must not be drawn as one.
func TestSettingsNoticeTones(t *testing.T) {
	form := newSettingsForm()
	form.fail("broken")
	if form.tone != noticeError {
		t.Fatalf("fail tone = %v", form.tone)
	}
	form.working("installing…")
	if form.tone != noticeProgress {
		t.Fatalf("working tone = %v", form.tone)
	}
	form.report("found Portland, OR")
	if form.tone != noticeGood {
		t.Fatalf("report tone = %v", form.tone)
	}
	form.clearNotice()
	if form.problem != "" {
		t.Fatalf("problem = %q after clearing", form.problem)
	}
}

// Both list panes have to fit the rows they were given: the section headers and
// the overflow markers come out of the same budget as the rows, and used to be
// appended after the window had already claimed all of it.
func TestSettingsListsFitTheirBudget(t *testing.T) {
	form := gpuForm(t)
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
	openCategory(t, form, "GPU")
	for _, rows := range []int{1, 2, 3, 5, 8, 40} {
		if got := len(form.renderCategories(r, 30, rows)); got > rows {
			t.Errorf("categories at %d rows rendered %d lines", rows, got)
		}
		if got := len(form.renderFields(r, 30, rows)); got > rows {
			t.Errorf("fields at %d rows rendered %d lines", rows, got)
		}
	}
}

// A plugin's description has to reach the row that shows it. The manifest
// format has always had the field; it was parsed and discarded, so a plugin
// could explain itself and never be heard.
func TestSettingsShowsPluginDescriptions(t *testing.T) {
	manifest := dash.Manifest{
		SchemaVersion: dash.ManifestSchemaVersion,
		ID:            "tidedeck.example",
		Name:          "Example",
		Kinds:         []string{dash.KindPanel},
		EntryPoints:   map[string][]string{dash.KindPanel: {"./render.sh"}},
		Panel: dash.PanelManifest{
			DisplayName: "Example",
			Schema: []dash.SchemaField{{
				Key: "account", Type: "string", Label: "account",
				Description: "Blank shows every account.",
			}},
		},
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(dash.Exec(manifest))
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)

	openCategory(t, form, "Example")
	var found *formField
	for i := range form.currentFields() {
		if form.currentFields()[i].label == "account" {
			found = &form.currentFields()[i]
			break
		}
	}
	if found == nil {
		t.Fatal("no account field on the plugin's page")
	}
	if found.description != "Blank shows every account." {
		t.Fatalf("description = %q, want it carried from the manifest", found.description)
	}
}

// A numeric field gets a numeric control, so a bad value is caught as it is
// typed rather than failing the whole save with one banner.
func TestSettingsNumberFieldValidatesAsTyped(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	for form.currentField() != nil && form.currentField().label != "latitude" {
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	field := form.currentField()
	if field == nil || field.kind != fieldNumber {
		t.Fatalf("latitude = %+v, want a numeric field", field)
	}

	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, ch := range "abc" {
		form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if form.editor == nil || form.editor.Err() == nil {
		t.Fatal("letters in a numeric field were not reported")
	}
	note, tone := form.fieldNote(*form.currentField())
	if tone != noticeError || note == "" {
		t.Fatalf("note = %q tone = %v, want the complaint on the row", note, tone)
	}

	// The bad value is kept so it can be corrected, not silently dropped.
	if got := form.editor.Value(); got != "abc" {
		t.Fatalf("value = %q, want it kept", got)
	}
}

// The gauge list is long enough now that enter opens a picker, and the picker
// draws each family as its glyphs, so the new shapes are seen, not just named.
func TestSettingsGaugeChoiceOpensAPickerOfShapes(t *testing.T) {
	form := gpuForm(t)
	openCategory(t, form, "GPU")
	form.Update(tea.KeyMsg{Type: tea.KeyDown}) // gauge style
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !form.editing {
		t.Fatal("the gauge choice did not open a picker")
	}
	frame := ansi.Strip(form.RenderWorkspace(tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{}), 100, 40))
	for _, want := range []string{"smooth", "line", "heat", "━"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("gauge picker frame is missing %q:\n%s", want, frame)
		}
	}
}

// The General page is the one you land on, and every field on it is written by
// hand rather than declared by a panel. Those fields were the last to get
// descriptions, and without them the page that gets seen most explained least.
func TestSettingsGeneralFieldsAreDescribed(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "General")
	for _, field := range form.currentFields() {
		if field.kind == fieldAction {
			continue
		}
		if field.description == "" {
			t.Errorf("General field %q has no description", field.label)
		}
	}
}

func TestWrapTextBreaksOnWords(t *testing.T) {
	lines := wrapText("Fetch from real sources. Off shows sample data instead.", 24, 2)
	if len(lines) != 2 {
		t.Fatalf("lines = %q, want 2", lines)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 24 {
			t.Errorf("line %q is wider than 24", line)
		}
		if strings.HasPrefix(line, " ") || strings.HasSuffix(line, " ") {
			t.Errorf("line %q has stray padding", line)
		}
	}

	// More than fits is marked as cut short rather than ending mid-word.
	long := wrapText(strings.Repeat("word ", 40), 20, 2)
	if len(long) != 2 {
		t.Fatalf("lines = %d, want the limit respected", len(long))
	}
	if !strings.HasSuffix(long[1], "…") {
		t.Errorf("last line = %q, want it marked as elided", long[1])
	}
}

// Every row in a settings pane has to sit on the pane's own background. The
// rows are soft rows, which default to the lifted modal surface; drawn into a
// pane that way they come out a different shade from the blank space around
// them and the whole screen looks banded.
func TestSettingsRowsSitOnThePaneBackground(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	pattern := regexp.MustCompile(`48;2;(\d+);(\d+);(\d+)`)
	// Read colours back out of rendered text so the comparison runs through
	// the same conversion the rows do.
	swatch := func(c lipgloss.Color) string {
		m := pattern.FindStringSubmatch(lipgloss.NewStyle().Background(c).Render(" "))
		return m[1] + "," + m[2] + "," + m[3]
	}
	seen := func(text string) map[string]bool {
		found := map[string]bool{}
		for _, m := range pattern.FindAllStringSubmatch(text, -1) {
			found[m[1]+","+m[2]+","+m[3]] = true
		}
		return found
	}

	form := weatherForm(t)
	openCategory(t, form, "General")

	for _, theme := range tideui.BuiltinThemes[:4] {
		r := tideui.NewRenderer(theme, tideui.StyleOptions{})
		body := strings.Join(form.renderFields(r, 60, 12), "\n") + "\n" +
			strings.Join(form.renderCategories(r, 30, 12), "\n")

		pane := swatch(theme.Bg)
		modal := swatch(r.Styles.Overlay.GetBackground().(lipgloss.Color))
		found := seen(body)

		if !found[pane] {
			t.Errorf("%s: nothing drawn on the pane background %s (saw %v)",
				theme.Name, pane, found)
		}
		if modal != pane && found[modal] {
			t.Errorf("%s: rows still drawn on the modal surface %s", theme.Name, modal)
		}
	}
}

// A panel's page says what it is for before any row is read: the title is green
// when the panel is on the dashboard and red when it is not. Colour alone would
// not say it — a red title and a green one are the same title to anyone who
// cannot tell them apart, and identical under PlainUI — so the rule under the
// header carries the word too.
func TestSettingsHeaderShowsPanelState(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	theme := tideui.BuiltinThemes[0]
	r := tideui.NewRenderer(theme, tideui.StyleOptions{})
	fg := regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`)
	swatch := func(c lipgloss.Color) string {
		m := fg.FindStringSubmatch(lipgloss.NewStyle().Foreground(c).Render("x"))
		return m[1] + "," + m[2] + "," + m[3]
	}
	titleColour := func(header string) string {
		m := fg.FindStringSubmatch(header)
		return m[1] + "," + m[2] + "," + m[3]
	}

	form := weatherForm(t)
	openCategory(t, form, "Weather")
	category := form.categories[form.category]

	// allColours reports every foreground in a rendered string.
	allColours := func(text string) map[string]bool {
		found := map[string]bool{}
		for _, m := range fg.FindAllStringSubmatch(text, -1) {
			found[m[1]+","+m[2]+","+m[3]] = true
		}
		return found
	}

	plain := swatch(r.Styles.Workspace.BodyFg)
	enabled := form.renderHeader(r, category, 50)
	green := swatch(r.Styles.Workspace.MetricGood)
	// The title reads the same on every page; only the rule carries the state.
	if got := titleColour(enabled); got != plain {
		t.Errorf("enabled title = %s, want the ordinary heading colour %s", got, plain)
	}
	if !strings.Contains(enabled, "enabled") {
		t.Error("the rule does not name the state")
	}
	// The rule has to agree with the title it sits under: one state, one colour.
	if got := allColours(strings.Split(enabled, "\n")[1]); len(got) != 1 || !got[green] {
		t.Errorf("enabled rule colours = %v, want only the good colour %s", keys(got), green)
	}

	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // turn the panel off
	disabled := form.renderHeader(r, form.categories[form.category], 50)
	red := swatch(r.Styles.Workspace.MetricBad)
	if got := titleColour(disabled); got != plain {
		t.Errorf("disabled title = %s, want the ordinary heading colour %s", got, plain)
	}
	if !strings.Contains(disabled, "disabled") {
		t.Error("the rule does not name the state")
	}
	if got := allColours(strings.Split(disabled, "\n")[1]); len(got) != 1 || !got[red] {
		t.Errorf("disabled rule colours = %v, want only the bad colour %s", keys(got), red)
	}

	// A page with nothing to enable keeps the ordinary title colour and an
	// unlabelled rule.
	openCategory(t, form, "General")
	general := form.renderHeader(r, form.categories[form.category], 50)
	if got := titleColour(general); got != plain {
		t.Errorf("General title = %s, want the same heading colour as every other page %s",
			got, plain)
	}
	if strings.Contains(general, "enabled") || strings.Contains(general, "disabled") {
		t.Error("General's rule claims a state it does not have")
	}
}

// The header is two lines — the title and the rule beneath it — and the editor
// pane budgets its rows around that.
func TestSettingsHeaderIsTitleAndRule(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
	header := form.renderHeader(r, form.categories[form.category], 50)
	if got := len(strings.Split(header, "\n")); got != 2 {
		t.Fatalf("header is %d lines, want 2", got)
	}
}

// The settings screen is a full takeover: it must be exactly the size of the
// terminal, whatever page is open and however short the window.
func TestSettingsScreenFitsTheTerminal(t *testing.T) {
	form := weatherForm(t)
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})

	for _, page := range []string{"General", "Weather", "Plugins"} {
		openCategory(t, form, page)
		for _, size := range [][2]int{{110, 24}, {110, 12}, {110, 8}, {88, 20}, {60, 14}} {
			width, height := size[0], size[1]
			view := form.RenderWorkspace(r, width, height)
			lines := strings.Split(view, "\n")
			if len(lines) != height {
				t.Errorf("%s at %dx%d: %d lines, want %d", page, width, height, len(lines), height)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got > width {
					t.Errorf("%s at %dx%d: line %d is %d cells", page, width, height, i, got)
				}
			}
		}
	}
}

// keys lists a set, for readable failure messages.
func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The note under the selected row is followed by a blank row, so the sentence
// reads as belonging to the row above it rather than running into the one below.
func TestSettingsNoteIsFollowedByABlankRow(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})

	field := form.currentField()
	if field == nil || field.description == "" {
		t.Fatal("the first Weather field has no description to sit under")
	}

	lines := form.renderFields(r, 60, 14)
	noteAt := -1
	for i, line := range lines {
		if strings.Contains(line, "Show this panel on the dashboard") {
			noteAt = i
		}
	}
	if noteAt < 0 {
		t.Fatal("the description was not rendered")
	}
	// The description wraps, so walk past the rest of it.
	last := noteAt
	for last+1 < len(lines) && strings.Contains(lines[last+1], "save.") {
		last++
	}
	if last+1 >= len(lines) {
		t.Fatal("nothing follows the note")
	}
	if got := strings.TrimSpace(ansi.Strip(lines[last+1])); got != "" {
		t.Errorf("row after the note = %q, want it blank", got)
	}
}

// Pressing down while editing commits the value and moves to the next field,
// rather than being swallowed. A field you can only leave with a key you have
// to already know about is a trap.
func TestSettingsArrowLeavesTextFieldAndMoves(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	for form.currentField() != nil && form.currentField().label != "city or ZIP" {
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	before := form.cursor

	form.Update(tea.KeyMsg{Type: tea.KeyEnter}) // start editing
	if !form.editing {
		t.Fatal("enter did not open an edit")
	}
	for _, ch := range "Portland" {
		form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	form.Update(tea.KeyMsg{Type: tea.KeyDown})
	if form.editing {
		t.Fatal("down left the field editing")
	}
	if form.cursor != before+1 {
		t.Errorf("cursor = %d, want it moved to %d", form.cursor, before+1)
	}
	if form.state.place != "Portland" {
		t.Errorf("place = %q, want the edit kept when moving off", form.state.place)
	}
	if !form.dirty {
		t.Error("the form is not marked dirty after the edit")
	}
}

// esc still discards, which is the whole point of having both keys.
func TestSettingsEscStillReverts(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	for form.currentField() != nil && form.currentField().label != "city or ZIP" {
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, ch := range "Portland" {
		form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if form.editing {
		t.Fatal("esc left the field editing")
	}
	if form.state.place != "" {
		t.Errorf("place = %q, want esc to discard the edit", form.state.place)
	}
}

// Every glyph the host supplies for its own panes is a colour emoji, which a terminal
// draws two cells wide. A one-cell glyph is a monochrome symbol from a text font: the
// mail pane declared ✉ and read as a grey mark beside the calendar and the weather.
// The check is the width, because that is what tells the two apart.
func TestPanelGlyphsAreColourEmoji(t *testing.T) {
	ids := []string{
		"agenda", "weather", "system", "gpu", "updates", "clock", "git", "news",
		"network", "storage", "services", "tasks", "notes", "markets", "calculator",
	}
	for _, id := range ids {
		glyph := panelGlyph(id)
		if glyph == "" {
			t.Fatalf("panel %q has no glyph at all", id)
		}
		if width := ansi.StringWidth(glyph); width != 2 {
			t.Errorf("panel %q glyph %q is %d cells: a colour glyph is two cells wide, one cell is a monochrome symbol",
				id, glyph, width)
		}
	}
}

// The settings screen draws the glyph a panel declares. It used the host's
// hand-written map for every panel, and no plugin is in that map, so a plugin's
// page wore the generic plug however its manifest was written - the mail page
// showed a plug while its pane header showed the envelope.
func TestSettingsIconUsesThePluginsGlyph(t *testing.T) {
	manifest := dash.Manifest{
		SchemaVersion: dash.ManifestSchemaVersion,
		ID:            "tidedeck.mail",
		Name:          "Mail",
		Kinds:         []string{dash.KindPanel},
		EntryPoints:   map[string][]string{dash.KindPanel: {"./render.sh"}},
		Panel: dash.PanelManifest{
			DisplayName: "Mail",
			Glyph:       "📧",
			Schema:      []dash.SchemaField{{Key: "account", Type: "string", Label: "account"}},
		},
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(dash.Exec(manifest))
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)

	icon := form.settingsIcon(settingsCategory{name: "Mail", panelID: "tidedeck.mail"})
	if icon != "📧" {
		t.Fatalf("settings icon = %q, want the manifest's envelope", icon)
	}
	// A host panel declares no glyph, so the map beside this still supplies one.
	if host := form.settingsIcon(settingsCategory{name: "Clock", panelID: "clock"}); host != "🕒" {
		t.Fatalf("host icon = %q, want the hand-written clock", host)
	}
}

// The mail plugin declares the glyph its pane header draws, and it has to be the colour
// one for the same reason: it sits in a row of colour emoji.
func TestMailPluginDeclaresAColourGlyph(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contrib", "mail", "manifest.json"))
	if err != nil {
		t.Fatalf("reading the mail plugin's manifest: %v", err)
	}
	var manifest struct {
		Panel struct {
			Glyph string `json:"glyph"`
		} `json:"panel"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parsing the mail plugin's manifest: %v", err)
	}
	if width := ansi.StringWidth(manifest.Panel.Glyph); width != 2 {
		t.Fatalf("the mail plugin declares glyph %q, %d cells wide; a colour emoji is two",
			manifest.Panel.Glyph, width)
	}
}

// And the settings screen has to draw that envelope, not merely know it exists:
// the glyph the manifest declares is the one the category list shows.
func TestSettingsDrawsTheMailPluginsEnvelope(t *testing.T) {
	manifest, err := dash.LoadManifest(filepath.Join("..", "..", "contrib", "mail"))
	if err != nil {
		t.Fatalf("loading the mail plugin's manifest: %v", err)
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(dash.Exec(manifest))
	deck.Attach(ws)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)

	r := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
	drawn := strings.Join(form.renderCategories(r, 40, 80), "\n")
	if !strings.Contains(drawn, "📧") {
		t.Fatalf("the settings list does not draw the mail envelope:\n%s", ansi.Strip(drawn))
	}
}
