package form

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
)

// Number is a numeric setting.
//
// dash declared FieldFloat from the start and the settings screen never grew a
// case for it, so latitude and longitude fell through to free text and the
// only thing that ever checked them was the save - which failed the whole save
// with one banner naming one field. Here the value is checked as it is typed,
// marked on its own row, and the arrow keys step it.
//
// An empty value is allowed and valid. Most numeric settings are optional, and
// a field that refused to be empty could never be cleared once set.
type Number struct {
	text *Text

	unit     string
	min, max float64
	step     float64
	bounded  bool
}

// NewNumber builds a numeric control.
func NewNumber(value string) *Number {
	n := &Number{}
	n.text = NewText(value).WithValidate(n.validate)
	return n
}

// WithUnit sets the unit shown after the value.
func (n *Number) WithUnit(unit string) *Number {
	n.unit = unit
	return n
}

// WithRange bounds the value. Stepping clamps to the range; typing a value
// outside it is marked rather than refused, so a number can be corrected
// digit by digit instead of becoming unfixable halfway through.
func (n *Number) WithRange(low, high float64) *Number {
	if low == high {
		return n
	}
	if low > high {
		low, high = high, low
	}
	n.min, n.max, n.bounded = low, high, true
	return n
}

// WithStep sets how far an arrow key moves the value. Without a step the field
// is typed rather than stepped.
func (n *Number) WithStep(step float64) *Number {
	n.step = math.Abs(step)
	return n
}

// WithPlaceholder sets what an empty field shows.
func (n *Number) WithPlaceholder(placeholder string) *Number {
	n.text.WithPlaceholder(placeholder)
	return n
}

func (n *Number) Value() string { return n.text.Value() }

func (n *Number) SetValue(value string) { n.text.SetValue(value) }

func (n *Number) Editing() bool { return n.text.Editing() }

func (n *Number) Err() error { return n.text.Err() }

// Float reports the value as a number, and whether there was one to report.
func (n *Number) Float() (float64, bool) {
	value := strings.TrimSpace(n.text.Value())
	if value == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// validate is what makes the field numeric. A lone "-" or a trailing "." is
// accepted while typing: they are how a negative or a decimal begins, and
// rejecting them would make those values impossible to enter.
func (n *Number) validate(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || value == "." || value == "-." {
		return nil
	}
	if strings.HasSuffix(value, ".") {
		value = strings.TrimSuffix(value, ".")
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number", strings.TrimSpace(value))
	}
	if n.bounded && (number < n.min || number > n.max) {
		return fmt.Errorf("must be between %s and %s",
			trimFloat(n.min), trimFloat(n.max))
	}
	return nil
}

// nudge steps the value, clamped to any range.
func (n *Number) nudge(delta float64) bool {
	if n.step == 0 {
		return false
	}
	number, ok := n.Float()
	if !ok {
		// Stepping an empty field starts somewhere sensible rather than doing
		// nothing: the bottom of the range, or zero when unbounded.
		if n.bounded {
			number = n.min
		} else {
			number = 0
		}
		n.text.SetValue(trimFloat(number))
		return true
	}
	number += delta
	if n.bounded {
		number = math.Max(n.min, math.Min(n.max, number))
	}
	n.text.SetValue(trimFloat(number))
	return true
}

func (n *Number) Update(msg tea.KeyMsg) Action {
	if !n.text.Editing() {
		switch msg.String() {
		case "left", "h":
			if n.nudge(-n.step) {
				return ActionChanged
			}
			return ActionIgnored
		case "right", "l":
			if n.nudge(n.step) {
				return ActionChanged
			}
			return ActionIgnored
		}
	}
	return n.text.Update(msg)
}

func (n *Number) View(r tideui.Renderer, width int) string {
	unit := ""
	if n.unit != "" && n.text.Value() != "" && !n.text.Editing() {
		unit = " " + n.unit
	}
	// The value comes first. A cell too narrow to hold both drops the unit
	// rather than the digits: "47" says more than "°N", and a unit that
	// overflowed would corrupt the row it sits in.
	if unitWidth := ansi.StringWidth(unit); unitWidth > 0 {
		if width-unitWidth < 3 {
			unit = ""
		} else {
			width -= unitWidth
		}
	}
	view := n.text.View(r, width)
	if unit == "" {
		return view
	}
	return view + r.Styles.OverlayHint.Inline(true).Render(unit)
}

func (n *Number) Hints() []tideui.SoftHint {
	if n.text.Editing() {
		return n.text.Hints()
	}
	hints := []tideui.SoftHint{{Key: "enter", Label: "edit"}}
	if n.step != 0 {
		hints = append([]tideui.SoftHint{{Key: "←→", Label: "step"}}, hints...)
	}
	return hints
}

// trimFloat writes a number without trailing zeroes, so stepping by 0.1 shows
// "47.7" rather than "47.700000".
func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
