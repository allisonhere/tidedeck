package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

// TestMain points the demo's persistence at a throwaway config directory so
// tests are hermetic and never read or write the developer's real layout.
func TestMain(m *testing.M) {
	if dir, err := os.MkdirTemp("", "tidedeck-test"); err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", dir)
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// The new panels must be placed in the presets that show them and named in the
// hidden list of every preset that does not. A panel that is neither placed nor
// hidden counts as visible-but-unplaced, and Show() drops it into that layout's
// root split the moment it is toggled.
func TestNewPanelsArePlacedOrHiddenInEveryPreset(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60

	expected := map[string]map[string]bool{ // preset -> panel -> should be visible
		"Overview":     {"gpu": true, "updates": false},
		"System":       {"gpu": true, "updates": true},
		"Productivity": {"gpu": false, "updates": false},
		"Developer":    {"gpu": false, "updates": false},
		"Minimal":      {"gpu": false, "updates": false},
	}
	for preset, panels := range expected {
		m.ws.ApplyPreset(preset)
		for id, want := range panels {
			if _, ok := m.ws.Lookup(id); !ok {
				t.Fatalf("panel %q is not registered", id)
			}
			if got := !m.ws.Hidden(id); got != want {
				t.Fatalf("preset %s: panel %q visible = %v, want %v", preset, id, got, want)
			}
		}
	}
}

// Both panels must actually draw in the layout they belong to.
func TestNewPanelsRenderInTheirPreset(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	m.ws.ApplyPreset("System")
	out := m.View()
	for _, want := range []string{"GPU", "Updates"} {
		if !strings.Contains(out, want) {
			t.Fatalf("System preset does not render %q", want)
		}
	}
}

// The system panel moved onto the deck, so it must be registered and its
// deck-supplied badge must be applied before the first frame rather than a
// tick later, when the hand-written registration set it directly.
func TestSystemPanelIsOnTheDeck(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	if _, ok := m.ws.Lookup("system"); !ok {
		t.Fatal("system is not registered")
	}
	if !strings.Contains(m.View(), "healthy") {
		t.Fatal("the system badge is missing on the first frame")
	}
}

// A dashboard started with live data configured must be live on the first
// frame. The deck's settings and mode used to be applied only when settings
// were saved, so a configured panel sat in demo mode until you opened the
// settings screen and pressed ctrl+s.
func TestStartupAppliesTheSavedConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.Live = true
	cfg.doc = dash.NewValues()
	cfg.doc.Set("weather.latitude", 52.52)
	cfg.doc.Set("weather.longitude", 13.405)
	cfg.doc.Set("weather.location", "Berlin")
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	if m.deck.Mode() != dash.ModeLive {
		t.Fatal("the deck was left in demo mode by a live configuration")
	}
	// Panels that hold their own data are filled before the first frame,
	// rather than a second later when the first tick arrives.
	clock, ok := m.ws.Lookup("clock")
	if !ok {
		t.Fatal("no clock panel")
	}
	body := ansi.Strip(clock.Render(tideui.PanelContext{
		ID: "clock", Width: 40, Renderer: viewRenderer(m.state),
	}))
	if strings.Contains(body, "Jan 1") {
		t.Fatalf("the clock rendered a zero time on the first frame:\n%s", body)
	}
	// A configured weather panel is told where it is, so it says it is
	// loading rather than that it has nowhere to look.
	weather, _ := m.ws.Lookup("weather")
	if got := ansi.Strip(weather.Render(tideui.PanelContext{
		ID: "weather", Width: 40, Renderer: viewRenderer(m.state),
	})); !strings.Contains(got, "Loading") {
		t.Fatalf("weather was not configured at startup: %q", got)
	}
}

// Installing a plugin from the settings screen registers its panel hidden,
// adds its own settings category, and removing it takes the panel away again.
// The install is a local directory, so nothing reaches the network.
func TestInstallAndRemovePluginFromSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	source := t.TempDir()
	manifest := `{"schemaVersion":1,"id":"author.thing","name":"Thing","version":"1.0.0",` +
		`"author":"author","description":"does a thing","kinds":["panel"],` +
		`"entryPoints":{"panel":["./run.sh"]}}`
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "run.sh"), []byte("#!/bin/sh\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	openCategory(t, m.settings, "Plugins")
	m.settings.state.pluginSource = source

	// The Install button queues the operation; the model runs it.
	install := fieldByLabel(t, m.settings, "Install")
	install.action()
	op, ok := m.settings.TakePluginOp()
	if !ok || op.kind != "install" {
		t.Fatalf("queued op = %+v, ok = %v", op, ok)
	}
	msg := pluginOpCmd(op)().(pluginOpMsg)
	m.applyPluginOp(msg)
	if msg.err != nil {
		t.Fatalf("install failed: %v", msg.err)
	}

	if _, ok := m.ws.Lookup("author.thing"); !ok {
		t.Fatal("installed plugin was not registered")
	}
	if !m.ws.Hidden("author.thing") {
		t.Fatal("an installed plugin should start hidden")
	}
	if !hasCategory(m.settings, "Thing") {
		t.Fatalf("plugin category missing: %+v", m.settings.categories)
	}

	// Remove it again.
	openCategory(t, m.settings, "Plugins")
	remove := fieldByLabel(t, m.settings, "remove Thing 1.0.0")
	remove.action()
	op, _ = m.settings.TakePluginOp()
	msg = pluginOpCmd(op)().(pluginOpMsg)
	m.applyPluginOp(msg)
	if msg.err != nil {
		t.Fatalf("remove failed: %v", msg.err)
	}
	if _, ok := m.ws.Lookup("author.thing"); ok {
		t.Fatal("plugin is still registered after removal")
	}
	if hasCategory(m.settings, "Thing") {
		t.Fatal("plugin category survived removal")
	}
}

// fieldByLabel finds a settings row by its label.
func fieldByLabel(t *testing.T, form *settingsForm, label string) formField {
	t.Helper()
	for _, field := range form.currentFields() {
		if field.label == label {
			return field
		}
	}
	t.Fatalf("no field labelled %q in %+v", label, form.currentFields())
	return formField{}
}

func hasCategory(form *settingsForm, name string) bool {
	for _, category := range form.categories {
		if category.name == name {
			return true
		}
	}
	return false
}

// A bare digit must not switch preset, since applying a preset resets the
// layout; the shortcut needs a modifier.
func TestPresetShortcutsNeedAlt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	start := m.ws.ActivePreset()

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if got := m.ws.ActivePreset(); got != start {
		t.Fatalf("a bare digit switched preset to %q", got)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2"), Alt: true})
	if got := m.ws.ActivePreset(); got == start {
		t.Fatalf("alt+2 did not switch preset (still %q)", got)
	}
}

// The calculator takes the keys it wants while focused, and typing must not
// trigger an application shortcut or change the preset.
func TestCalculatorTakesTypedInput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	// Showing the panel should focus it by itself.
	m.ws.Show("calculator")
	if got := m.ws.Focused(); got != "calculator" {
		t.Fatalf("showing the calculator left focus on %q", got)
	}
	preset := m.ws.ActivePreset()

	for _, r := range "12*8" {
		m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	panel, ok := m.ws.Lookup("calculator")
	if !ok {
		t.Fatal("no calculator panel")
	}
	body := ansi.Strip(panel.Render(tideui.PanelContext{ID: "calculator", Width: 30, Renderer: viewRenderer(m.state)}))
	if !strings.Contains(body, "= 96") {
		t.Fatalf("calculator =\n%s", body)
	}
	if m.ws.ActivePreset() != preset {
		t.Fatalf("typing changed the preset: %q -> %q", preset, m.ws.ActivePreset())
	}
	// Backspace edits the expression too.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	body = ansi.Strip(panel.Render(tideui.PanelContext{ID: "calculator", Width: 30, Renderer: viewRenderer(m.state)}))
	if !strings.Contains(body, "12*") {
		t.Fatalf("calculator after backspace =\n%s", body)
	}
}

// A plugin that declares an input is re-run after a keystroke, so the panel
// shows what was typed without waiting out its interval.
func TestPluginInputRefresh(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	source := t.TempDir()
	manifest := `{"schemaVersion":1,"id":"test.input","name":"Input","version":"1.0.0",` +
		`"author":"test","description":"input panel","kinds":["panel"],` +
		`"entryPoints":{"panel":["./run.sh"]},"panel":{"displayName":"Input","refreshSeconds":300,` +
		`"input":"query","inputChars":"ab","schema":[{"key":"query","type":"string","label":"Query"}]}}`
	script := "#!/bin/sh\nprintf '{\"rows\":[{\"type\":\"text\",\"label\":\"query\",\"value\":\"%s\"}]}' \"$TIDEDECK_PLUGIN_QUERY\"\n"
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	m.width, m.height = 120, 40
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	openCategory(t, m.settings, "Plugins")
	m.settings.state.pluginSource = source
	fieldByLabel(t, m.settings, "Install").action()
	op, _ := m.settings.TakePluginOp()
	m.applyPluginOp(pluginOpCmd(op)().(pluginOpMsg))
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // fields -> categories
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // close settings

	m.ws.Show("test.input")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(model)
	if cmd == nil {
		t.Fatal("typing produced no refresh command")
	}
	m = update(t, m, cmd()) // run the plugin refresh

	panel, ok := m.ws.Lookup("test.input")
	if !ok {
		t.Fatal("the plugin panel is missing")
	}
	body := ansi.Strip(panel.Render(tideui.PanelContext{ID: "test.input", Width: 30, Renderer: viewRenderer(m.state)}))
	if !strings.Contains(body, "a") {
		t.Fatalf("typed input did not reach the plugin:\n%s", body)
	}
}

// The whole user path: open the panel picker, reveal the calculator, close the
// picker, and type. This is how a panel is enabled in practice.
func TestCalculatorTypingAfterPickerReveal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !m.ws.PanelPicker().Opened() {
		t.Fatal("w did not open the panel picker")
	}
	index := 0
	for i, id := range m.ws.PanelIDs() {
		if id == "calculator" {
			index = i
		}
	}
	for i := 0; i < index; i++ {
		m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle it on
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})                       // close the picker

	if m.ws.Hidden("calculator") {
		t.Fatal("the picker did not reveal the calculator")
	}
	if got := m.ws.Focused(); got != "calculator" {
		t.Fatalf("focus after revealing = %q, want calculator", got)
	}

	for _, r := range "12*8" {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
		if cmd != nil {
			m = update(t, m, cmd())
		}
	}
	panel, _ := m.ws.Lookup("calculator")
	body := ansi.Strip(panel.Render(tideui.PanelContext{ID: "calculator", Width: 30, Renderer: viewRenderer(m.state)}))
	if !strings.Contains(body, "= 96") {
		t.Fatalf("calculator after typing:\n%s", body)
	}
}

// Pressing c on a panel with copyable content puts it on the clipboard.
func TestCopyFromCalculator(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	m.ws.Show("calculator")
	for _, r := range "12*8" {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
		if cmd != nil {
			m = update(t, m, cmd())
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(model)
	if !strings.Contains(m.state.status, "96") {
		t.Fatalf("status = %q, want the copied value", m.state.status)
	}
	if m.state.clipboard != "96" {
		t.Fatalf("clipboard = %q, want 96", m.state.clipboard)
	}
	// The sequence rides in the frame, so the renderer cannot split it, and it
	// is cleared once it has been sent.
	if frame := m.View(); !strings.Contains(frame, "OTY=") {
		t.Fatal("the frame does not carry the OSC 52 sequence")
	}
	if m.state.clipboard != "" {
		t.Fatalf("clipboard not cleared after a frame: %q", m.state.clipboard)
	}
}

func TestOSC52Encoding(t *testing.T) {
	if got := osc52("96"); got != "\x1b]52;c;OTY=\x07" {
		t.Fatalf("osc52 = %q", got)
	}
}

// letterPanel is a minimal panel that accepts letters, like a text filter, so
// the shortcut/input collision can be tested without a subprocess.
type letterPanel struct{ text string }

func (p *letterPanel) Meta() dash.Meta {
	return dash.Meta{ID: "test.letter", Title: "Letter", Role: tideui.RoleOptional, MinWidth: 8, MinHeight: 3, Hidden: true}
}
func (p *letterPanel) View(tideui.PanelContext) string { return p.text }
func (p *letterPanel) Type(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		p.text += string(r)
		return true
	}
	return false
}
func (p *letterPanel) Backspace() bool {
	if p.text == "" {
		return false
	}
	p.text = p.text[:len(p.text)-1]
	return true
}

func focusLetterPanel(t *testing.T, m model) (model, *letterPanel) {
	t.Helper()
	panel := &letterPanel{}
	m.deck.Register(panel)
	m.deck.AttachPanel(m.ws, panel)
	m.ws.Show("test.letter")
	if got := m.ws.Focused(); got != "test.letter" {
		t.Fatalf("focus = %q, want test.letter", got)
	}
	return m, panel
}

// A focused panel that accepts text must not swallow the application's
// single-key shortcuts: m still arranges and s still opens settings.
func TestFocusedInputDoesNotSwallowShortcuts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	m, panel := focusLetterPanel(t, m)

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if !m.ws.Arranging() {
		t.Fatal("m did not enter arrange mode while a text input was focused")
	}
	if panel.text != "" {
		t.Fatalf("m was typed into the filter: %q", panel.text)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // leave arrange mode

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.settings.Opened() {
		t.Fatal("s did not open settings while a text input was focused")
	}
	if panel.text != "" {
		t.Fatalf("s was typed into the filter: %q", panel.text)
	}
}

// Typing a rune that is not a shortcut starts an edit session, after which
// even the reserved letters reach the panel.
func TestTypingStartsAnEditSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	m, panel := focusLetterPanel(t, m)

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.editing {
		t.Fatal("typing a letter did not start an edit session")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if m.ws.Arranging() || m.settings.Opened() {
		t.Fatal("a shortcut fired while an edit session was active")
	}
	if panel.text != "rsm" {
		t.Fatalf("panel text = %q, want rsm", panel.text)
	}
}

// "/" opens a session even when the first character would otherwise be a
// shortcut, so a filter can begin with s, m, and so on.
func TestSlashTypesAReservedFirstLetter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	m, panel := focusLetterPanel(t, m)

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.editing {
		t.Fatal("/ did not start an edit session")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.settings.Opened() {
		t.Fatal("s opened settings instead of being typed")
	}
	if panel.text != "s" {
		t.Fatalf("panel text = %q, want s", panel.text)
	}
}

// A slot saves the current layout and alt+N loads it back, so the presets
// double as five saveable layouts.
func TestSaveLayoutSlotAndLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 140, 44

	m.ws.ApplyPreset("Minimal")
	if !m.ws.Hidden("network") {
		t.Fatal("Minimal should hide network")
	}
	m.saveSlot(0)
	if !m.slotSaved[0] {
		t.Fatal("slot 1 should be marked saved")
	}

	m.ws.ApplyPreset("System")
	if m.ws.Hidden("network") {
		t.Fatal("System should show network")
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true})
	if !m.ws.Hidden("network") {
		t.Fatal("alt+1 did not restore the saved slot")
	}
	if m.activeSlot != 1 {
		t.Fatalf("activeSlot = %d, want 1", m.activeSlot)
	}
}

// A slot with nothing saved falls back to the built-in preset at its index.
func TestAltSlotFallsBackToPreset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 140, 44

	m.ws.ApplyPreset("Minimal")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2"), Alt: true})
	if got := m.ws.ActivePreset(); got != "System" {
		t.Fatalf("alt+2 = %q, want the System preset", got)
	}
	if m.activeSlot != 0 {
		t.Fatalf("activeSlot = %d, want 0 for a preset", m.activeSlot)
	}
}

// A saved slot is written to the store, so a fresh model loads it.
func TestSavedSlotSurvivesRestart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 140, 44
	m.ws.ApplyPreset("Minimal")
	m.saveSlot(0)

	m2 := newModel()
	if !m2.slotSaved[0] {
		t.Fatal("the saved slot was not reloaded from the store")
	}
	m2 = update(t, m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true})
	if !m2.ws.Hidden("network") {
		t.Fatal("alt+1 on a fresh model did not load the saved slot")
	}
}

// S opens the save chooser, the cursor picks a slot, and Enter overwrites it.
func TestSaveLayoutChooserOverwritesTheChosenSlot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 140, 44
	m.ws.ApplyPreset("Minimal")

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if !m.slotOpen {
		t.Fatal("S did not open the save chooser")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // second slot
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.slotOpen {
		t.Fatal("enter did not close the chooser")
	}
	if !m.slotSaved[1] {
		t.Fatal("slot 2 was not saved")
	}
	if m.slotSaved[0] {
		t.Fatal("slot 1 should still be empty")
	}
}

// Enter on the news list copies the selected story's link and marks it read.
func TestNewsEnterCopiesLinkAndMarksRead(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	m = update(t, m, tickMsg(time.Now())) // demo headlines, with links
	m.ws.Focus("news")
	if got := m.ws.Focused(); got != "news" {
		t.Fatalf("focus = %q, want news", got)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.state.clipboard == "" {
		t.Fatal("enter copied nothing")
	}
	if !strings.Contains(m.state.status, "marked read") {
		t.Fatalf("status = %q", m.state.status)
	}
	// The copy rides in the frame rather than a separate write.
	if frame := m.View(); !strings.Contains(frame, "\x1b]52;c;") {
		t.Fatal("the frame did not carry the copy sequence")
	}
}

// A click on a story row copies its link and marks it read.
func TestNewsClickCopiesLink(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	m = update(t, m, tickMsg(time.Now()))
	m.ws.Focus("news")
	_ = m.View() // the panel records its row map while rendering

	rect, ok := m.ws.PanelRect("news")
	if !ok {
		t.Fatal("news has no panel rectangle")
	}
	// The first content row sits just inside the top border.
	msg := tea.MouseMsg{X: rect.X + 2, Y: rect.Y + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	if !m.handlePanelClick(msg) {
		t.Fatal("the click was not routed to the news panel")
	}
	if m.state.clipboard == "" {
		t.Fatal("the click copied nothing")
	}
	if !strings.Contains(m.state.status, "marked read") {
		t.Fatalf("status = %q", m.state.status)
	}
}

// The arrow keys move the news cursor instead of focus while the list is
// focused.
func TestArrowsMoveNewsCursor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	m = update(t, m, tickMsg(time.Now()))
	m.ws.Focus("news")
	focus := m.ws.Focused()

	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.ws.Focused() != focus {
		t.Fatalf("down moved focus from %q to %q", focus, m.ws.Focused())
	}
	panel, ok := m.deck.Lookup("news")
	if !ok {
		t.Fatal("no news panel")
	}
	copier, ok := panel.(dash.Copier)
	if !ok {
		t.Fatal("news does not offer a selection to copy")
	}
	if link, ok := copier.Copy(); !ok || link == "" {
		t.Fatalf("no link selected after down: %q %v", link, ok)
	}
}

// Escape ends the session, so the shortcuts work again.
func TestEscapeEndsEditSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 120, 40
	m, _ = focusLetterPanel(t, m)

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.editing {
		t.Fatal("typing did not start a session")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing {
		t.Fatal("escape did not end the session")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if !m.ws.Arranging() {
		t.Fatal("m did not arrange after escape ended the session")
	}
}
