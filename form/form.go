// Package form provides the input controls a settings screen needs: a text
// field with a real caret, a choice that can be picked from a list rather than
// only cycled, a toggle, a bounded number, and a button that cannot fire twice.
//
// The package sits downstream of tideui, the way provider and dash do. It has
// to: a control owns edit state - a caret position, an open picker, a pending
// action - and tideui's promise is to be purely presentational. Nothing here is
// drawn with a colour of its own; every control renders through a
// tideui.Renderer and inherits whatever theme the host resolved.
//
// A control draws only the value cell, never the whole row. The screen owns the
// row - its rail, label, and selection - because only the screen knows how its
// rows are laid out. That split is what lets the same control sit in a settings
// list, a modal, or a panel body.
package form

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
)

// Action is what a keystroke did to a control, so the host knows whether to
// mark its form dirty, restore a value, or let the key fall through to its own
// navigation.
type Action int

const (
	// ActionIgnored means the control did not take the key, so the host should
	// handle it. A control returns this for every key it does not own, which
	// is what keeps arrow keys working as navigation on a row that is not
	// being edited.
	ActionIgnored Action = iota
	// ActionChanged means the value changed.
	ActionChanged
	// ActionEditing means the control took the key and is now taking keys
	// exclusively - the host must stop treating them as navigation.
	ActionEditing
	// ActionCommitted means an edit finished and the value was kept.
	ActionCommitted
	// ActionCancelled means an edit finished and the value was restored.
	ActionCancelled
)

// Control is one editable setting.
//
// View renders the value cell within width and must never exceed it; a control
// that overflows corrupts the row it sits in. Editing reports whether the
// control is consuming keys exclusively, which is how the host knows to stop
// interpreting j/k as navigation while someone is typing a value containing a
// j or a k.
type Control interface {
	Value() string
	SetValue(value string)
	Update(msg tea.KeyMsg) Action
	View(r tideui.Renderer, width int) string
	Editing() bool
	// Err reports a value the control considers unusable. It is advisory: the
	// value is still stored and still editable, so a half-typed entry can be
	// finished rather than being rejected keystroke by keystroke.
	Err() error
	// Hints are the keys that work right now, for the screen's hint bar. They
	// change with state - a text field being edited offers different keys from
	// one merely selected.
	Hints() []tideui.SoftHint
}

// Overlayer is implemented by a control that can take over the screen, such as
// a choice that opens a picker. The host draws the overlay and stops drawing
// its own rows while one is open.
type Overlayer interface {
	Overlay(r tideui.Renderer, width, height int) (tideui.Overlay, bool)
}

// clamp keeps an index inside a slice.
func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// wrap moves an index around a ring, so stepping past either end of a choice
// arrives at the other.
func wrap(index, length int) int {
	if length <= 0 {
		return 0
	}
	return (index%length + length) % length
}

// indexOf finds a value among options, reporting -1 when it is absent. A
// stored value that is no longer offered is the normal case after a rename, so
// callers treat -1 as "start at the beginning" rather than as an error.
func indexOf(options []string, value string) int {
	for i, option := range options {
		if option == value {
			return i
		}
	}
	return -1
}
