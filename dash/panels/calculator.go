package panels

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
)

// calculator is a live scratchpad: it takes digits and operators while it has
// focus and shows the running result. It is the panel that implements Input,
// which is what lets a panel accept typing at all.
type calculator struct {
	dash.State[calcState]
}

// calcState is the typed expression and what it evaluates to.
type calcState struct {
	expr   string
	result string
	err    bool
}

// Calculator builds the calculator panel.
func Calculator() dash.Panel { return &calculator{} }

func (c *calculator) Meta() dash.Meta {
	return dash.Meta{
		ID: "calculator", Title: "Calculator",
		Role: tideui.RoleOptional, Priority: 30,
		MinWidth: 18, MinHeight: 4, HideBelow: 92,
		// Off until the picker shows it, like the other optional panels, so it
		// does not claim a slot in every preset.
		Hidden: true,
	}
}

// Type accepts a digit, operator or parenthesis and recomputes. Anything else
// is declined, so application shortcuts still work while the panel is focused.
func (c *calculator) Type(r rune) bool {
	if !strings.ContainsRune("0123456789.+-*/()", r) {
		return false
	}
	state := c.Load()
	c.Store(evaluate(state.expr + string(r)))
	return true
}

// Backspace removes the last character.
func (c *calculator) Backspace() bool {
	state := c.Load()
	if state.expr == "" {
		return false
	}
	runes := []rune(state.expr)
	c.Store(evaluate(string(runes[:len(runes)-1])))
	return true
}

// reset clears the pad, for the panel's own "clear" action.
func (c *calculator) reset() { c.Store(calcState{}) }

// Actions offers a clear key. "c" is reserved for copy, which the application
// handles, so clearing lives on "x".
func (c *calculator) Actions() []dash.Action {
	return []dash.Action{{
		ID: "clear", Key: "x", Label: "clear",
		Run: func() string { c.reset(); return "calculator cleared" },
	}}
}

// Copy returns the result, so "c" puts the value on the clipboard. A pad that
// does not compute yet has nothing worth copying.
func (c *calculator) Copy() (string, bool) {
	state := c.Load()
	if state.err || state.result == "" {
		return "", false
	}
	return state.result, true
}

func (c *calculator) View(ctx tideui.PanelContext) string {
	state := c.Load()
	ws := ctx.Renderer.Styles.Workspace
	bg := ws.Bg

	expr := state.expr
	if expr == "" {
		expr = "type a sum, e.g. 12*8"
	}
	expression := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Render(expr)
	lines := []string{expression}
	if state.expr != "" {
		style := lipgloss.NewStyle().Background(bg).Foreground(ws.FrameActive).Bold(true)
		if state.err {
			style = style.Foreground(ws.MetricBad)
		}
		lines = append(lines, style.Render("= "+state.result))
	}
	if ctx.Zoomed {
		lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(ws.HintFg).
			Render("digits and + - * / ( ) · backspace to edit"))
	}
	return ctx.Renderer.RenderLines(lines, ctx.Width, bg)
}

// evaluate computes the expression, keeping the message when it does not parse.
func evaluate(expr string) calcState {
	state := calcState{expr: expr}
	if strings.TrimSpace(expr) == "" {
		return state
	}
	value, err := evalExpr(expr)
	if err != nil {
		state.err, state.result = true, err.Error()
		return state
	}
	state.result = strconv.FormatFloat(value, 'f', -1, 64)
	return state
}

// evalExpr evaluates numbers with decimals, + - * /, parentheses and unary
// minus. It parses the input rather than evaluating it, so the panel cannot run
// anything but arithmetic.
func evalExpr(input string) (float64, error) {
	parser := &exprParser{text: []rune(strings.ReplaceAll(input, " ", ""))}
	value, err := parser.parseExpr()
	if err != nil {
		return 0, err
	}
	if parser.pos != len(parser.text) {
		return 0, fmt.Errorf("unexpected %q", parser.text[parser.pos])
	}
	return value, nil
}

type exprParser struct {
	text []rune
	pos  int
}

func (p *exprParser) parseExpr() (float64, error) {
	value, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.text) {
		switch p.text[p.pos] {
		case '+':
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value += right
		case '-':
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value -= right
		default:
			return value, nil
		}
	}
	return value, nil
}

func (p *exprParser) parseTerm() (float64, error) {
	value, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.text) {
		switch p.text[p.pos] {
		case '*':
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			value *= right
		case '/':
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("divide by zero")
			}
			value /= right
		default:
			return value, nil
		}
	}
	return value, nil
}

func (p *exprParser) parseFactor() (float64, error) {
	if p.pos >= len(p.text) {
		return 0, fmt.Errorf("incomplete")
	}
	switch ch := p.text[p.pos]; ch {
	case '+', '-':
		p.pos++
		value, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if ch == '-' {
			return -value, nil
		}
		return value, nil
	case '(':
		p.pos++
		value, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.text) || p.text[p.pos] != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.pos++
		return value, nil
	default:
		return p.parseNumber()
	}
}

func (p *exprParser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.text) && (unicode.IsDigit(p.text[p.pos]) || p.text[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("unexpected %q", p.text[p.pos])
	}
	value, err := strconv.ParseFloat(string(p.text[start:p.pos]), 64)
	if err != nil {
		return 0, fmt.Errorf("bad number")
	}
	return value, nil
}
