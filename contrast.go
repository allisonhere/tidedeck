package tideui

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

// Contrast correction, exported so applications building a Theme from a source
// they do not control — a desktop palette, a user's config file — can hold it
// to the same readability floors the built-in themes meet, instead of each one
// carrying its own copy of the colour maths.

// IsDark reports whether a colour is dark enough to want light text on it.
func IsDark(c lipgloss.Color) bool { return isDark(c) }

// ContrastRatio returns the WCAG contrast ratio between two colours, from 1
// (identical) to 21 (black on white). Unparseable colours return 1.
func ContrastRatio(a, b lipgloss.Color) float64 { return contrastRatio(a, b) }

// ReadableText returns preferred when it clears minimum against bg, and
// otherwise the nearest of black or white that does.
func ReadableText(preferred, bg lipgloss.Color, minimum float64) lipgloss.Color {
	return readableText(preferred, bg, minimum)
}

// MutedText returns a de-emphasised version of text that still clears the
// toolkit's floor for secondary content against bg.
func MutedText(text, bg lipgloss.Color) lipgloss.Color { return mutedText(text, bg) }

// AdjustLightness moves a colour's HSL lightness by delta, clamped to [0,1].
func AdjustLightness(c lipgloss.Color, delta float64) lipgloss.Color {
	return adjustLightness(c, delta)
}

// MixColors blends tint into base by amount, where 0 is base and 1 is tint.
func MixColors(base, tint lipgloss.Color, amount float64) lipgloss.Color {
	br, bg, bb, baseOK := hexToRGB(base)
	tr, tg, tb, tintOK := hexToRGB(tint)
	if !baseOK || !tintOK {
		return base
	}
	amount = clamp01(amount)
	mix := func(a, b float64) int {
		return int(math.Round((a*(1-amount) + b*amount) * 255))
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", mix(br, tr), mix(bg, tg), mix(bb, tb)))
}

// AccentReadableOn keeps an accent's hue while moving its lightness until it
// clears minRatio against bg. Unlike ReadableText it never falls back to black
// or white: an accent that loses its hue stops being an accent. A colour that
// cannot reach the ratio is returned as close as it got.
func AccentReadableOn(accent, bg lipgloss.Color, minRatio float64) lipgloss.Color {
	if accent == "" {
		return ""
	}
	if _, _, _, ok := hexToRGB(accent); !ok {
		return accent
	}
	if contrastRatio(accent, bg) >= minRatio {
		return accent
	}
	const step = 0.06
	const maxSteps = 12
	direction := step
	if !isDark(bg) {
		direction = -step
	}
	cur := accent
	for range maxSteps {
		next := adjustLightness(cur, direction)
		if next == cur {
			break
		}
		cur = next
		if contrastRatio(cur, bg) >= minRatio {
			return cur
		}
	}
	return cur
}

// SurfaceForRatio returns a colour derived from bg that clears minRatio against
// it, for a status bar or overlay that would otherwise be invisible against the
// page behind it.
func SurfaceForRatio(bg lipgloss.Color, minRatio float64) lipgloss.Color {
	return selectionBgForRatio(bg, minRatio)
}

// Contrast floors used by CorrectThemeContrast. They are the floors the
// built-in themes already meet, so correcting a hand-tuned palette is close to
// a no-op and only a genuinely unreadable one is moved.
//
// The focused pane border is not among them: BuildStyles lifts that to
// paneFocusMinContrast when it renders, so a theme is free to hold a quieter
// accent and have the renderer brighten it in the one place it must dominate.
const (
	contrastText    = 4.5 // body text against its own background
	contrastAccent  = 3.0 // accents used as text or fills
	contrastChrome  = 1.3 // borders, which are meant to stay quiet
	contrastSurface = 2.0 // a bar or overlay lifted off the page behind it
)

// CorrectThemeContrast nudges each colour of t until it is readable against the
// theme's own background, and returns the corrected theme. It is safe to call
// on a theme that already passes: colours that clear their floor are returned
// untouched, so a hand-tuned palette is not repainted.
//
// Name and Bg are never changed: the background is the reference everything
// else is corrected against.
func CorrectThemeContrast(t Theme) Theme {
	out := t
	out.Fg = readableText(t.Fg, t.Bg, contrastText)
	out.Dimmed = mutedText(out.Fg, t.Bg)
	out.BorderFocus = AccentReadableOn(t.BorderFocus, t.Bg, contrastAccent)
	out.Selected = AccentReadableOn(t.Selected, t.Bg, contrastAccent)
	out.OverlayBorder = AccentReadableOn(t.OverlayBorder, t.Bg, contrastAccent)
	out.Unread = AccentReadableOn(t.Unread, t.Bg, contrastAccent)
	out.Error = AccentReadableOn(t.Error, t.Bg, contrastAccent)
	out.Border = AccentReadableOn(t.Border, t.Bg, contrastChrome)

	// A status bar the same colour as the page has no edge; lift it until it
	// reads as its own surface.
	if contrastRatio(out.StatusBar, t.Bg) < 1.2 {
		out.StatusBar = selectionBgForRatio(t.Bg, contrastSurface)
		out.Overlay = out.StatusBar
	}
	out.StatusFg = readableText(t.StatusFg, out.StatusBar, contrastText)
	return out
}

// Hue returns a colour's hue in degrees (0-360) and its saturation (0-1).
// ok is false for a colour that is not a parseable hex value. A greyscale
// colour reports a hue of 0 with a saturation of 0, which is what callers need
// to recognise a palette that has no colour to vary.
func Hue(c lipgloss.Color) (degrees, saturation float64, ok bool) {
	r, g, b, ok := hexToRGB(c)
	if !ok {
		return 0, 0, false
	}
	h, s, _ := rgbToHSL(r, g, b)
	return h * 360, s, true
}

// ShiftHue rotates a colour around the colour wheel by degrees, keeping its
// saturation and lightness. It is the companion to AdjustLightness for building
// a set of related colours — a graph's lanes, a chart's series — that belong to
// the same palette instead of being picked arbitrarily.
//
// A greyscale colour has no hue to rotate and is returned unchanged, so a
// monochrome theme cannot be turned into a colourful one by accident.
func ShiftHue(c lipgloss.Color, degrees float64) lipgloss.Color {
	r, g, b, ok := hexToRGB(c)
	if !ok {
		return c
	}
	h, s, l := rgbToHSL(r, g, b)
	if s == 0 {
		return c
	}
	// A whole number of turns is the identity. Returning the input avoids the
	// one-bit drift an RGB -> HSL -> RGB round trip would otherwise introduce.
	if math.Mod(degrees, 360) == 0 {
		return c
	}
	h = math.Mod(math.Mod(h+degrees/360, 1)+1, 1)
	nr, ng, nb := hslToRGB(h, s, l)
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		uint8(math.Round(nr*255)), uint8(math.Round(ng*255)), uint8(math.Round(nb*255))))
}
