package form

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
)

// key builds the key message a control would receive for a named key.
func key(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+w":
		return tea.KeyMsg{Type: tea.KeyCtrlW}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// typeIn sends each rune of text as its own keystroke, the way a person types.
func typeIn(c Control, text string) {
	for _, r := range text {
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func testRenderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
}

// Every control must satisfy Control, which is what lets the settings screen
// hold them in one slice and dispatch without knowing which is which.
var (
	_ Control = (*Text)(nil)
	_ Control = (*Choice)(nil)
	_ Control = (*Toggle)(nil)
	_ Control = (*Number)(nil)
	_ Control = (*Button)(nil)

	_ Overlayer = (*Choice)(nil)
)

func TestTextEditsAndCommits(t *testing.T) {
	text := NewText("hello")
	if got := text.Update(key("enter")); got != ActionEditing {
		t.Fatalf("enter = %v, want ActionEditing", got)
	}
	if !text.Editing() {
		t.Fatal("not editing after enter")
	}
	typeIn(text, " there")
	if text.Value() != "hello there" {
		t.Fatalf("value = %q", text.Value())
	}
	if got := text.Update(key("enter")); got != ActionChanged {
		t.Fatalf("commit = %v, want ActionChanged", got)
	}
	if text.Editing() {
		t.Fatal("still editing after commit")
	}
}

// esc must put back exactly what was there, not merely stop editing.
func TestTextCancelRestores(t *testing.T) {
	text := NewText("original")
	text.Update(key("enter"))
	typeIn(text, "-edited")
	if text.Value() == "original" {
		t.Fatal("edit did not take")
	}
	if got := text.Update(key("esc")); got != ActionCancelled {
		t.Fatalf("esc = %v, want ActionCancelled", got)
	}
	if text.Value() != "original" {
		t.Fatalf("value = %q, want the value restored", text.Value())
	}
}

// A control that is not being edited must let keys through, or a settings row
// would swallow the navigation that moves off it.
func TestTextIgnoresKeysWhenIdle(t *testing.T) {
	text := NewText("x")
	for _, name := range []string{"up", "down", "left", "right", "j", "k"} {
		if got := text.Update(key(name)); got != ActionIgnored {
			t.Fatalf("idle %q = %v, want ActionIgnored", name, got)
		}
	}
}

// The editing keys a hand-rolled caret never had.
func TestTextWordAndLineEditing(t *testing.T) {
	text := NewText("")
	text.Update(key("enter"))
	typeIn(text, "one two three")

	text.Update(key("ctrl+w")) // delete word backward
	if text.Value() != "one two " {
		t.Fatalf("after ctrl+w: %q", text.Value())
	}
	text.Update(key("ctrl+u")) // delete to line start
	if text.Value() != "" {
		t.Fatalf("after ctrl+u: %q", text.Value())
	}
}

// A value that fails validation is kept and marked, never refused. Refusing it
// would make a partial value impossible to finish typing.
func TestTextValidationMarksButKeeps(t *testing.T) {
	text := NewText("").WithValidate(func(value string) error {
		if strings.Contains(value, "!") {
			return errors.New("no bangs")
		}
		return nil
	})
	text.Update(key("enter"))
	typeIn(text, "oh!")
	if text.Value() != "oh!" {
		t.Fatalf("value = %q, want the rejected text kept", text.Value())
	}
	if text.Err() == nil {
		t.Fatal("Err = nil, want the validation error")
	}
	text.Update(key("backspace"))
	if text.Err() != nil {
		t.Fatalf("Err = %v, want nil once the value is valid again", text.Err())
	}
}

func TestTextNormalizesOnCommit(t *testing.T) {
	text := NewText("").WithNormalize(strings.TrimSpace)
	text.Update(key("enter"))
	typeIn(text, "  padded  ")
	text.Update(key("enter"))
	if text.Value() != "padded" {
		t.Fatalf("value = %q, want it normalized on commit", text.Value())
	}
}

func TestTextShowsPlaceholderWhenEmpty(t *testing.T) {
	text := NewText("").WithPlaceholder("every account")
	view := text.View(testRenderer(), 30)
	if !strings.Contains(view, "every account") {
		t.Fatalf("view = %q, want the placeholder", view)
	}
}

func TestToggleTakesArrowsAndSpace(t *testing.T) {
	toggle := NewToggle(false)
	if got := toggle.Update(key(" ")); got != ActionChanged || !toggle.On() {
		t.Fatalf("space = %v on=%v", got, toggle.On())
	}
	// right on an already-on toggle changes nothing, so the key should fall
	// through rather than reporting a change that did not happen.
	if got := toggle.Update(key("right")); got != ActionIgnored {
		t.Fatalf("right when on = %v, want ActionIgnored", got)
	}
	if got := toggle.Update(key("left")); got != ActionChanged || toggle.On() {
		t.Fatalf("left = %v on=%v", got, toggle.On())
	}
}

func TestChoiceStepsAndWraps(t *testing.T) {
	choice := NewChoice([]string{"a", "b", "c"}, "a")
	choice.Update(key("right"))
	if choice.Value() != "b" {
		t.Fatalf("value = %q", choice.Value())
	}
	choice.Update(key("left"))
	choice.Update(key("left"))
	if choice.Value() != "c" {
		t.Fatalf("value = %q, want a wrap to the end", choice.Value())
	}
}

// A short list steps on enter; a long one opens a picker, because cycling
// blind through nine options is not a control.
func TestChoiceOpensPickerOnlyWhenLong(t *testing.T) {
	short := NewChoice([]string{"a", "b"}, "a")
	short.Update(key("enter"))
	if short.Editing() {
		t.Fatal("short choice opened a picker")
	}

	long := NewChoice([]string{"a", "b", "c", "d", "e", "f"}, "a")
	if got := long.Update(key("enter")); got != ActionEditing {
		t.Fatalf("enter = %v, want ActionEditing", got)
	}
	if !long.Editing() {
		t.Fatal("long choice did not open a picker")
	}
	if _, ok := long.Overlay(testRenderer(), 40, 12); !ok {
		t.Fatal("no overlay while the picker is open")
	}
}

func TestChoicePickerRevertsOnEsc(t *testing.T) {
	choice := NewChoice([]string{"a", "b", "c", "d", "e", "f"}, "a")
	choice.Update(key("enter"))
	choice.Update(key("down"))
	choice.Update(key("down"))
	if choice.Value() != "c" {
		t.Fatalf("value = %q, want the cursor to preview", choice.Value())
	}
	if got := choice.Update(key("esc")); got != ActionCancelled {
		t.Fatalf("esc = %v, want ActionCancelled", got)
	}
	if choice.Value() != "a" {
		t.Fatalf("value = %q, want the original restored", choice.Value())
	}
	if choice.Editing() {
		t.Fatal("picker still open after esc")
	}
}

// The sample must never replace the name: on the old screen the gauge preview
// overwrote the value, so the row could not say which style was selected.
func TestChoiceViewKeepsNameBesideSample(t *testing.T) {
	choice := NewChoice([]string{"blocks", "bars", "dots"}, "blocks").
		WithSample(func(tideui.Renderer, string, int) string { return "▁▂▃▄" })
	view := choice.View(testRenderer(), 30)
	if !strings.Contains(view, "blocks") {
		t.Fatalf("view = %q, want the option name", view)
	}
	if !strings.Contains(view, "▁▂▃▄") {
		t.Fatalf("view = %q, want the sample too", view)
	}
}

func TestNumberRejectsLettersKeepsPartials(t *testing.T) {
	number := NewNumber("")
	number.Update(key("enter"))
	typeIn(number, "47.")
	if number.Err() != nil {
		t.Fatalf("Err = %v, want a trailing dot accepted while typing", number.Err())
	}
	typeIn(number, "6")
	if number.Err() != nil {
		t.Fatalf("Err = %v, want 47.6 valid", number.Err())
	}
	if got, ok := number.Float(); !ok || got != 47.6 {
		t.Fatalf("Float = %v %v", got, ok)
	}

	letters := NewNumber("")
	letters.Update(key("enter"))
	typeIn(letters, "abc")
	if letters.Err() == nil {
		t.Fatal("Err = nil, want letters marked invalid")
	}
	if letters.Value() != "abc" {
		t.Fatalf("value = %q, want the bad value kept so it can be fixed", letters.Value())
	}
}

func TestNumberStepsAndClamps(t *testing.T) {
	number := NewNumber("5").WithRange(0, 10).WithStep(2)
	number.Update(key("right"))
	if number.Value() != "7" {
		t.Fatalf("value = %q", number.Value())
	}
	number.Update(key("right"))
	number.Update(key("right"))
	if number.Value() != "10" {
		t.Fatalf("value = %q, want a clamp at the maximum", number.Value())
	}
	for range 10 {
		number.Update(key("left"))
	}
	if number.Value() != "0" {
		t.Fatalf("value = %q, want a clamp at the minimum", number.Value())
	}
}

// An out-of-range typed value is marked, not refused, so it can be corrected.
func TestNumberMarksOutOfRange(t *testing.T) {
	number := NewNumber("").WithRange(-90, 90)
	number.Update(key("enter"))
	typeIn(number, "120")
	if number.Err() == nil {
		t.Fatal("Err = nil, want an out-of-range value marked")
	}
	if number.Value() != "120" {
		t.Fatalf("value = %q, want it kept", number.Value())
	}
}

// Empty is valid: most numeric settings are optional, and a field that refused
// to be empty could never be cleared once set.
func TestNumberAllowsEmpty(t *testing.T) {
	number := NewNumber("").WithRange(-90, 90)
	if number.Err() != nil {
		t.Fatalf("Err = %v, want empty to be valid", number.Err())
	}
	if _, ok := number.Float(); ok {
		t.Fatal("Float reported a value for an empty field")
	}
}

// Without a step the field is typed, not stepped, so the arrow keys stay
// available to the host for navigation.
func TestNumberWithoutStepIgnoresArrows(t *testing.T) {
	number := NewNumber("5")
	if got := number.Update(key("right")); got != ActionIgnored {
		t.Fatalf("right = %v, want ActionIgnored without a step", got)
	}
}

func TestButtonRunsOnceWhileBusy(t *testing.T) {
	runs := 0
	button := NewButton("Install", func() string {
		runs++
		return "installing…"
	})
	button.Update(key("enter"))
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
	message, ok := button.TakeMessage()
	if !ok || message != "installing…" {
		t.Fatalf("message = %q %v", message, ok)
	}
	if _, ok := button.TakeMessage(); ok {
		t.Fatal("message returned twice")
	}

	button.Busy()
	button.Update(key("enter"))
	if runs != 1 {
		t.Fatalf("runs = %d, want the second press swallowed while busy", runs)
	}
	button.Done()
	button.Update(key("enter"))
	if runs != 2 {
		t.Fatalf("runs = %d, want it runnable again once done", runs)
	}
}

// Every control must render within the width it is given; one that overflows
// corrupts the row it sits in.
func TestViewsRespectWidth(t *testing.T) {
	r := testRenderer()
	controls := map[string]Control{
		"text":   NewText("a value long enough to need truncating somewhere"),
		"choice": NewChoice([]string{"alpha", "beta", "gamma", "delta", "epsilon"}, "alpha"),
		"toggle": NewToggle(true),
		"number": NewNumber("47.6").WithUnit("°N"),
		"button": NewButton("Go", func() string { return "" }),
	}
	for name, control := range controls {
		for _, width := range []int{4, 8, 16, 40} {
			view := control.View(r, width)
			if got := ansi.StringWidth(view); got > width {
				t.Errorf("%s at width %d rendered %d cells: %q", name, width, got, view)
			}
		}
	}
}

// The keys that move between rows and panes everywhere else must keep working
// mid-edit. Swallowed, they make a text field a trap: nothing happens, and the
// only way out is a key you have to already know about.
func TestTextHandsBackNavigationKeys(t *testing.T) {
	for _, name := range []string{"up", "down", "tab", "shift+tab"} {
		text := NewText("start")
		text.Begin()
		typeIn(text, "-edited")

		if got := text.Update(key(name)); got != ActionIgnored {
			t.Errorf("%s = %v, want ActionIgnored so the host can navigate", name, got)
		}
		if text.Editing() {
			t.Errorf("%s left the field still editing", name)
		}
		// Moving off a field keeps what was typed; only esc discards.
		if got := text.Value(); got != "start-edited" {
			t.Errorf("%s: value = %q, want the edit kept", name, got)
		}
	}
}

// left and right stay with the caret, which is the whole point of having one.
func TestTextKeepsCaretKeys(t *testing.T) {
	for _, name := range []string{"left", "right"} {
		text := NewText("hello")
		text.Begin()
		if got := text.Update(key(name)); got == ActionIgnored {
			t.Errorf("%s = ActionIgnored, want the caret to move", name)
		}
		if !text.Editing() {
			t.Errorf("%s ended the edit", name)
		}
	}
}

// An option with no text is a real choice — "every account", "the inbox" — and
// drawing it as an empty row would make it look broken.
func TestChoiceLabelsTheBlankOption(t *testing.T) {
	r := testRenderer()

	choice := NewChoice([]string{"", "Gmail", "work"}, "")
	if view := choice.View(r, 30); !strings.Contains(view, "any") {
		t.Errorf("view = %q, want the default blank label", view)
	}

	named := NewChoice([]string{"", "Gmail"}, "").WithBlankLabel("every account")
	if view := named.View(r, 30); !strings.Contains(view, "every account") {
		t.Errorf("view = %q, want the given blank label", view)
	}
	// The stored value is still empty; only the display changed.
	if named.Value() != "" {
		t.Errorf("value = %q, want it left empty", named.Value())
	}

	// And in the picker, where a blank row would be worst.
	long := NewChoice([]string{"", "a", "b", "c", "d", "e"}, "").WithBlankLabel("every account")
	long.Update(key("enter"))
	overlay, ok := long.Overlay(r, 44, 14)
	if !ok {
		t.Fatal("no picker")
	}
	if !strings.Contains(overlay.Content, "every account") {
		t.Error("the picker draws the blank option as an empty row")
	}
}

// A value wider than its cell has to scroll so the caret stays visible. The
// input computes that window when the caret moves, against its own Width — so a
// width that only arrives at render time leaves the window stale, and typing
// runs off the end of a field that never scrolls. What you type must be what
// you see.
func TestTextFollowsTheCaretInANarrowCell(t *testing.T) {
	r := testRenderer()
	const value = "/home/allie/.local/share/tidemail/mail.db"

	text := NewText("")
	text.Begin()
	typeIn(text, value)

	for _, width := range []int{20, 30, 44} {
		view := ansi.Strip(text.View(r, width))
		if got := ansi.StringWidth(view); got > width {
			t.Errorf("width %d: rendered %d cells", width, got)
		}
		// The caret is at the end, so the end of the value is what must show.
		if !strings.Contains(view, "mail.db") {
			t.Errorf("width %d: view = %q, want the caret end of the value", width, view)
		}
		// And not the head, which is what a stale window shows.
		if strings.HasPrefix(strings.TrimSpace(view), "/home/allie") && width < 41 {
			t.Errorf("width %d: view = %q, window did not follow the caret", width, view)
		}
	}

	// Moving the caret home scrolls back.
	text.Update(key("ctrl+a"))
	if view := ansi.Strip(text.View(r, 20)); !strings.Contains(view, "/home") {
		t.Errorf("after ctrl+a: view = %q, want the start of the value", view)
	}
}
