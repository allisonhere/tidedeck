package tideui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFrameColorFadesFocusedBorderTowardIdle(t *testing.T) {
	styles := NewRenderer(CatppuccinMocha, StyleOptions{}).Styles
	ws := styles.Workspace

	at := func(level float64) lipgloss.Color {
		return FocusChrome{IdleFade: level}.FrameColor(styles, true, false, "")
	}

	if got := at(0); got != ws.FrameActive {
		t.Errorf("unfaded focused frame = %s, want the active colour %s", got, ws.FrameActive)
	}
	if got := at(1); got != ws.FrameIdle {
		t.Errorf("fully faded focused frame = %s, want the idle colour %s", got, ws.FrameIdle)
	}
	if mid := at(0.5); mid == ws.FrameActive || mid == ws.FrameIdle {
		t.Errorf("half-faded frame = %s, want a colour between %s and %s", mid, ws.FrameActive, ws.FrameIdle)
	}
}

func TestFrameColorFadesAnAccentedPanelFromItsOwnColour(t *testing.T) {
	// A panel with an accent is lit in that accent rather than FrameActive, so
	// the fade has to start from the colour the border actually has.
	styles := NewRenderer(CatppuccinMocha, StyleOptions{}).Styles
	accent := lipgloss.Color("#f38ba8")

	lit := FocusChrome{}.FrameColor(styles, true, false, accent)
	faded := FocusChrome{IdleFade: 1}.FrameColor(styles, true, false, accent)
	mid := FocusChrome{IdleFade: 0.5}.FrameColor(styles, true, false, accent)

	if lit == faded {
		t.Fatal("an accented frame did not fade at all")
	}
	if mid == lit || mid == faded {
		t.Errorf("half-faded accented frame = %s, want a colour between %s and %s", mid, lit, faded)
	}
}

func TestIdleFadeLeavesUnfocusedPanelsAlone(t *testing.T) {
	// The fade is about the focus highlight. An unfocused panel has none, so a
	// fully idle workspace must not change how it is drawn.
	styles := NewRenderer(CatppuccinMocha, StyleOptions{}).Styles

	normal := FocusChrome{}.FrameColor(styles, false, false, "")
	idle := FocusChrome{IdleFade: 1}.FrameColor(styles, false, false, "")
	if normal != idle {
		t.Errorf("unfocused frame changed with the idle fade: %s -> %s", normal, idle)
	}

	dimNormal := FocusChrome{}.FrameColor(styles, false, true, "")
	dimIdle := FocusChrome{IdleFade: 1}.FrameColor(styles, false, true, "")
	if dimNormal != dimIdle {
		t.Errorf("dimmed frame changed with the idle fade: %s -> %s", dimNormal, dimIdle)
	}
}

func TestSetFocusIdleAppliesAtOnceWithoutAnimation(t *testing.T) {
	// With no animator the value is not interpolated, so it must take effect
	// immediately rather than waiting for ticks that will never come.
	ws := NewWorkspace()

	ws.SetFocusIdle(1)
	if got := ws.FocusIdle(); got != 1 {
		t.Errorf("FocusIdle() = %v, want 1", got)
	}
	ws.SetFocusIdle(0)
	if got := ws.FocusIdle(); got != 0 {
		t.Errorf("FocusIdle() = %v, want 0", got)
	}
}

func TestSetFocusIdleIsInterpolatedWhenAnimating(t *testing.T) {
	ws := NewWorkspace(WithAnimation(AnimationOptions{Enabled: true}))
	ws.SetFocusIdle(0)
	for ws.Animation().Tick() {
	}

	ws.SetFocusIdle(1)
	if got := ws.FocusIdle(); got >= 1 {
		t.Fatalf("fade jumped straight to %v; it should trail until the app ticks", got)
	}

	for i := 0; ws.Animation().Tick(); i++ {
		if i > 1000 {
			t.Fatal("fade never settled")
		}
	}
	if got := ws.FocusIdle(); got != 1 {
		t.Errorf("settled at %v, want 1", got)
	}
}

func TestSetFocusIdleClampsOutOfRangeLevels(t *testing.T) {
	ws := NewWorkspace()
	ws.SetFocusIdle(5)
	if got := ws.FocusIdle(); got != 1 {
		t.Errorf("FocusIdle() = %v, want it clamped to 1", got)
	}
	ws.SetFocusIdle(-2)
	if got := ws.FocusIdle(); got != 0 {
		t.Errorf("FocusIdle() = %v, want it clamped to 0", got)
	}
}
