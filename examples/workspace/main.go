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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// demoState is the application's mutable state: the current time, the
// presentation styles the settings screen edits, and transient status. Panel
// data lives on the deck, not here.
type demoState struct {
	theme tideui.Theme
	// layoutThemeKey tracks which preset or saved slot supplied the current
	// workspace theme. It is runtime state, not a configuration key.
	layoutThemeKey string
	density        tideui.Density
	now            time.Time
	status         string
	// clipboard is text to write as an OSC 52 sequence in the next frame. It is
	// carried through the View rather than written from a command, so it goes
	// out in the same buffered write as the frame and cannot be split by it.
	clipboard string

	gauge     tideui.GaugeStyle
	spark     tideui.SparklineStyle
	clockFont tideui.ClockFont
	icons     tideui.IconStyle

	lastStatus string
	statusAge  int
}

type model struct {
	width, height int
	state         *demoState
	ws            *tideui.Workspace
	picker        tideui.ThemePicker

	cfg      config
	settings *settingsForm
	// deck holds the panels that own their own data, rendering and settings.
	deck *dash.Deck
	// paneSizes is the size each pane was last drawn at, by panel id: a panel
	// that sizes its fetches from its pane is due a fetch when the pane changes,
	// and this is how the app notices - the panel cannot, it is only drawn.
	paneSizes map[string][2]int

	// pickerTarget is "" when the picker is editing the workspace theme, or a
	// panel id when it is editing that panel's theme. pickerPrev/pickerHad
	// preserve the panel's previous theme so Escape can restore it.
	pickerTarget string
	pickerPrev   tideui.Theme
	pickerHad    bool

	// editing is true while a focused panel that accepts typing owns the
	// keyboard, and editTarget is the panel it belongs to so a focus change
	// (by mouse, say) ends the session. Without the distinction a text field
	// would swallow the single-key shortcuts the moment it is focused.
	editing    bool
	editTarget string

	// The five layout slots. Each starts as the built-in preset at the same
	// index and alt+1..5 loads the slot, saved or not. Saving overwrites a
	// slot with the current layout; saved slots live under their own store key
	// so they survive a restart. activeSlot is the slot last loaded (0 when a
	// named preset is active) and drives the status strip.
	store        tideui.LayoutStore
	slots        [slotCount][]byte
	slotSaved    [slotCount]bool
	slotOpen     bool
	slotCursor   int
	activeSlot   int
	layoutThemes map[string]string
}

// slotCount is how many saveable layout slots the alt+1..5 keys address.
const slotCount = 5

type tickMsg time.Time

func tickCmd(rate time.Duration) tea.Cmd {
	return tea.Tick(rate, func(time.Time) tea.Msg { return tickMsg(time.Time{}) })
}

func newModel() model {
	started := time.Now()
	cfg := loadConfig()
	state := &demoState{
		theme: tideui.CatppuccinMocha, density: tideui.Dense, now: started,
		gauge:     tideui.GaugeStyle(gaugeOrDefault(cfg.GaugeStyle)),
		spark:     tideui.SparklineStyle(sparkOrDefault(cfg.SparkStyle)),
		clockFont: tideui.ClockFont(clockFontOrDefault(cfg.ClockFont)),
		icons:     tideui.IconStyle(iconStyleOrDefault(cfg.Icons)),
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
	deck.Register(panels.Agenda(), panels.System(), panels.Weather(), panels.Radar(), panels.GPU(), panels.Updates(), panels.Clock(), panels.Git(), panels.News(), panels.Network(), panels.Storage(), panels.Services(), panels.Tasks(), panels.Notes(), panels.Markets(), panels.Calculator())

	// Plugins are discovered once, at startup: drop a directory into
	// <config>/tidedeck/plugins to add one, delete it to remove one. A
	// manifest that does not validate is skipped with its reason rather than
	// failing the rest, and a plugin starts hidden until it is enabled from
	// the panel picker or settings.
	plugins, problems := dash.LoadPlugins(pluginsDir())
	deck.Register(plugins...)
	deck.OnStatus(func(message string) { state.status = message })
	switch {
	case len(problems) > 0:
		state.status = "plugin skipped: " + problems[0].Error()
	case len(plugins) > 0:
		state.status = "plugin loaded: " + plugins[0].Meta().Title
	}
	deck.Attach(ws)
	registerPresets(ws)

	ws.Layout(overviewLayout())
	ws.ApplyPreset("Overview")
	ws.Focus("agenda")

	settings := newSettingsForm()
	settings.SetWorkspace(ws)
	settings.SetDeck(deck)

	m := model{
		state:        state,
		deck:         deck,
		paneSizes:    map[string][2]int{},
		ws:           ws,
		picker:       tideui.NewThemePicker(tideui.ThemePickerOptions{InitialTheme: state.theme.Name}),
		cfg:          cfg,
		settings:     settings,
		store:        store,
		layoutThemes: copyStringMap(cfg.LayoutThemes),
	}
	m.syncLayoutTheme()
	m.loadSlots()
	// Apply the saved configuration the same way a save does. Doing it here
	// rather than repeating a shorter version of it is what makes a
	// configured panel actually configured on the first frame: until this
	// call the deck has neither its settings nor its mode, so a dashboard
	// started with live data on used to sit in demo mode until you opened
	// settings and saved.
	m.applyConfig()
	return m
}

// applyConfig hands the saved configuration to the deck and the styles.
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
	applyPanelGlyphs(m.ws, m.deck, m.cfg)
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
	applyPanelGlyphs(m.ws, m.deck, config{GlyphMode: m.settings.GlyphMode(), PanelGlyphs: m.settings.state.panelGlyphs})
}

func glyphsShown(cfg config, id string) bool {
	switch glyphModeOrDefault(cfg.GlyphMode) {
	case glyphModeOff:
		return false
	case glyphModePerPane:
		if value, ok := cfg.PanelGlyphs[id]; ok {
			return value
		}
	}
	return true
}

func applyPanelGlyphs(ws *tideui.Workspace, deck *dash.Deck, cfg config) {
	if ws == nil || deck == nil {
		return
	}
	for _, panel := range deck.Panels() {
		id := panel.Meta().ID
		view, ok := ws.Lookup(id)
		if !ok {
			continue
		}
		title := panel.Meta().Title
		if title == "" {
			title = id
		}
		if glyphsShown(cfg, id) {
			glyph := panel.Meta().Glyph
			if glyph == "" {
				glyph = panelGlyph(id)
			}
			title = glyph + " " + title
		}
		view.Title(title)
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
		"notes", "git", "markets", "updates", "radar")
	ws.AddPreset("System", tideui.VStack(
		tideui.Weighted(tideui.HStack(
			tideui.Weighted(tideui.Leaf("system"), 2), tideui.Leaf("gpu"), tideui.Leaf("network"),
		), 6),
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("storage"), tideui.Leaf("services"), tideui.Leaf("updates"),
		), 4),
	), "weather", "agenda", "news", "tasks", "notes", "git", "markets", "clock", "radar")
	ws.AddPreset("Productivity", tideui.HStack(
		tideui.Weighted(tideui.VStack(tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("tasks")), 2),
		tideui.VStack(tideui.Leaf("notes"), tideui.Leaf("clock")),
	), "weather", "system", "gpu", "network", "storage", "services", "news", "git", "markets", "updates", "radar")
	ws.AddPreset("Developer", tideui.VStack(
		tideui.HStack(tideui.Leaf("git"), tideui.Leaf("system")),
		tideui.HStack(tideui.Weighted(tideui.Leaf("services"), 2), tideui.Leaf("news")),
	), "weather", "agenda", "clock", "gpu", "network", "storage", "tasks", "notes", "markets", "updates", "radar")
	ws.AddPreset("Minimal", tideui.HStack(
		tideui.Leaf("clock"), tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("weather"),
	), "system", "gpu", "network", "storage", "services", "news", "tasks", "notes", "git", "markets", "updates", "radar")
}

// --- Layout slots ---------------------------------------------------------
//
// The five presets double as saveable slots: alt+1..5 loads a slot, which
// holds its built-in preset until the user overwrites it. A saved slot is a
// full layout (panels, weights and what is hidden) stored under its own key,
// so overwriting one never disturbs the others or the last-used layout.

// slotKey is the store key a slot saves under.
func slotKey(index int) string { return fmt.Sprintf("tidedeck-demo.slot%d", index+1) }

// slotLabel names a slot: the built-in preset it holds until overwritten.
func (m model) slotLabel(index int) string {
	if names := m.ws.PresetNames(); index >= 0 && index < len(names) {
		return names[index]
	}
	return fmt.Sprintf("slot %d", index+1)
}

// loadSlots reads saved slots so alt+1..5 works on the first frame.
func (m *model) loadSlots() {
	if m.store == nil {
		return
	}
	for i := 0; i < slotCount; i++ {
		data, err := m.store.Load(slotKey(i))
		if err != nil || len(data) == 0 {
			continue
		}
		m.slots[i] = data
		m.slotSaved[i] = true
	}
}

// applySlot loads a slot's layout, falling back to the built-in preset when
// the slot has never been saved.
func (m *model) applySlot(index int) {
	if index < 0 || index >= slotCount {
		return
	}
	if data := m.slots[index]; len(data) > 0 {
		if err := m.ws.RestoreJSON(data); err != nil {
			m.state.status = fmt.Sprintf("slot %d failed: %v", index+1, err)
			return
		}
		m.activeSlot = index + 1
		m.syncLayoutTheme()
		m.state.status = fmt.Sprintf("slot %d: %s", index+1, m.slotLabel(index))
		return
	}
	names := m.ws.PresetNames()
	if index < len(names) {
		m.ws.ApplyPreset(names[index])
		m.activeSlot = 0
		m.syncLayoutTheme()
		m.state.status = "preset: " + names[index]
	}
}

// saveSlot overwrites a slot with the current layout and writes it to the
// store so it survives a restart.
func (m *model) saveSlot(index int) {
	if index < 0 || index >= slotCount || m.store == nil {
		return
	}
	data, err := m.ws.PersistedJSON()
	if err != nil {
		m.state.status = "save failed: " + err.Error()
		return
	}
	if err := m.store.Save(slotKey(index), data); err != nil {
		m.state.status = "save failed: " + err.Error()
		return
	}
	m.slots[index] = data
	m.slotSaved[index] = true
	m.activeSlot = index + 1
	m.slotOpen = false
	m.state.status = fmt.Sprintf("saved layout to slot %d: %s", index+1, m.slotLabel(index))
}

// handleSlotChooser drives the save-layout modal.
func (m *model) handleSlotChooser(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.slotOpen = false
	case "j", "down":
		m.slotCursor = min(m.slotCursor+1, slotCount-1)
	case "k", "up":
		m.slotCursor = max(m.slotCursor-1, 0)
	case "1", "2", "3", "4", "5":
		m.slotCursor = int(msg.String()[0] - '1')
	case "enter":
		m.saveSlot(m.slotCursor)
	}
}

// activeLayoutLabel is the layout name in the status strip: the loaded slot
// while one is active, otherwise the named preset.
func (m model) activeLayoutLabel() string {
	if m.activeSlot > 0 {
		return fmt.Sprintf("slot %d: %s", m.activeSlot, m.slotLabel(m.activeSlot-1))
	}
	return m.ws.ActivePreset()
}

// renderSlotChooser draws the save-layout modal.
func (m model) renderSlotChooser(r tideui.Renderer, width int) tideui.Overlay {
	panelWidth := min(48, max(30, width-8))
	innerWidth := max(1, panelWidth-4)
	rows := []string{r.Styles.OverlayHint.Width(innerWidth).Render("save the current layout to…")}
	for i := 0; i < slotCount; i++ {
		suffix := "preset"
		if m.slotSaved[i] {
			suffix = "saved"
		}
		rows = append(rows, r.RenderSoftRow(tideui.SoftRow{
			Prefix:   fmt.Sprintf("%d ", i+1),
			Text:     m.slotLabel(i),
			Suffix:   suffix,
			Selected: i == m.slotCursor,
		}, innerWidth))
	}
	rows = append(rows, "", r.RenderSoftHints(innerWidth,
		tideui.SoftHint{Key: "enter", Label: "save"},
		tideui.SoftHint{Key: "esc", Label: "cancel"},
	))
	return r.SoftPanelOverlay(tideui.SoftPanel{
		Prefix:  "tide",
		Title:   "save layout",
		Content: r.RenderSoftBody(panelWidth, strings.Join(rows, "\n")),
		Width:   panelWidth,
	})
}

func (m model) Init() tea.Cmd { return tickCmd(time.Second) }

// currentLayoutThemeKey gives presets and saved slots separate theme scopes.
// A slot does not share a theme with the preset it originally came from once
// the user has saved over it.
func (m model) currentLayoutThemeKey() string {
	if m.activeSlot > 0 {
		return fmt.Sprintf("slot:%d", m.activeSlot)
	}
	if preset := m.ws.ActivePreset(); preset != "" {
		return "preset:" + preset
	}
	return "layout"
}

// syncLayoutTheme applies the theme assigned to the active preset or slot.
// Layouts without an assignment inherit the current theme, preserving the
// behavior of existing configs and layouts.
func (m *model) syncLayoutTheme() {
	key := m.currentLayoutThemeKey()
	if key == m.state.layoutThemeKey {
		return
	}
	m.state.layoutThemeKey = key
	name := m.layoutThemes[key]
	if name == "" {
		return
	}
	if theme, ok := layoutThemeByName(name); ok {
		m.state.theme = theme
	}
}

// rememberLayoutTheme assigns the current global theme to the active layout
// and persists the assignment alongside the normal application config.
func (m *model) rememberLayoutTheme() {
	key := m.currentLayoutThemeKey()
	m.state.layoutThemeKey = key
	if m.layoutThemes == nil {
		m.layoutThemes = map[string]string{}
	}
	m.layoutThemes[key] = m.state.theme.Name
	m.cfg.LayoutThemes = copyStringMap(m.layoutThemes)
	if err := m.cfg.save(); err != nil {
		m.state.status = "layout theme save failed: " + err.Error()
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.state.now = time.Now()
		m.syncOmarchyTheme()
		// Panels fetch on their own intervals and hold their own data.
		m.deck.Refresh(context.Background(), m.state.now)
		// A panel may have just reported different choices for its own settings -
		// the mailboxes of a newly chosen account - so let the open page follow.
		m.settings.SyncOptions()
		m.deck.Tick(m.state.now)
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
		// A pane-sized panel that has just been resized is due a fetch now: the
		// pane it was drawn in is the pane it asked for, and the layout changes
		// when a pane is zoomed or removed rather than on any schedule.
		return m, tea.Batch(tickCmd(time.Second), m.refreshResizedPanes())
	case lookupMsg:
		if m.settings.Opened() {
			m.settings.ApplyLookup(msg.place, msg.err)
		}
		return m, nil
	case pluginOpMsg:
		m.applyPluginOp(msg)
		return m, nil
	case launchMsg:
		if msg.err != nil {
			m.state.status = "could not run " + msg.argv[0] + ": " + msg.err.Error()
		} else {
			m.state.status = "back from " + msg.argv[0]
		}
		// The program just changed the data this pane previews - TideMail marks
		// the message it opened as read - so the panel is due again instead of
		// stale until its next interval.
		m.deck.RefreshNow(msg.panel)
		return m, m.refreshFocusedCmd()
	case panelRefreshedMsg:
		// The panel's State changed off the UI goroutine; re-render it.
		return m, nil
	case tea.MouseMsg:
		// The settings screen is a full takeover: the dashboard is not on
		// screen, so a click cannot mean anything there. Without this guard
		// clicks went to the panels underneath, focusing them and copying
		// their values while the user was looking at settings.
		if m.settings.Opened() {
			return m, nil
		}
		if m.handlePanelClick(msg) {
			return m, nil
		}
		if m.ws.HandleMouse(msg) {
			return m, nil
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// syncOmarchyTheme keeps only themes explicitly sourced from Omarchy live.
// Built-in and manually selected panel themes remain stable, while a new
// Omarchy palette is reflected on the next dashboard tick.
func (m *model) syncOmarchyTheme() {
	theme, ok := omarchyCurrentTheme()
	if !ok {
		return
	}
	changed := false
	if strings.HasPrefix(m.state.theme.Name, "omarchy · ") && m.state.theme.Name != theme.Name {
		m.state.theme = theme
		changed = true
	}
	for _, panel := range m.ws.Panels() {
		current, has := panel.PanelTheme()
		if has && strings.HasPrefix(current.Name, "omarchy · ") && current.Name != theme.Name {
			panel.Theme(theme)
			changed = true
		}
	}
	if changed {
		m.state.status = "Omarchy theme synced: " + strings.TrimPrefix(theme.Name, "omarchy · ")
	}
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

type pluginOpMsg struct {
	op       pluginOp
	manifest dash.Manifest
	err      error
}

// pluginOpCmd runs a plugin install, update or removal off the UI goroutine: a
// clone is a network call, so it must not run in Update.
func pluginOpCmd(op pluginOp) tea.Cmd {
	return func() tea.Msg {
		switch op.kind {
		case "install":
			manifest, err := dash.Install(pluginsDir(), op.value)
			return pluginOpMsg{op: op, manifest: manifest, err: err}
		case "update":
			manifest, err := dash.Update(pluginsDir(), op.value)
			return pluginOpMsg{op: op, manifest: manifest, err: err}
		case "remove":
			return pluginOpMsg{op: op, err: dash.Remove(pluginsDir(), op.value)}
		default:
			return pluginOpMsg{op: op, err: fmt.Errorf("unknown plugin operation %q", op.kind)}
		}
	}
}

// launchMsg reports that a program a pane's selection opened has exited.
type launchMsg struct {
	panel string
	argv  []string
	err   error
}

// launchCmd hands the terminal to a program and takes it back when the program
// exits, so the dashboard suspends rather than opening a second terminal: the
// tool the pane opens is then the one thing on screen, exactly as running it by
// hand would be, and no window is left behind.
func launchCmd(panel string, argv []string) tea.Cmd {
	command := exec.Command(argv[0], argv[1:]...)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return launchMsg{panel: panel, argv: argv, err: err}
	})
}

// applyPluginOp registers, replaces or removes the panel for a finished plugin
// operation, then rebuilds the settings pages so the Plugins list and the
// plugin's own category are current.
func (m *model) applyPluginOp(msg pluginOpMsg) {
	if msg.err != nil {
		m.state.status = "plugin: " + msg.err.Error()
		if m.settings.Opened() {
			m.settings.ApplyPluginOp("", msg.err)
		}
		return
	}
	switch msg.op.kind {
	case "install", "update":
		panel := dash.Exec(msg.manifest)
		m.deck.Register(panel)
		m.deck.AttachPanel(m.ws, panel)
		m.deck.Configure(m.values())
		m.refreshBadges()
		if msg.op.kind == "install" {
			m.state.status = "installed " + msg.manifest.Name + " — enable it to run"
			if m.settings.Opened() {
				m.settings.ApplyPluginOp("installed "+msg.manifest.Name+" — enable it above", nil)
			}
		} else {
			m.state.status = "updated " + msg.manifest.Name
			if m.settings.Opened() {
				m.settings.ApplyPluginOp("updated "+msg.manifest.Name, nil)
			}
		}
	case "remove":
		m.deck.Unregister(msg.op.value)
		m.ws.RemovePanel(msg.op.value)
		m.refreshBadges()
		m.state.status = "plugin removed"
		if m.settings.Opened() {
			m.settings.ApplyPluginOp("plugin removed", nil)
		}
	}
	if m.settings.Opened() {
		m.settings.Reload(m.cfg)
	}
	applyPanelGlyphs(m.ws, m.deck, m.cfg)
}

type panelRefreshedMsg struct{}

// refreshFocusedCmd re-runs the focused panel off the UI goroutine, so a plugin
// that took input shows the new value without waiting out its interval. It is
// nil for a panel that does not fetch - a built-in recomputes as it types.
func (m model) refreshFocusedCmd() tea.Cmd {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return nil
	}
	fetcher, ok := panel.(dash.Fetcher)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		_ = fetcher.Refresh(context.Background())
		return panelRefreshedMsg{}
	}
}

// refreshResizedPanes returns a command that fetches every pane-sized panel whose
// pane has changed shape since the last frame. The panel was drawn into the old
// pane and asked its source for that many pixels, so a zoom, a closed pane or a
// wider window leaves it holding a picture for a pane that is no longer there.
// Only a panel that says its fetches depend on the pane is asked: for everything
// else a resize is a redraw, not a request.
func (m *model) refreshResizedPanes() tea.Cmd {
	rects := m.ws.Solved().Rects
	if len(rects) == 0 {
		return nil
	}
	if m.paneSizes == nil {
		m.paneSizes = map[string][2]int{}
	}
	var due []string
	for id, rect := range rects {
		size := [2]int{rect.Width, rect.Height}
		if last, seen := m.paneSizes[id]; seen && last == size {
			continue
		}
		m.paneSizes[id] = size
		panel, ok := m.deck.Lookup(id)
		if !ok {
			continue
		}
		if sized, ok := panel.(dash.PaneSized); ok && sized.PaneSized() {
			due = append(due, id)
		}
	}
	if len(due) == 0 {
		return nil
	}
	return func() tea.Msg {
		for _, id := range due {
			panel, ok := m.deck.Lookup(id)
			if !ok {
				continue
			}
			if fetcher, ok := panel.(dash.Fetcher); ok {
				_ = fetcher.Refresh(context.Background())
			}
		}
		return panelRefreshedMsg{}
	}
}

// focusedCopy returns the focused panel's copyable text, if it has any.
func (m model) focusedCopy() (string, bool) {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return "", false
	}
	copier, ok := panel.(dash.Copier)
	if !ok {
		return "", false
	}
	return copier.Copy()
}

// osc52 is the escape sequence that sets the system clipboard.
func osc52(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + encoded + "\x07"
}

// summarize shortens a value for the status line.
func summarize(text string) string {
	runes := []rune(text)
	if len(runes) <= 30 {
		return text
	}
	return string(runes[:29]) + "…"
}

// focusedInput returns the focused panel's Input when it accepts typing.
func (m model) focusedInput() (dash.Input, bool) {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return nil, false
	}
	input, ok := panel.(dash.Input)
	return input, ok
}

// reservedShortcuts are the single-key commands the application and the
// workspace bind. A focused input panel must not shadow them: if it did, a
// text filter would eat "m arrange" and "s settings", and once it matched
// nothing the panel would just look empty.
const reservedShortcuts = "mwqtscdTS"

// handlePanelInput routes a key to a focused panel that accepts typing. While
// the panel is only focused it does not get the reserved shortcuts, so "m
// arrange" still works. Typing an accepted rune - or pressing "/" to start even
// when the first letter is reserved - opens an edit session, and inside it the
// panel's reserved letters are typed rather than run ("m" in a filter). A rune
// the panel does not accept still falls through. Escape ends the session. It
// returns the command to run and whether the key was consumed.
func (m *model) handlePanelInput(msg tea.KeyMsg) (tea.Cmd, bool) {
	input, ok := m.focusedInput()
	if !ok {
		m.editing, m.editTarget = false, ""
		return nil, false
	}
	// A focus change outside the keyboard (a mouse click) ends the session.
	if m.editing && m.editTarget != m.ws.Focused() {
		m.editing, m.editTarget = false, ""
	}
	if m.editing {
		switch msg.Type {
		case tea.KeyEsc:
			m.editing, m.editTarget = false, ""
			m.state.status = "stopped typing"
			return nil, true
		case tea.KeyBackspace:
			if input.Backspace() {
				return m.refreshFocusedCmd(), true
			}
			return nil, false
		case tea.KeyRunes:
			// A session takes the runes the panel accepts, including the ones
			// that are shortcuts - that is the point of the session. A rune the
			// panel does not accept still falls through, so the calculator can
			// copy (c) without leaving the expression.
			consumed := false
			for _, r := range msg.Runes {
				if input.Type(r) {
					consumed = true
				}
			}
			if consumed {
				return m.refreshFocusedCmd(), true
			}
			return nil, false
		default:
			return nil, false
		}
	}
	if msg.Type != tea.KeyRunes {
		return nil, false
	}
	if len(msg.Runes) == 1 && msg.Runes[0] == '/' {
		m.editing, m.editTarget = true, m.ws.Focused()
		m.state.status = "typing — esc to stop"
		return nil, true
	}
	if len(msg.Runes) == 1 && strings.ContainsRune(reservedShortcuts, msg.Runes[0]) {
		return nil, false
	}
	consumed := false
	for _, r := range msg.Runes {
		if input.Type(r) {
			consumed = true
		}
	}
	if !consumed {
		return nil, false
	}
	m.editing, m.editTarget = true, m.ws.Focused()
	m.state.status = "typing — esc to stop"
	return m.refreshFocusedCmd(), true
}

// moveSelection hands a move to the focused pane's cursor only while that pane
// has the keyboard - entered with space, or owning the screen. In the tiled
// dashboard, directional keys are reserved for moving between panes; otherwise a
// list panel such as News can trap the user's focus in its first few rows.
func (m *model) moveSelection(delta int) bool {
	if m.ws.EnteredPane() == "" && m.ws.Zoomed() == "" {
		return false
	}
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return false
	}
	cursor, ok := panel.(dash.Cursor)
	if !ok {
		return false
	}
	return cursor.Move(delta)
}

// activateFocused runs the focused panel's primary action - for the news list,
// mark the selected story read and copy its link. It reports whether the panel
// has such an action, so a key the panel does not use stays the workspace's.
func (m *model) activateFocused() bool {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return false
	}
	activator, ok := panel.(dash.Activator)
	if !ok {
		return false
	}
	copied, status := activator.Activate()
	if copied != "" {
		m.state.clipboard = copied
	}
	m.state.status = status
	return true
}

// launchFocused hands the terminal to the program the focused pane's selection
// opens. It reports whether the pane had such an action, so the caller can tell
// "nothing to open" from "opened nothing".
func (m *model) launchFocused() (tea.Cmd, bool) {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return nil, false
	}
	launcher, ok := panel.(dash.Launcher)
	if !ok {
		return nil, false
	}
	argv, status, declared := launcher.Launch()
	if !declared {
		return nil, false
	}
	m.state.status = status
	if len(argv) == 0 {
		// Nothing selected, and the strip now says so rather than the pane
		// silently doing nothing.
		return nil, true
	}
	return launchCmd(m.ws.Focused(), argv), true
}

// editFocused hands the terminal to the program that *changes* the focused pane's
// selection, when the pane declares one. Enter acts on a row and e changes it, so
// a pane that only previews something does not send the reader out of the app to
// edit it. It reports whether the pane had such an action, so the key falls
// through to the rest of the application when it did not.
func (m *model) editFocused() (tea.Cmd, bool) {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return nil, false
	}
	editor, ok := panel.(dash.Editor)
	if !ok {
		return nil, false
	}
	argv, status, declared := editor.Edit()
	if !declared {
		return nil, false
	}
	m.state.status = status
	if len(argv) == 0 {
		return nil, true
	}
	return launchCmd(m.ws.Focused(), argv), true
}

// handleEnteredKey routes a key to the pane that has the keyboard. Only what
// walking that pane needs is taken - esc to leave it, Enter for its primary
// action, the arrows and j/k for its cursor - so the application's shortcuts and
// the workspace's own keys still work while a pane is entered. A pane that wants
// a key itself keeps it, because the panel input runs before this.
func (m *model) handleEnteredKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return nil, m.ws.LeavePane()
	case "enter":
		if cmd, handled := m.launchFocused(); handled {
			return cmd, true
		}
		// A pane whose primary action is not a program - the news list copies
		// the selected story - runs here too.
		return nil, m.activateFocused()
	case "up", "k":
		return nil, m.moveSelection(-1)
	case "down", "j":
		return nil, m.moveSelection(1)
	case "e":
		if cmd, handled := m.editFocused(); handled {
			return cmd, true
		}
	}
	return nil, false
}

// handlePanelClick routes a left click inside a panel's content to its Clicker,
// so a panel can act on the row under the pointer. It reports whether a panel
// handled the click; otherwise the workspace gets it to focus a panel.
func (m *model) handlePanelClick(msg tea.MouseMsg) bool {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false
	}
	id, x, y, ok := m.ws.PanelAt(msg.X, msg.Y)
	if !ok {
		return false
	}
	panel, ok := m.deck.Lookup(id)
	if !ok {
		return false
	}
	clicker, ok := panel.(dash.Clicker)
	if !ok {
		return false
	}
	copied, status, hit := clicker.Click(x, y)
	if !hit {
		return false
	}
	m.ws.Focus(id)
	if copied != "" {
		m.state.clipboard = copied
	}
	m.state.status = status
	return true
}

func (m model) refreshBadges() {
	// Panels on the deck advertise their own badges.
	for id, badge := range m.deck.Badges() {
		if panel, ok := m.ws.Lookup(id); ok {
			panel.Badge(badge.Text).BadgeTone(badge.Tone)
		}
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.slotOpen {
		m.handleSlotChooser(msg)
		return m, nil
	}
	if m.settings.Opened() {
		action := m.settings.Update(msg)
		// Preview the chosen gauge styles live, so every style is visible as it
		// is cycled rather than only after ctrl+s.
		m.applyStylePreview()
		if query := m.settings.TakeLookup(); query != "" {
			return m, lookupCmd(query)
		}
		if op, ok := m.settings.TakePluginOp(); ok {
			return m, pluginOpCmd(op)
		}
		switch action {
		case settingsSaved:
			m.cfg = m.settings.SavedConfig()
			// Layout themes are managed by the workspace theme picker, not the
			// settings form. Carry them through a settings save unchanged.
			m.cfg.LayoutThemes = copyStringMap(m.layoutThemes)
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
		if cmd, handled := m.handlePanelInput(msg); handled {
			return m, cmd
		}
		// A pane that has the keyboard reads the keys first: walking a list is
		// not a moment for the workspace's own keys to move focus out from under
		// the reader.
		if m.ws.EnteredPane() != "" {
			if cmd, handled := m.handleEnteredKey(msg); handled {
				return m, cmd
			}
		}
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
		case "c":
			if text, ok := m.focusedCopy(); ok {
				m.state.status = "copied: " + summarize(text)
				m.state.clipboard = text
			}
			return m, nil
		case "d":
			m.state.density = nextDensity(m.state.density)
			return m, nil
		case "enter":
			// Enter is the zoom, everywhere, whatever the panel is: one key, one
			// meaning, and no panel makes it mean something else. A pane's own
			// action runs inside the pane, where space has put the keyboard.
			if m.ws.Zoomed() != "" {
				m.ws.Unzoom()
			} else {
				m.ws.Zoom(m.ws.Focused())
			}
			return m, nil
		case "T":
			m.openPanelThemePicker()
			return m, nil
		case "S":
			m.slotOpen = true
			m.slotCursor = 0
			if m.activeSlot > 0 {
				m.slotCursor = m.activeSlot - 1
			}
			m.state.status = "save layout"
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
			if m.moveSelection(-1) {
				return m, nil
			}
			m.ws.FocusDirection(tideui.DirUp)
			return m, nil
		case "down":
			if m.moveSelection(1) {
				return m, nil
			}
			m.ws.FocusDirection(tideui.DirDown)
			return m, nil
		case "j":
			if m.moveSelection(1) {
				return m, nil
			}
		case "k":
			if m.moveSelection(-1) {
				return m, nil
			}
		case "left":
			m.ws.FocusDirection(tideui.DirLeft)
			return m, nil
		case "right":
			m.ws.FocusDirection(tideui.DirRight)
			return m, nil
		case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5":
			// A modifier keeps the digit keys free: a bare number used to
			// switch preset and reset the layout, which is easy to hit by
			// accident. Each digit loads a layout slot (overwrite it with S).
			index := int(msg.String()[len(msg.String())-1] - '1')
			m.applySlot(index)
			return m, nil
		}
	}
	m.ws.HandleKey(msg)
	return m, nil
}

// openWorkspaceThemePicker edits the workspace-wide theme with live preview.
func (m *model) openWorkspaceThemePicker() {
	m.syncLayoutTheme()
	m.pickerTarget = ""
	m.pickerHad = false
	m.picker = tideui.NewThemePicker(tideui.ThemePickerOptions{
		Themes: themePickerThemes(), InitialTheme: m.state.theme.Name,
	})
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
	currentTheme := m.state.theme.Name
	if theme, has := panel.PanelTheme(); has {
		m.pickerPrev, m.pickerHad = theme, true
		currentTheme = theme.Name
	} else {
		m.pickerPrev, m.pickerHad = tideui.Theme{}, false
	}
	m.picker = tideui.NewThemePicker(tideui.ThemePickerOptions{
		Themes: themePickerThemes(), InitialTheme: currentTheme,
	})
	m.picker.Open(currentTheme)
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
			m.rememberLayoutTheme()
		}
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
	// Presets can also be selected by the workspace command palette. Reconcile
	// its new theme scope before rendering the next frame.
	m.syncLayoutTheme()
	// The terminal reports its window in pixels alongside its size in cells, so
	// one measurement gives both the shape of a cell (which sizes a picture) and
	// its width (which says whether a pane is bigger than the picture's source).
	cellWidth, cellHeight := tideui.CellSizeOf(os.Stdout)
	options := tideui.StyleOptions{
		Density: m.state.density, PaneCorners: tideui.RoundCorners,
		Gauge: m.state.gauge, Sparkline: m.state.spark, ClockFont: m.state.clockFont,
		IconStyle: m.state.icons, ModalShadow: true,
	}
	if cellWidth > 0 && cellHeight > 0 {
		options.CellWidth = cellWidth
		options.CellAspect = cellHeight / cellWidth
	}
	renderer := tideui.NewRenderer(m.state.theme, options)
	if m.settings.Opened() {
		return m.settings.RenderWorkspace(renderer, m.width, m.height)
	}
	wr := tideui.NewWorkspaceRenderer(renderer)

	primary := "tideDeck  ·  " + m.state.theme.Name + "  ·  " + string(m.state.density)
	if label := m.activeLayoutLabel(); label != "" {
		primary += "  ·  " + label
	}
	dataLabel := "demo data"
	if m.deck.Mode() == dash.ModeLive {
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
	wr.Options.StatusHints = m.statusHints()
	// Status messages show as the strip's capsule, so they stay visible even
	// when the secondary metadata is truncated on a narrow terminal.
	wr.Options.StatusNotice = m.state.status
	base := wr.Render(m.ws, m.width, m.height)
	out := ""
	if out == "" {
		if overlay := m.ws.Overlay(renderer); overlay != nil && overlay.Visible {
			out = renderer.OverlayModal(base, overlay.Content, m.width, m.height)
		}
	}
	if out == "" && m.slotOpen {
		if overlay := m.renderSlotChooser(renderer, m.width); overlay.Visible {
			out = renderer.OverlayModal(base, overlay.Content, m.width, m.height)
		}
	}
	if out == "" && m.picker.Opened() {
		overlay := m.picker.SoftModal(renderer, min(48, m.width-4), m.height, "tidedeck")
		out = renderer.OverlayModal(base, overlay.Content, m.width, m.height)
	}
	if out == "" {
		out = base
	}
	// A copy rides in the frame rather than being written from a command: the
	// renderer writes frames on its own goroutine, so a separate write could be
	// split by one and the terminal would drop the malformed sequence. Clearing
	// here sends it exactly once.
	if m.state.clipboard != "" {
		out = osc52(m.state.clipboard) + out
		m.state.clipboard = ""
	}
	return out
}

// statusHints advertises the input affordance while a panel that accepts
// typing is focused, so "/" is discoverable rather than folklore.
func (m model) statusHints() []tideui.KeyHint {
	if m.slotOpen {
		return []tideui.KeyHint{tideui.Hint("enter", "save"), tideui.Hint("esc", "cancel")}
	}
	if m.editing {
		return []tideui.KeyHint{tideui.Hint("esc", "stop typing")}
	}
	if _, ok := m.focusedInput(); ok {
		return []tideui.KeyHint{tideui.Hint("/", "type"), tideui.Hint("s", "settings")}
	}
	return []tideui.KeyHint{tideui.Hint("s", "settings"), tideui.Hint("S", "save layout")}
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
