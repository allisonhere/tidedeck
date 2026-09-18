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
	// Half-block cells, not placeholder cells: TERM_PROGRAM is set by whatever
	// terminal the suite is run in, and this asserts what the panel draws.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TMUX", "")
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
