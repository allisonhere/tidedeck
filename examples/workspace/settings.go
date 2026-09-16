package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/provider"
)

type fieldKind int

const (
	fieldBool fieldKind = iota
	fieldText
	fieldAction
	fieldPanel
	fieldChoice
)

type formField struct {
	label        string
	kind         fieldKind
	flag         *bool
	text         *string
	action       func() settingsAction
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
	live           bool
	weatherEnabled bool
	latitude       string
	longitude      string
	location       string
	place          string // city or ZIP to look up
	fahrenheit     bool
	windMPH        bool
	zones          string
	clock24        bool
	gauge          string
	spark          string
	clockFont      string
	icons          string
	panelGauges    map[string]string
	panelSparks    map[string]string
	aurHelper      string
	feeds          string // custom URLs only; catalogue URLs live in feedPresets
	feedPresets    []bool // one per provider.NewsSources(), same order
	calendars      string
	todo           string
	notes          string
	repos          string
	symbols        string
	systemd        string
	docker         string
	iface          string
}

func formFromConfig(cfg config) formState {
	feedPresets, customFeeds := splitFeeds(cfg.Feeds)
	return formState{
		live:           cfg.Live,
		weatherEnabled: cfg.Weather.Enabled,
		latitude:       formatFloat(cfg.Weather.Latitude),
		longitude:      formatFloat(cfg.Weather.Longitude),
		location:       cfg.Weather.Location,
		place:          cfg.Weather.Location,
		fahrenheit:     cfg.Weather.Fahrenheit,
		windMPH:        cfg.Weather.WindMPH,
		zones:          cfg.Zones,
		clock24:        cfg.Clock24,
		gauge:          gaugeOrDefault(cfg.GaugeStyle),
		spark:          sparkOrDefault(cfg.SparkStyle),
		clockFont:      clockFontOrDefault(cfg.ClockFont),
		icons:          iconStyleOrDefault(cfg.Icons),
		panelGauges:    copyStringMap(cfg.PanelGauges),
		panelSparks:    copyStringMap(cfg.PanelSparks),
		aurHelper:      cfg.AURHelper,
		feeds:          customFeeds,
		feedPresets:    feedPresets,
		calendars:      cfg.Calendars,
		todo:           cfg.Todo,
		notes:          cfg.Notes,
		repos:          cfg.Repos,
		symbols:        cfg.Symbols,
		systemd:        cfg.Systemd,
		docker:         cfg.Docker,
		iface:          cfg.Interface,
	}
}

func (s formState) toConfig() (config, error) {
	latitude, err := parseOptionalFloat(s.latitude)
	if err != nil {
		return config{}, fmt.Errorf("latitude: %v", err)
	}
	longitude, err := parseOptionalFloat(s.longitude)
	if err != nil {
		return config{}, fmt.Errorf("longitude: %v", err)
	}
	return config{
		Live: s.live,
		Weather: weatherConfig{
			Enabled:    s.weatherEnabled,
			Latitude:   latitude,
			Longitude:  longitude,
			Location:   strings.TrimSpace(s.location),
			Fahrenheit: s.fahrenheit,
			WindMPH:    s.windMPH,
		},
		Zones:       s.zones,
		Clock24:     s.clock24,
		GaugeStyle:  gaugeOrDefault(s.gauge),
		SparkStyle:  sparkOrDefault(s.spark),
		ClockFont:   clockFontOrDefault(s.clockFont),
		Icons:       iconStyleOrDefault(s.icons),
		PanelGauges: panelOverrides(s.panelGauges),
		PanelSparks: panelOverrides(s.panelSparks),
		AURHelper:   strings.TrimSpace(s.aurHelper),
		Feeds:       joinFeeds(s.feedPresets, s.feeds),
		Calendars:   s.calendars,
		Todo:        s.todo,
		Notes:       s.notes,
		Repos:       normalizeRepoList(s.repos),
		Symbols:     s.symbols,
		Systemd:     s.systemd,
		Docker:      s.docker,
		Interface:   s.iface,
	}, nil
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

// gaugeOrDefault falls back to the solid style when no gauge style is set.
func gaugeOrDefault(style string) string {
	for _, known := range tideui.GaugeStyles() {
		if style == string(known) {
			return style
		}
	}
	return string(tideui.GaugeSolid)
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
}

type settingsView int

const (
	viewCategories settingsView = iota
	viewFields
)

// settingsForm is the modal configuration panel. Every provider setting is
// edited here; nothing requires environment variables or a hand-edited file.
// It is organized as a category list that opens into a page of fields, so a
// long configuration stays readable.
type settingsForm struct {
	opened     bool
	state      *formState
	cfg        config // config produced by the last successful save
	categories []settingsCategory
	view       settingsView
	category   int
	cursor     int
	editing    bool
	caret      int
	problem    string
	dirty      bool

	// Cached repository summary, so a row that renders on every keystroke does
	// not stat the filesystem each time.
	repoSummarySource string
	repoSummaryText   string

	// ws backs the per-category panel toggles, which change live visibility
	// rather than a config value. It is nil when the form runs without a
	// workspace (as in unit tests), which simply omits those fields.
	ws *tideui.Workspace

	pendingLookup string
	lookingUp     bool
}

// SavedConfig returns the config produced by the most recent save.
func (s settingsForm) SavedConfig() config { return s.cfg }

// GaugeStyle returns the gauge style currently selected in the form, so the
// caller can preview it live before it is saved.
func (s settingsForm) GaugeStyle() string {
	if s.state == nil {
		return string(tideui.GaugeSolid)
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

// ClockFont returns the large-clock font currently selected in the form.
// Icons reports the live icon style, so the preview updates while the setting
// is being changed.
func (s settingsForm) Icons() string { return iconStyleOrDefault(s.state.icons) }

func (s settingsForm) ClockFont() string {
	if s.state == nil {
		return string(tideui.ClockFontDash)
	}
	return clockFontOrDefault(s.state.clockFont)
}

func newSettingsForm() *settingsForm { return &settingsForm{} }

// repoSummary describes the configured repositories in the space a row has:
// how many there are, and how many of them are not actually repositories. The
// full value is still what gets edited; showing it raw truncated mid-path and
// pushed the field's own label off the row.
func (s *settingsForm) repoSummary(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none set"
	}
	// Checking paths touches the filesystem, and a row renders on every
	// keystroke, so the answer is kept until the value changes.
	if s.repoSummarySource != value || s.repoSummaryText == "" {
		repos := parseRepoList(value)
		missing, remote := 0, 0
		for _, repo := range repos {
			switch {
			case repo.Remote:
				remote++
			case !repo.IsRepo:
				missing++
			}
		}
		text := fmt.Sprintf("%d %s", len(repos), plural(len(repos), "repo", "repos"))
		if missing > 0 {
			text += fmt.Sprintf(" · %d not found", missing)
		}
		// A clone URL is a different mistake from a wrong path, and saying
		// which one it is saves a round of guessing.
		if remote > 0 {
			text += fmt.Sprintf(" · %d not cloned", remote)
		}
		s.repoSummarySource, s.repoSummaryText = value, text
	}
	return s.repoSummaryText
}

// plural picks a word form for a count.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// SetWorkspace attaches the workspace whose panels the Panels category toggles.
func (s *settingsForm) SetWorkspace(ws *tideui.Workspace) { s.ws = ws }

// Open loads cfg into the form and displays the category list.
func (s *settingsForm) Open(cfg config) {
	state := formFromConfig(cfg)
	s.state = &state
	s.opened = true
	s.categories = s.buildCategories()
	s.view = viewCategories
	s.category = 0
	s.cursor = 0
	s.editing = false
	s.caret = 0
	s.problem = ""
	s.dirty = false
}

// Opened reports whether the panel is displayed.
func (s settingsForm) Opened() bool { return s.opened }

func (s *settingsForm) buildCategories() []settingsCategory {
	categories := []settingsCategory{
		{name: "General", fields: []formField{
			{label: "Live data", kind: fieldBool, flag: &s.state.live},
			{label: "gauge style", kind: fieldChoice, choice: &s.state.gauge, options: gaugeStyleNames(), gaugePreview: true},
			{label: "spark style", kind: fieldChoice, choice: &s.state.spark, options: sparkStyleNames(), sparkPreview: true},
			{label: "clock font", kind: fieldChoice, choice: &s.state.clockFont, options: clockFontNames()},
			{label: "icons", kind: fieldChoice, choice: &s.state.icons, options: iconStyleNames()},
		}},
		{name: "Weather", panelID: "weather", fields: []formField{
			{label: "live weather", kind: fieldBool, flag: &s.state.weatherEnabled},
			{label: "city or ZIP", kind: fieldText, text: &s.state.place},
			{label: "Look up coordinates", kind: fieldAction, action: s.lookupCoordinates},
			{label: "latitude", kind: fieldText, text: &s.state.latitude},
			{label: "longitude", kind: fieldText, text: &s.state.longitude},
			{label: "location", kind: fieldText, text: &s.state.location},
			{label: "fahrenheit", kind: fieldBool, flag: &s.state.fahrenheit},
			{label: "wind mph", kind: fieldBool, flag: &s.state.windMPH},
		}},
		{name: "Agenda", panelID: "agenda", fields: nil},
		{name: "Clock", panelID: "clock", fields: []formField{
			{label: "24-hour", kind: fieldBool, flag: &s.state.clock24},
			{label: "zones", kind: fieldText, text: &s.state.zones},
		}},
		{name: "System", panelID: "system", fields: nil},
		{name: "GPU", panelID: "gpu", fields: nil},
		{name: "Updates", panelID: "updates", fields: []formField{
			{label: "aur helper", kind: fieldText, text: &s.state.aurHelper},
		}},
		{name: "Network", panelID: "network", fields: []formField{
			{label: "interface", kind: fieldText, text: &s.state.iface},
		}},
		{name: "Storage", panelID: "storage", fields: nil},
		{name: "Services", panelID: "services", fields: []formField{
			{label: "systemd units", kind: fieldText, text: &s.state.systemd},
			{label: "docker socket", kind: fieldText, text: &s.state.docker},
		}},
		{name: "News", panelID: "news", fields: s.newsFields()},
		{name: "Calendar", fields: []formField{
			{label: "sources (.ics or URL)", kind: fieldText, text: &s.state.calendars},
		}},
		{name: "Tasks", panelID: "tasks", fields: []formField{
			{label: "todo.txt", kind: fieldText, text: &s.state.todo},
		}},
		{name: "Notes", panelID: "notes", fields: []formField{
			{label: "paths", kind: fieldText, text: &s.state.notes},
		}},
		{name: "Git", panelID: "git", fields: []formField{
			{label: "repositories", kind: fieldText, text: &s.state.repos, summary: s.repoSummary},
		}},
		{name: "Markets", panelID: "markets", fields: []formField{
			{label: "symbols", kind: fieldText, text: &s.state.symbols},
		}},
	}
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
		if gauge := s.panelGaugeField(id); gauge != nil {
			prepend = append(prepend, *gauge)
		}
		if spark := s.panelSparkField(id); spark != nil {
			prepend = append(prepend, *spark)
		}
		if len(prepend) > 0 {
			categories[i].fields = append(prepend, categories[i].fields...)
		}
	}
	return categories
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

func (s *settingsForm) panelField(id string) *formField {
	if s.ws == nil || id == "" {
		return nil
	}
	panel, ok := s.ws.Lookup(id)
	if !ok || !panel.CanHide() {
		return nil
	}
	return &formField{label: "enabled", kind: fieldPanel, panel: id}
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
		s.problem = "enter a city or postal code first"
		return settingsNone
	}
	s.pendingLookup = query
	s.lookingUp = true
	s.problem = "looking up " + query + "…"
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
	s.lookingUp = false
	if err != nil {
		s.problem = err.Error()
		return
	}
	s.applyPlace(place)
}

// applyPlace writes a geocoding result into the form and enables live data,
// since looking up a real place clearly means "use it".
func (s *settingsForm) applyPlace(place provider.Place) {
	s.state.latitude = formatFloat(place.Latitude)
	s.state.longitude = formatFloat(place.Longitude)
	s.state.location = place.Name
	s.state.weatherEnabled = true
	s.state.live = true
	s.dirty = true
	s.problem = "found " + place.Label() + " — ctrl+s to apply"
}

// Update handles all input while the panel is open.
func (s *settingsForm) Update(msg tea.KeyMsg) settingsAction {
	if !s.opened {
		return settingsNone
	}
	key := msg.String()
	if s.editing {
		return s.updateEditing(msg, key)
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
	case "up", "k", "shift+tab":
		s.category = wrapIndex(s.category-1, len(s.categories))
	case "down", "j", "tab":
		s.category = wrapIndex(s.category+1, len(s.categories))
	case "enter", "right", "l", " ":
		if len(s.categories) > 0 {
			s.view = viewFields
			s.cursor = 0
			s.problem = ""
		}
	}
	return settingsNone
}

func (s *settingsForm) updateFields(key string) settingsAction {
	fields := s.currentFields()
	switch key {
	case "esc", "backspace":
		s.view = viewCategories
		s.editing = false
	case "left", "h":
		// Cycle a choice field; otherwise this backs out of the category.
		if field := s.currentField(); field != nil && field.kind == fieldChoice {
			field.setChoice(stepChoice(field.choiceValue(), field.options, -1))
			s.dirty = true
		} else {
			s.view = viewCategories
			s.editing = false
		}
	case "right", "l":
		if field := s.currentField(); field != nil && field.kind == fieldChoice {
			field.setChoice(stepChoice(field.choiceValue(), field.options, 1))
			s.dirty = true
		}
	case "up", "k", "shift+tab":
		s.cursor = wrapIndex(s.cursor-1, len(fields))
	case "down", "j", "tab":
		s.cursor = wrapIndex(s.cursor+1, len(fields))
	case "enter", " ":
		return s.activate()
	}
	return settingsNone
}

func (s *settingsForm) updateEditing(msg tea.KeyMsg, key string) settingsAction {
	field := s.currentField()
	if field == nil || field.text == nil {
		s.editing = false
		return settingsNone
	}
	length := len([]rune(*field.text))
	s.caret = min(max(s.caret, 0), length)
	switch key {
	case "esc":
		s.editing = false
	case "enter":
		s.editing = false
	case "ctrl+s":
		s.editing = false
		return s.save()
	case "left":
		if s.caret > 0 {
			s.caret--
		}
	case "right":
		if s.caret < length {
			s.caret++
		}
	case "home":
		s.caret = 0
	case "end":
		s.caret = length
	case "backspace":
		runes := []rune(*field.text)
		if s.caret > 0 {
			runes = append(runes[:s.caret-1], runes[s.caret:]...)
			s.caret--
			*field.text = string(runes)
			s.dirty = true
		}
	default:
		if msg.Type == tea.KeyRunes {
			runes := []rune(*field.text)
			merged := append([]rune{}, runes[:s.caret]...)
			merged = append(merged, msg.Runes...)
			merged = append(merged, runes[s.caret:]...)
			s.caret += len(msg.Runes)
			*field.text = string(merged)
			s.dirty = true
		}
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
	case fieldText:
		s.editing = true
		s.caret = len([]rune(*field.text))
	case fieldAction:
		if field.action != nil {
			return field.action()
		}
	case fieldPanel:
		if s.ws != nil {
			s.ws.TogglePanel(field.panel)
		}
	case fieldChoice:
		field.setChoice(stepChoice(field.choiceValue(), field.options, 1))
		s.dirty = true
	}
	return settingsNone
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
func (s settingsForm) panelVisible(id string) bool {
	return s.ws == nil || !s.ws.Hidden(id)
}

func (s *settingsForm) save() settingsAction {
	if s.state == nil {
		return settingsNone
	}
	cfg, err := s.state.toConfig()
	if err != nil {
		s.problem = err.Error()
		return settingsNone
	}
	s.problem = ""
	s.opened = false
	s.editing = false
	s.dirty = false
	s.cfg = cfg
	return settingsSaved
}

func (s settingsForm) value(field formField) string {
	switch field.kind {
	case fieldBool:
		if field.flag != nil && *field.flag {
			return "on"
		}
		return "off"
	case fieldText:
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

// Render draws the settings panel as a soft modal overlay.
func (s settingsForm) Render(r tideui.Renderer, width, height int) tideui.Overlay {
	if !s.opened {
		return tideui.Overlay{}
	}
	panelWidth := min(70, max(34, width-4))
	innerWidth := max(1, panelWidth-4)

	var lines []string
	if s.dirty {
		lines = append(lines, r.Styles.StatusNotice.Width(innerWidth).
			Render(" unsaved changes — ctrl+s to apply "))
	}
	if s.problem != "" {
		lines = append(lines, r.Styles.StatusError.Width(innerWidth).Render("  "+s.problem))
	}

	rowsAvailable := max(1, height-8)
	if s.view == viewCategories {
		lines = append(lines, s.renderCategories(r, innerWidth, rowsAvailable)...)
		lines = append(lines, "", r.RenderSoftHints(innerWidth,
			tideui.SoftHint{Key: "↑/↓", Label: "choose"},
			tideui.SoftHint{Key: "enter", Label: "open"},
			tideui.SoftHint{Key: "ctrl+s", Label: "apply"},
			tideui.SoftHint{Key: "esc", Label: "discard"},
		))
	} else {
		lines = append(lines, r.Styles.OverlayTitle.Width(innerWidth).
			Render(strings.ToLower(s.categories[s.category].name)))
		lines = append(lines, s.renderFields(r, innerWidth, rowsAvailable)...)
		lines = append(lines, "", r.RenderSoftHints(innerWidth,
			tideui.SoftHint{Key: "↑/↓", Label: "field"},
			tideui.SoftHint{Key: "enter", Label: "toggle / edit"},
			tideui.SoftHint{Key: "esc", Label: "back"},
			tideui.SoftHint{Key: "ctrl+s", Label: "apply"},
		))
	}
	return r.SoftPanelOverlay(tideui.SoftPanel{
		Prefix:  "tidedeck",
		Title:   "settings",
		Content: r.RenderSoftBody(panelWidth, strings.Join(lines, "\n")),
		Width:   panelWidth,
	})
}

func (s settingsForm) renderCategories(r tideui.Renderer, width, rows int) []string {
	first, last := visibleWindow(len(s.categories), s.category, rows)
	var lines []string
	if first > 0 {
		lines = append(lines, r.Styles.OverlayHint.Width(width).Render("  ▲ more"))
	}
	for index := first; index < last; index++ {
		category := s.categories[index]
		suffix := fmt.Sprintf("%d", len(category.fields))
		if category.panelID != "" && !s.panelVisible(category.panelID) {
			suffix = "off"
		}
		lines = append(lines, r.RenderSoftRow(tideui.SoftRow{
			Prefix:   "  ",
			Text:     category.name,
			Suffix:   suffix,
			Selected: index == s.category,
		}, width))
	}
	if last < len(s.categories) {
		lines = append(lines, r.Styles.OverlayHint.Width(width).Render("  ▼ more"))
	}
	return lines
}

func (s settingsForm) renderFields(r tideui.Renderer, width, rows int) []string {
	fields := s.currentFields()
	first, last := visibleWindow(len(fields), s.cursor, rows)
	var lines []string
	if first > 0 {
		lines = append(lines, r.Styles.OverlayHint.Width(width).Render("  ▲ more"))
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
		case fieldChoice:
			row.Prefix = "    "
			switch {
			case field.gaugePreview:
				style := field.choiceValue()
				if style == "" || style == "default" {
					style = s.GaugeStyle()
				}
				row.Suffix = r.GaugeSample(tideui.GaugeStyle(style), 8)
			case field.sparkPreview:
				style := field.choiceValue()
				if style == "" || style == "default" {
					style = s.SparkStyle()
				}
				row.Suffix = r.SparkSample(tideui.SparklineStyle(style), 8)
			default:
				row.Suffix = "‹ " + s.value(field) + " ›"
			}
		case fieldAction:
			row.Prefix = "  "
			row.Text = "[ " + field.label + " ]"
			row.Accent = true
		default:
			row.Prefix = "    "
		}
		if index == s.cursor && s.editing && field.kind == fieldText && field.text != nil {
			// Editing shows the raw value, not a summary: a field cannot be
			// edited through a description of itself. The window follows the
			// caret so a value longer than the row stays reachable.
			budget := max(12, width-len([]rune(row.Text))-8)
			row.Suffix = editingView(*field.text, s.caret, budget)
		}
		lines = append(lines, r.RenderSoftRow(row, width))
	}
	if last < len(fields) {
		lines = append(lines, r.Styles.OverlayHint.Width(width).Render("  ▼ more"))
	}
	return lines
}

// editingView renders the part of a value around the caret, marking the caret
// where it actually is. The marker used to be appended at the end of the
// value regardless of caret position, so moving left and typing inserted text
// nowhere near the visible cursor.
func editingView(value string, caret, width int) string {
	runes := []rune(value)
	caret = min(max(caret, 0), len(runes))
	if width < 6 {
		width = 6
	}
	budget := width - 1 // the caret marker occupies a cell
	if len(runes) <= budget {
		return string(runes[:caret]) + caretMark + string(runes[caret:])
	}
	start := min(max(caret-budget/2, 0), len(runes)-budget)
	end := start + budget
	head, tail := "", ""
	if start > 0 {
		head, start = "…", start+1
	}
	if end < len(runes) {
		tail, end = "…", end-1
	}
	if start > end {
		start = end
	}
	window := runes[start:end]
	at := min(max(caret-start, 0), len(window))
	return head + string(window[:at]) + caretMark + string(window[at:]) + tail
}

const caretMark = "▏"

func visibleWindow(total, cursor, limit int) (int, int) {
	if limit >= total {
		return 0, total
	}
	start := cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func wrapIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	return (index%length + length) % length
}
