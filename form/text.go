package form

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
)

// Text is a free-text setting.
//
// The editing half is bubbles/textinput, which brings what a hand-rolled caret
// does not: word motions, ctrl+a/e/k/u/w, delete forward, paste, and a
// horizontally scrolling viewport that keeps the caret visible in a value
// wider than its cell.
//
// Validation is advisory. textinput stores a value that fails validation and
// records the error rather than refusing the keystroke, which is the only way
// a partial value can ever be finished - "47." is not a number yet, but a
// field that rejected it could never be typed into.
type Text struct {
	input textinput.Model

	editing bool
	// before is the value to restore on esc. An edit is a transaction: it
	// either commits on enter or leaves nothing behind.
	before string

	// width is the last cell width the control was drawn at. A text input
	// scrolls horizontally against its own Width, and that is recomputed when
	// the value or the caret moves - not when it is drawn. The host only knows
	// the width at render time, so the width has to be carried back into the
	// input and the caret re-seated, or the window stays where it was and
	// typing runs off the end of a field that never scrolls.
	width int

	// summary shortens a long value for the idle row, the way a comma-joined
	// repository list is shown as "4 repos". The raw value is still what gets
	// edited.
	summary func(string) string
	// normalize tidies a value when the edit commits.
	normalize func(string) string
}

// NewText builds a text control.
func NewText(value string) *Text {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(value)
	return &Text{input: input}
}

// WithPlaceholder sets what an empty field shows. A field that does something
// useful when blank should say so here rather than looking unset.
func (t *Text) WithPlaceholder(placeholder string) *Text {
	t.input.Placeholder = placeholder
	return t
}

// WithValidate sets the validity test, reported through Err.
func (t *Text) WithValidate(validate func(string) error) *Text {
	if validate == nil {
		t.input.Validate = nil
		return t
	}
	t.input.Validate = textinput.ValidateFunc(validate)
	return t
}

// WithSummary sets how a long value is shortened when the field is not being
// edited.
func (t *Text) WithSummary(summary func(string) string) *Text {
	t.summary = summary
	return t
}

// WithNormalize sets the tidy-up applied when an edit commits.
func (t *Text) WithNormalize(normalize func(string) string) *Text {
	t.normalize = normalize
	return t
}

// WithSuggestions offers completions, which the tab key accepts. A field whose
// useful values are knowable - an interface name, a helper binary - should
// offer them rather than expecting them to be remembered.
func (t *Text) WithSuggestions(suggestions []string) *Text {
	if len(suggestions) == 0 {
		return t
	}
	t.input.ShowSuggestions = true
	t.input.SetSuggestions(suggestions)
	return t
}

func (t *Text) Value() string { return t.input.Value() }

func (t *Text) SetValue(value string) { t.input.SetValue(value) }

func (t *Text) Editing() bool { return t.editing }

func (t *Text) Err() error { return t.input.Err }

// Begin starts an edit, placing the caret at the end of the value.
func (t *Text) Begin() {
	t.editing = true
	t.before = t.input.Value()
	t.input.Focus()
	t.input.CursorEnd()
}

// commit ends the edit, keeping the value.
func (t *Text) commit() {
	if t.normalize != nil {
		t.input.SetValue(t.normalize(t.input.Value()))
	}
	t.editing = false
	t.input.Blur()
}

// cancel ends the edit, restoring what was there before it started.
func (t *Text) cancel() {
	t.input.SetValue(t.before)
	t.editing = false
	t.input.Blur()
}

func (t *Text) Update(msg tea.KeyMsg) Action {
	if !t.editing {
		// enter and space both open an edit, because a settings row is
		// activated with either and a text field should not be the one row
		// where that is untrue.
		switch msg.String() {
		case "enter", " ":
			t.Begin()
			return ActionEditing
		}
		return ActionIgnored
	}

	switch msg.String() {
	case "up", "down", "tab", "shift+tab":
		// None of these mean anything inside a one-line field, and swallowing
		// them silently makes the field a trap: the keys that move between
		// rows and panes everywhere else simply stop working. Commit and hand
		// the key back, so moving off the field keeps what was typed.
		t.commit()
		return ActionIgnored
	case "esc":
		changed := t.input.Value() != t.before
		t.cancel()
		if changed {
			return ActionCancelled
		}
		return ActionCommitted
	case "enter":
		before := t.before
		t.commit()
		if t.input.Value() != before {
			return ActionChanged
		}
		return ActionCommitted
	}

	before := t.input.Value()
	t.input, _ = t.input.Update(msg)
	if t.input.Value() != before {
		return ActionChanged
	}
	// Still editing even when the value did not change - a caret move is the
	// control's business, not the host's.
	return ActionEditing
}

func (t *Text) View(r tideui.Renderer, width int) string {
	if width < 3 {
		width = 3
	}
	if t.editing {
		// One cell is left for the caret, which textinput draws inside the
		// value rather than beside it.
		t.setWidth(max(1, width-1))
		t.styleInput(r)
		return ansi.Truncate(t.input.View(), width, "")
	}

	value := t.input.Value()
	if t.summary != nil {
		value = t.summary(value)
	}
	styles := r.Styles
	if value == "" {
		if t.input.Placeholder == "" {
			return ""
		}
		return styles.OverlayHint.Inline(true).Render(
			ansi.Truncate(t.input.Placeholder, width, "…"))
	}
	return ansi.Truncate(value, width, "…")
}

// setWidth gives the input its viewport and re-seats the caret in it. Setting
// Width alone is not enough: the scroll offset is computed when the caret
// moves, so a width that arrives afterwards leaves the window stale.
func (t *Text) setWidth(width int) {
	if t.input.Width == width && t.width == width {
		return
	}
	t.input.Width = width
	t.width = width
	t.input.SetCursor(t.input.Position())
}

// styleInput maps the resolved theme onto textinput's inline styles. The
// control names no colour of its own, so a theme change reaches the caret and
// the placeholder without the control knowing a theme changed.
func (t *Text) styleInput(r tideui.Renderer) {
	ws := r.Styles.Workspace
	t.input.TextStyle = lipgloss.NewStyle().Foreground(ws.BodyFg)
	t.input.PlaceholderStyle = lipgloss.NewStyle().Foreground(ws.HintFg)
	t.input.CompletionStyle = lipgloss.NewStyle().Foreground(ws.HintFg)
	t.input.Cursor.Style = lipgloss.NewStyle().Foreground(ws.SelectionBar)
	t.input.Cursor.TextStyle = lipgloss.NewStyle().Foreground(ws.BodyFg)
}

func (t *Text) Hints() []tideui.SoftHint {
	if !t.editing {
		return []tideui.SoftHint{{Key: "enter", Label: "edit"}}
	}
	hints := []tideui.SoftHint{
		{Key: "enter", Label: "done"},
		{Key: "esc", Label: "revert"},
		{Key: "↑↓", Label: "keep & move"},
	}
	if t.input.ShowSuggestions {
		hints = append(hints, tideui.SoftHint{Key: "tab", Label: "complete"})
	}
	return hints
}
