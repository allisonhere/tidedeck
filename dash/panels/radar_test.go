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
	// The credit names whichever service covers the place, asked for the same way the
	// panel asks - so the test cannot drift from the routing.
	credit := provider.RadarSourceFor(30.2672, -97.7431).Credit()
	if !strings.Contains(ansi.Strip(view), credit) {
		t.Fatalf("the panel does not credit %q:\n%q", credit, ansi.Strip(view))
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

// The block of tiles is shaped like the space the picture is drawn into - a wide
// pane gets a row of tiles, a square one a square block - and is never more tiles
// than the budget.
func TestRadarGridFollowsThePaneSize(t *testing.T) {
	renderer := func(cellWidth, cellAspect float64) tideui.Renderer {
		return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: cellWidth, CellAspect: cellAspect})
	}
	// Every size here is the space the picture gets, which is the pane less its
	// caption - the panel passes that, not the whole pane.
	cases := []struct {
		name               string
		renderer           tideui.Renderer
		width, height      int
		wantCols, wantRows int
	}{
		{"a square space is a square block", renderer(8, 2), 120, 60, 2, 2},
		{"a small space is two tiles across", renderer(8, 2), 40, 10, 2, 1},
		{"a tall space is a column of tiles", renderer(8, 2), 40, 58, 1, 3},
		{"a strip is a row of tiles, up to the budget", renderer(7, 2.43), 118, 6, 6, 1},
		{"a space too big for the budget gets the best of it", renderer(8, 2), 150, 30, 3, 1},
		{"an unknown cell size is assumed to be the usual one", renderer(0, 0), 40, 10, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols, rows := radarGrid(tc.renderer, tc.width, tc.height)
			if cols != tc.wantCols || rows != tc.wantRows {
				t.Fatalf("a %dx%d picture space is worth %dx%d tiles, want %dx%d",
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
	// And the pane in pixels, which is what a server-rendered source is asked for: 120
	// cells of 8 px wide, and 58 rows of 8 px at a cell twice as tall as it is wide -
	// 58 and not 60, because the caption's two rows are not picture.
	if asked.Width != 960 || asked.Height != 928 {
		t.Fatalf("a 120x60 pane asked for a %dx%d picture, want 960x928", asked.Width, asked.Height)
	}
	// And a small pane is one tile, so the ordinary case costs one request.
	panel.View(tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})})
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	if asked.Cols != 2 || asked.Rows != 1 {
		t.Fatalf("a 40x12 pane asked for %dx%d tiles, want 2x1", asked.Cols, asked.Rows)
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
	wide := radarCaption(60, "17:50", "Kansas City", "243 km wide", "RainViewer")
	if !strings.Contains(wide, "243 km wide") || !strings.Contains(wide, "RainViewer") {
		t.Fatalf("wide caption = %q, want the scale and the source", wide)
	}
	narrow := radarCaption(26, "17:50", "Kansas City", "243 km wide", "RainViewer")
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
	if got := radarCaption(60, "17:50", "Kansas City", "", "RainViewer"); strings.Contains(got, "··") || strings.Contains(got, " · · ") {
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
	if len(asked) != 1 || asked[0].Cols != 2 || asked[0].Rows != 1 {
		t.Fatalf("the first fetch = %+v, want the 2x1 block a 40x12 pane is worth", asked)
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

// basemapFetcher stands in for the basemap service: a picture of one solid colour
// with the radar's own geometry, and a count of how many times it was asked.
func basemapFetcher(calls *int) func(provider.BasemapOptions) func(context.Context) (tideui.MapFrame, error) {
	return func(provider.BasemapOptions) func(context.Context) (tideui.MapFrame, error) {
		return func(context.Context) (tideui.MapFrame, error) {
			*calls++
			img := image.NewRGBA(image.Rect(0, 0, 64, 64))
			for y := 0; y < 64; y++ {
				for x := 0; x < 64; x++ {
					img.Set(x, y, color.RGBA{R: 255, B: 255, A: 255}) // magenta
				}
			}
			return tideui.MapFrame{Image: img, Centre: image.Pt(32, 32)}, nil
		}
	}
}

// linedFrame is a radar frame with nothing in it but one red line: everything else
// is the transparent sky a basemap is there to fill.
func linedFrame() tideui.RadarFrame {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		img.Set(32, y, color.RGBA{R: 255, A: 255})
	}
	return tideui.RadarFrame{
		Time:   time.Date(2026, 9, 18, 18, 5, 0, 0, time.Local),
		Image:  img,
		Centre: image.Pt(40, 40), KilometresPerPixel: 0.475,
	}
}

// The map is drawn under the frame, so the sky the radar cannot speak for shows the
// ground instead of the panel's background - which is the entire point of fetching
// one.
func TestRadarDrawsTheMapUnderTheFrame(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	calls := 0
	panel := &radar{newFetcher: frameFetcher(linedFrame()), newBasemap: basemapFetcher(&calls)}
	// Imagery, not a paper map: a photograph of the ground is drawn in its own colours,
	// where a topographic sheet is re-inked for a dark pane (below).
	values := radarValues(t, 30.2672, -97.7431)
	values.Set(radarBasemapKey, "night lights")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("the basemap was fetched %d times, want once", calls)
	}
	picture := panel.drawnFrame(linedFrame(), tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})})
	if _, _, _, alpha := picture.At(0, 0).RGBA(); alpha == 0 {
		t.Fatal("the sky is still transparent: the map is not under the frame")
	}
	r, g, b, _ := picture.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 255 {
		t.Fatalf("what shows through the radar = %d,%d,%d, want the map's own colour", r>>8, g>>8, b>>8)
	}
	// And where the radar does speak, the radar wins.
	r, g, b, _ = picture.At(32, 20).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("the echo = %d,%d,%d, want the radar's own colour over the map", r>>8, g>>8, b>>8)
	}
}

// The map is still imagery: it is fetched once for a place and kept, however often
// the panel refreshes. Moving the radar, or a pane big enough to want a different
// block, is what makes it due again.
func TestRadarFetchesTheMapOnceForAPlace(t *testing.T) {
	noPlaceholderCellsForTests(t)
	calls := 0
	panel := &radar{newFetcher: frameFetcher(linedFrame()), newBasemap: basemapFetcher(&calls)}
	if err := panel.Configure(radarValues(t, 30.2672, -97.7431)); err != nil {
		t.Fatal(err)
	}
	small := tideui.PanelContext{Width: 40, Height: 12, Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})}
	panel.View(small)
	for i := 0; i < 3; i++ {
		if err := panel.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("three refreshes fetched the map %d times, want once", calls)
	}
	// A bigger pane is a different block of ground, so it is worth another fetch.
	big := tideui.PanelContext{Width: 120, Height: 60, Renderer: small.Renderer}
	panel.View(big)
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("after a resize the map was fetched %d times, want twice", calls)
	}
}

// Turned off, the panel draws exactly what it drew before there was a map: the
// radar, and the panel's own background behind it.
func TestRadarWithNoMapDrawsOnlyTheFrame(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	calls := 0
	panel := &radar{newFetcher: frameFetcher(linedFrame()), newBasemap: basemapFetcher(&calls)}
	values := radarValues(t, 30.2672, -97.7431)
	values.Set(radarBasemapKey, radarBasemapOff)
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("the map was fetched %d times with it turned off", calls)
	}
	picture := panel.drawnFrame(linedFrame(), tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})})
	if _, _, _, alpha := picture.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("a map was drawn with the setting off")
	}
}

// A topographic sheet is dark ink on light paper, and a dashboard is dark glass: the
// map is re-inked in the theme's own colours rather than shown as a lit sheet, which is
// what makes it belong to the pane instead of glowing in it.
func TestRadarReInksAPaperMapForADarkPane(t *testing.T) {
	noPlaceholderCellsForTests(t)
	trueColor(t)
	calls := 0
	panel := &radar{newFetcher: frameFetcher(linedFrame()), newBasemap: basemapFetcher(&calls)}
	values := radarValues(t, 30.2672, -97.7431)
	values.Set(radarBasemapKey, "topographic")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	picture := panel.drawnFrame(linedFrame(), tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})})
	red, green, blue, alpha := picture.At(0, 0).RGBA()
	if alpha == 0 {
		t.Fatal("the paper map was not drawn at all")
	}
	if red>>8 == 255 && green>>8 == 0 && blue>>8 == 255 {
		t.Fatal("a paper map was drawn in its own colours on a dark pane")
	}
	// And it is the theme's colours: the same map under another theme comes out
	// differently, which is the whole reason for re-inking it rather than dimming it.
	other := panel.drawnFrame(linedFrame(), tideui.PanelContext{Width: 40, Height: 12,
		Renderer: tideui.NewRenderer(tideui.CatppuccinLatte, tideui.StyleOptions{})})
	otherRed, _, _, _ := other.At(0, 0).RGBA()
	if otherRed == red {
		t.Fatal("the re-inked map did not change with the theme")
	}
}

// Both sources are credited, and as one part, so the caption's own dropping never
// keeps one and throws the other away.
func TestRadarCaptionCreditsTheMapService(t *testing.T) {
	// With a map, both services are named, and the radar's source is whichever one
	// covers the place.
	withMap := radarCaption(90, "17:50", "Kansas City", "243 km wide", "NEXRAD · IEM · NASA GIBS")
	if !strings.Contains(withMap, "NEXRAD · IEM") || !strings.Contains(withMap, "NASA GIBS") {
		t.Fatalf("caption = %q, want both services credited", withMap)
	}
	withoutMap := radarCaption(90, "17:50", "Kansas City", "243 km wide", "NEXRAD · IEM")
	if strings.Contains(withoutMap, "GIBS") {
		t.Fatalf("caption = %q, want no map credit when there is no map", withoutMap)
	}
	if narrow := radarCaption(26, "17:50", "Kansas City", "243 km wide", "NEXRAD · IEM · NASA GIBS"); !strings.Contains(narrow, "NEXRAD · IEM") {
		t.Fatalf("narrow caption = %q, want the radar's source kept at least", narrow)
	}
}
