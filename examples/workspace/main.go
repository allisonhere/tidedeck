// Command workspace (TideDeck) is a dense, glanceable terminal information
// dashboard built on the TideUI workspace. It demonstrates reusable dashboard
// widgets, a deterministic fake data source, semantic responsive layout,
// presets, a drill-down (zoom) pattern, density modes, contextual actions,
// panel picker, command palette, and live rearrangement.
//
// Run with: go run ./examples/workspace
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
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
	feed    *demoFeed
	status  string

	weatherUnit  string
	agendaOffset int

	tasks     []tideui.Task
	headlines []tideui.Headline
	services  []tideui.ServiceStatus
	notes     []tideui.Note
	repos     []tideui.RepoActivity
	mounts    []tideui.StorageMount
}

type model struct {
	width, height int
	state         *demoState
	ws            *tideui.Workspace
	picker        tideui.ThemePicker

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
	state := &demoState{
		theme: tideui.CatppuccinMocha, density: tideui.Dense, now: started, feed: feed, weatherUnit: "F",
		tasks:     feed.Tasks(),
		headlines: feed.Headlines(),
		services:  feed.Services(),
		notes:     feed.Notes(),
		repos:     feed.RepoActivity(),
		mounts:    feed.Storage(),
	}

	store := fileStore{path: filepath.Join(userConfigDir(), "tidedeck", "layout.json")}
	ws := tideui.NewWorkspace(
		tideui.WithPersistence("tidedeck-demo"),
		tideui.WithStore(store),
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

	registerPanels(ws, state)
	registerPresets(ws)

	ws.Layout(overviewLayout())
	ws.ApplyPreset("Overview")
	ws.Focus("agenda")

	return model{state: state, ws: ws, picker: tideui.NewThemePicker(tideui.ThemePickerOptions{InitialTheme: state.theme.Name})}
}

func registerPanels(ws *tideui.Workspace, state *demoState) {
	ws.Panel("weather", weatherPanel(state)).
		Title("Weather").Role(tideui.RolePrimary).Priority(90).MinWidth(18).MinHeight(7).
		Actions(
			tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "weather refreshed" }).Labeled("refresh"),
			tideui.Action("units", "u", func(*tideui.Workspace) {
				if state.weatherUnit == "F" {
					state.weatherUnit = "C"
				} else {
					state.weatherUnit = "F"
				}
				state.status = "units: °" + state.weatherUnit
			}).Labeled("units"),
		)

	ws.Panel("agenda", agendaPanel(state)).
		Title("Agenda").Role(tideui.RolePrimary).Priority(100).MinWidth(20).MinHeight(7).
		Actions(
			tideui.Action("next", "n", func(*tideui.Workspace) { state.agendaOffset++; state.status = "agenda: +day" }).Labeled("next"),
			tideui.Action("prev", "p", func(*tideui.Workspace) { state.agendaOffset--; state.status = "agenda: -day" }).Labeled("prev"),
			tideui.Action("today", "0", func(*tideui.Workspace) { state.agendaOffset = 0; state.status = "agenda: today" }).Labeled("today"),
		)

	ws.Panel("clock", clockPanel(state)).
		Title("Clock").Role(tideui.RoleSecondary).Priority(50).MinWidth(16).MinHeight(7).HideBelow(96).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "clock synced" }).Labeled("sync"))

	ws.Panel("system", systemPanel(state)).
		Title("System").Role(tideui.RolePrimary).Priority(95).MinWidth(20).MinHeight(7).
		Badge("healthy").BadgeTone(tideui.ToneGood).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "system sampled" }).Labeled("refresh"))

	ws.Panel("network", networkPanel(state)).
		Title("Network").Role(tideui.RoleSecondary).Priority(70).MinWidth(18).MinHeight(7).HideBelow(104).
		Badge("up").BadgeTone(tideui.ToneGood).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "network sampled" }).Labeled("refresh"))

	ws.Panel("storage", storagePanel(state)).
		Title("Storage").Role(tideui.RoleSecondary).Priority(55).MinWidth(18).MinHeight(6).HideBelow(120).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "storage sampled" }).Labeled("refresh"))

	ws.Panel("services", servicesPanel(state)).
		Title("Services").Role(tideui.RoleSecondary).Priority(65).MinWidth(20).MinHeight(6).HideBelow(92).
		Actions(
			tideui.Action("restart", "r", func(*tideui.Workspace) {
				for i := range state.services {
					if state.services[i].Tone != tideui.ToneGood {
						state.services[i].State = "healthy"
						state.services[i].Tone = tideui.ToneGood
						state.status = state.services[i].Name + " restarted"
						return
					}
				}
				state.status = "all services healthy"
			}).Labeled("restart"),
			tideui.Action("logs", "l", func(*tideui.Workspace) { state.status = "opening service logs" }).Labeled("logs"),
		)

	ws.Panel("news", newsPanel(state)).
		Title("News").Subtitle("feeds").Role(tideui.RoleSecondary).Priority(68).MinWidth(22).MinHeight(6).HideBelow(78).
		Actions(
			tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "feeds refreshed" }).Labeled("refresh"),
			tideui.Action("mark", "m", func(*tideui.Workspace) {
				for i := range state.headlines {
					if state.headlines[i].Unread {
						state.headlines[i].Unread = false
						state.status = "marked read"
						return
					}
				}
				state.status = "nothing unread"
			}).Labeled("mark read"),
		)

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

	ws.Panel("git", gitPanel(state)).
		Title("Git Activity").Role(tideui.RoleOptional).Priority(40).MinWidth(18).MinHeight(5).HideBelow(150).
		Actions(tideui.Action("fetch", "r", func(*tideui.Workspace) { state.status = "fetched all remotes" }).Labeled("fetch"))

	// Markets ships with its own contrasting theme to show that a panel can
	// opt out of the workspace palette entirely. Press T / ctrl+T to assign or
	// clear the focused panel's theme at runtime.
	ws.Panel("markets", marketsPanel(state)).
		Title("Markets").Role(tideui.RoleOptional).Priority(50).MinWidth(18).MinHeight(6).HideBelow(160).
		Theme(tideui.GruvboxLight).
		Actions(tideui.Action("refresh", "r", func(*tideui.Workspace) { state.status = "quotes refreshed" }).Labeled("refresh"))
}

// overviewLayout balances the dashboard by content density: dense panels
// (Agenda, News) get taller rows and more width, while the sparse bottom row
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
			tideui.Leaf("clock"),
		), 4),
	)
}

func registerPresets(ws *tideui.Workspace) {
	ws.AddPreset("Overview", overviewLayout(),
		"notes", "git", "markets")
	ws.AddPreset("System", tideui.VStack(
		tideui.Weighted(tideui.HStack(
			tideui.Weighted(tideui.Leaf("system"), 2), tideui.Leaf("network"), tideui.Leaf("services"),
		), 6),
		tideui.Weighted(tideui.HStack(
			tideui.Leaf("storage"), tideui.Leaf("markets"), tideui.Leaf("clock"),
		), 4),
	), "weather", "agenda", "news", "tasks", "notes", "git")
	ws.AddPreset("Productivity", tideui.HStack(
		tideui.Weighted(tideui.VStack(tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("tasks")), 2),
		tideui.VStack(tideui.Leaf("notes"), tideui.Leaf("clock")),
	), "weather", "system", "network", "storage", "services", "news", "git", "markets")
	ws.AddPreset("Developer", tideui.VStack(
		tideui.HStack(tideui.Leaf("git"), tideui.Leaf("system")),
		tideui.HStack(tideui.Weighted(tideui.Leaf("services"), 2), tideui.Leaf("news")),
	), "weather", "agenda", "clock", "network", "storage", "tasks", "notes", "markets")
	ws.AddPreset("Minimal", tideui.HStack(
		tideui.Leaf("clock"), tideui.Weighted(tideui.Leaf("agenda"), 2), tideui.Leaf("weather"),
	), "system", "network", "storage", "services", "news", "tasks", "notes", "git", "markets")
}

func (m model) Init() tea.Cmd { return tickCmd(time.Second) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.state.now = time.Now()
		m.refreshBadges()
		if m.ws.Arranging() {
			target := 1.0
			if m.state.now.Second()%2 == 0 {
				target = 0.45
			}
			m.ws.Animation().Set("dockPulse", target)
		}
		m.ws.Animation().Tick()
		return m, tickCmd(time.Second)
	case tea.MouseMsg:
		if m.ws.HandleMouse(msg) {
			return m, nil
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) refreshBadges() {
	unread := 0
	for _, h := range m.state.headlines {
		if h.Unread {
			unread++
		}
	}
	if panel, ok := m.ws.Lookup("news"); ok {
		if unread > 0 {
			panel.Badge(fmt.Sprintf("%d", unread)).BadgeTone(tideui.ToneMuted)
		} else {
			panel.Badge("")
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
	issues := 0
	for _, s := range m.state.services {
		if s.Tone != tideui.ToneGood {
			issues++
		}
	}
	if panel, ok := m.ws.Lookup("services"); ok {
		if issues > 0 {
			panel.Badge(fmt.Sprintf("%d", issues)).BadgeTone(tideui.ToneWarning)
		} else {
			panel.Badge("").BadgeTone(tideui.ToneGood)
		}
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picker.Opened() {
		m.updateThemePicker(msg)
		return m, nil
	}
	if m.ws.PanelPicker().Opened() || m.ws.CommandPalette().Opened() {
		m.ws.HandleKey(msg)
		return m, nil
	}

	if !m.ws.Arranging() && !m.ws.Resizing() {
		switch msg.String() {
		case "q", "ctrl+c":
			_ = m.ws.Persist()
			return m, tea.Quit
		case "t":
			m.openWorkspaceThemePicker()
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
		Density: m.state.density, PaneCorners: tideui.RoundCorners, ModalShadow: true,
	})
	wr := tideui.NewWorkspaceRenderer(renderer)

	primary := "tideDeck  ·  " + m.state.theme.Name + "  ·  " + string(m.state.density)
	if preset := m.ws.ActivePreset(); preset != "" {
		primary += "  ·  " + preset
	}
	secondary := "updated " + m.state.now.Format("15:04") + "  ·  demo data"
	if m.state.status != "" {
		secondary = m.state.status + "  ·  " + secondary
	}
	if panel, ok := m.ws.Lookup(m.ws.Focused()); ok {
		if theme, has := panel.PanelTheme(); has {
			secondary = panel.TitleText() + "[" + theme.Name + "]  ·  " + secondary
		}
	}
	wr.Options.StatusLeft = primary
	wr.Options.StatusSecondary = secondary
	base := wr.Render(m.ws, m.width, m.height)

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
