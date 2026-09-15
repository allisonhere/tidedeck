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

// TestResizeModeFromFocusedPanel checks the R mode: a direction key selects the
// boundary beside the focused panel, the next press moves it, focus stays put,
// and Esc leaves the mode.
func TestResizeModeFromFocusedPanel(t *testing.T) {
	m := newModel()
	m.width, m.height = 120, 40
	m.ws.Focus("weather")
	before := m.ws.Solve(120, 39).Rects["weather"].Width

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if !m.ws.Resizing() {
		t.Fatal("R did not enter resize mode")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRight}) // select the divider
	if _, ok := m.ws.SelectedDivider(); !ok {
		t.Fatal("right did not select a divider")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRight}) // move it
	after := m.ws.Solve(120, 39).Rects["weather"].Width
	if after <= before {
		t.Fatalf("resize did not grow the focused panel: %d -> %d", before, after)
	}
	if m.ws.Focused() != "weather" {
		t.Fatalf("resize mode changed focus to %q", m.ws.Focused())
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.ws.Resizing() {
		t.Fatal("esc did not leave resize mode")
	}
}
