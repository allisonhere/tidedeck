package panels

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// trueColor pins the colour profile for one test. The profile is a package-level
// global in lipgloss and every test in the binary renders through it, so the
// previous one goes back on the way out.
func trueColor(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

// noPlaceholderCellsForTests points the image transport at a terminal without the
// protocol, so an assertion about what the panel draws does not depend on the
// terminal the suite happens to run in: TERM_PROGRAM is set by the developer's own
// and would otherwise switch the panel to placeholder cells.
func noPlaceholderCellsForTests(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TMUX", "")
}

// radarValues builds the settings a radar panel reads: its own two, plus the
// coordinates and place name the weather panel owns. The values API is
// dash.NewValues() plus Set (see dash/values.go:22 and :210), and the keys are
// the unexported constants in this package.
func radarValues(t *testing.T, lat, lon float64) dash.Values {
	t.Helper()
	values := dash.NewValues()
	values.Set(weatherLatitudeKey, lat)
	values.Set(weatherLongitudeKey, lon)
	values.Set(weatherLocationKey, "Austin")
	values.Set(radarEnabledKey, true)
	values.Set(radarZoomKey, float64(provider.RadarDefaultZoom))
	return values
}

// A panel with no coordinates has nowhere to look, and says so rather than
// drawing an empty box: the same promise the weather panel keeps.
func TestRadarSaysWhereToSetALocation(t *testing.T) {
	panel := Radar().(*radar)
	panel.Configure(dash.NewValues())
	// Wide enough for the whole message: RenderLines truncates at the pane's
	// width, and the panel's promise is that it names Weather somewhere.
	view := ansi.Strip(panel.View(tideui.PanelContext{Width: 46, Height: 10}))
	if !strings.Contains(strings.ToLower(view), "weather") {
		t.Fatalf("the empty radar panel = %q, want it to name the Weather panel", view)
	}
}

// With coordinates it draws the frame it fetched, and says when that frame was -
// a radar picture with no time on it is a picture of a rumour.
func TestRadarDrawsTheFrameAndItsTime(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	when := time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local)
	panel := &radar{newFetcher: func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
		return func(context.Context) (tideui.RadarFrame, error) {
			img := image.NewRGBA(image.Rect(0, 0, 4, 4))
			for y := 0; y < 4; y++ {
				for x := 0; x < 4; x++ {
					img.Set(x, y, color.RGBA{R: 255, A: 255})
				}
			}
			return tideui.RadarFrame{Time: when, Image: img}, nil
		}
	}}
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := panel.View(tideui.PanelContext{Width: 40, Height: 12, Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})})
	if !strings.Contains(view, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Background(lipgloss.Color("#ff0000")).Render("▀")) {
		t.Fatalf("the frame was not drawn:\n%q", view)
	}
	if !strings.Contains(ansi.Strip(view), "18:05") {
		t.Fatalf("the panel does not say when the frame is from:\n%q", ansi.Strip(view))
	}
	if !strings.Contains(ansi.Strip(view), "RainViewer") {
		t.Fatalf("the panel does not credit its source:\n%q", ansi.Strip(view))
	}
}

// A radar frame cannot be faked: the deck refreshes this panel in its demo mode
// too, because a sample storm would be a fabrication rather than a stand-in.
func TestRadarIsLiveEvenInDemoMode(t *testing.T) {
	live, ok := Radar().(dash.AlwaysLive)
	if !ok {
		t.Fatal("the radar panel does not declare whether it is always live")
	}
	if !live.AlwaysLive() {
		t.Fatal("the radar panel would go blank in demo mode")
	}
}

// frameFetcher stands in for the provider: a fetch that returns one frame, so a
// test can say exactly what the panel has to draw.
func frameFetcher(frame tideui.RadarFrame) func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
	return func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
		return func(context.Context) (tideui.RadarFrame, error) { return frame, nil }
	}
}

// An empty sky and a broken panel draw the same rectangle, so a frame with
// nothing in it says so rather than leaving the reader to guess. This is what the
// panel looked like over Austin: the caption, and then nothing.
func TestRadarSaysWhenTheSkyIsEmpty(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	panel := &radar{newFetcher: frameFetcher(tideui.RadarFrame{
		Time:  time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local),
		Image: image.NewRGBA(image.Rect(0, 0, 8, 8)),
	})}
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(panel.View(tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})}))
	if !strings.Contains(view, "no precipitation in range") {
		t.Fatalf("an empty frame did not say so:\n%q", view)
	}
}

// And the sentence is not noise when there is weather: a frame with anything in
// it is left to speak for itself.
func TestRadarDoesNotSayEmptyWhenThereIsWeather(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		img.Set(4, y, color.RGBA{G: 255, A: 255})
	}
	panel := &radar{newFetcher: frameFetcher(tideui.RadarFrame{
		Time: time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local), Image: img,
	})}
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(panel.View(tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})}))
	if strings.Contains(view, "no precipitation") {
		t.Fatalf("a frame with weather claimed to be empty:\n%q", view)
	}
}

// The measurement behind that sentence, on a frame whose transparent and opaque
// pixels are known.
func TestRadarEchoIsTheFractionOfTheFrameWithWeather(t *testing.T) {
	half := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 4; x++ {
			half.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	if got := radarEcho(half); got != 0.5 {
		t.Fatalf("radarEcho = %v, want 0.5", got)
	}
	if got := radarEcho(image.NewRGBA(image.Rect(0, 0, 8, 8))); got != 0 {
		t.Fatalf("radarEcho of an empty frame = %v, want 0", got)
	}
	if got := radarEcho(nil); got != 0 {
		t.Fatalf("radarEcho(nil) = %v, want 0", got)
	}
}

// The reader's own position is drawn into the picture, because the middle of a
// radar tile is where they are: without it a lone echo gives no sense of distance.
func TestRadarMarksWhereTheReaderIs(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 64, 64))
	marked, ok := markCentre(blank, color.RGBA{R: 255, G: 255, B: 255, A: 255}).(*image.RGBA)
	if !ok {
		t.Fatal("markCentre did not return a drawable image")
	}
	_, _, _, centreAlpha := marked.At(32, 32).RGBA()
	_, _, _, cornerAlpha := marked.At(2, 2).RGBA()
	_, _, _, tipAlpha := marked.At(32, 32-64/48).RGBA()
	if centreAlpha == 0 {
		t.Fatal("the centre of the picture was left unmarked")
	}
	if tipAlpha == 0 {
		t.Fatal("the mark is a single pixel, which a scaled-down picture loses")
	}
	if cornerAlpha != 0 {
		t.Fatal("the mark was drawn across the whole frame")
	}
	// The frame in state is the one the next draw reuses, so the original is untouched.
	if _, _, _, alpha := blank.At(32, 32).RGBA(); alpha != 0 {
		t.Fatal("markCentre marked the picture it was given")
	}
}

// The caption drops its least useful part first: a credit the layout truncated
// away is not a credit.
func TestRadarCaptionDropsTheZoomBeforeTheSource(t *testing.T) {
	wide := radarCaption(60, "17:50", "Kansas City", 7)
	if !strings.Contains(wide, "zoom 7") || !strings.Contains(wide, "RainViewer") {
		t.Fatalf("a wide caption = %q, want zoom and source", wide)
	}
	narrow := radarCaption(26, "17:50", "Kansas City", 7)
	if !strings.Contains(narrow, "RainViewer") {
		t.Fatalf("a narrow caption = %q, want the source kept", narrow)
	}
	if ansi.StringWidth(narrow) > 26 {
		t.Fatalf("a narrow caption = %q, which is %d cells wide", narrow, ansi.StringWidth(narrow))
	}
}
