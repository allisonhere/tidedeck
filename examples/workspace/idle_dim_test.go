package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIdleDimFadesOnlyAfterTheQuietSpell(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	m.cfg.IdleDim = true

	m.lastInput = time.Now()
	m.applyIdleDim()
	if got := m.ws.FocusIdle(); got != 0 {
		t.Errorf("faded while the keyboard was still busy: FocusIdle() = %v", got)
	}

	// Just short of the window: still lit.
	m.lastInput = time.Now().Add(-idleDimAfter + time.Second)
	m.applyIdleDim()
	if got := m.ws.FocusIdle(); got != 0 {
		t.Errorf("faded early at %v before the window: FocusIdle() = %v", time.Second, got)
	}

	// Past the window: the target is set, and the animator carries it there.
	m.lastInput = time.Now().Add(-idleDimAfter)
	m.applyIdleDim()
	for i := 0; m.ws.Animation().Tick(); i++ {
		if i > 1000 {
			t.Fatal("fade never settled")
		}
	}
	if got := m.ws.FocusIdle(); got != 1 {
		t.Errorf("did not fade after the quiet spell: FocusIdle() = %v", got)
	}
}

func TestAKeyRestoresTheHighlightImmediately(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	m.cfg.IdleDim = true

	m.lastInput = time.Now().Add(-idleDimAfter)
	m.applyIdleDim()
	for m.ws.Animation().Tick() {
	}
	if m.ws.FocusIdle() != 1 {
		t.Fatal("setup: workspace did not fade")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := next.(model)

	// A key means the person is here: the highlight comes straight back rather
	// than easing in while they are already working.
	if idle := got.ws.FocusIdle(); idle != 0 {
		t.Errorf("highlight did not come back on a keypress: FocusIdle() = %v", idle)
	}
	if time.Since(got.lastInput) > time.Second {
		t.Error("keypress did not refresh lastInput")
	}
}

func TestIdleDimSwitchedOffNeverFades(t *testing.T) {
	m := newModel()
	m.width, m.height = 200, 60
	m.cfg.IdleDim = false
	m.lastInput = time.Now().Add(-10 * idleDimAfter)

	m.applyIdleDim()
	for m.ws.Animation().Tick() {
	}

	if got := m.ws.FocusIdle(); got != 0 {
		t.Errorf("faded with the setting off: FocusIdle() = %v", got)
	}
}

func TestFreshModelIsNotAlreadyIdle(t *testing.T) {
	// lastInput starts at the launch time, not the zero time, or the first
	// tick would find the app "idle since 1970" and fade immediately.
	m := newModel()
	m.cfg.IdleDim = true

	m.applyIdleDim()
	if got := m.ws.FocusIdle(); got != 0 {
		t.Errorf("a freshly launched dashboard was already idle: FocusIdle() = %v", got)
	}
}

func TestIdleDimIsGlobalNotPerPanel(t *testing.T) {
	// The setting is one workspace-wide switch. Guard that it did not arrive as
	// a per-panel map like panel_gauges and panel_sparks.
	for _, key := range configKeys() {
		if key == "panel_idle_dim" {
			t.Fatal("idle dim leaked in as a per-panel setting")
		}
	}
	found := false
	for _, key := range configKeys() {
		if key == "idle_dim" {
			found = true
		}
	}
	if !found {
		t.Error("idle_dim is not a config key")
	}
}
