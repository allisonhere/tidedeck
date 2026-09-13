package tideui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A palette taken from a desktop theme can be anything at all; correction has
// to make it readable without discarding its identity.
func TestCorrectThemeContrastMeetsFloors(t *testing.T) {
	unreadable := Theme{
		Name:          "hostile",
		Bg:            lipgloss.Color("#101014"),
		Fg:            lipgloss.Color("#131318"), // nearly invisible on Bg
		Border:        lipgloss.Color("#121216"),
		BorderFocus:   lipgloss.Color("#14141a"),
		Selected:      lipgloss.Color("#15151b"),
		Unread:        lipgloss.Color("#111115"),
		Dimmed:        lipgloss.Color("#101015"),
		StatusBar:     lipgloss.Color("#101014"), // identical to Bg
		StatusFg:      lipgloss.Color("#101014"),
		Error:         lipgloss.Color("#121217"),
		Overlay:       lipgloss.Color("#101014"),
		OverlayBorder: lipgloss.Color("#101014"),
	}
	got := CorrectThemeContrast(unreadable)

	if got.Bg != unreadable.Bg || got.Name != unreadable.Name {
		t.Fatal("correction changed the background or the name")
	}
	for _, c := range []struct {
		name  string
		color lipgloss.Color
		min   float64
	}{
		{"Fg", got.Fg, contrastText},
		{"BorderFocus", got.BorderFocus, contrastAccent},
		{"Selected", got.Selected, contrastAccent},
		{"Border", got.Border, contrastChrome},
		{"Unread", got.Unread, contrastAccent},
		{"Error", got.Error, contrastAccent},
	} {
		if r := ContrastRatio(c.color, got.Bg); r < c.min {
			t.Errorf("%s contrast %.2f against Bg, want >= %.1f", c.name, r, c.min)
		}
	}
	if ContrastRatio(got.StatusBar, got.Bg) < 1.2 {
		t.Error("a status bar identical to the page was not lifted off it")
	}
	if r := ContrastRatio(got.StatusFg, got.StatusBar); r < contrastText {
		t.Errorf("status text contrast %.2f against its own bar", r)
	}
}

// Correction leaves alone whatever already clears its floor, so a hand-tuned
// palette keeps its identity and only a failing colour moves.
func TestCorrectThemeContrastLeavesGoodThemesAlone(t *testing.T) {
	for _, theme := range BuiltinThemes {
		got := CorrectThemeContrast(theme)
		if got.Bg != theme.Bg || got.Name != theme.Name {
			t.Errorf("%s: correction changed the background or name", theme.Name)
		}
		// Every field that already passed must come back untouched.
		for _, c := range []struct {
			name          string
			before, after lipgloss.Color
			floor         float64
		}{
			{"Fg", theme.Fg, got.Fg, contrastText},
			{"BorderFocus", theme.BorderFocus, got.BorderFocus, contrastAccent},
			{"Selected", theme.Selected, got.Selected, contrastAccent},
			{"Border", theme.Border, got.Border, contrastChrome},
			{"Error", theme.Error, got.Error, contrastAccent},
		} {
			if ContrastRatio(c.before, theme.Bg) >= c.floor && c.after != c.before {
				t.Errorf("%s: %s already passed at %.2f but was changed from %s to %s",
					theme.Name, c.name, ContrastRatio(c.before, theme.Bg), c.before, c.after)
			}
		}
		if ContrastRatio(got.Fg, got.Bg) < contrastText {
			t.Errorf("%s: corrected foreground is still unreadable", theme.Name)
		}
	}
	// Idempotent: correcting twice changes nothing further.
	once := CorrectThemeContrast(Theme{Name: "x", Bg: lipgloss.Color("#202020"),
		Fg: lipgloss.Color("#212121"), BorderFocus: lipgloss.Color("#222222")})
	if twice := CorrectThemeContrast(once); twice != once {
		t.Fatal("correction is not idempotent")
	}
}

func TestAccentReadableOnKeepsHue(t *testing.T) {
	bg := lipgloss.Color("#11111b")
	// A dark blue accent on a dark background has to brighten, staying blue.
	got := AccentReadableOn(lipgloss.Color("#16213e"), bg, 7.0)
	if ContrastRatio(got, bg) < 7.0 {
		t.Fatalf("accent %s still fails its floor", got)
	}
	if got == lipgloss.Color("#ffffff") || got == lipgloss.Color("#000000") {
		t.Fatalf("accent collapsed to monochrome: %s", got)
	}
	r, g, b, ok := hexToRGB(got)
	if !ok || !(b > r && b > g) {
		t.Fatalf("accent lost its blue identity: %s", got)
	}
	// A colour that already passes is returned untouched.
	fine := lipgloss.Color("#89b4fa")
	if AccentReadableOn(fine, bg, 3.0) != fine {
		t.Fatal("a passing accent was adjusted anyway")
	}
	if AccentReadableOn("", bg, 4.5) != "" {
		t.Fatal("empty accent should stay empty")
	}
}

func TestMixColorsAndSurface(t *testing.T) {
	black, white := lipgloss.Color("#000000"), lipgloss.Color("#ffffff")
	if got := MixColors(black, white, 0); got != black {
		t.Fatalf("mix at 0 = %s", got)
	}
	if got := MixColors(black, white, 1); got != white {
		t.Fatalf("mix at 1 = %s", got)
	}
	if got := MixColors(black, white, 0.5); got != lipgloss.Color("#808080") {
		t.Fatalf("mix at 0.5 = %s", got)
	}
	// An unparseable input is returned rather than producing a bogus colour.
	if got := MixColors(lipgloss.Color("5"), white, 0.5); got != lipgloss.Color("5") {
		t.Fatalf("mix with a non-hex colour = %s", got)
	}
	dark := lipgloss.Color("#11111b")
	if ContrastRatio(SurfaceForRatio(dark, 2.0), dark) < 2.0 {
		t.Fatal("surface did not reach its ratio")
	}
}

func TestHueAndShiftHue(t *testing.T) {
	red := lipgloss.Color("#ff0000")
	h, s, ok := Hue(red)
	if !ok || s == 0 || (h > 1 && h < 359) {
		t.Fatalf("Hue(red) = %.1f, %.2f, %v", h, s, ok)
	}
	// Rotating a third of the way round takes red to green, then to blue.
	green := ShiftHue(red, 120)
	blue := ShiftHue(red, 240)
	if green == red || blue == red || green == blue {
		t.Fatalf("hue rotation collapsed: %s %s %s", red, green, blue)
	}
	gr, gg, gb, _ := hexToRGB(green)
	if !(gg > gr && gg > gb) {
		t.Fatalf("red + 120 is not green: %s", green)
	}
	// A whole number of turns is exactly the identity, with no round-trip drift.
	for _, turn := range []float64{0, 360, -360, 720} {
		if got := ShiftHue(red, turn); got != red {
			t.Fatalf("rotating by %.0f changed the colour: %s", turn, got)
		}
	}
	if ShiftHue(red, -120) != ShiftHue(red, 240) {
		t.Fatal("negative rotation does not wrap")
	}
	// Greyscale has no hue to rotate, so a monochrome palette stays monochrome.
	grey := lipgloss.Color("#808080")
	if got := ShiftHue(grey, 90); got != grey {
		t.Fatalf("greyscale was tinted: %s", got)
	}
	if _, s, _ := Hue(grey); s != 0 {
		t.Fatalf("greyscale reported saturation %.2f", s)
	}
	if _, _, ok := Hue(lipgloss.Color("5")); ok {
		t.Fatal("a non-hex colour reported a hue")
	}
	if got := ShiftHue(lipgloss.Color("5"), 90); got != lipgloss.Color("5") {
		t.Fatal("a non-hex colour was rotated")
	}
}
