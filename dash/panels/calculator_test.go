package panels

import (
	"strings"
	"testing"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func TestCalculatorEvaluates(t *testing.T) {
	cases := map[string]string{
		"12*8":    "96",
		"1+2+3":   "6",
		"2*(3+4)": "14",
		"10/4":    "2.5",
		"-3+5":    "2",
		"1.5*2":   "3",
	}
	for expr, want := range cases {
		state := evaluate(expr)
		if state.err || state.result != want {
			t.Fatalf("evaluate(%q) = %+v, want %s", expr, state, want)
		}
	}
	for _, expr := range []string{"1/0", "1+", "(1+2", "abc"} {
		if state := evaluate(expr); !state.err {
			t.Fatalf("evaluate(%q) should be an error, got %+v", expr, state)
		}
	}
}

// Typing goes into the panel, and a key it does not want is declined so
// application shortcuts still work.
func TestCalculatorTakesTyping(t *testing.T) {
	c := Calculator().(*calculator)
	if c.Type('q') {
		t.Fatal("a non-expression key should be declined")
	}
	for _, r := range "12*8" {
		if !c.Type(r) {
			t.Fatalf("declined %q", r)
		}
	}
	if state := c.Load(); state.result != "96" {
		t.Fatalf("state = %+v", state)
	}
	if body := ansi.Strip(c.View(tideui.PanelContext{ID: "calculator", Width: 30, Renderer: renderer()})); !strings.Contains(body, "= 96") {
		t.Fatalf("view = %q", body)
	}

	if !c.Backspace() {
		t.Fatal("backspace was declined")
	}
	state := c.Load()
	if state.expr != "12*" || !state.err {
		t.Fatalf("after backspace = %+v, want an incomplete 12*", state)
	}
}

func TestCalculatorIsAnInputPanel(t *testing.T) {
	panel := Calculator()
	if _, ok := panel.(dash.Input); !ok {
		t.Fatal("the calculator should implement dash.Input")
	}
	if !panel.Meta().Hidden {
		t.Fatal("the calculator should start hidden like the other optional panels")
	}
}

func TestCalculatorClearAction(t *testing.T) {
	c := Calculator().(*calculator)
	for _, r := range "7+7" {
		c.Type(r)
	}
	actions := c.Actions()
	// Clear cannot be "c" any more: "c" is the copy key.
	if len(actions) != 1 || actions[0].Key != "x" {
		t.Fatalf("actions = %#v", actions)
	}
	actions[0].Run()
	if got := c.Load().expr; got != "" {
		t.Fatalf("clear left %q", got)
	}
}

// Copy returns the result once the pad computes, and nothing before that.
func TestCalculatorCopy(t *testing.T) {
	c := Calculator().(*calculator)
	if _, ok := c.Copy(); ok {
		t.Fatal("an empty pad should copy nothing")
	}
	for _, r := range "12*8" {
		c.Type(r)
	}
	if text, ok := c.Copy(); !ok || text != "96" {
		t.Fatalf("copy = %q,%v want 96", text, ok)
	}
	// An expression that does not compute has nothing to copy.
	c.Type('+')
	if _, ok := c.Copy(); ok {
		t.Fatal("an incomplete expression should copy nothing")
	}
}
