package panels

import (
	"context"
	"image"
	"image/color"
	"math"
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

// The reader's own position is drawn into the picture at the pixel the frame
// names. Without it a lone echo gives no sense of distance - and the middle of the
// picture is only that position when the block of tiles is odd-sized, which is
// exactly why it is passed in rather than assumed.
func TestRadarMarksWhereTheFrameSaysTheReaderIs(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 64, 64))
	// Deliberately off centre: a mark that always landed in the middle would pass
	// the old code and be wrong for an even-sided block of tiles.
	centre := image.Pt(20, 40)
	marked, ok := markCentre(blank, centre, color.RGBA{R: 255, G: 255, B: 255, A: 255}).(*image.RGBA)
	if !ok {
		t.Fatal("markCentre did not return a drawable image")
	}
	_, _, _, centreAlpha := marked.At(centre.X, centre.Y).RGBA()
	_, _, _, tipAlpha := marked.At(centre.X, centre.Y-64/48).RGBA()
	_, _, _, middleAlpha := marked.At(32, 32).RGBA()
	_, _, _, cornerAlpha := marked.At(2, 2).RGBA()
	if centreAlpha == 0 {
		t.Fatal("the position the frame named was left unmarked")
	}
	if tipAlpha == 0 {
		t.Fatal("the mark is a single pixel, which a scaled-down picture loses")
	}
	if middleAlpha != 0 {
		t.Fatal("the mark was drawn in the middle of the picture rather than where the frame said")
	}
	if cornerAlpha != 0 {
		t.Fatal("the mark was drawn across the whole frame")
	}
	// The frame in state is the one the next draw reuses, so the original is untouched.
	if _, _, _, alpha := blank.At(centre.X, centre.Y).RGBA(); alpha != 0 {
		t.Fatal("markCentre marked the picture it was given")
	}
}

// A pane bigger than one tile is asked for as more tiles rather than a stretched
// 512: the service's own size is fixed, so sharpness is a question of how many of
// them the panel orders.
func TestRadarGridFollowsThePaneSize(t *testing.T) {
	renderer := func(cellWidth, cellAspect float64) tideui.Renderer {
		return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: cellWidth, CellAspect: cellAspect})
	}
	cases := []struct {
		name               string
		renderer           tideui.Renderer
		width, height      int
		wantCols, wantRows int
	}{
		{"a normal pane is one tile", renderer(8, 2), 40, 12, 1, 1},
		{"a wide pane is two tiles across", renderer(8, 2), 120, 30, 2, 1},
		{"a zoomed pane is a block", renderer(8, 2), 120, 60, 2, 2},
		{"the block stays inside the budget", renderer(8, 2), 200, 100, 3, 1},
		{"an unknown cell size is assumed to be the usual one", renderer(0, 0), 40, 12, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols, rows := radarGrid(tc.renderer, tc.width, tc.height)
			if cols != tc.wantCols || rows != tc.wantRows {
				t.Fatalf("a %dx%d pane is worth %dx%d tiles, want %dx%d",
					tc.width, tc.height, cols, rows, tc.wantCols, tc.wantRows)
			}
			if cols*rows > radarTileBudget {
				t.Fatalf("%dx%d tiles is %d requests, more than the budget of %d",
					cols, rows, cols*rows, radarTileBudget)
			}
		})
	}
}

// The block the panel asks for is the block the pane is worth: the fetch is built
// from the pane last drawn, because a fetch is asked for without a pane size.
func TestRadarAsksForTheTilesItsPaneIsWorth(t *testing.T) {
	noPlaceholderCellsForTests(t)
	var asked provider.RadarOptions
	panel := &radar{newFetcher: func(opts provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
		asked = opts
		return func(context.Context) (tideui.RadarFrame, error) { return tideui.RadarFrame{}, nil }
	}}
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	// Draw once at a big pane (120 cells of 8px at an aspect of 2 is 960x960 px,
	// four 512px tiles), then configure again the way the deck does.
	panel.View(tideui.PanelContext{Width: 120, Height: 60,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})})
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if asked.Cols != 2 || asked.Rows != 2 {
		t.Fatalf("a 120x60 pane asked for %dx%d tiles, want 2x2", asked.Cols, asked.Rows)
	}
	// And a small pane is one tile, so the ordinary case costs one request.
	panel.View(tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})})
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if asked.Cols != 1 || asked.Rows != 1 {
		t.Fatalf("a 40x12 pane asked for %dx%d tiles, want 1x1", asked.Cols, asked.Rows)
	}
}

// A ring is a distance or it is decoration: it must be a round number of
// kilometres and it must fit inside the picture.
func TestRadarRingIsAroundNumberThatFits(t *testing.T) {
	if got := ringKilometres(0.475, image.Pt(256, 256), image.Rect(0, 0, 512, 512)); got != 100 {
		t.Fatalf("a ring around the middle of a 512px frame = %d km, want 100", got)
	}
	// A reader at the very corner: no round ring fits, and a ring drawn off the
	// picture would be seen as nothing.
	if got := ringKilometres(0.475, image.Pt(8, 8), image.Rect(0, 0, 512, 512)); got != 0 {
		t.Fatalf("a ring around the corner = %d km, want none", got)
	}
}

// The ring is drawn at the radius its distance means, which is the whole point of
// carrying kilometres per pixel.
func TestRadarDrawsTheRingAtItsDistance(t *testing.T) {
	centre := image.Pt(256, 256)
	kmPerPixel := 0.475
	picture := image.NewRGBA(image.Rect(0, 0, 512, 512))
	if drawRing(picture, centre, kmPerPixel, color.RGBA{R: 255, A: 255}) == nil {
		t.Fatal("drawRing returned no image")
	}
	radius := int(math.Round(100 / kmPerPixel))
	if _, _, _, alpha := picture.At(centre.X+radius, centre.Y).RGBA(); alpha == 0 {
		t.Fatalf("nothing was drawn %d km east of the reader (%d px)", 100, radius)
	}
	if _, _, _, alpha := picture.At(centre.X+radius/2, centre.Y).RGBA(); alpha != 0 {
		t.Fatal("the ring was filled in rather than drawn as a line")
	}
	// A ring with nowhere to go draws nothing at all.
	corner := image.NewRGBA(image.Rect(0, 0, 512, 512))
	drawRing(corner, image.Pt(8, 8), kmPerPixel, color.RGBA{R: 255, A: 255})
	if _, _, _, alpha := corner.At(8+8, 8).RGBA(); alpha != 0 {
		t.Fatal("a ring that does not fit was drawn anyway")
	}
}

// The caption says what the picture is worth on the ground - the one number that
// turns it from a pattern into a distance - and drops the scale before the place,
// and the place before the source. A credit the layout truncated is not a credit.
func TestRadarCaptionNamesTheScale(t *testing.T) {
	wide := radarCaption(60, "17:50", "Kansas City", "243 km wide")
	if !strings.Contains(wide, "243 km wide") || !strings.Contains(wide, "RainViewer") {
		t.Fatalf("wide caption = %q, want the scale and the source", wide)
	}
	narrow := radarCaption(26, "17:50", "Kansas City", "243 km wide")
	if strings.Contains(narrow, "243 km wide") {
		t.Fatalf("narrow caption = %q, want it to have dropped the scale", narrow)
	}
	if !strings.Contains(narrow, "RainViewer") || !strings.Contains(narrow, "17:50") {
		t.Fatalf("narrow caption = %q, want the time and the source kept", narrow)
	}
	if ansi.StringWidth(narrow) > 26 {
		t.Fatalf("narrow caption = %q, which is %d cells wide", narrow, ansi.StringWidth(narrow))
	}
	// A frame with no scale on it (one the provider could not measure) does not
	// leave an empty part behind.
	if got := radarCaption(60, "17:50", "Kansas City", ""); strings.Contains(got, "··") || strings.Contains(got, " · · ") {
		t.Fatalf("a caption with no scale = %q", got)
	}
}

// A pane resized after the fetch was built is asked for the tiles it is worth now,
// on the next refresh: the settings do not change when a pane does, and a panel
// zoomed to the full screen should stop drawing a stretched tile.
func TestRadarRefetchesWhenThePaneIsResized(t *testing.T) {
	noPlaceholderCellsForTests(t)
	var asked []provider.RadarOptions
	panel := &radar{newFetcher: func(opts provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
		asked = append(asked, opts)
		return func(context.Context) (tideui.RadarFrame, error) { return tideui.RadarFrame{}, nil }
	}}
	small := tideui.PanelContext{Width: 40, Height: 12, Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})}
	big := tideui.PanelContext{Width: 120, Height: 60, Renderer: small.Renderer}
	// Drawn small, then configured: one tile.
	panel.View(small)
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0].Cols != 1 {
		t.Fatalf("the first fetch = %+v, want one tile", asked)
	}
	// Then the pane is zoomed to the whole window, and the next refresh notices.
	panel.View(big)
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 {
		t.Fatalf("a resized pane caused %d fetches, want a second one", len(asked))
	}
	if asked[1].Cols != 2 || asked[1].Rows != 2 {
		t.Fatalf("the fetch after the resize = %dx%d tiles, want 2x2", asked[1].Cols, asked[1].Rows)
	}
	// And a refresh with no resize does not rebuild the fetch again.
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 {
		t.Fatalf("an unchanged pane caused %d fetches, want no more than the two", len(asked))
	}
}

// The picture the panel hands to the renderer is the same picture from one draw to
// the next. A terminal is told about a picture once and identifies it afterwards
// by the id carried in its cells, so a fresh copy per frame re-sends the whole
// frame - a quarter of a megabyte a second for a mosaic - and re-copies it too.
func TestRadarComposesTheFrameOnce(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	panel := &radar{}
	ctx := tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})}
	frame := tideui.RadarFrame{
		Time:   time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local),
		Image:  image.NewRGBA(image.Rect(0, 0, 64, 64)),
		Centre: image.Pt(30, 30), KilometresPerPixel: 0.475,
	}
	first := panel.drawnFrame(frame, ctx)
	if first == nil {
		t.Fatal("the frame was not composed")
	}
	if again := panel.drawnFrame(frame, ctx); again != first {
		t.Fatal("the frame was composed again for a second draw of the same frame")
	}
	// A new frame is a new picture, and so is the same frame drawn in a pane that
	// has moved the reader's place... which the frame itself carries, so the frame
	// time is the thing that changes.
	fresh := frame
	fresh.Time = frame.Time.Add(5 * time.Minute)
	if composed := panel.drawnFrame(fresh, ctx); composed == first {
		t.Fatal("a new frame reused the picture composed for the old one")
	}
	// And the same frame drawn with another theme's colours is a different picture:
	// the crosshair and the ring are drawn in the theme's own colours.
	other := ctx
	other.Renderer = tideui.NewRenderer(tideui.CatppuccinLatte, tideui.StyleOptions{})
	if composed := panel.drawnFrame(frame, other); composed == first {
		t.Fatal("a frame composed for one theme was reused for another")
	}
}
