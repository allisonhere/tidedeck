package tideui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// An ANSI reset is not scoped to the span that emitted it. `ESC[0m` clears
// every attribute on the terminal, including the background of whatever style
// happens to be wrapping that span. Rendering
//
//	outer.Background(bg).Render("a " + inner.Render("KEY") + " b")
//
// therefore paints "a " with bg, loses bg at the inner span's reset, and draws
// " b" — and any padding after it — on the terminal's own background. In a
// filled row that reads as a hole punched through the middle of the line.
//
// Lipgloss does not re-open the outer style after a nested reset, so any host
// that composes styled segments and then wraps them for background hits this.
// StyleOver and Continuous are the fix, applied inside every TideUI surface
// that paints behind caller-supplied content, and exported so hosts composing
// their own rows get the same guarantee.

// ansiReset matches the two spellings of a full SGR reset that lipgloss and
// termenv emit. A parameterised reset such as ESC[0;1m is deliberately not
// matched: it re-establishes attributes itself.
var ansiReset = regexp.MustCompile("\x1b\\[0?m")

// StyleOver renders text with style, keeping style's own attributes continuous
// across any styling text already carries.
//
// Use it anywhere caller-supplied or multi-segment content is rendered through
// a style with a background. For plain text it is identical to style.Render.
func StyleOver(style lipgloss.Style, text string) string {
	return style.Render(Continuous(style, text))
}

// Continuous re-applies style's colour attributes after every reset in text,
// adding no layout of its own. Use it when the padding, width or placement is
// already handled and only the background continuity is missing; otherwise
// prefer StyleOver.
//
// Text with no styling is returned unchanged, as is any text rendered under a
// colour profile that strips styling.
func Continuous(style lipgloss.Style, text string) string {
	if text == "" || !strings.Contains(text, "\x1b[") {
		return text
	}
	attrs := styleAttributes(style)
	if attrs == "" {
		return text
	}
	return reopenAfterResets(text, attrs)
}

// ContinuousBackground is Continuous for a bare background colour, for fills
// that have no style value to borrow from.
func ContinuousBackground(text string, background lipgloss.Color) string {
	if background == "" {
		return text
	}
	return Continuous(lipgloss.NewStyle().Background(background), text)
}

// reopenAfterResets re-emits attrs immediately after each reset in text. A
// reset at the very end is left closed so the sequence does not leak styling
// into whatever is concatenated next, and a reset the text already re-opens
// for itself is left alone: these helpers are applied at several layers of a
// render, so the transform has to be idempotent rather than piling up
// redundant sequences at every layer it passes through.
func reopenAfterResets(text, attrs string) string {
	matches := ansiReset.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + len(matches)*len(attrs))
	last := 0
	for _, loc := range matches {
		b.WriteString(text[last:loc[1]])
		if loc[1] < len(text) && !opensBackground(text[loc[1]:]) {
			b.WriteString(attrs)
		}
		last = loc[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

// opensBackground reports whether text begins with an SGR sequence that
// establishes a background, which makes re-opening redundant. Only the
// background matters: it is the attribute a reset takes away that the
// surrounding fill needs back.
func opensBackground(text string) bool {
	if !strings.HasPrefix(text, "\x1b[") {
		return false
	}
	end := strings.IndexByte(text, 'm')
	if end < 0 {
		return false
	}
	for _, param := range strings.Split(text[2:end], ";") {
		switch {
		case param == "48", param == "49": // extended colour, default background
			return true
		case len(param) == 2 && param[0] == '4' && param[1] <= '7': // 40-47
			return true
		case len(param) == 3 && strings.HasPrefix(param, "10") && param[2] <= '7': // 100-107
			return true
		}
	}
	return false
}

// styleAttributes returns the SGR sequence style opens with, or "" when the
// style adds no visible attributes under the active colour profile. Layout —
// width, padding, margins, borders, alignment — is deliberately excluded: the
// sequence is re-emitted mid-line, where layout must not be repeated.
func styleAttributes(style lipgloss.Style) string {
	probe := lipgloss.NewStyle().
		Foreground(style.GetForeground()).
		Background(style.GetBackground()).
		Bold(style.GetBold()).
		Faint(style.GetFaint()).
		Italic(style.GetItalic()).
		Underline(style.GetUnderline()).
		Reverse(style.GetReverse()).
		Blink(style.GetBlink()).
		Strikethrough(style.GetStrikethrough()).
		Render(attributeProbe)
	prefix, _, found := strings.Cut(probe, attributeProbe)
	if !found {
		return ""
	}
	return prefix
}

// attributeProbe is a sentinel lipgloss will not rewrite, used to find where
// a style's opening sequence ends.
const attributeProbe = "\x00"
