package form

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
)

// pickAbove is how many options a choice must have before enter opens a list
// instead of simply stepping. Below it a picker is ceremony; at or above it,
// stepping means discovering the options one keystroke at a time with no way
// to see how many remain.
const pickAbove = 5

// Sample draws a preview of an option - a gauge bar, a sparkline - to show
// what the value looks like rather than only what it is called. It returns the
// empty string for an option it has no preview for.
type Sample func(r tideui.Renderer, option string, width int) string

// Choice is a setting with a fixed set of values.
//
// Arrow keys step through the options, as before. What is new is that enter
// opens a list when there are enough options to be worth seeing: nine
// sparkline styles cycled blind is not a control, it is a guessing game.
//
// The picker keeps the theme picker's preview/commit split - moving the cursor
// previews, enter commits, esc restores - because a choice that changes the
// look of the dashboard should show the change while you are choosing.
type Choice struct {
	options []string
	index   int

	open      bool
	cursor    int
	committed int

	sample Sample
	title  string
	// blank is how the empty option reads. An option with no text is a real
	// choice - "every account", "the default socket" - and drawing it as an
	// empty row would make it look like a bug.
	blank string
}

// NewChoice builds a choice. A value that is not among the options starts at
// the first one, which is what happens when an option is renamed out from
// under a saved config.
func NewChoice(options []string, value string) *Choice {
	c := &Choice{options: options}
	c.index = max(0, indexOf(options, value))
	c.committed = c.index
	return c
}

// WithSample sets the preview renderer. The preview is drawn beside the name,
// never instead of it - the old screen replaced the name with the sample, so
// the one thing the row could not tell you was which style was selected.
func (c *Choice) WithSample(sample Sample) *Choice {
	c.sample = sample
	return c
}

// WithBlankLabel sets how the empty option reads. It defaults to "any".
func (c *Choice) WithBlankLabel(label string) *Choice {
	c.blank = label
	return c
}

// display is an option as it should be shown.
func (c *Choice) display(option string) string {
	if option != "" {
		return option
	}
	if c.blank != "" {
		return c.blank
	}
	return "any"
}

// WithTitle names the picker.
func (c *Choice) WithTitle(title string) *Choice {
	c.title = title
	return c
}

func (c *Choice) Value() string {
	if len(c.options) == 0 {
		return ""
	}
	return c.options[clamp(c.index, 0, len(c.options)-1)]
}

func (c *Choice) SetValue(value string) {
	c.index = max(0, indexOf(c.options, value))
	c.committed = c.index
}

// Editing reports whether the picker is open. A stepping choice never holds
// the keyboard; an open picker does.
func (c *Choice) Editing() bool { return c.open }

func (c *Choice) Err() error { return nil }

// step moves by delta, wrapping.
func (c *Choice) step(delta int) {
	c.index = wrap(c.index+delta, len(c.options))
}

func (c *Choice) Update(msg tea.KeyMsg) Action {
	if len(c.options) == 0 {
		return ActionIgnored
	}
	if c.open {
		return c.updateOpen(msg)
	}
	switch msg.String() {
	case "left", "h":
		c.step(-1)
		return ActionChanged
	case "right", "l":
		c.step(1)
		return ActionChanged
	case "enter", " ":
		if len(c.options) >= pickAbove {
			c.open = true
			c.cursor = c.index
			c.committed = c.index
			return ActionEditing
		}
		c.step(1)
		return ActionChanged
	}
	return ActionIgnored
}

// updateOpen drives the picker. Moving the cursor moves the value too, so the
// host's live preview shows the option under the cursor.
func (c *Choice) updateOpen(msg tea.KeyMsg) Action {
	switch msg.String() {
	case "up", "k":
		c.cursor = wrap(c.cursor-1, len(c.options))
		c.index = c.cursor
		return ActionChanged
	case "down", "j":
		c.cursor = wrap(c.cursor+1, len(c.options))
		c.index = c.cursor
		return ActionChanged
	case "home", "g":
		c.cursor, c.index = 0, 0
		return ActionChanged
	case "end", "G":
		c.cursor = len(c.options) - 1
		c.index = c.cursor
		return ActionChanged
	case "enter", " ":
		c.open = false
		c.index = c.cursor
		if c.index != c.committed {
			c.committed = c.index
			return ActionChanged
		}
		return ActionCommitted
	case "esc":
		c.open = false
		changed := c.index != c.committed
		c.index, c.cursor = c.committed, c.committed
		if changed {
			return ActionCancelled
		}
		return ActionCommitted
	}
	return ActionEditing
}

func (c *Choice) View(r tideui.Renderer, width int) string {
	if len(c.options) == 0 || width < 3 {
		return ""
	}
	name := c.display(c.Value())
	if c.sample != nil {
		// The sample is worth a cell only once the name has one. Budgeting the
		// name first is what keeps the selected value readable in a narrow
		// pane instead of being replaced by a picture of itself.
		if sampleWidth := width - ansi.StringWidth(name) - 5; sampleWidth >= 4 {
			if sample := c.sample(r, name, min(sampleWidth, 8)); sample != "" {
				return ansi.Truncate(name+"  "+sample, width, "…")
			}
		}
	}
	if len(c.options) >= pickAbove {
		// A bare name gives no sign that there is a list behind it.
		return ansi.Truncate(name+" ▾", width, "…")
	}
	return ansi.Truncate("‹ "+name+" ›", width, "…")
}

// Overlay draws the picker, and reports whether there is one to draw.
func (c *Choice) Overlay(r tideui.Renderer, width, height int) (tideui.Overlay, bool) {
	if !c.open {
		return tideui.Overlay{}, false
	}
	if width <= 0 {
		width = 40
	}
	title := c.title
	if title == "" {
		title = "choose"
	}
	rowsAvailable := max(1, height-4)
	first, last := tideui.VisibleRange(len(c.options), c.cursor, rowsAvailable)
	innerWidth := max(1, width-4)

	rows := make([]string, 0, last-first+2)
	for index := first; index < last; index++ {
		row := tideui.SoftRow{Text: c.display(c.options[index]), Selected: index == c.cursor}
		if c.sample != nil {
			row.Suffix = c.sample(r, c.options[index], 8)
		}
		rows = append(rows, r.RenderSoftRow(row, innerWidth))
	}
	// Say where you are when the list does not fit, so a long list has a
	// bottom rather than just continuing.
	if last-first < len(c.options) {
		rows = append(rows, r.RenderSoftRow(tideui.SoftRow{
			Text:  positionLabel(c.cursor, len(c.options)),
			Muted: true,
		}, innerWidth))
	}
	rows = append(rows, "", r.RenderSoftHints(innerWidth,
		tideui.SoftHint{Key: "enter", Label: "confirm"},
		tideui.SoftHint{Key: "esc", Label: "revert"},
	))

	return r.SoftPanelOverlay(tideui.SoftPanel{
		Prefix:  "tide",
		Title:   strings.ToLower(title),
		Content: r.RenderSoftBody(width, strings.Join(rows, "\n")),
		Width:   width,
	}), true
}

func (c *Choice) Hints() []tideui.SoftHint {
	if c.open {
		return []tideui.SoftHint{
			{Key: "enter", Label: "confirm"},
			{Key: "esc", Label: "revert"},
		}
	}
	if len(c.options) >= pickAbove {
		return []tideui.SoftHint{
			{Key: "←→", Label: "step"},
			{Key: "enter", Label: "list"},
		}
	}
	return []tideui.SoftHint{{Key: "←→", Label: "change"}}
}

// positionLabel reads "3 of 9", so a windowed list says how much of it you are
// looking at.
func positionLabel(cursor, total int) string {
	return strconv.Itoa(cursor+1) + " of " + strconv.Itoa(total)
}
