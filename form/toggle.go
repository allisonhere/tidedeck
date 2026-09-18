package form

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
)

// Toggle is an on/off setting.
//
// It takes the arrow keys as well as enter and space. That matters for more
// than convenience: on the old screen left and right cycled a choice but did
// nothing to a toggle, and left additionally meant "leave this page" on every
// other row - so the same key did two unrelated things depending on where the
// cursor happened to be. Here an arrow key changes the value under the cursor,
// whatever kind of value it is.
type Toggle struct {
	on bool
}

// NewToggle builds a toggle.
func NewToggle(on bool) *Toggle { return &Toggle{on: on} }

// On reports the state, for hosts that hold a bool rather than a string.
func (t *Toggle) On() bool { return t.on }

// Set sets the state.
func (t *Toggle) Set(on bool) { t.on = on }

func (t *Toggle) Value() string {
	if t.on {
		return "true"
	}
	return "false"
}

func (t *Toggle) SetValue(value string) { t.on = value == "true" }

// Editing is always false: a toggle changes on one keystroke and never holds
// the keyboard.
func (t *Toggle) Editing() bool { return false }

func (t *Toggle) Err() error { return nil }

func (t *Toggle) Update(msg tea.KeyMsg) Action {
	switch msg.String() {
	case "enter", " ":
		t.on = !t.on
		return ActionChanged
	case "left", "h":
		if !t.on {
			return ActionIgnored
		}
		t.on = false
		return ActionChanged
	case "right", "l":
		if t.on {
			return ActionIgnored
		}
		t.on = true
		return ActionChanged
	}
	return ActionIgnored
}

// Mark is the checkbox drawn in the row's prefix. It degrades with the rest of
// the chrome: a theme that asks for ASCII gets ASCII.
func (t *Toggle) Mark(r tideui.Renderer) string {
	if t.on {
		if r.Styles.PlainUI {
			return "[x] "
		}
		return "[✓] "
	}
	return "[ ] "
}

// View is empty. The checkbox in the prefix already says what the value is,
// and the old screen's trailing "on"/"off" said it a second time in the same
// row.
func (t *Toggle) View(r tideui.Renderer, width int) string { return "" }

func (t *Toggle) Hints() []tideui.SoftHint {
	return []tideui.SoftHint{{Key: "space", Label: "toggle"}}
}
