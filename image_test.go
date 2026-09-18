package tideui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// solid is a picture of one colour, so a test can say exactly which colour a cell
// should be.
func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// A cell carries two samples: the top half is the glyph's foreground, the bottom
// its background. One column, one row, red over blue.
func TestRenderImagePutsTheTopSampleInTheForeground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	r := NewRenderer(CatppuccinMocha, StyleOptions{})
	img := image.NewRGBA(image.Rect(0, 0, 1, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})

	got := r.RenderImage(img, 1, 1)
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Background(lipgloss.Color("#0000ff")).Render("▀")
	if !strings.Contains(got, want) {
		t.Fatalf("cell = %q, want it to contain %q", got, want)
	}
}

// The picture keeps its aspect and is centred: a cell is twice as tall as it is
// wide, so a square source at 40 cells is 20 rows, and a tall one is narrower
// than the box rather than stretched into it.
func TestRenderImageFitsInsideTheBoxAndCentres(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	r := NewRenderer(CatppuccinMocha, StyleOptions{})

	square := solid(64, 64, color.RGBA{R: 255, A: 255})
	lines := strings.Split(ansi.Strip(r.RenderImage(square, 40, 24)), "\n")
	if len(lines) != 20 {
		t.Fatalf("a square source drew %d rows, want 20", len(lines))
	}
	if got := ansi.StringWidth(lines[0]); got != 40 {
		t.Fatalf("line width = %d, want the box width 40", got)
	}

	// Four times as tall as it is wide: the box is 40 by 24, and 24 rows is 48
	// samples, so a 1:4 picture is 12 cells wide and centred in the 40.
	tall := solid(16, 64, color.RGBA{G: 255, A: 255})
	lines = strings.Split(ansi.Strip(r.RenderImage(tall, 40, 24)), "\n")
	if len(lines) != 24 {
		t.Fatalf("a tall source drew %d rows, want 24", len(lines))
	}
	if strings.Count(lines[0], "▀") != 12 {
		t.Fatalf("a tall source drew %q, want 12 cells", lines[0])
	}
	if !strings.HasPrefix(lines[0], strings.Repeat(" ", 14)) {
		t.Fatalf("the picture is not centred: %q", lines[0])
	}

	// A wide source is capped by the width, not the height.
	wide := solid(256, 16, color.RGBA{B: 255, A: 255})
	if lines := strings.Split(ansi.Strip(r.RenderImage(wide, 40, 24)), "\n"); len(lines) >= 20 {
		t.Fatalf("a 16:1 source drew %d rows", len(lines))
	}
}

// A transparent sample shows the panel through it: a radar tile is transparent
// where it does not rain, and black there would be a lie about the weather.
func TestRenderImageCompositesTransparencyOntoTheBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	r := NewRenderer(CatppuccinMocha, StyleOptions{})
	bg := r.Styles.Workspace.Bg

	clear := solid(2, 2, color.RGBA{})
	got := r.RenderImage(clear, 1, 1)
	want := lipgloss.NewStyle().Foreground(bg).Background(bg).Render("▀")
	if !strings.Contains(got, want) {
		t.Fatalf("a transparent image drew %q, want the panel background", got)
	}
}

// A terminal with no colour at all still gets the shape: the brightness as a
// ramp, because a panel that draws nothing looks broken.
func TestRenderImageFallsBackToARampWithoutColour(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(termenv.TrueColor)
	r := NewRenderer(CatppuccinMocha, StyleOptions{})

	got := r.RenderImage(solid(4, 4, color.RGBA{R: 255, A: 255}), 4, 4)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("a colourless terminal got escape codes: %q", got)
	}
	line := strings.Split(got, "\n")[0]
	if ansi.StringWidth(line) != 4 {
		t.Fatalf("ramp line = %q, want 4 cells", line)
	}
	if strings.TrimSpace(line) == "" {
		t.Fatalf("a bright image drew an empty ramp: %q", line)
	}

	// And nothing is drawn for nothing: no image, no box.
	if got := r.RenderImage(nil, 10, 4); got != "" {
		t.Fatalf("a nil image drew %q", got)
	}
	if got := r.RenderImage(solid(2, 2, color.RGBA{A: 255}), 0, 4); got != "" {
		t.Fatalf("a zero-width box drew %q", got)
	}
}

// A frame is a picture and the time it is about, which is all a radar panel
// needs to draw, and all a provider has to return.
func TestRadarFrameCarriesItsTime(t *testing.T) {
	when := time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local)
	frame := RadarFrame{Time: when, Image: solid(2, 2, color.RGBA{A: 255})}
	if !frame.Time.Equal(when) {
		t.Fatalf("frame time = %v, want %v", frame.Time, when)
	}
	if frame.Image == nil {
		t.Fatal("frame has no image")
	}
}
