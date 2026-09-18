package form

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/tideui"
)

// Button is a setting that does something rather than storing something - look
// up a place, install a plugin, remove one.
//
// It knows whether it is running. The old screen had no such state: pressing
// enter on Install twice started two installs, and the only sign that anything
// was happening was a message in the error colour. A running button shows a
// spinner and refuses to fire again.
type Button struct {
	label   string
	run     func() string
	running bool
	spin    spinner.Model
	// message is what the last run reported, held until the host takes it.
	message string
}

// NewButton builds a button. run is called when it fires and returns the
// message to show; a button that starts asynchronous work returns the message
// that says so and calls Done when the work finishes.
func NewButton(label string, run func() string) *Button {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return &Button{label: label, run: run, spin: spin}
}

// Label is the button's text.
func (b *Button) Label() string { return b.label }

// Running reports whether the button's work is in flight.
func (b *Button) Running() bool { return b.running }

// Busy marks the button as running, for work the host performs itself.
func (b *Button) Busy() { b.running = true }

// Done marks the work finished.
func (b *Button) Done() { b.running = false }

// Tick advances the spinner. The host calls it on the frames it is already
// drawing; the button does not ask for any of its own.
func (b *Button) Tick(msg tea.Msg) {
	if !b.running {
		return
	}
	b.spin, _ = b.spin.Update(msg)
}

// Value is empty: a button stores nothing.
func (b *Button) Value() string { return "" }

// SetValue does nothing.
func (b *Button) SetValue(string) {}

// Editing is always false: a button never holds the keyboard.
func (b *Button) Editing() bool { return false }

func (b *Button) Err() error { return nil }

func (b *Button) Update(msg tea.KeyMsg) Action {
	switch msg.String() {
	case "enter", " ":
		if b.running || b.run == nil {
			// Swallow the key rather than ignoring it. A second enter on a
			// running button must not fall through to the host and be read as
			// navigation.
			return ActionEditing
		}
		b.message = b.run()
		return ActionChanged
	}
	return ActionIgnored
}

// TakeMessage returns the message from the last run and clears it, so a
// message is shown once rather than sticking until the next one replaces it.
func (b *Button) TakeMessage() (string, bool) {
	if b.message == "" {
		return "", false
	}
	message := b.message
	b.message = ""
	return message, true
}

func (b *Button) View(r tideui.Renderer, width int) string {
	if !b.running {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(r.Styles.Workspace.HintFg).
		Render(b.spin.View())
}

// Mark is what the row's prefix draws: the button's own bracketed label lives
// in the row text, so the prefix only needs to indent it the way a checkbox
// indents its label.
func (b *Button) Mark(r tideui.Renderer) string { return "  " }

func (b *Button) Hints() []tideui.SoftHint {
	if b.running {
		return []tideui.SoftHint{{Key: "", Label: "working…"}}
	}
	return []tideui.SoftHint{{Key: "enter", Label: "run"}}
}
