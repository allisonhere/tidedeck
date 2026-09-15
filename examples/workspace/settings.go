package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
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

// settingsForm is the modal configuration panel. Every provider setting is
// edited here; nothing requires environment variables or a hand-edited file.
type settingsForm struct {
	opened  bool
	state   *formState
	cfg     config // config produced by the last successful save
	fields  []formField
	cursor  int
	editing bool
	caret   int
	problem string
}

// SavedConfig returns the config produced by the most recent save.
func (s settingsForm) SavedConfig() config { return s.cfg }

func newSettingsForm() *settingsForm { return &settingsForm{} }

// Open loads cfg into the form and displays it.
func (s *settingsForm) Open(cfg config) {
	state := formFromConfig(cfg)
	s.state = &state
	s.opened = true
	s.editing = false
	s.caret = 0
	s.problem = ""
	s.cursor = 0
	s.fields = s.buildFields()
}

// Opened reports whether the panel is displayed.
func (s settingsForm) Opened() bool { return s.opened }

func (s *settingsForm) buildFields() []formField {
	return []formField{
		{label: "Live data", kind: fieldBool, flag: &s.state.live},
		{label: "Weather · enabled", kind: fieldBool, flag: &s.state.weatherEnabled},
		{label: "Weather · latitude", kind: fieldText, text: &s.state.latitude},
		{label: "Weather · longitude", kind: fieldText, text: &s.state.longitude},
		{label: "Weather · location", kind: fieldText, text: &s.state.location},
		{label: "Weather · fahrenheit", kind: fieldBool, flag: &s.state.fahrenheit},
		{label: "Weather · wind mph", kind: fieldBool, flag: &s.state.windMPH},
		{label: "Clock · zones", kind: fieldText, text: &s.state.zones},
		{label: "News · feeds", kind: fieldText, text: &s.state.feeds},
		{label: "Calendar · .ics", kind: fieldText, text: &s.state.calendars},
		{label: "Tasks · todo.txt", kind: fieldText, text: &s.state.todo},
		{label: "Notes · paths", kind: fieldText, text: &s.state.notes},
		{label: "Git · repository paths", kind: fieldText, text: &s.state.repos},
		{label: "Markets · symbols", kind: fieldText, text: &s.state.symbols},
		{label: "Services · units", kind: fieldText, text: &s.state.systemd},
		{label: "Services · docker", kind: fieldText, text: &s.state.docker},
		{label: "Network · interface", kind: fieldText, text: &s.state.iface},
		{label: "Save & apply", kind: fieldAction, action: func() settingsAction { return s.save() }},
		{label: "Discard & close", kind: fieldAction, action: func() settingsAction { return settingsCancelled }},
	}
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
	switch key {
	case "esc", "q":
		s.opened = false
		return settingsCancelled
	case "up", "k", "shift+tab":
		s.move(-1)
	case "down", "j", "tab":
		s.move(1)
	case "ctrl+s":
		return s.save()
	case "enter", " ":
		return s.activate()
	}
	return settingsNone
}

func (s *settingsForm) updateEditing(msg tea.KeyMsg, key string) settingsAction {
	field := &s.fields[s.cursor]
	if field.text != nil {
		length := len([]rune(*field.text))
		if s.caret > length {
			s.caret = length
		}
		if s.caret < 0 {
			s.caret = 0
		}
	}
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
		if s.caret < len([]rune(*field.text)) {
			s.caret++
		}
	case "home":
		s.caret = 0
	case "end":
		s.caret = len([]rune(*field.text))
	case "backspace":
		runes := []rune(*field.text)
		if s.caret > 0 {
			runes = append(runes[:s.caret-1], runes[s.caret:]...)
			s.caret--
			*field.text = string(runes)
		}
	default:
		if msg.Type == tea.KeyRunes {
			runes := []rune(*field.text)
			insert := msg.Runes
			merged := append([]rune{}, runes[:s.caret]...)
			merged = append(merged, insert...)
			merged = append(merged, runes[s.caret:]...)
			s.caret += len(insert)
			*field.text = string(merged)
		}
	}
	return settingsNone
}

func (s *settingsForm) move(delta int) {
	s.cursor = (s.cursor + delta + len(s.fields)) % len(s.fields)
	s.editing = false
}

func (s *settingsForm) activate() settingsAction {
	field := s.fields[s.cursor]
	switch field.kind {
	case fieldBool:
		*field.flag = !*field.flag
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
	panelWidth := min(74, max(36, width-4))
	innerWidth := max(1, panelWidth-4)
	rowsAvailable := max(1, height-6)
	first, last := visibleWindow(len(s.fields), s.cursor, rowsAvailable)

	var lines []string
	if s.problem != "" {
		lines = append(lines, r.Styles.StatusError.Width(innerWidth).Render("  "+s.problem))
	}
	for index := first; index < last; index++ {
		field := s.fields[index]
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
		default:
			row.Prefix = "    "
		}
		if index == s.cursor && s.editing && field.kind == fieldText {
			row.Suffix = s.value(field) + "▏"
		}
		lines = append(lines, r.RenderSoftRow(row, innerWidth))
	}
	lines = append(lines, "", r.RenderSoftHints(innerWidth,
		tideui.SoftHint{Key: "↑/↓", Label: "move"},
		tideui.SoftHint{Key: "enter", Label: "edit/toggle"},
		tideui.SoftHint{Key: "ctrl+s", Label: "save"},
		tideui.SoftHint{Key: "esc", Label: "close"},
	))
	return r.SoftPanelOverlay(tideui.SoftPanel{
		Prefix:  "tidedeck",
		Title:   "settings",
		Content: r.RenderSoftBody(panelWidth, strings.Join(lines, "\n")),
		Width:   panelWidth,
	})
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
