package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
)

// TestThemePickerLivePreviewAndRevert guards the dashboard's theme UX: the
// workspace must recolour while the cursor moves, revert on escape, and keep
// the theme on confirm.
func TestThemePickerLivePreviewAndRevert(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	start := m.state.theme.Name

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.picker.Opened() {
		t.Fatal("theme picker did not open")
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.state.theme.Name == start {
		t.Fatalf("theme did not preview while navigating (still %s)", start)
	}
	preview := m.state.theme.Name

	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state.theme.Name != start {
		t.Fatalf("escape did not revert theme: %s, want %s", m.state.theme.Name, start)
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state.theme.Name != preview {
		t.Fatalf("confirm = %s, want %s", m.state.theme.Name, preview)
	}
	if m.picker.Opened() {
		t.Fatal("picker should close after confirm")
	}
	if got := m.layoutThemes["preset:Overview"]; got != preview {
		t.Fatalf("layout theme = %q, want %q", got, preview)
	}
}

func TestPresetThemesRestorePerLayout(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	m.layoutThemes = map[string]string{
		"preset:Overview": tideui.CatppuccinMocha.Name,
		"preset:System":   tideui.Nord.Name,
	}

	m.ws.ApplyPreset("System")
	m.syncLayoutTheme()
	if got := m.state.theme.Name; got != tideui.Nord.Name {
		t.Fatalf("System theme = %q, want %q", got, tideui.Nord.Name)
	}
	m.ws.ApplyPreset("Overview")
	m.syncLayoutTheme()
	if got := m.state.theme.Name; got != tideui.CatppuccinMocha.Name {
		t.Fatalf("Overview theme = %q, want %q", got, tideui.CatppuccinMocha.Name)
	}
}

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	result, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", next)
	}
	return result
}

// TestPanelThemePickerAssignsOnConfirm checks that T opens the picker targeted
// at the focused panel, previews live on that panel, and commits on Enter
// without changing the workspace theme.
func TestPanelThemePickerAssignsOnConfirm(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	m.ws.Focus("weather")
	workspace := m.state.theme.Name

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if !m.picker.Opened() {
		t.Fatal("T did not open the theme picker")
	}
	if m.pickerTarget != "weather" {
		t.Fatalf("picker target = %q, want weather", m.pickerTarget)
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	preview := m.picker.PreviewTheme().Name
	panel, _ := m.ws.Lookup("weather")
	if theme, has := panel.PanelTheme(); !has || theme.Name != preview {
		t.Fatalf("panel did not preview live: %v", has)
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if theme, has := panel.PanelTheme(); !has || theme.Name != preview {
		t.Fatalf("confirm did not keep the panel theme: %v", has)
	}
	if m.state.theme.Name != workspace {
		t.Fatalf("workspace theme changed to %s, want %s", m.state.theme.Name, workspace)
	}
	if m.pickerTarget != "" {
		t.Fatal("picker target not reset after confirm")
	}
}

// TestPanelThemePickerEscapeRestores checks that cancelling restores the panel
// theme that was in place before the picker opened, including clearing it when
// the panel had none.
func TestPanelThemePickerEscapeRestores(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40

	// A panel with no theme must be cleared again on cancel.
	m.ws.Focus("weather")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	weather, _ := m.ws.Lookup("weather")
	if _, has := weather.PanelTheme(); has {
		t.Fatal("escape should have cleared a panel that started unthemed")
	}

	// A themed panel must be restored to its previous theme.
	weather.Theme(tideui.GruvboxLight)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if theme, has := weather.PanelTheme(); !has || theme.Name != tideui.GruvboxLight.Name {
		t.Fatalf("escape did not restore previous panel theme: %v", has)
	}
}

// TestClearFocusedPanelTheme checks the ctrl+T shortcut.
func TestClearFocusedPanelTheme(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	m.ws.Focus("weather")
	panel, _ := m.ws.Lookup("weather")
	panel.Theme(tideui.Nord)

	m = update(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	if _, has := panel.PanelTheme(); has {
		t.Fatal("ctrl+T did not clear the focused panel theme")
	}
}

// TestShiftArrowResizesFocusedPanel checks that shift+arrow resizes the
// focused pane directly, with no mode to enter or leave.
func TestShiftArrowResizesFocusedPanel(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	m.ws.Focus("weather")
	m.ws.Solve(120, 39)
	before := m.ws.Solve(120, 39).Rects["weather"].Width

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shift+right")})
	after := m.ws.Solve(120, 39).Rects["weather"].Width
	if after <= before {
		t.Fatalf("shift+right did not grow the focused pane: %d -> %d", before, after)
	}
	if m.ws.Focused() != "weather" {
		t.Fatalf("resize changed focus to %q", m.ws.Focused())
	}
	if status := m.ws.ResizeStatus(); status == "" {
		t.Fatal("expected a resize status percentage")
	}
}
