// Command workspace (TideDeck) is a dense, glanceable terminal information
// dashboard built on the TideUI workspace. It demonstrates reusable dashboard
// widgets, a deterministic fake data source, semantic responsive layout,
// presets, a drill-down (zoom) pattern, density modes, contextual actions,
// panel picker, command palette, and live rearrangement.
//
// Run with: go run ./examples/workspace
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/dash/panels"
	"github.com/allisonhere/tideui/provider"
)

// fileStore is a tiny durable LayoutStore so the demo persists its layout.
type fileStore struct{ path string }

func (s fileStore) Load(key string) ([]byte, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	raw, ok := all[key]
	if !ok {
		return nil, fmt.Errorf("no layout for %q", key)
	}
	return raw, nil
}

func (s fileStore) Save(key string, value []byte) error {
	all := map[string]json.RawMessage{}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	all[key] = json.RawMessage(value)
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, out, 0o644)
}

// demoState is the mutable demo data. Time-varying widgets read from the feed;
// collections the user can act on are held here so actions persist.
type demoState struct {
	theme   tideui.Theme
	density tideui.Density
	now     time.Time
	source  dataSource
	live    *liveSource // non-nil when live data is enabled in settings
	status  string

	gauge     tideui.GaugeStyle
	spark     tideui.SparklineStyle
	clockFont tideui.ClockFont
	icons     tideui.IconStyle

	lastStatus string
	statusAge  int

	tasks []tideui.Task
	notes []tideui.Note
}

type model struct {
	width, height int
	state         *demoState
	ws            *tideui.Workspace
	picker        tideui.ThemePicker

	cfg      config
	feed     *demoFeed
	settings *settingsForm
	// deck holds the panels that own their own data, rendering and settings.
	// Panels are migrated onto it one at a time; everything not yet migrated
	// still goes through demoState and the provider snapshot below.
	deck *dash.Deck

	// pickerTarget is "" when the picker is editing the workspace theme, or a
	// panel id when it is editing that panel's theme. pickerPrev/pickerHad
	// preserve the panel's previous theme so Escape can restore it.
	pickerTarget string
	pickerPrev   tideui.Theme
	pickerHad    bool
}

type tickMsg time.Time

func tickCmd(rate time.Duration) tea.Cmd {
	return tea.Tick(rate, func(time.Time) tea.Msg { return tickMsg(time.Time{}) })
}

func newModel() model {
	started := time.Now()
	feed := newDemoFeed(7, started)
	cfg := loadConfig()
	var source dataSource = feed
	state := &demoState{
		theme: tideui.CatppuccinMocha, density: tideui.Dense, now: started, source: source,
		gauge:     tideui.GaugeStyle(gaugeOrDefault(cfg.GaugeStyle)),
		spark:     tideui.SparklineStyle(sparkOrDefault(cfg.SparkStyle)),
		clockFont: tideui.ClockFont(clockFontOrDefault(cfg.ClockFont)),
		icons:     tideui.IconStyle(iconStyleOrDefault(cfg.Icons)),
		tasks:     feed.Tasks(),
		notes:     feed.Notes(),
	}
	store := fileStore{path: filepath.Join(userConfigDir(), "tidedeck", "layout.json")}
	ws := tideui.NewWorkspace(
		tideui.WithPersistence("tidedeck-demo"),
		tideui.WithStore(store),
		// Columns keep a one-cell gutter; stacked panels sit flush so there is
		// no blank row between rows.
		tideui.WithGaps(1, 0),
		tideui.WithAdaptiveLayout(),
		tideui.WithAnimation(tideui.AnimationOptions{Enabled: true, TickRate: time.Second}),
		// A dashboard focus language: an accent marker, a soft accent border,
		// and a thin rail instead of a filled title capsule, so the focused
		// panel stands out without overpowering the glanceable content.
		tideui.WithFocusPresentation(tideui.FocusPresentation{
			ActiveBorder: true, ActiveTitle: true, TitleCapsule: false,
			FocusRail: true, DimInactive: true, MutedSecondary: true,
			AccentMarker: true, InactiveSelection: true,
			StatusStrip: true, KeyHints: true,
		}),
	)

	deck := dash.New()
	// Panels are registered in the order the hand-written registrations used to
	// sit, so the deck attaches them — and the settings list orders them — the
	// way the dashboard always did.
	deck.Register(panels.Agenda(), panels.System(), panels.Weather(), panels.GPU(), panels.Updates(), panels.Clock(), panels.Git(), panels.News(), panels.Network(), panels.Storage(), panels.Services())
	deck.OnStatus(func(message string) { state.status = message })
	registerPanels(ws, state, deck)
	registerPresets(ws)

	ws.Layout(overviewLayout())
	ws.ApplyPreset("Overview")
	ws.Focus("agenda")

	settings := newSettingsForm()
	settings.SetWorkspace(ws)
	settings.SetDeck(deck)

	m := model{
		state:    state,
		deck:     deck,
		ws:       ws,
		picker:   tideui.NewThemePicker(tideui.ThemePickerOptions{InitialTheme: state.theme.Name}),
		cfg:      cfg,
		feed:     feed,
		settings: settings,
	}
	// Apply the saved configuration the same way a save does. Doing it here
	// rather than repeating a shorter version of it is what makes a
	// configured panel actually configured on the first frame: until this
	// call the deck has neither its settings nor its mode, so a dashboard
	// started with live data on used to sit in demo mode until you opened
	// settings and saved.
	m.applyConfig()
	return m
}

// applyConfig switches the data source to match the saved configuration.
// deckMode maps the live-data setting onto the deck's mode.
func deckMode(live bool) dash.Mode {
	if live {
		return dash.ModeLive
	}
	return dash.ModeDemo
}

// values is the whole configuration document: the keys the typed config still
// owns, plus the keys panels own. Panels are configured from this rather than
// from the struct, so a setting that has moved onto a panel has exactly one
// source of truth.
func (m *model) values() dash.Values {
	document, err := m.cfg.document()
	if err != nil {
		return dash.NewValues()
	}
	return document
}

func (m *model) applyConfig() {
	m.state.gauge = tideui.GaugeStyle(gaugeOrDefault(m.cfg.GaugeStyle))
	m.state.spark = tideui.SparklineStyle(sparkOrDefault(m.cfg.SparkStyle))
	m.state.clockFont = tideui.ClockFont(clockFontOrDefault(m.cfg.ClockFont))
	m.state.icons = tideui.IconStyle(iconStyleOrDefault(m.cfg.Icons))
	applyPanelGauges(m.ws, m.cfg)
	applyPanelSparks(m.ws, m.cfg)
	m.deck.SetMode(deckMode(m.cfg.Live))
	m.deck.Configure(m.values())
	if m.cfg.Live {
		m.state.live = newLiveSource(m.cfg)
		m.state.source = m.state.live
		m.state.tasks = nil
		m.state.notes = nil
	} else {
		m.state.live = nil
		m.state.source = m.feed
		m.state.tasks = m.feed.Tasks()
		m.state.notes = m.feed.Notes()
	}
	// Give the deck's panels their first data now rather than on the tick a
	// second from now: a panel that holds its own data renders empty until
	// something fills it, and the first frame is drawn before that tick.
	m.deck.Tick(m.state.now)
	// Deck badges are read from the panels, so apply them now too; otherwise a
	// migrated panel loses the header badge its hand-written registration used
	// to set directly, until the first tick a second later.
	m.refreshBadges()
}

// applyStylePreview mirrors the settings form's current metric styles onto the
// live workspace so the dashboard updates as each style is cycled.
func (m *model) applyStylePreview() {
	m.state.gauge = tideui.GaugeStyle(m.settings.GaugeStyle())
	m.state.spark = tideui.SparklineStyle(m.settings.SparkStyle())
	m.state.clockFont = tideui.ClockFont(m.settings.ClockFont())
	m.state.icons = tideui.IconStyle(m.settings.Icons())
	for _, panel := range m.ws.Panels() {
		id := panel.ID()
		switch style := m.settings.PanelGaugeStyle(id); style {
		case "", "default":
			panel.ClearGauge()
		default:
			panel.Gauge(tideui.GaugeStyle(style))
		}
		switch style := m.settings.PanelSparkStyle(id); style {
		case "", "default":
			panel.ClearSparkline()
		default:
			panel.Sparkline(tideui.SparklineStyle(style))
		}
	}
}

// applyPanelGauges gives each panel its configured gauge style, falling back to
// the workspace default for "default" or unset entries.
func applyPanelGauges(ws *tideui.Workspace, cfg config) {
	for _, panel := range ws.Panels() {
		switch style := cfg.PanelGauges[panel.ID()]; style {
		case "", "default":
			panel.ClearGauge()
		default:
			panel.Gauge(tideui.GaugeStyle(style))
		}
	}
}

// applyPanelSparks gives each panel its configured sparkline style.
func applyPanelSparks(ws *tideui.Workspace, cfg config) {
	for _, panel := range ws.Panels() {
		switch style := cfg.PanelSparks[panel.ID()]; style {
		case "", "default":
			panel.ClearSparkline()
		default:
			panel.Sparkline(tideui.SparklineStyle(style))
		}
	}
}

func registerPanels(ws *tideui.Workspace, state *demoState, deck *dash.Deck) {
	// Registered panels keep their declared order, so the deck attaches here
	// rather than before or after the block, leaving the picker and settings
	// lists unchanged.
	deck.Attach(ws)

	ws.Panel("tasks", tasksPanel(state)).
		Title("Tasks").Role(tideui.RoleSecondary).Priority(72).MinWidth(20).MinHeight(6).HideBelow(88).
		Actions(
			tideui.Action("toggle", "space", func(*tideui.Workspace) {
				for i := range state.tasks {
					if !state.tasks[i].Done {
						state.tasks[i].Done = true
						state.status = "completed: " + state.tasks[i].Title
						return
					}
				}
				state.status = "no open tasks"
			}).Labeled("toggle"),
			tideui.Action("add", "a", func(*tideui.Workspace) {
				state.tasks = append(state.tasks, tideui.Task{Title: "New task", Due: "today", Tone: tideui.ToneAccent})
				state.status = "task added"
			}).Labeled("add"),
			tideui.Action("edit", "e", func(*tideui.Workspace) { state.status = "editing task" }).Labeled("edit"),
		)

	ws.Panel("notes", notesPanel(state)).
		Title("Notes").Role(tideui.RoleOptional).Priority(45).MinWidth(18).MinHeight(5).HideBelow(130).
		Actions(tideui.Action("edit", "e", func(*tideui.Workspace) { state.status = "editing note" }).Labeled("edit"))

	// Markets ships with its own contrasting theme to show that a panel can
	// opt out of the workspace palette entirely. Press T / ctrl+T to assign or
	// clear the focused panel's theme at runtime.
	ws.Panel("markets", marketsPanel(state)).
		Title("Markets").Role(tideui.RoleOptional).Priority(50).MinWidth(18).MinHeight(6).HideBelow(160).
		Theme(tideui.GruvboxLight).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "quotes refreshed" }).Labeled("refresh"))
}

// overviewLayout balances the dashboard by content density: dense panels
// (Calendar, News) get taller rows and more width, while the sparse bottom row
// (Tasks, Storage, Clock) is deliberately short so it does not stretch.
func overviewLayout() tideui.LayoutNode {
	return tideui.VStack(
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("weather"),
			tideui.Weighted(tideui.Leaf("agenda"), 2),
			tideui.Leaf("system"),
		), 6),
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("services"),
			tideui.Weighted(tideui.Leaf("news"), 2),
			tideui.Leaf("network"),
		), 6),
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("tasks"),
			tideui.Leaf("storage"),
			tideui.Leaf("gpu"),
			tideui.Leaf("clock"),
		), 4),
	)
}

func registerPresets(ws *tideui.Workspace) {
	ws.AddPreset("Overview", overviewLayout(),
		"notes", "git", "markets", "updates")
	ws.AddPreset("System", tideui.VStack(
		tideui.Weighted(tideui.HStack(
			tideui.Weighted(tideui.Leaf("system"), 2), tideui.Leaf("gpu"), tideui.Leaf("network"),
		), 6),
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("storage"), tideui.Leaf("services"), tideui.Leaf("updates"),
		), 4),
	), "weather", "agenda", "news", "tasks", "notes", "git", "markets", "clock")
	ws.AddPreset("Productivity", tideui.HStack(
		tideui.Weighted(tideui.VStack(tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("tasks")), 2),
		tideui.VStack(tideui.Leaf("notes"), tideui.Leaf("clock")),
	), "weather", "system", "gpu", "network", "storage", "services", "news", "git", "markets", "updates")
	ws.AddPreset("Developer", tideui.VStack(
		tideui.HStack(tideui.Leaf("git"), tideui.Leaf("system")),
		tideui.HStack(tideui.Weighted(tideui.Leaf("services"), 2), tideui.Leaf("news")),
	), "weather", "agenda", "clock", "gpu", "network", "storage", "tasks", "notes", "markets", "updates")
	ws.AddPreset("Minimal", tideui.HStack(
		tideui.Leaf("clock"), tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("weather"),
	), "system", "gpu", "network", "storage", "services", "news", "tasks", "notes", "git", "markets", "updates")
}

func (m model) Init() tea.Cmd { return tickCmd(time.Second) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.state.now = time.Now()
		// Panels on the deck fetch on their own intervals and hold their own
		// data; the snapshot copying below is the path not yet migrated.
		m.deck.Refresh(context.Background(), m.state.now)
		m.deck.Tick(m.state.now)
		if m.state.live != nil {
			m.state.live.refresh(context.Background())
			snapshot := m.state.live.snapshotCopy()
			if snapshot.Tasks != nil {
				m.state.tasks = snapshot.Tasks
			}
			if snapshot.Notes != nil {
				m.state.notes = snapshot.Notes
			}
		}
		// Auto-clear transient status feedback a few seconds after it stops
		// changing, so it is prominent but never sticks around.
		if m.state.status != m.state.lastStatus {
			m.state.lastStatus = m.state.status
			m.state.statusAge = 0
		} else if m.state.status != "" {
			m.state.statusAge++
			if m.state.statusAge > 6 {
				m.state.status = ""
				m.state.lastStatus = ""
				m.state.statusAge = 0
			}
		}
		m.refreshBadges()
		m.ws.Animation().Tick()
		return m, tickCmd(time.Second)
	case lookupMsg:
		if m.settings.Opened() {
			m.settings.ApplyLookup(msg.place, msg.err)
		}
		return m, nil
	case tea.MouseMsg:
		if m.ws.HandleMouse(msg) {
			return m, nil
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

type lookupMsg struct {
	place provider.Place
	err   error
}

// lookupCmd runs a geocoding request off the UI goroutine and delivers the
// result as a message.
func lookupCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		place, err := provider.Geocode(ctx, query)
		return lookupMsg{place: place, err: err}
	}
}

func (m model) refreshBadges() {
	// Panels on the deck advertise their own badges.
	for id, badge := range m.deck.Badges() {
		if panel, ok := m.ws.Lookup(id); ok {
			panel.Badge(badge.Text).BadgeTone(badge.Tone)
		}
	}
	open := 0
	for _, t := range m.state.tasks {
		if !t.Done {
			open++
		}
	}
	if panel, ok := m.ws.Lookup("tasks"); ok {
		if open > 0 {
			panel.Badge(fmt.Sprintf("%d", open)).BadgeTone(tideui.ToneMuted)
		} else {
			panel.Badge("")
		}
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settings.Opened() {
		action := m.settings.Update(msg)
		// Preview the chosen gauge styles live, so every style is visible as it
		// is cycled rather than only after ctrl+s.
		m.applyStylePreview()
		if query := m.settings.TakeLookup(); query != "" {
			return m, lookupCmd(query)
		}
		switch action {
		case settingsSaved:
			m.cfg = m.settings.SavedConfig()
			if err := m.cfg.save(); err != nil {
				m.state.status = "config save failed: " + err.Error()
			} else {
				m.state.status = "settings applied"
			}
			m.applyConfig()
		case settingsCancelled:
			m.applyConfig()
			m.state.status = "settings unchanged"
		}
		return m, nil
	}
	if m.picker.Opened() {
		m.updateThemePicker(msg)
		return m, nil
	}
	if m.ws.PanelPicker().Opened() || m.ws.CommandPalette().Opened() {
		m.ws.HandleKey(msg)
		return m, nil
	}

	if !m.ws.Arranging() {
		switch msg.String() {
		case "q", "ctrl+c":
			_ = m.ws.Persist()
			return m, tea.Quit
		case "t":
			m.openWorkspaceThemePicker()
			return m, nil
		case "s":
			m.settings.Open(m.cfg)
			m.state.status = "settings"
			return m, nil
		case "d":
			m.state.density = nextDensity(m.state.density)
			return m, nil
		case "enter":
			// Drill-down pattern: Enter zooms the focused panel into detail,
			// Esc (handled by the workspace) returns.
			if m.ws.Zoomed() != "" {
				m.ws.Unzoom()
			} else {
				m.ws.Zoom(m.ws.Focused())
			}
			return m, nil
		case "T":
			m.openPanelThemePicker()
			return m, nil
		case "ctrl+t":
			if panel, ok := m.ws.Lookup(m.ws.Focused()); ok {
				panel.ClearTheme()
				m.state.status = "theme cleared → " + panel.TitleText()
			}
			return m, nil
		case "shift+p":
			if m.ws.Peeked() == "" {
				m.ws.Peek("notes")
			} else {
				m.ws.Unpeek()
			}
			return m, nil
		case "up":
			m.ws.FocusDirection(tideui.DirUp)
			return m, nil
		case "down":
			m.ws.FocusDirection(tideui.DirDown)
			return m, nil
		case "left":
			m.ws.FocusDirection(tideui.DirLeft)
			return m, nil
		case "right":
			m.ws.FocusDirection(tideui.DirRight)
			return m, nil
		case "1", "2", "3", "4", "5":
			names := m.ws.PresetNames()
			index := int(msg.String()[0] - '1')
			if index >= 0 && index < len(names) {
				m.ws.ApplyPreset(names[index])
				m.state.status = "preset: " + names[index]
			}
			return m, nil
		}
	}
	m.ws.HandleKey(msg)
	return m, nil
}

// openWorkspaceThemePicker edits the workspace-wide theme with live preview.
func (m *model) openWorkspaceThemePicker() {
	m.pickerTarget = ""
	m.pickerHad = false
	m.picker.SetTitle("Theme")
	m.picker.Open(m.state.theme.Name)
}

// openPanelThemePicker edits the focused panel's theme with live preview.
// Escape restores the panel's previous theme (or clears it if it had none).
func (m *model) openPanelThemePicker() {
	focus := m.ws.Focused()
	panel, ok := m.ws.Lookup(focus)
	if !ok {
		return
	}
	m.pickerTarget = focus
	if theme, has := panel.PanelTheme(); has {
		m.pickerPrev, m.pickerHad = theme, true
		m.picker.Open(theme.Name)
	} else {
		m.pickerPrev, m.pickerHad = tideui.Theme{}, false
		m.picker.Open(m.state.theme.Name)
	}
	m.picker.SetTitle("Theme · " + panel.TitleText())
	m.state.status = "pick a theme for " + panel.TitleText()
}

// updateThemePicker previews live, keeps the preview on confirm, and restores
// the previous theme on cancel.
func (m *model) updateThemePicker(msg tea.KeyMsg) {
	action := m.picker.Update(msg)
	switch action {
	case tideui.ThemePickerConfirm:
		theme := m.picker.ConfirmedTheme()
		m.applyPickedTheme(theme)
		if m.pickerTarget == "" {
			m.state.status = "workspace theme: " + theme.Name
		} else if panel, ok := m.ws.Lookup(m.pickerTarget); ok {
			m.state.status = "theme → " + panel.TitleText()
		}
		m.pickerTarget = ""
	case tideui.ThemePickerCancel:
		m.restorePickedTheme()
		m.pickerTarget = ""
	default:
		m.applyPickedTheme(m.picker.PreviewTheme())
	}
}

func (m *model) applyPickedTheme(theme tideui.Theme) {
	if m.pickerTarget == "" {
		m.state.theme = theme
		return
	}
	if panel, ok := m.ws.Lookup(m.pickerTarget); ok {
		panel.Theme(theme)
	}
}

func (m *model) restorePickedTheme() {
	if m.pickerTarget == "" {
		m.state.theme = m.picker.ConfirmedTheme()
		return
	}
	panel, ok := m.ws.Lookup(m.pickerTarget)
	if !ok {
		return
	}
	if m.pickerHad {
		panel.Theme(m.pickerPrev)
	} else {
		panel.ClearTheme()
	}
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	renderer := tideui.NewRenderer(m.state.theme, tideui.StyleOptions{
		Density: m.state.density, PaneCorners: tideui.RoundCorners,
		Gauge: m.state.gauge, Sparkline: m.state.spark, ClockFont: m.state.clockFont,
		IconStyle:   m.state.icons,
		ModalShadow: true,
	})
	wr := tideui.NewWorkspaceRenderer(renderer)

	primary := "tideDeck  ·  " + m.state.theme.Name + "  ·  " + string(m.state.density)
	if preset := m.ws.ActivePreset(); preset != "" {
		primary += "  ·  " + preset
	}
	dataLabel := "demo data"
	if m.state.live != nil {
		dataLabel = "live"
	}
	secondary := "updated " + m.state.now.Format("15:04") + "  ·  " + dataLabel
	if panel, ok := m.ws.Lookup(m.ws.Focused()); ok {
		if theme, has := panel.PanelTheme(); has {
			secondary = panel.TitleText() + "[" + theme.Name + "]  ·  " + secondary
		}
	}
	wr.Options.StatusLeft = primary
	wr.Options.StatusSecondary = secondary
	wr.Options.StatusHints = []tideui.KeyHint{tideui.Hint("s", "settings")}
	// Status messages show as the strip's capsule, so they stay visible even
	// when the secondary metadata is truncated on a narrow terminal.
	wr.Options.StatusNotice = m.state.status
	base := wr.Render(m.ws, m.width, m.height)

	if m.settings.Opened() {
		overlay := m.settings.Render(renderer, m.width, m.height)
		if overlay.Visible {
			return renderer.OverlayModal(base, overlay.Content, m.width, m.height)
		}
	}
	if overlay := m.ws.Overlay(renderer); overlay != nil && overlay.Visible {
		return renderer.OverlayModal(base, overlay.Content, m.width, m.height)
	}
	if m.picker.Opened() {
		overlay := m.picker.SoftModal(renderer, min(48, m.width-4), m.height, "tidedeck")
		return renderer.OverlayModal(base, overlay.Content, m.width, m.height)
	}
	return base
}

func main() {
	program := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func nextDensity(d tideui.Density) tideui.Density {
	switch d {
	case tideui.Comfortable:
		return tideui.Compact
	case tideui.Compact:
		return tideui.Dense
	default:
		return tideui.Comfortable
	}
}

func userConfigDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return dir
	}
	return "."
}
