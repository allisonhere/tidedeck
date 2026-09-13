package tideui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// bleed matches the signature of the bug: a reset followed by padding that
// therefore renders on the terminal's own background rather than the style's.
var bleed = regexp.MustCompile(`\x1b\[0?m {2,}`)

func trueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestStyleOverKeepsBackgroundAcrossNestedResets(t *testing.T) {
	trueColor(t)
	outer := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e")).
		Foreground(lipgloss.Color("#cdd6f4"))
	inner := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Render("KEY")
	text := "a " + inner + "   b"

	if !bleed.MatchString(outer.Render(text)) {
		t.Fatal("expected plain Render to show the bug this helper exists to fix")
	}
	got := StyleOver(outer, text)
	if bleed.MatchString(got) {
		t.Fatalf("StyleOver left a hole: %q", got)
	}
	// Width padding must stay filled too.
	padded := StyleOver(outer.Width(40), text)
	if bleed.MatchString(padded) {
		t.Fatalf("padding bled: %q", padded)
	}
	if lipgloss.Width(padded) != 40 {
		t.Fatalf("StyleOver changed layout: width %d", lipgloss.Width(padded))
	}
	// The visible text is untouched.
	if want := "a KEY   b"; !strings.Contains(stripANSI(got), want) {
		t.Fatalf("text changed: %q", stripANSI(got))
	}
}

func TestStyleOverLeavesPlainTextAlone(t *testing.T) {
	trueColor(t)
	style := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e")).Width(12)
	if got, want := StyleOver(style, "plain"), style.Render("plain"); got != want {
		t.Fatalf("StyleOver(%q) = %q, want %q", "plain", got, want)
	}
	if got := Continuous(style, ""); got != "" {
		t.Fatalf("empty text became %q", got)
	}
}

// The helpers run at several layers of one render, so applying them twice must
// not add a second copy of the same sequence.
func TestContinuousIsIdempotent(t *testing.T) {
	trueColor(t)
	style := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e"))
	text := "a " + lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Render("KEY") + " b"
	once := Continuous(style, text)
	if twice := Continuous(style, once); twice != once {
		t.Fatalf("second pass changed the string:\n once: %q\ntwice: %q", once, twice)
	}
	// A different style over already-continuous text must not stack either.
	other := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e")).Foreground(lipgloss.Color("#fff"))
	if got := Continuous(other, once); got != once {
		t.Fatalf("layered style re-opened an already-open background: %q", got)
	}
}

func TestContinuousDoesNotLeakStylingPastTheEnd(t *testing.T) {
	trueColor(t)
	style := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e"))
	text := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Render("KEY")
	got := Continuous(style, text)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("a trailing reset was reopened, which would leak styling: %q", got)
	}
}

func TestContinuousRespectsColourProfile(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	style := lipgloss.NewStyle().Background(lipgloss.Color("#1e1e2e"))
	// With styling stripped there is nothing to re-open and nothing to add.
	if got := Continuous(style, "a KEY b"); got != "a KEY b" {
		t.Fatalf("ascii profile produced %q", got)
	}
}

func TestOpensBackgroundRecognisesEachColourForm(t *testing.T) {
	for _, seq := range []string{
		"\x1b[48;2;30;30;46m", "\x1b[48;5;236m", "\x1b[41m", "\x1b[101m",
		"\x1b[49m", "\x1b[1;38;2;1;2;3;48;2;4;5;6m",
	} {
		if !opensBackground(seq) {
			t.Fatalf("%q should be recognised as opening a background", seq)
		}
	}
	for _, seq := range []string{
		"\x1b[0m", "\x1b[m", "\x1b[1m", "\x1b[38;2;1;2;3m", "plain", "", "\x1b[38;5;4m",
	} {
		if opensBackground(seq) {
			t.Fatalf("%q should not be treated as opening a background", seq)
		}
	}
}

// Every surface that paints behind caller-supplied content must survive that
// content carrying its own styling.
func TestRenderSurfacesSurviveStyledContent(t *testing.T) {
	trueColor(t)
	r := NewRenderer(CatppuccinMocha, StyleOptions{Density: Compact})
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Render("TAG")
	styled := "before " + tag + "   after"

	cases := map[string]string{
		"RenderRow":      r.RenderRow(Row{Text: styled}, 60),
		"RenderRow/sel":  r.RenderRow(Row{Text: styled, Selected: true}, 60),
		"RenderBlock":    r.RenderBlock(Block{Header: styled, Body: styled}, 60),
		"RenderSoftRow":  r.RenderSoftRow(SoftRow{Text: styled}, 60),
		"RenderSoftBody": r.RenderSoftBody(60, styled),
		"pane":           r.renderPane(Pane{Title: "T", Content: styled}, 60, 10),
		"pane/header":    r.renderPane(Pane{Title: styled, Content: "x"}, 60, 10),
		"status":         r.renderStatus(StatusBar{Left: styled, Right: "hint"}, 60),
		"soft panel":     r.RenderSoftPanel(SoftPanel{Prefix: "app", Title: "t", Content: styled, Width: 50}),
	}
	for name, got := range cases {
		for i, line := range strings.Split(got, "\n") {
			if bleed.MatchString(line) {
				t.Errorf("%s line %d shows the terminal background through styled content: %q",
					name, i, line)
			}
		}
	}
}

// A full layout is the real case: every pane's content is host-composed.
func TestRenderedLayoutHasNoBackgroundHoles(t *testing.T) {
	trueColor(t)
	r := NewRenderer(CatppuccinMocha, StyleOptions{Density: Compact, PaneCorners: RoundCorners})
	key := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true).Render("s")
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("stage")
	// The shape every Tide app builds: styled segments joined by plain spaces.
	composed := " " + key + " " + label + "   " + key + " " + label

	for _, mode := range []LayoutMode{StackedRight, ThreeColumn, SidebarOnly, Tabbed, Floating} {
		view := r.Render(Layout{
			Width: 100, Height: 20, Mode: mode,
			Panes: [3]Pane{
				{Title: "ONE", Hint: "1", Content: composed, Focused: true},
				{Title: "TWO", Content: composed},
				{Title: "THREE", Content: composed},
			},
			Status: &StatusBar{Left: composed, Right: "right"},
		})
		for i, line := range strings.Split(view, "\n") {
			if bleed.MatchString(line) {
				t.Errorf("mode %d line %d has a background hole: %q", mode, i, line)
			}
		}
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}
