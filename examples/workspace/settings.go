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
)

type formField struct {
	label  string
	kind   fieldKind
	flag   *bool
	text   *string
	action func() settingsAction
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
	feeds          string
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
		feeds:          cfg.Feeds,
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
		Zones:     s.zones,
		Feeds:     s.feeds,
		Calendars: s.calendars,
		Todo:      s.todo,
		Notes:     s.notes,
		Repos:     s.repos,
		Symbols:   s.symbols,
		Systemd:   s.systemd,
		Docker:    s.docker,
		Interface: s.iface,
	}, nil
}

func parseOptionalFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

// settingsCategory groups fields under a heading so the panel stays navigable
// as the number of settings grows.
type settingsCategory struct {
	name   string
	fields []formField
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

	pendingLookup string
	lookingUp     bool
}

// SavedConfig returns the config produced by the most recent save.
func (s settingsForm) SavedConfig() config { return s.cfg }

func newSettingsForm() *settingsForm { return &settingsForm{} }

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
	return []settingsCategory{
		{name: "General", fields: []formField{
			{label: "Live data", kind: fieldBool, flag: &s.state.live},
		}},
		{name: "Weather", fields: []formField{
			{label: "enabled", kind: fieldBool, flag: &s.state.weatherEnabled},
			{label: "latitude", kind: fieldText, text: &s.state.latitude},
			{label: "longitude", kind: fieldText, text: &s.state.longitude},
			{label: "location", kind: fieldText, text: &s.state.location},
			{label: "fahrenheit", kind: fieldBool, flag: &s.state.fahrenheit},
			{label: "wind mph", kind: fieldBool, flag: &s.state.windMPH},
			{label: "city or ZIP", kind: fieldText, text: &s.state.place},
			{label: "Look up coordinates", kind: fieldAction, action: s.lookupCoordinates},
		}},
		{name: "Clock", fields: []formField{
			{label: "zones", kind: fieldText, text: &s.state.zones},
		}},
		{name: "News", fields: []formField{
			{label: "feeds", kind: fieldText, text: &s.state.feeds},
		}},
		{name: "Calendar", fields: []formField{
			{label: ".ics files", kind: fieldText, text: &s.state.calendars},
		}},
		{name: "Tasks", fields: []formField{
			{label: "todo.txt", kind: fieldText, text: &s.state.todo},
		}},
		{name: "Notes", fields: []formField{
			{label: "paths", kind: fieldText, text: &s.state.notes},
		}},
		{name: "Git", fields: []formField{
			{label: "repository paths", kind: fieldText, text: &s.state.repos},
		}},
		{name: "Markets", fields: []formField{
			{label: "symbols", kind: fieldText, text: &s.state.symbols},
		}},
		{name: "Services", fields: []formField{
			{label: "systemd units", kind: fieldText, text: &s.state.systemd},
			{label: "docker socket", kind: fieldText, text: &s.state.docker},
		}},
		{name: "Network", fields: []formField{
			{label: "interface", kind: fieldText, text: &s.state.iface},
		}},
	}
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
	case "esc", "left", "h", "backspace":
		s.view = viewCategories
		s.editing = false
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
	}
	return settingsNone
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
			return *field.text
		}
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
			tideui.SoftHint{Key: "enter", Label: "edit / run"},
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
		lines = append(lines, r.RenderSoftRow(tideui.SoftRow{
			Prefix:   "  ",
			Text:     category.name,
			Suffix:   fmt.Sprintf("%d", len(category.fields)),
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
		case fieldAction:
			row.Prefix = "  ▸ "
		default:
			row.Prefix = "    "
		}
		if index == s.cursor && s.editing && field.kind == fieldText {
			row.Suffix = s.value(field) + "▏"
		}
		lines = append(lines, r.RenderSoftRow(row, width))
	}
	if last < len(fields) {
		lines = append(lines, r.Styles.OverlayHint.Width(width).Render("  ▼ more"))
	}
	return lines
}

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
