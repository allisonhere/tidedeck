package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/spinner"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/form"
	"github.com/allisonhere/tideui/provider"
)

type fieldKind int

const (
	fieldBool fieldKind = iota
	fieldText
	fieldAction
	fieldPanel
	fieldChoice
	fieldGlyph
	fieldNumber
)

const (
	glyphModeOn      = "on"
	glyphModeOff     = "off"
	glyphModePerPane = "per_panel"
)

const settingsMaxWidth = 128

type formField struct {
	label string
	// key is the configuration key a panel-declared row came from. Rows the
	// form writes by hand leave it empty. It is what lets a row be found again
	// when the panel reports new options for it.
	key    string
	kind   fieldKind
	flag   *bool
	text   *string
	action func() settingsAction
	// button is the work an action starts, for one whose work runs in the
	// background: it refuses to fire again while that work is under way, and
	// draws a spinner on the row that started it.
	button *form.Button
	// path is dash.PathFile or dash.PathDir for a setting that names one, which
	// o browses for; list says a pick is added to the value, not put in its
	// place.
	path         string
	list         bool
	panel        string   // panel id, for fieldPanel
	choice       *string  // current value, for fieldChoice
	options      []string // selectable values, for fieldChoice
	gaugePreview bool     // fieldChoice shows a sample gauge instead of its name
	sparkPreview bool     // fieldChoice shows a sample sparkline instead of its name
	// choiceMap/choiceKey back a choice stored in a map (e.g. per-panel
	// gauges) instead of a single string.
	choiceMap map[string]string
	choiceKey string
	// summary renders a long text value as something that fits a row. The raw
	// text is still what gets edited; this is only what the row shows when the
	// field is not being edited.
	summary func(string) string
	// input draws a text field as an input box, with a placeholder while it is
	// empty, so a row you are meant to type into reads as one rather than as a
	// blank value.
	input       bool
	placeholder string
	// description explains the setting in one sentence, shown under the row
	// while it is selected. A manifest has always been able to supply one;
	// until now it was parsed and dropped.
	description string
	// validate reports a value the field cannot use, as it is typed.
	validate func(string) error
	// unit, minimum, maximum and step describe a fieldNumber.
	unit             string
	minimum, maximum float64
	step             float64
}

// choiceValue reads the field's current choice from its pointer or map.
func (f formField) choiceValue() string {
	if f.choice != nil {
		return *f.choice
	}
	if f.choiceMap != nil {
		return f.choiceMap[f.choiceKey]
	}
	return ""
}

// setChoice writes the field's choice to its pointer or map.
func (f *formField) setChoice(value string) {
	if f.choice != nil {
		*f.choice = value
		return
	}
	if f.choiceMap != nil {
		f.choiceMap[f.choiceKey] = value
	}
}

// noticeTone says how the editor's one-line message should read. All three
// kinds used to share one string rendered in the error colour, so "found
// Portland, OR - ctrl+s to apply" and "installing foo…" both looked like
// something had gone wrong.
type noticeTone int

const (
	noticeError noticeTone = iota
	noticeProgress
	noticeGood
)

// settingsAction reports how a settings update ended.
type settingsAction int

const (
	settingsNone settingsAction = iota
	settingsSaved
	settingsCancelled
)

// formState is the editable view of a config. Numbers and lists are held as
// text so they can be typed directly, and parsed back into a config on save.
type formState struct {
	live bool
	// idleDim fades the focused panel's frame after a quiet spell. One switch
	// for the whole workspace, not a per-panel override.
	idleDim bool
	// place is the city or ZIP to geocode. It is the only weather value the
	// form still owns, because it is a query rather than a setting: nothing
	// stores it, and the panel has no use for it.
	place     string
	gauge     string
	spark     string
	clockFont string
	icons     string
	glyphMode string
	// doc is the document the edited config came from, carried through the
	// form so saving preserves keys the form never shows.
	doc dash.Values
	// panelText and panelFlag hold the edit buffer for each panel-owned
	// setting, keyed by its configuration key. A panel declares its fields
	// rather than the form hard-coding them, so these are allocated from the
	// schema when the form opens rather than being struct fields like those
	// above.
	panelText   map[string]*string
	panelFlag   map[string]*bool
	panelGauges map[string]string
	panelSparks map[string]string
	panelGlyphs map[string]bool
	// panelShown is the edit buffer for panel visibility. It exists so that
	// toggling a panel is a pending edit like every other row: it used to
	// call Workspace.TogglePanel straight away, which commits, so cancelling
	// the settings screen reported "settings unchanged" and left the panel
	// off anyway.
	panelShown  map[string]bool
	feeds       string // custom URLs only; catalogue URLs live in feedPresets
	feedPresets []bool // one per provider.NewsSources(), same order
	// pluginSource is the plugin install field. It is transient: the value is
	// acted on, not stored, so it is not a configuration key.
	pluginSource string
}

func formFromConfig(cfg config) formState {
	feedPresets, customFeeds := splitFeeds(cfg.Feeds)
	return formState{
		doc:         cfg.doc,
		live:        cfg.Live,
		idleDim:     cfg.IdleDim,
		place:       cfg.doc.String(weatherLocationKey),
		gauge:       gaugeOrDefault(cfg.GaugeStyle),
		spark:       sparkOrDefault(cfg.SparkStyle),
		clockFont:   clockFontOrDefault(cfg.ClockFont),
		icons:       iconStyleOrDefault(cfg.Icons),
		glyphMode:   glyphModeOrDefault(cfg.GlyphMode),
		panelGauges: copyStringMap(cfg.PanelGauges),
		panelSparks: copyStringMap(cfg.PanelSparks),
		panelGlyphs: copyBoolMap(cfg.PanelGlyphs),
		panelShown:  map[string]bool{},
		feeds:       customFeeds,
		feedPresets: feedPresets,
	}
}

func (s formState) toConfig(deck *dash.Deck) (config, error) {
	cfg := config{
		Live:        s.live,
		IdleDim:     s.idleDim,
		GaugeStyle:  gaugeOrDefault(s.gauge),
		SparkStyle:  sparkOrDefault(s.spark),
		ClockFont:   clockFontOrDefault(s.clockFont),
		Icons:       iconStyleOrDefault(s.icons),
		GlyphMode:   glyphModeOrDefault(s.glyphMode),
		PanelGauges: panelOverrides(s.panelGauges),
		PanelSparks: panelOverrides(s.panelSparks),
		PanelGlyphs: panelGlyphOverrides(s.panelGlyphs),
		Feeds:       joinFeeds(s.feedPresets, s.feeds),
	}
	document, err := s.applyPanelFields(s.doc, deck)
	if err != nil {
		return config{}, err
	}
	return cfg.withDoc(document), nil
}

// iconStyleNames lists the selectable icon styles as strings.
func iconStyleNames() []string {
	styles := tideui.IconStyles()
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = string(style)
	}
	return names
}

// iconStyleOrDefault falls back to plain icons, which render in any font.
func iconStyleOrDefault(style string) string {
	for _, known := range tideui.IconStyles() {
		if string(known) == style {
			return style
		}
	}
	return string(tideui.IconEmoji)
}

// gaugeStyleNames lists the selectable gauge styles as strings.
func gaugeStyleNames() []string {
	styles := tideui.GaugeStyles()
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = string(style)
	}
	return names
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func glyphModeNames() []string { return []string{glyphModeOn, glyphModeOff, glyphModePerPane} }

func glyphModeOrDefault(mode string) string {
	for _, known := range glyphModeNames() {
		if mode == known {
			return mode
		}
	}
	return glyphModeOn
}

func panelGlyphOverrides(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool)
	for id, enabled := range in {
		if !enabled {
			out[id] = false
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// panelOverrides drops entries that follow the workspace default so the saved
// config only stores real per-panel choices.
func panelOverrides(in map[string]string) map[string]string {
	out := map[string]string{}
	for id, style := range in {
		if style == "" || style == "default" {
			continue
		}
		out[id] = style
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// gaugeOrDefault falls back to the segment family when no gauge style is set.
func gaugeOrDefault(style string) string {
	for _, known := range tideui.GaugeStyles() {
		if style == string(known) {
			return style
		}
	}
	return string(tideui.GaugeBlock)
}

// sparkStyleNames lists the selectable sparkline styles as strings.
func sparkStyleNames() []string {
	styles := tideui.SparklineStyles()
	names := make([]string, len(styles))
	for i, style := range styles {
		names[i] = string(style)
	}
	return names
}

// sparkOrDefault falls back to the blocks style when no sparkline style is set.
func sparkOrDefault(style string) string {
	for _, known := range tideui.SparklineStyles() {
		if style == string(known) {
			return style
		}
	}
	return string(tideui.SparkBlocks)
}

// clockFontNames lists the selectable large-clock fonts as strings.
func clockFontNames() []string {
	fonts := tideui.ClockFonts()
	names := make([]string, len(fonts))
	for i, font := range fonts {
		names[i] = string(font)
	}
	return names
}

// clockFontOrDefault falls back to the dash font when none is set.
func clockFontOrDefault(font string) string {
	for _, known := range tideui.ClockFonts() {
		if font == string(known) {
			return font
		}
	}
	return string(tideui.ClockFontDash)
}

func parseOptionalFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

// settingsCategory groups fields under a heading so the panel stays navigable
// as the number of settings grows. panelID names the dashboard panel the
// category configures; when set, an enable/disable toggle is added to the top
// of the page so the panel can be shown or hidden from its own settings.
type settingsCategory struct {
	name    string
	fields  []formField
	panelID string
	// gauge and spark say whether the panel draws each metric style, so its
	// page offers only the styles it uses.
	gauge bool
	spark bool
}

type settingsView int

const (
	viewCategories settingsView = iota
	viewFields
)

type settingsPane int

const (
	settingsNav settingsPane = iota
	settingsEditor
)

// settingsForm owns the Settings workspace. The model remains schema-driven;
// rendering presents it as a persistent navigation/editor layout.
type settingsForm struct {
	opened     bool
	state      *formState
	cfg        config // config produced by the last successful save
	categories []settingsCategory
	view       settingsView
	focus      settingsPane
	category   int
	cursor     int
	editing    bool
	caret      int
	editBefore string
	problem    string
	tone       noticeTone
	dirty      bool
	// editor is the control driving the selected row. It is created when a row
	// is activated and discarded when the edit ends, so only one row holds
	// state at a time and a control never outlives the field it edits.
	editor  form.Control
	editKey string

	// deck supplies the settings of panels that own their own. It may be nil,
	// in which case only the hand-written categories are shown.
	deck *dash.Deck

	// ws backs the per-category panel toggles, which change live visibility
	// rather than a config value. It is nil when the form runs without a
	// workspace (as in unit tests), which simply omits those fields.
	ws *tideui.Workspace

	pendingLookup string
	lookingUp     bool
	// lookupButton and pluginButton hold the running state of the work those
	// actions start; busyRow is the label of the row that started the running
	// plugin operation, which is the one that draws the spinner.
	lookupButton, pluginButton *form.Button
	busyRow                    string
	// picker is the path picker, while one is open over the rows.
	picker *pathPicker
	// search is the / search across every page, while it is open.
	search *settingsSearch

	// pendingPlugin is a plugin install, update or removal the model runs off
	// the UI goroutine, the way pendingLookup is a geocoding request.
	pendingPlugin pluginOp
}

// pluginOp is a queued plugin operation. value is the source for an install,
// or the plugin id for an update or removal.
type pluginOp struct {
	kind  string // "install", "update" or "remove"
	value string
}

// SavedConfig returns the config produced by the most recent save.
func (s settingsForm) SavedConfig() config { return s.cfg }

// GaugeStyle returns the gauge style currently selected in the form, so the
// caller can preview it live before it is saved.
func (s settingsForm) GaugeStyle() string {
	if s.state == nil {
		return string(tideui.GaugeBlock)
	}
	return gaugeOrDefault(s.state.gauge)
}

// PanelGaugeStyle returns a panel's chosen gauge override, or "" when it
// follows the workspace style.
func (s settingsForm) PanelGaugeStyle(id string) string {
	if s.state == nil {
		return ""
	}
	return s.state.panelGauges[id]
}

// SparkStyle returns the sparkline style currently selected in the form.
func (s settingsForm) SparkStyle() string {
	if s.state == nil {
		return string(tideui.SparkBlocks)
	}
	return sparkOrDefault(s.state.spark)
}

// PanelSparkStyle returns a panel's chosen sparkline override, or "" when it
// follows the workspace style.
func (s settingsForm) PanelSparkStyle(id string) string {
	if s.state == nil {
		return ""
	}
	return s.state.panelSparks[id]
}

// Icons reports the live icon style, so the preview updates while the setting
// is being changed. Like every other accessor here it has to tolerate a nil
// state: main's style preview calls it on every keystroke, including before
// the form has ever been opened.
func (s settingsForm) Icons() string {
	if s.state == nil {
		return iconStyleOrDefault("")
	}
	return iconStyleOrDefault(s.state.icons)
}

func (s settingsForm) GlyphMode() string {
	if s.state == nil {
		return glyphModeOn
	}
	return glyphModeOrDefault(s.state.glyphMode)
}

func (s settingsForm) PanelGlyphEnabled(id string) bool {
	if s.state == nil {
		return true
	}
	value, ok := s.state.panelGlyphs[id]
	if !ok {
		return true
	}
	return value
}

// ClockFont returns the large-clock font currently selected in the form.
func (s settingsForm) ClockFont() string {
	if s.state == nil {
		return string(tideui.ClockFontDash)
	}
	return clockFontOrDefault(s.state.clockFont)
}

func newSettingsForm() *settingsForm { return &settingsForm{} }

// lookup is the button for a coordinate lookup, and plugins the one shared by
// every plugin operation: installs, updates and removals all write the one
// plugins directory, so only one may run at a time. Both live as long as the
// form, so closing and reopening settings mid-install keeps the guard.
func (s *settingsForm) lookup() *form.Button {
	if s.lookupButton == nil {
		s.lookupButton = form.NewButton("Look up coordinates", nil)
	}
	return s.lookupButton
}

func (s *settingsForm) plugins() *form.Button {
	if s.pluginButton == nil {
		s.pluginButton = form.NewButton("plugins", nil)
	}
	return s.pluginButton
}

// LookupDone and PluginOpDone end the work their buttons started. The model
// calls them whether or not settings is open, since the work finishes either
// way.
func (s *settingsForm) LookupDone() {
	s.lookingUp = false
	s.lookup().Done()
}

func (s *settingsForm) PluginOpDone() {
	s.plugins().Done()
	s.busyRow = ""
}

// Working reports whether a button's work is under way, so the model can draw
// its spinner at animation speed rather than once a second.
func (s *settingsForm) Working() bool {
	return s.lookup().Running() || s.plugins().Running()
}

// TickButtons advances the spinners of any running work.
func (s *settingsForm) TickButtons() {
	s.lookup().Tick(spinner.TickMsg{})
	s.plugins().Tick(spinner.TickMsg{})
}

// SetWorkspace attaches the workspace whose panels the Panels category toggles.
func (s *settingsForm) SetWorkspace(ws *tideui.Workspace) { s.ws = ws }

// SetDeck gives the form the registry whose panels declare their own settings.
func (s *settingsForm) SetDeck(deck *dash.Deck) { s.deck = deck }

// Open loads cfg into the form and displays the category list.
func (s *settingsForm) Open(cfg config) {
	state := formFromConfig(cfg)
	s.state = &state
	s.loadPanelFields(cfg)
	s.opened = true
	s.picker = nil
	s.search = nil
	s.categories = s.buildCategories()
	s.view = viewCategories
	s.focus = settingsNav
	s.category = 0
	s.cursor = 0
	s.editing = false
	s.caret = 0
	s.clearNotice()
	s.dirty = false
}

// Opened reports whether the panel is displayed.
func (s settingsForm) Opened() bool { return s.opened }

func (s *settingsForm) buildCategories() []settingsCategory {
	categories := []settingsCategory{
		{name: "General", fields: []formField{
			{label: "Live data", kind: fieldBool, flag: &s.state.live,
				description: "Fetch from real sources. Off shows sample data instead."},
			{label: "gauge style", kind: fieldChoice, choice: &s.state.gauge,
				options: gaugeStyleNames(), gaugePreview: true,
				description: "How progress bars are drawn across the dashboard."},
			{label: "spark style", kind: fieldChoice, choice: &s.state.spark,
				options: sparkStyleNames(), sparkPreview: true,
				description: "How history sparklines are drawn across the dashboard."},
			{label: "clock font", kind: fieldChoice, choice: &s.state.clockFont,
				options:     clockFontNames(),
				description: "The face the large clock uses."},
			{label: "icons", kind: fieldChoice, choice: &s.state.icons,
				options:     iconStyleNames(),
				description: "Emoji, glyphs from a patched font, or plain text."},
			{label: "panel glyphs", kind: fieldChoice, choice: &s.state.glyphMode,
				options:     glyphModeNames(),
				description: "Show an icon in each panel's title. Per panel lets each one decide."},
			{label: "dim focus when idle", kind: fieldBool, flag: &s.state.idleDim,
				description: "Fade the focused panel's frame after 20s with no keys. Any key brings it straight back."},
		}},
		{name: "Plugins", fields: s.pluginFields()},
		{name: "Weather", panelID: "weather", fields: []formField{
			{label: "city or ZIP", kind: fieldText, text: &s.state.place,
				description: "A place to look up. Searching fills in the coordinates below.",
				placeholder: "search for a place"},
			{label: "Look up coordinates", kind: fieldAction, action: s.lookupCoordinates, button: s.lookup()},
		}},
		{name: "GPU", panelID: "gpu", fields: nil},
		{name: "News", panelID: "news", fields: s.newsFields()},
	}
	// Panels that own their settings declare them; the categories above are
	// the ones not yet migrated. Merging by panel id keeps a panel's page in
	// the position the list above gives it, so the order does not jump as
	// panels move over.
	categories = mergeCategories(categories, s.panelCategories())
	s.orderCategories(categories)

	// Put each panel's visibility toggle and metric styles at the top of its
	// own settings page.
	for i := range categories {
		id := categories[i].panelID
		if id == "" {
			continue
		}
		var prepend []formField
		if field := s.panelField(id); field != nil {
			prepend = append(prepend, *field)
		}
		// Only offer a metric style the panel actually draws, so a list panel
		// is not asked to pick a gauge it never renders.
		if categories[i].gauge {
			if gauge := s.panelGaugeField(id); gauge != nil {
				prepend = append(prepend, *gauge)
			}
		}
		if categories[i].spark {
			if spark := s.panelSparkField(id); spark != nil {
				prepend = append(prepend, *spark)
			}
		}
		if len(prepend) > 0 {
			categories[i].fields = append(prepend, categories[i].fields...)
		}
		categories[i].fields = append(categories[i].fields, s.panelGlyphField(id))
	}
	return categories
}

func (s *settingsForm) panelGlyphField(id string) formField {
	if s.state.panelGlyphs == nil {
		s.state.panelGlyphs = map[string]bool{}
	}
	if _, ok := s.state.panelGlyphs[id]; !ok {
		s.state.panelGlyphs[id] = true
	}
	return formField{label: "show glyph", kind: fieldGlyph, panel: id,
		description: "Show this panel's icon in its title bar."}
}

// panelGaugeField builds a per-panel gauge style choice. "default" follows the
// workspace setting; any other value overrides it for this panel.
func (s *settingsForm) panelGaugeField(id string) *formField {
	if s.ws == nil || id == "" {
		return nil
	}
	if _, ok := s.ws.Lookup(id); !ok {
		return nil
	}
	if s.state.panelGauges == nil {
		s.state.panelGauges = map[string]string{}
	}
	if s.state.panelGauges[id] == "" {
		s.state.panelGauges[id] = "default"
	}
	options := append([]string{"default"}, gaugeStyleNames()...)
	return &formField{
		label:        "gauge style",
		kind:         fieldChoice,
		choiceMap:    s.state.panelGauges,
		choiceKey:    id,
		options:      options,
		panel:        id,
		gaugePreview: true,
		description:  "Overrides the dashboard-wide gauge style for this panel only.",
	}
}

// panelSparkField builds a per-panel sparkline style choice. "default" follows
// the workspace setting; any other value overrides it for this panel.
func (s *settingsForm) panelSparkField(id string) *formField {
	if s.ws == nil || id == "" {
		return nil
	}
	if _, ok := s.ws.Lookup(id); !ok {
		return nil
	}
	if s.state.panelSparks == nil {
		s.state.panelSparks = map[string]string{}
	}
	if s.state.panelSparks[id] == "" {
		s.state.panelSparks[id] = "default"
	}
	options := append([]string{"default"}, sparkStyleNames()...)
	return &formField{
		label:        "spark style",
		kind:         fieldChoice,
		choiceMap:    s.state.panelSparks,
		choiceKey:    id,
		options:      options,
		panel:        id,
		sparkPreview: true,
		description:  "Overrides the dashboard-wide sparkline style for this panel only.",
	}
}

// panelField builds the enable/disable row for a panel, or nil when there is no
// workspace or the panel cannot be hidden.
// newsFields builds one tick row per curated source plus the free-text row for
// any other feed URL. Each row holds a pointer into the presets slice, so the
// slice is sized once here, before any pointer is taken, and never grown
// again while the form is open.
func (s *settingsForm) newsFields() []formField {
	catalogue := provider.NewsSources()
	if len(s.state.feedPresets) != len(catalogue) {
		grown := make([]bool, len(catalogue))
		copy(grown, s.state.feedPresets)
		s.state.feedPresets = grown
	}
	fields := make([]formField, 0, len(catalogue)+1)
	for i, source := range catalogue {
		fields = append(fields, formField{
			label: source.Name, kind: fieldBool, flag: &s.state.feedPresets[i],
		})
	}
	return append(fields, formField{
		label: "other feed urls", kind: fieldText, text: &s.state.feeds,
	})
}

// orderCategories puts the panel pages in the order the panels themselves are
// in, so the settings list matches the dashboard and does not drift as panels
// move onto the registry. Pages without a panel - General - stay at the front
// in the order they were written.
func (s *settingsForm) orderCategories(categories []settingsCategory) {
	if s.ws == nil {
		return
	}
	rank := map[string]int{}
	for i, id := range s.ws.PanelIDs() {
		rank[id] = i
	}
	sort.SliceStable(categories, func(i, j int) bool {
		return categoryRank(categories[i], rank) < categoryRank(categories[j], rank)
	})
}

// categoryRank sorts a page by its panel's position, with non-panel pages
// first and unknown panels last.
func categoryRank(category settingsCategory, rank map[string]int) int {
	if category.panelID == "" {
		return -1
	}
	if index, ok := rank[category.panelID]; ok {
		return index
	}
	return len(rank)
}

// mergeCategories inserts panel-declared categories into the hand-written
// list: a panel already listed keeps its position and gains its fields, and
// one that is not listed is appended.
func mergeCategories(existing, declared []settingsCategory) []settingsCategory {
	at := make(map[string]int, len(existing))
	for i, category := range existing {
		if category.panelID != "" {
			at[category.panelID] = i
		}
	}
	for _, category := range declared {
		if index, ok := at[category.panelID]; ok {
			existing[index].fields = append(existing[index].fields, category.fields...)
			// The panel is what knows whether it draws a gauge or sparkline.
			existing[index].gauge = category.gauge
			existing[index].spark = category.spark
			continue
		}
		existing = append(existing, category)
	}
	return existing
}

// loadPanelFields allocates an edit buffer for every field the registered
// panels declare, seeded from the document. The buffers are addressed by
// configuration key, so a panel names the key that is already in the file and
// nothing is renamed by moving a setting onto a panel.
func (s *settingsForm) loadPanelFields(cfg config) {
	s.state.panelText = map[string]*string{}
	s.state.panelFlag = map[string]*bool{}
	if s.deck == nil {
		return
	}
	document := cfg.doc
	for _, category := range s.deck.Schema() {
		for _, field := range category.Fields {
			if field.Key == "" {
				continue // an action stores nothing
			}
			switch field.Kind {
			case dash.FieldBool:
				// An absent key means the panel's declared default applies,
				// which Bool alone cannot express: unset and a deliberate
				// false look the same, so a setting that defaults to on would
				// show as off on a fresh config.
				value := field.Default == "true"
				if document.Has(field.Key) {
					value = document.Bool(field.Key)
				}
				s.state.panelFlag[field.Key] = &value
			default:
				value := document.String(field.Key)
				if value == "" {
					// Absent means the panel's own default applies; show it,
					// rather than an empty row that hides what is in use.
					value = field.Default
				}
				s.state.panelText[field.Key] = &value
			}
		}
	}
}

// panelCategories turns each registered panel's declared fields into settings
// rows. The rows are ordinary formFields, so editing, choices and the summary
// display work exactly as they do for the hand-written ones.
func (s *settingsForm) panelCategories() []settingsCategory {
	if s.deck == nil {
		return nil
	}
	var out []settingsCategory
	for _, category := range s.deck.Schema() {
		rows := make([]formField, 0, len(category.Fields))
		for _, field := range category.Fields {
			row := formField{
				key:         field.Key,
				label:       field.Label,
				summary:     field.Summary,
				description: field.Description,
				placeholder: field.Placeholder,
				validate:    field.Validate,
				path:        field.Path,
				list:        field.List,
			}
			switch field.Kind {
			case dash.FieldBool:
				row.kind, row.flag = fieldBool, s.state.panelFlag[field.Key]
			case dash.FieldChoice:
				row.kind, row.choice, row.options = fieldChoice, s.state.panelText[field.Key], field.Options
			case dash.FieldFloat:
				// FieldFloat was declared from the start and never had a case
				// here, so latitude and longitude fell through to free text
				// and were only ever checked by the save.
				row.kind, row.text = fieldNumber, s.state.panelText[field.Key]
				row.unit, row.step = field.Unit, field.Step
				if field.Bounded() {
					row.minimum, row.maximum = field.Min, field.Max
				}
			case dash.FieldAction:
				run := field.Run
				row.kind, row.action = fieldAction, func() settingsAction {
					if run != nil {
						s.report(run())
					}
					return settingsNone
				}
			default:
				row.kind, row.text = fieldText, s.state.panelText[field.Key]
				// A plain setting whose program reported the values it can
				// usefully take becomes a list. A blank box the reader has to
				// guess at is the worst kind of setting, and the program
				// already knows the answer - it just looked.
				if len(field.Options) > 0 {
					row.kind = fieldChoice
					row.choice = s.state.panelText[field.Key]
					row.options = withCurrent(field.Options, row.choiceValue())
				}
			}
			if row.kind != fieldAction && row.text == nil && row.flag == nil && row.choice == nil {
				continue // a field whose buffer is missing would edit nothing
			}
			rows = append(rows, row)
		}
		out = append(out, settingsCategory{
			name: category.Name, panelID: category.PanelID, fields: rows,
			gauge: category.Gauge, spark: category.Spark,
		})
	}
	return out
}

// applyPanelFields writes the edit buffers back into the document, applying
// any normalisation the field asked for.
func (s formState) applyPanelFields(document dash.Values, deck *dash.Deck) (dash.Values, error) {
	out := document.Clone()
	if deck == nil {
		return out, nil
	}
	for _, category := range deck.Schema() {
		for _, field := range category.Fields {
			if field.Key == "" {
				continue
			}
			if flag, ok := s.panelFlag[field.Key]; ok && flag != nil {
				out.Set(field.Key, *flag)
				continue
			}
			text, ok := s.panelText[field.Key]
			if !ok || text == nil {
				continue
			}
			value := *text
			if field.Normalize != nil {
				value = field.Normalize(value)
			}
			if field.Kind == dash.FieldFloat {
				// A number stays a number in the file: writing it back as
				// text would change the shape of a key that has always been
				// numeric, and a typo is refused here rather than stored.
				number, err := parseOptionalFloat(value)
				if err != nil {
					return out, fmt.Errorf("%s: %v", field.Label, err)
				}
				out.Set(field.Key, number)
				continue
			}
			out.Set(field.Key, value)
		}
	}
	return out, nil
}

// pluginFields builds the Plugins page: a field to paste a source into, an
// Install button, and a row per installed plugin for updating or removing it.
// Enabling a plugin is its own category's visibility toggle, so it is not
// duplicated here.
func (s *settingsForm) pluginFields() []formField {
	fields := []formField{
		{label: "plugin source", kind: fieldText, text: &s.state.pluginSource,
			input: true, placeholder: "git URL[#dir] or local path"},
		{label: "Install", kind: fieldAction, action: s.installPlugin, button: s.plugins()},
	}
	for _, info := range dash.Installed(pluginsDir()) {
		if info.Problem != nil {
			problem := info.Problem
			name := info.DisplayName()
			fields = append(fields, formField{label: "broken: " + name, kind: fieldAction, action: func() settingsAction {
				s.fail(problem.Error())
				return settingsNone
			}})
			continue
		}
		id := info.Manifest.ID
		label := info.DisplayName() + " " + info.Manifest.Version
		fields = append(fields,
			formField{label: "update " + label, kind: fieldAction, button: s.plugins(), action: func() settingsAction {
				s.beginPluginOp("update", id)
				return settingsNone
			}},
			formField{label: "remove " + label, kind: fieldAction, button: s.plugins(), action: func() settingsAction {
				s.beginPluginOp("remove", id)
				return settingsNone
			}},
		)
	}
	return fields
}

// installPlugin queues an install for the URL or path in the field. The clone
// runs off the UI goroutine, like the geocoder: the model takes the queued
// operation and reports the result back.
func (s *settingsForm) installPlugin() settingsAction {
	source := strings.TrimSpace(s.state.pluginSource)
	if source == "" {
		s.fail("paste a plugin URL or path first")
		return settingsNone
	}
	s.beginPluginOp("install", source)
	return settingsNone
}

// beginPluginOp queues a plugin operation and shows it as in progress.
func (s *settingsForm) beginPluginOp(kind, value string) {
	s.pendingPlugin = pluginOp{kind: kind, value: value}
	s.plugins().Busy()
	switch kind {
	case "install":
		s.working("installing " + value + "…")
	case "update":
		s.working("updating " + value + "…")
	case "remove":
		s.working("removing " + value + "…")
	default:
		s.working(kind + " " + value + "…")
	}
}

// TakePluginOp returns and clears a queued plugin operation, if any.
func (s *settingsForm) TakePluginOp() (pluginOp, bool) {
	if s.pendingPlugin.kind == "" {
		return pluginOp{}, false
	}
	op := s.pendingPlugin
	s.pendingPlugin = pluginOp{}
	return op, true
}

// ApplyPluginOp records the result of a plugin operation. On success the
// install field is cleared, so a second Enter does not try the same source
// again.
func (s *settingsForm) ApplyPluginOp(message string, err error) {
	s.PluginOpDone()
	if err != nil {
		s.fail(err.Error())
		return
	}
	s.report(message)
	if s.state != nil && strings.TrimSpace(s.state.pluginSource) != "" {
		s.state.pluginSource = ""
	}
}

// Reload rebuilds the form from the configuration after the set of panels
// changed - a plugin was installed or removed - keeping the open category.
// Panel edit buffers are re-seeded from the saved document, so unsaved edits to
// other fields are dropped; installing or removing is a deliberate enough act
// that this is the predictable behaviour.
func (s *settingsForm) Reload(cfg config) {
	if !s.opened {
		return
	}
	current := ""
	if s.category >= 0 && s.category < len(s.categories) {
		current = s.categories[s.category].name
	}
	s.loadPanelFields(cfg)
	s.categories = s.buildCategories()
	for i, category := range s.categories {
		if category.name == current {
			s.category = i
			break
		}
	}
	if s.cursor >= len(s.currentFields()) {
		s.cursor = 0
	}
}

func (s *settingsForm) panelField(id string) *formField {
	if s.ws == nil || id == "" {
		return nil
	}
	panel, ok := s.ws.Lookup(id)
	if !ok || !panel.CanHide() {
		return nil
	}
	return &formField{label: "enabled", kind: fieldPanel, panel: id,
		description: "Show this panel on the dashboard. Takes effect when you save."}
}

func (s *settingsForm) currentFields() []formField {
	if s.category < 0 || s.category >= len(s.categories) {
		return nil
	}
	return s.categories[s.category].fields
}

func (s *settingsForm) currentField() *formField {
	fields := s.currentFields()
	if len(fields) == 0 {
		return nil
	}
	index := min(max(s.cursor, 0), len(fields)-1)
	return &s.categories[s.category].fields[index]
}

// lookupCoordinates queues a geocoding request for the model to run in the
// background, so the panel can show progress instead of freezing.
func (s *settingsForm) lookupCoordinates() settingsAction {
	query := strings.TrimSpace(s.state.place)
	if query == "" {
		s.fail("enter a city or postal code first")
		return settingsNone
	}
	s.pendingLookup = query
	s.lookingUp = true
	s.lookup().Busy()
	s.working("looking up " + query + "…")
	return settingsNone
}

// TakeLookup returns and clears a queued geocoding query, if any.
func (s *settingsForm) TakeLookup() string {
	query := s.pendingLookup
	s.pendingLookup = ""
	return query
}

// ApplyLookup records the result of a background lookup.
func (s *settingsForm) ApplyLookup(place provider.Place, err error) {
	s.LookupDone()
	if err != nil {
		s.fail(err.Error())
		return
	}
	s.applyPlace(place)
}

// applyPlace writes a geocoding result into the weather panel's fields and
// enables live data, since looking up a real place clearly means "use it".
//
// It writes through the panel's edit buffers rather than into form state: the
// panel owns those keys, so the lookup fills in the same rows you could have
// typed by hand, and a panel that is not registered simply has nothing to fill.
func (s *settingsForm) applyPlace(place provider.Place) {
	s.setPanelText(weatherLatitudeKey, formatFloat(place.Latitude))
	s.setPanelText(weatherLongitudeKey, formatFloat(place.Longitude))
	s.setPanelText(weatherLocationKey, place.Name)
	s.setPanelFlag(weatherEnabledKey, true)
	s.state.live = true
	s.dirty = true
	s.report("found " + place.Label() + " — ctrl+s to apply")
}

// Configuration keys the settings screen fills in on the weather panel's
// behalf. They are the panel's keys; the form only writes them because the
// geocoder is a settings affordance, not something a panel can run.
const (
	weatherEnabledKey   = "weather.enabled"
	weatherLatitudeKey  = "weather.latitude"
	weatherLongitudeKey = "weather.longitude"
	weatherLocationKey  = "weather.location"
)

func (s *settingsForm) setPanelText(key, value string) {
	if buffer, ok := s.state.panelText[key]; ok && buffer != nil {
		*buffer = value
	}
}

func (s *settingsForm) setPanelFlag(key string, value bool) {
	if buffer, ok := s.state.panelFlag[key]; ok && buffer != nil {
		*buffer = value
	}
}

// Update handles all input while the panel is open.
func (s *settingsForm) Update(msg tea.KeyMsg) settingsAction {
	if !s.opened {
		return settingsNone
	}
	key := msg.String()
	if s.picker != nil {
		s.updatePicker(key, msg.Runes)
		return settingsNone
	}
	if s.search != nil {
		s.updateSearch(key, msg.Runes)
		return settingsNone
	}
	if s.editing {
		return s.updateEditing(msg, key)
	}
	if key == "/" {
		s.openSearch()
		return settingsNone
	}
	if key == "ctrl+s" {
		return s.save()
	}
	if s.view == viewCategories {
		return s.updateCategories(key)
	}
	return s.updateFields(key)
}

func (s *settingsForm) updateCategories(key string) settingsAction {
	switch key {
	case "esc", "q":
		s.opened = false
		return settingsCancelled
	case "up", "k":
		s.category = wrapIndex(s.category-1, len(s.categories))
	case "down", "j":
		s.category = wrapIndex(s.category+1, len(s.categories))
	case "tab":
		s.focus = settingsEditor
		s.view = viewFields
		s.cursor = 0
	case "shift+tab":
		s.focus = settingsEditor
		s.view = viewFields
		s.cursor = max(0, len(s.currentFields())-1)
	case "enter", "right", "l", " ":
		if len(s.categories) > 0 {
			s.view = viewFields
			s.focus = settingsEditor
			s.cursor = 0
			s.clearNotice()
		}
	}
	return settingsNone
}

func (s *settingsForm) updateFields(key string) settingsAction {
	fields := s.currentFields()
	switch key {
	case "esc", "backspace":
		s.view = viewCategories
		s.focus = settingsNav
		s.editing = false
	case "left", "h":
		// Cycle a choice field; otherwise this backs out of the category.
		if field := s.currentField(); field != nil && field.kind == fieldChoice {
			field.setChoice(stepChoice(field.choiceValue(), field.options, -1))
			s.dirty = true
			s.notifyDeck()
		} else {
			s.view = viewCategories
			s.focus = settingsNav
			s.editing = false
		}
	case "right", "l":
		if field := s.currentField(); field != nil && field.kind == fieldChoice {
			field.setChoice(stepChoice(field.choiceValue(), field.options, 1))
			s.dirty = true
			s.notifyDeck()
		}
	case "up", "k":
		s.cursor = wrapIndex(s.cursor-1, len(fields))
	case "down", "j":
		s.cursor = wrapIndex(s.cursor+1, len(fields))
	case "tab", "shift+tab":
		s.focus = settingsNav
		s.view = viewCategories
	case "enter", " ":
		return s.activate()
	case "o":
		s.openPicker()
	}
	return settingsNone
}

// updateEditing hands a keystroke to the control that owns the row. The
// control decides what the key means; the form only records that something
// changed and notices when the edit is over.
func (s *settingsForm) updateEditing(msg tea.KeyMsg, key string) settingsAction {
	field := s.currentField()
	if field == nil || s.editor == nil {
		s.endEdit()
		return settingsNone
	}
	// ctrl+s saves from inside an edit, so a value can be committed and
	// written in one keystroke.
	if key == "ctrl+s" {
		s.writeBack(field, s.editor.Value())
		s.endEdit()
		return s.save()
	}

	action := s.editor.Update(msg)
	switch action {
	case form.ActionChanged:
		s.writeBack(field, s.editor.Value())
		s.dirty = true
		s.notifyDeck()
	case form.ActionCancelled:
		s.writeBack(field, s.editBefore)
	case form.ActionCommitted:
		s.writeBack(field, s.editor.Value())
	case form.ActionIgnored:
		// The control finished with the key without using it - a navigation
		// key pressed mid-edit. Commit, then let it do what it does everywhere
		// else rather than dropping it.
		s.writeBack(field, s.editor.Value())
		if s.editor.Value() != s.editBefore {
			s.dirty = true
		}
		s.endEdit()
		return s.updateFields(key)
	}
	if !s.editor.Editing() {
		// A committed value is normalized by the control, so read it once more
		// before letting it go.
		s.writeBack(field, s.editor.Value())
		if s.editor.Value() != s.editBefore {
			s.dirty = true
		}
		s.endEdit()
	}
	return settingsNone
}

func (s *settingsForm) activate() settingsAction {
	field := s.currentField()
	if field == nil {
		return settingsNone
	}
	switch field.kind {
	case fieldBool:
		*field.flag = !*field.flag
		s.dirty = true
	case fieldText, fieldNumber:
		s.beginEdit(field)
	case fieldAction:
		if field.button != nil && field.button.Running() {
			// Pressing again while the work runs would start it twice - two
			// installs into one directory. The key is swallowed, and the
			// spinner on the row already says why.
			return settingsNone
		}
		if field.action != nil {
			action := field.action()
			if field.button != nil && field.button.Running() {
				s.busyRow = field.label
			}
			return action
		}
	case fieldPanel:
		if s.state.panelShown == nil {
			s.state.panelShown = map[string]bool{}
		}
		s.state.panelShown[field.panel] = !s.panelVisible(field.panel)
		s.dirty = true
	case fieldGlyph:
		if s.state.panelGlyphs == nil {
			s.state.panelGlyphs = map[string]bool{}
		}
		s.state.panelGlyphs[field.panel] = !s.PanelGlyphEnabled(field.panel)
		s.dirty = true
	case fieldChoice:
		// A long list opens a picker rather than stepping blind; the control
		// decides which, because it knows how many options there are.
		s.beginEdit(field)
	}
	return settingsNone
}

// notifyDeck hands the deck the settings as they stand and marks the panel due.
//
// A panel whose options depend on another of its settings - the mailboxes of
// the chosen account - cannot offer the right ones until it has been told what
// was chosen. Without this the list would correct itself on the panel's next
// scheduled run, which for the mail panel is a minute away.
func (s *settingsForm) notifyDeck() {
	if s.deck == nil || s.state == nil {
		return
	}
	id := s.currentPanelID()
	if id == "" {
		return
	}
	document, err := s.state.applyPanelFields(s.state.doc.Clone(), s.deck)
	if err != nil {
		return // a half-typed value is not a reason to stop editing
	}
	// Only the page being edited is re-read. A full Configure forgets every
	// panel's interval, and the next tick then re-fetches the whole dashboard -
	// news, weather, markets, updates - none of which was edited, at one
	// keystroke per typed character. A save still applies everything at once,
	// because a save may have changed anything.
	s.deck.ConfigurePanel(id, document)
}

// currentPanelID is the panel whose page is open, or "" for a page that is not
// a panel's.
func (s settingsForm) currentPanelID() string {
	if s.category < 0 || s.category >= len(s.categories) {
		return ""
	}
	return s.categories[s.category].panelID
}

// SyncOptions refreshes the choices a panel reports for its own settings, in
// place, so the open page follows what the panel last discovered without
// discarding anything being edited. Rebuilding the form would re-seed every
// buffer from the saved document and lose the very choice that caused the
// options to change.
func (s *settingsForm) SyncOptions() {
	if !s.opened || s.deck == nil {
		return
	}
	id := s.currentPanelID()
	if id == "" {
		return
	}
	for _, category := range s.deck.Schema() {
		if category.PanelID != id {
			continue
		}
		latest := make(map[string][]string, len(category.Fields))
		for _, field := range category.Fields {
			if len(field.Options) > 0 {
				latest[field.Key] = field.Options
			}
		}
		fields := s.categories[s.category].fields
		for i := range fields {
			if fields[i].key == "" {
				continue // a row the form writes by hand owns its own options
			}
			options, ok := latest[fields[i].key]
			switch {
			case ok && fields[i].kind == fieldText:
				// The panel had not run yet when this page was built, so the
				// row became a text box. Promote it now that there is a list
				// to offer - otherwise opening settings within a second of
				// launch left the field a text box for good.
				fields[i].kind = fieldChoice
				fields[i].choice = fields[i].text
				fields[i].options = withCurrent(options, fields[i].choiceValue())
				// A row promoted while it is being edited is still holding the
				// text editor it was given, and an editor is only ever built
				// when an edit starts. Ending the edit is what turns the box
				// on screen into the list: without it, opening settings fast
				// enough to catch the old kind, then pressing enter, left a
				// text box that no later sync could ever replace.
				if s.editing && i == s.cursor {
					s.endEdit()
				}
			case ok && fields[i].kind == fieldChoice:
				fields[i].options = withCurrent(options, fields[i].choiceValue())
			}
		}
		return
	}
}

// newControl builds the control for a field.
func (s *settingsForm) newControl(field *formField) form.Control {
	switch field.kind {
	case fieldNumber:
		number := form.NewNumber(*field.text).
			WithUnit(field.unit).
			WithStep(field.step).
			WithPlaceholder(field.placeholder)
		if field.minimum != field.maximum {
			number.WithRange(field.minimum, field.maximum)
		}
		return number
	case fieldChoice:
		choice := form.NewChoice(field.options, field.choiceValue()).
			WithTitle(field.label).
			WithBlankLabel(field.placeholder)
		switch {
		case field.gaugePreview:
			choice.WithSample(func(r tideui.Renderer, option string, width int) string {
				return r.GaugeSample(tideui.GaugeStyle(s.styleOrDefault(option, s.GaugeStyle())), width)
			})
		case field.sparkPreview:
			choice.WithSample(func(r tideui.Renderer, option string, width int) string {
				return r.SparkSample(tideui.SparklineStyle(s.styleOrDefault(option, s.SparkStyle())), width)
			})
		}
		return choice
	default:
		text := form.NewText(*field.text).
			WithPlaceholder(field.placeholder).
			WithSummary(field.summary).
			WithValidate(field.validate)
		return text
	}
}

// styleOrDefault resolves the "default" option to the style it stands for, so
// a preview of "default" shows what it will actually look like.
func (s settingsForm) styleOrDefault(option, fallback string) string {
	if option == "" || option == "default" {
		return fallback
	}
	return option
}

// beginEdit hands the row to a control and reports whether the control kept
// the keyboard. Not every activation opens an edit: a choice short enough to
// step changes its value on the spot and is done, which is why the control is
// asked rather than the form guessing from the field's kind.
func (s *settingsForm) beginEdit(field *formField) bool {
	control := s.newControl(field)
	before := s.fieldValue(field)
	switch control.Update(tea.KeyMsg{Type: tea.KeyEnter}) {
	case form.ActionEditing:
		s.editor, s.editKey, s.editing = control, field.label, true
		s.editBefore = before
		return true
	case form.ActionChanged:
		s.writeBack(field, control.Value())
		s.dirty = true
		s.notifyDeck()
	}
	return false
}

// fieldValue reads whichever buffer a field is backed by.
func (s settingsForm) fieldValue(field *formField) string {
	if field.kind == fieldChoice {
		return field.choiceValue()
	}
	if field.text != nil {
		return *field.text
	}
	return ""
}

// writeBack stores the control's value into the field's buffer.
func (s *settingsForm) writeBack(field *formField, value string) {
	if field.kind == fieldChoice {
		field.setChoice(value)
		return
	}
	if field.text != nil {
		*field.text = value
	}
}

// endEdit discards the control.
func (s *settingsForm) endEdit() {
	s.editor, s.editKey, s.editing = nil, "", false
}

// stepChoice moves delta options from current, wrapping around.
func stepChoice(current string, options []string, delta int) string {
	if len(options) == 0 {
		return current
	}
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	return options[wrapIndex(index+delta, len(options))]
}

// panelVisible reports whether a panel's enable row should show a tick.
// panelVisible reports whether a panel is shown, preferring the pending edit
// over the workspace's current state so a toggle takes effect on the row
// before it takes effect on the dashboard.
func (s settingsForm) panelVisible(id string) bool {
	if s.state != nil {
		if shown, ok := s.state.panelShown[id]; ok {
			return shown
		}
	}
	return s.ws == nil || !s.ws.Hidden(id)
}

// fail, working and report set the editor's message and say how to read it.
// Going through these rather than assigning s.problem directly is what keeps a
// success from being rendered as a failure.
func (s *settingsForm) fail(message string) {
	s.problem, s.tone = message, noticeError
}

func (s *settingsForm) working(message string) {
	s.problem, s.tone = message, noticeProgress
}

func (s *settingsForm) report(message string) {
	s.problem, s.tone = message, noticeGood
}

func (s *settingsForm) clearNotice() {
	s.problem, s.tone = "", noticeError
}

// applyPanelVisibility pushes the pending show/hide edits onto the workspace.
// It runs only on save, which is what makes cancelling a toggle possible.
func (s *settingsForm) applyPanelVisibility() {
	if s.ws == nil || s.state == nil {
		return
	}
	for id, shown := range s.state.panelShown {
		if shown == !s.ws.Hidden(id) {
			continue
		}
		s.ws.TogglePanel(id)
	}
}

func (s *settingsForm) save() settingsAction {
	if s.state == nil {
		return settingsNone
	}
	cfg, err := s.state.toConfig(s.deck)
	if err != nil {
		s.fail(err.Error())
		return settingsNone
	}
	s.applyPanelVisibility()
	s.clearNotice()
	s.opened = false
	s.editing = false
	s.dirty = false
	s.cfg = cfg
	return settingsSaved
}

// value is the row's right-hand cell for the kinds that are not drawn by a
// control. Booleans return nothing: the tick in the prefix already says on or
// off, and the old row said it a second time in the same line.
func (s settingsForm) value(field formField) string {
	switch field.kind {
	case fieldBool, fieldGlyph, fieldPanel:
		return ""
	case fieldText, fieldNumber:
		if field.text != nil {
			if field.summary != nil {
				return field.summary(*field.text)
			}
			return *field.text
		}
	case fieldChoice:
		return field.choiceValue()
	}
	return ""
}

// RenderWorkspace renders Settings as a TideUI workspace. The dashboard is
// intentionally not passed in here: entering settings never changes its
// layout, focus, visibility, preset, or zoom state, so leaving settings is a
// simple return to the same workspace object.
func (s settingsForm) RenderWorkspace(r tideui.Renderer, width, height int) string {
	if !s.opened || width <= 0 || height <= 0 {
		return ""
	}
	layoutWidth := min(width, settingsMaxWidth)
	narrow := layoutWidth < 96
	leftWidth := max(24, min(34, layoutWidth/3))
	rightWidth := max(1, layoutWidth-leftWidth-1)
	if narrow {
		leftWidth = layoutWidth
		rightWidth = layoutWidth
	}

	navRows := max(1, height-5)
	left := strings.Join(s.renderCategories(r, max(1, leftWidth-4), navRows), "\n")

	right := "choose a category from the navigation pane"
	if s.search != nil {
		// The search spans every page, so it takes the pane rather than
		// sitting under one page's header.
		right = strings.Join(fitRows(max(1, height-6), func(limit int) []string {
			return s.searchLines(r, max(1, rightWidth-4), limit)
		}), "\n")
	} else if s.category >= 0 && s.category < len(s.categories) {
		category := s.categories[s.category]
		rows := max(1, height-6)
		headerWidth := max(1, rightWidth-4)
		right = s.renderHeader(r, category, headerWidth) + "\n"
		if len(category.fields) == 0 {
			right += paneHint(r).Render("No additional settings for this panel.")
		} else {
			right += strings.Join(s.renderFields(r, max(1, rightWidth-4), rows), "\n")
		}
	}
	if s.problem != "" {
		style := r.Styles.StatusError
		switch s.tone {
		case noticeProgress:
			style = r.Styles.StatusHint
		case noticeGood:
			style = r.Styles.StatusSuccess
		}
		right = style.Render(ansi.Truncate(s.problem, max(1, rightWidth-4), "…")) + "\n" + right
	}

	mode := tideui.SidebarOnly
	panes := [3]tideui.Pane{
		{Title: "Settings", Content: left, Focused: s.focus == settingsNav, Hint: "↑/↓"},
		{Title: "Editor", Content: right, Focused: s.focus == settingsEditor, Hint: "tab focus"},
	}
	if narrow {
		// Two tabs, one focused. The old branch overwrote one pane and left
		// the other, so both ended up with the same title and both claimed
		// focus - "Settings | Settings", with the wrong one highlighted.
		mode = tideui.Tabbed
		panes[0].Focused = s.focus == settingsNav
		panes[1].Focused = s.focus == settingsEditor
		panes[2] = tideui.Pane{}
	}
	hints := s.hintBar()
	layout := tideui.Layout{
		Width: layoutWidth, Height: height, Mode: mode, Panes: panes,
		SidebarRatio: float64(leftWidth) / float64(max(1, layoutWidth)),
		Status:       &tideui.StatusBar{Left: "TideDeck › Settings", Right: hints},
	}
	// A control that opens a list draws it over the screen. Without this the
	// picker would take the keyboard while nothing on screen had changed.
	if overlayer, ok := s.editor.(form.Overlayer); ok && s.editing {
		pickerWidth := max(24, min(48, layoutWidth-8))
		if overlay, open := overlayer.Overlay(r, pickerWidth, height-4); open {
			layout.Modal = &overlay
		}
	}
	view := r.Render(layout)
	if layoutWidth == width {
		return view
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, view)
}

// renderHeader draws a category's title and the rule beneath it.
//
// A panel's page carries its own state: the title is green when the panel is on
// the dashboard and red when it is not, so the page says what it is for before
// any row is read. Colour alone would not say it - a red title and a green one
// are the same title to anyone who cannot tell them apart, and identical under
// PlainUI - so the rule underneath is labelled with the state as well.
func (s settingsForm) renderHeader(r tideui.Renderer, category settingsCategory, width int) string {
	bg := paneSurface(r)
	title := s.settingsIcon(category) + " " + strings.ToUpper(category.name)

	// The title stays the ordinary heading colour; the rule beneath it carries
	// the state. Colouring both made the page shout, and the title is the one
	// part that should read the same on every page.
	label, tone := "", tideui.ToneNeutral
	if category.panelID != "" {
		if s.panelVisible(category.panelID) {
			label, tone = "enabled", tideui.ToneGood
		} else {
			label, tone = "disabled", tideui.ToneDanger
		}
	}
	rule := r.RenderSectionDivider(
		tideui.SectionDivider{Label: label, Width: width, Tone: tone}, bg)
	return paneTitle(r).Render(title) + "\n" + rule
}

// paneSurface is the background a settings pane's body actually has. Rows are
// drawn directly into a Pane, not into a modal, so they have to be resolved
// against this rather than against the lifted modal surface - otherwise every
// row is a different shade from the blank space around it and the pane looks
// banded.
func paneSurface(r tideui.Renderer) lipgloss.Color { return r.Styles.Theme.Bg }

// paneHint styles quiet text on the pane background: section headers, overflow
// markers, and the "no settings" note.
func paneHint(r tideui.Renderer) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(paneSurface(r)).
		Foreground(r.Styles.Workspace.HintFg)
}

// paneTitle styles a page heading on the pane background.
func paneTitle(r tideui.Renderer) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(paneSurface(r)).
		Foreground(r.Styles.Workspace.BodyFg).
		Bold(true)
}

// hintBar says which keys work right now. It used to be one fixed string that
// never mentioned enter, space, or the arrows, so the screen documented none of
// the keys that actually edited anything.
func (s settingsForm) hintBar() string {
	parts := make([]string, 0, 4)
	if s.picker != nil {
		return "type to filter · tab complete · enter open or choose · backspace up · esc cancel"
	}
	if s.search != nil {
		return "type to search · ↑↓ choose · enter go there · esc cancel"
	}
	if s.editor != nil && s.editing {
		// Only the control's keys, because they are the only ones that work:
		// esc means revert here, not "go back", and listing both would give
		// the same key two meanings in one bar.
		for _, hint := range s.editor.Hints() {
			if hint.Key == "" {
				parts = append(parts, hint.Label)
				continue
			}
			parts = append(parts, hint.Key+" "+hint.Label)
		}
		return strings.Join(parts, " · ")
	}
	if field := s.currentField(); field != nil && s.focus == settingsEditor {
		switch field.kind {
		case fieldBool, fieldPanel, fieldGlyph:
			parts = append(parts, "space toggle")
		case fieldChoice:
			parts = append(parts, "←→ change")
			if len(field.options) >= 5 {
				parts = append(parts, "enter list")
			}
		case fieldAction:
			parts = append(parts, "enter run")
		case fieldNumber:
			if field.step != 0 {
				parts = append(parts, "←→ step")
			}
			parts = append(parts, "enter edit")
		default:
			parts = append(parts, "enter edit")
		}
		if field.path != "" {
			parts = append(parts, "o browse")
		}
	}
	if s.dirty {
		parts = append(parts, "ctrl+s save")
	}
	parts = append(parts, "/ search", "tab focus", "esc back")
	return strings.Join(parts, " · ")
}

func (s settingsForm) settingsIcon(category settingsCategory) string {
	switch category.panelID {
	case "":
		if category.name == "Plugins" {
			return "🧩"
		}
		return "⚙️"
	default:
		// A plugin declares its own glyph in its manifest; the hand-written map
		// below is only for the host's panes, which declare none. Using the map
		// for everything meant every plugin page wore the generic plug.
		if glyph := s.declaredGlyph(category.panelID); glyph != "" {
			return glyph
		}
		return panelGlyph(category.panelID)
	}
}

// declaredGlyph is the glyph a panel itself declares, which for a plugin is the
// one in its manifest.
func (s settingsForm) declaredGlyph(id string) string {
	if s.deck == nil {
		return ""
	}
	for _, panel := range s.deck.Panels() {
		if panel.Meta().ID == id {
			return panel.Meta().Glyph
		}
	}
	return ""
}

func panelGlyph(id string) string {
	switch id {
	case "agenda":
		return "📅"
	case "weather":
		return "🌤️"
	case "system":
		return "💻"
	case "gpu":
		return "🎮"
	case "updates":
		return "⬆️"
	case "clock":
		return "🕒"
	case "git":
		return "🌿"
	case "news":
		return "📰"
	case "network":
		return "🌐"
	case "storage":
		return "💾"
	case "services":
		return "⚙️"
	case "tasks":
		return "✅"
	case "notes":
		return "📝"
	case "markets":
		return "📈"
	case "calculator":
		return "🧮"
	default:
		return "🔌"
	}
}

// renderCategories draws the navigation list within rows lines.
//
// The window is shrunk until everything fits, because the section headers and
// the "more" markers are drawn from the same budget as the rows and are not
// known until the window is chosen. Windowing to exactly rows and then
// appending up to four more lines is how this pane used to overflow its pane.
func (s settingsForm) renderCategories(r tideui.Renderer, width, rows int) []string {
	return fitRows(rows, func(limit int) []string {
		return s.categoryLines(r, width, limit)
	})
}

func (s settingsForm) categoryLines(r tideui.Renderer, width, rows int) []string {
	first, last := tideui.VisibleRange(len(s.categories), s.category, rows)
	var lines []string
	if first > 0 {
		lines = append(lines, paneHint(r).Width(width).Render("  ▲ more"))
	}
	for index := first; index < last; index++ {
		category := s.categories[index]
		if index == 0 {
			lines = append(lines, paneHint(r).Render("GENERAL"))
		} else if category.panelID != "" && s.categories[index-1].panelID == "" {
			lines = append(lines, paneHint(r).Render("PANELS"))
		}
		suffix := fmt.Sprintf("%d", len(category.fields))
		if category.panelID != "" && !s.panelVisible(category.panelID) {
			suffix = "off"
		}
		lines = append(lines, r.RenderSoftRowOn(tideui.SoftRow{
			Prefix:   "  ",
			Text:     s.settingsIcon(category) + " " + category.name,
			Suffix:   suffix,
			Selected: index == s.category,
		}, width, paneSurface(r)))
	}
	if last < len(s.categories) {
		lines = append(lines, paneHint(r).Width(width).Render("  ▼ more"))
	}
	return lines
}

// renderFields draws the editor list within rows lines, shrinking the window
// until the "more" markers it adds fit alongside the rows.
func (s settingsForm) renderFields(r tideui.Renderer, width, rows int) []string {
	if s.picker != nil {
		return fitRows(rows, func(limit int) []string { return s.pickerLines(r, width, limit) })
	}
	return fitRows(rows, func(limit int) []string {
		return s.fieldLines(r, width, limit)
	})
}

func (s settingsForm) fieldLines(r tideui.Renderer, width, rows int) []string {
	fields := s.currentFields()
	first, last := tideui.VisibleRange(len(fields), s.cursor, rows)
	var lines []string
	if first > 0 {
		lines = append(lines, paneHint(r).Width(width).Render("  ▲ more"))
	}
	for index := first; index < last; index++ {
		field := fields[index]
		row := tideui.SoftRow{
			Text:     field.label,
			Suffix:   s.value(field),
			Selected: index == s.cursor,
		}
		switch field.kind {
		case fieldBool:
			if field.flag != nil && *field.flag {
				row.Prefix = "[x] "
			} else {
				row.Prefix = "[ ] "
			}
		case fieldPanel:
			if s.panelVisible(field.panel) {
				row.Prefix = "[x] "
			} else {
				row.Prefix = "[ ] "
			}
		case fieldGlyph:
			if s.PanelGlyphEnabled(field.panel) {
				row.Prefix = "[x] "
			} else {
				row.Prefix = "[ ] "
			}
		case fieldChoice:
			row.Prefix = "    "
			budget := max(12, width-ansi.StringWidth(row.Text)-8)
			row.Suffix = s.newControl(&field).View(r, budget)
		case fieldAction:
			row.Prefix = "  "
			row.Text = "[ " + field.label + " ]"
			row.Accent = true
			if b := field.button; b != nil && b.Running() && (b != s.plugins() || s.busyRow == field.label) {
				row.Text = b.View(r, width) + " " + field.label + "…"
				row.Accent = false
			}
		case fieldText, fieldNumber:
			row.Prefix = "    "
			// Cap the cell so a long value cannot squeeze the label away; the
			// full value is still what the field holds and edits.
			budget := max(12, width-ansi.StringWidth(row.Text)-8)
			if field.input {
				row.Suffix = inputView(s.value(field), field.placeholder, budget)
				break
			}
			row.Suffix = s.newControl(&field).View(r, budget)
		default:
			row.Prefix = "    "
		}
		if index == s.cursor && s.editing && s.editor != nil {
			// Editing shows the raw value, not a summary: a field cannot be
			// edited through a description of itself. The control keeps the
			// caret visible in a value wider than the row.
			budget := max(12, width-ansi.StringWidth(row.Text)-8)
			view := s.editor.View(r, budget)
			if field.input {
				row.Suffix = "[ " + view + " ]"
			} else {
				row.Suffix = view
			}
		}
		lines = append(lines, r.RenderSoftRowOn(row, width, paneSurface(r)))

		// The selected row gets a line explaining itself, or saying why its
		// value cannot be used. A label alone cannot say that a docker socket
		// of "1" means the default one; a description can, and a plugin has
		// always been able to supply one.
		if index == s.cursor {
			if note, tone := s.fieldNote(field); note != "" {
				// Wrapped rather than truncated: an explanation cut off
				// mid-sentence is barely better than no explanation.
				for _, line := range wrapText(note, max(8, width-8), 2) {
					lines = append(lines, r.RenderSoftRowOn(tideui.SoftRow{
						Prefix: "      ",
						Text:   line,
						Muted:  tone != noticeError,
						Accent: tone == noticeError,
					}, width, paneSurface(r)))
				}
				// A blank row after the note, so the sentence belongs to the
				// row above it rather than running into the row below.
				lines = append(lines, r.RenderSoftRowOn(tideui.SoftRow{}, width, paneSurface(r)))
			}
		}
	}
	if last < len(fields) {
		lines = append(lines, paneHint(r).Width(width).Render("  ▼ more"))
	}
	return lines
}

// withBlank puts an explicit "any" in front of a discovered option list, so a
// setting whose blank value already does something sensible can still be
// chosen rather than only left alone.
func withBlank(options []string) []string {
	for _, option := range options {
		if option == "" {
			return options
		}
	}
	return append([]string{""}, options...)
}

// withCurrent is the list a panel-reported setting should offer: the blank
// "leave it unset" value, and the value the setting already holds.
//
// A panel re-reads its options on every run, and a list that narrows - the
// mailboxes of the account just chosen - can leave the saved value out.
// form.Choice draws the first option for a value that is not in its list,
// because a control cannot show a value it was not given: the row would read
// "the inbox" while the setting said "Receipts", and confirming the list would
// write that lie back over the value. Keeping it in the list means the row
// tells the truth, the picker opens on the value, enter keeps it - and
// switching back to the account the mailbox belongs to does not mean finding
// it again.
func withCurrent(options []string, current string) []string {
	options = withBlank(options)
	if current == "" {
		return options
	}
	for _, option := range options {
		if option == current {
			return options
		}
	}
	// A fresh slice: options may be the manifest's own, and a choice's list
	// must not write into the field declaration it came from.
	return append(append(make([]string, 0, len(options)+1), options...), current)
}

// wrapText breaks text on word boundaries into at most limit lines, marking
// the last one with an ellipsis when there was more to say.
func wrapText(text string, width, limit int) []string {
	words := strings.Fields(text)
	if len(words) == 0 || width <= 0 {
		return nil
	}
	var lines []string
	line := ""
	for _, word := range words {
		switch {
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			if len(lines) == limit {
				return elide(lines, width)
			}
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) > limit {
		return elide(lines[:limit], width)
	}
	return lines
}

// elide marks a wrapped block as cut short.
func elide(lines []string, width int) []string {
	last := len(lines) - 1
	lines[last] = ansi.Truncate(lines[last], max(1, width-1), "") + "…"
	return lines
}

// fieldNote is the line under the selected row: the reason a value is unusable
// when there is one, and otherwise the field's description. The complaint wins,
// because a field that is both described and wrong needs the complaint.
func (s settingsForm) fieldNote(field formField) (string, noticeTone) {
	if s.editor != nil && s.editing {
		if err := s.editor.Err(); err != nil {
			return err.Error(), noticeError
		}
	}
	return field.description, noticeGood
}

// inputView draws a text field as an input box. An empty field shows its
// placeholder, so a row meant to be typed into reads as one. A long value is
// truncated to the budget rather than crowding the label off the row.
func inputView(value, placeholder string, budget int) string {
	text := value
	if text == "" {
		text = placeholder
	}
	if budget < 8 {
		budget = 8
	}
	if ansi.StringWidth(text) > budget-4 {
		text = ansi.Truncate(text, budget-4, "…")
	}
	if text == "" {
		return "[ ]"
	}
	return "[ " + text + " ]"
}

// fitRows draws a list with the largest window whose rendered height still
// fits in rows. build may add lines of its own - headers, overflow markers -
// so asking for a window of rows and trusting the result is not enough.
func fitRows(rows int, build func(limit int) []string) []string {
	for limit := rows; limit > 1; limit-- {
		if lines := build(limit); len(lines) <= rows {
			return lines
		}
	}
	lines := build(1)
	if len(lines) > rows {
		return lines[:rows]
	}
	return lines
}

func wrapIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	return (index%length + length) % length
}
