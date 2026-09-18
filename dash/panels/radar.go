package panels

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// Radar builds the radar panel. Like the weather panel it has nowhere to look
// until it is told where - and where is the weather panel's own location, so the
// place search fills both panels at once.
func Radar() dash.Panel {
	return &radar{newFetcher: provider.Radar, newBasemap: provider.Basemap}
}

type radar struct {
	dash.State[tideui.RadarFrame]

	mu         sync.Mutex
	fetch      func(context.Context) (tideui.RadarFrame, error)
	newFetcher func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error)
	// The map under the frame: which layer a setting asked for, how to fetch one, and
	// the still picture itself with the place it is of. Imagery does not change
	// between refreshes, so it is fetched once per place rather than once per frame.
	newBasemap func(provider.BasemapOptions) func(context.Context) (tideui.MapFrame, error)
	basemapFor string
	basemap    image.Image
	basemapOf  basemapRequest
	location   string
	zoom       int
	// drawn is the frame with the reader's place and the distance ring composed
	// into it, kept between draws: see drawnFrame.
	drawn      image.Image
	drawnKey   radarDrawing
	latitude   float64
	longitude  float64
	enabled    bool
	cols, rows int
	// The pane the panel was last drawn into, and the renderer it was drawn with.
	// A fetch is asked for without a pane, so the block of tiles is chosen from
	// the last pane drawn - and the cell size, which only the renderer knows.
	paneWidth, paneHeight int
	renderer              tideui.Renderer
	// quiet is true when the newest frame has nothing in it. An empty sky and a
	// broken panel look identical, so the panel is the only thing that can tell
	// the reader which one this is.
	quiet bool
}

const (
	radarEnabledKey = "radar.enabled"
	radarZoomKey    = "radar.zoom"
	radarBasemapKey = "radar.basemap"
)

// radarBasemapOff is the setting's "no map" value. It is a value and not a missing
// key, so that turning the map off is a choice the file records rather than an
// absence it has to infer.
const radarBasemapOff = "off"

// basemapRequest is the place a fetched map is of: everything the picture under the
// radar depends on. It is comparable on purpose - the panel keeps one and asks for a
// new map when it changes, which is what "once per place" means in practice.
type basemapRequest struct {
	layer               string
	latitude, longitude float64
	zoom, cols, rows    int
}

// radarTileBudget is the most tiles one refresh will fetch. The service is free
// and somebody else pays for it, and a glance at a dashboard panel is not worth a
// download - but panes are much wider than they are tall, and the picture that
// fills one is a row of tiles rather than a single square, so the cap has room for
// a row of six.
const radarTileBudget = 6

// defaultCellWidth is the cell width assumed when the terminal will not report
// one, in pixels: the usual monospace cell at the usual size.
const defaultCellWidth = 8

func (r *radar) Meta() dash.Meta {
	return dash.Meta{
		ID: "radar", Title: "Radar", Subtitle: "rain",
		Role: tideui.RoleSecondary, Priority: 72,
		MinWidth: 24, MinHeight: 8,
		Interval: 5 * time.Minute,
	}
}

// AlwaysLive keeps the panel fetching in the deck's demo mode too. A radar frame
// is a photograph of the sky at a moment: there is no honest sample of one, and a
// demo would be a fabricated storm. It is the same argument the plugin panels
// make - a real source, whatever mode the dashboard is in.
func (r *radar) AlwaysLive() bool { return true }

// PaneSized: the frame it fetches is as big as the pane it is drawn in, so a pane
// that changes shape is due a fetch rather than a stretched picture.
func (r *radar) PaneSized() bool { return true }

// Schema declares only what is the radar's own. The coordinates are the weather
// panel's: one location, not two copies that drift apart.
func (r *radar) Schema() []dash.Field {
	return []dash.Field{
		{Key: radarEnabledKey, Label: "live radar", Kind: dash.FieldBool, Default: "true",
			Description: "Uses the Weather panel's location."},
		{Key: radarBasemapKey, Label: "map under it", Kind: dash.FieldChoice,
			Options:     []string{radarBasemapOff, "night lights", "relief"},
			Default:     provider.BasemapDefaultLayer,
			Description: "A still satellite map of the same ground, drawn under the radar. Fetched once per location, not once a frame."},
		{Key: radarZoomKey, Label: "detail", Kind: dash.FieldFloat, Default: strconv.Itoa(provider.RadarDefaultZoom),
			Description: "Zoom 4 is a state, 7 is a metro area and its surroundings. 7 is as deep as the service's data goes.",
			Min:         3, Max: float64(provider.RadarMaxZoom), Step: 1},
	}
}

func (r *radar) Configure(values dash.Values) error {
	// The place the weather panel is looking at, read rather than duplicated:
	// looking up a city once should move both panels.
	latitude := values.Float(weatherLatitudeKey)
	longitude := values.Float(weatherLongitudeKey)
	location := strings.TrimSpace(values.String(weatherLocationKey))

	r.mu.Lock()
	defer r.mu.Unlock()
	r.location = location
	r.zoom = int(values.Float(radarZoomKey))
	if r.zoom == 0 {
		r.zoom = provider.RadarDefaultZoom
	}
	r.latitude, r.longitude = latitude, longitude
	r.enabled = boolOr(values, radarEnabledKey, true) && (latitude != 0 || longitude != 0)
	r.basemapFor = basemapLayerFor(values)
	if r.basemapFor == "" {
		// Turned off, and nothing kept: the picture and the place it was of go
		// together, and a stale one showing the wrong ground later would be worse
		// than none.
		r.basemap, r.basemapOf = nil, basemapRequest{}
	}
	r.rebuildLocked()
	return nil
}

// basemapLayerFor is the map layer a setting asked for, or nothing when it asked
// for none. An unknown name is the default rather than nothing: the settings screen
// only offers real layers, so an unknown one is a hand-edited file, and a file that
// asks for a map should get one.
func basemapLayerFor(values dash.Values) string {
	switch name := strings.ToLower(strings.TrimSpace(values.String(radarBasemapKey))); name {
	case radarBasemapOff:
		return ""
	case "", "night lights", "relief":
		// An absent key is the setting's own default, which is a map: the schema
		// offers it as the default, and a fresh install that disagreed with the
		// settings screen would be a bug nobody could see.
		if name == "" {
			return provider.BasemapDefaultLayer
		}
		return name
	default:
		return provider.BasemapDefaultLayer
	}
}

// rebuildLocked builds the fetch for the coordinates, the zoom and the pane last
// drawn. The pane decides how many tiles: a panel is told where to look, never how
// big it is, and a fetch is asked for without a pane - so the block is recomputed
// whenever the pane it was chosen for has changed. Rebuilding an unchanged fetch
// would be a request per refresh for nothing.
func (r *radar) rebuildLocked() {
	if !r.enabled {
		r.fetch, r.cols, r.rows = nil, 0, 0
		return
	}
	cols, rows := radarGrid(r.renderer, r.paneWidth, r.paneHeight)
	if r.fetch != nil && cols == r.cols && rows == r.rows {
		return
	}
	r.cols, r.rows = cols, rows
	r.fetch = r.newFetcher(provider.RadarOptions{
		Latitude: r.latitude, Longitude: r.longitude, Zoom: r.zoom, Cols: cols, Rows: rows,
	})
}

// radarFillEnough is how much of a pane a picture has to cover to read as filling
// it. Five sixths is where the leftover reads as a margin rather than as a picture
// sitting in a pane - and it is deliberately not one, because closing the last
// sixth of a small pane costs as many requests as the whole picture did, and the
// service is somebody else's to pay for.
const radarFillEnough = 0.85

// radarBlockScore ranks a candidate block of tiles, in the order the picture's
// faults matter to a reader: a picture with fewer pixels than the pane is a soft
// picture, a picture whose shape is not the pane's leaves part of the pane as
// background, and among the blocks that are neither, the cheapest one is the
// kindest to a service that is free.
type radarBlockScore struct {
	sharp  bool    // the block has at least the pane's pixels: nothing is upscaled
	enough bool    // it covers enough of the pane to read as filling it
	fill   float64 // how much of the pane it covers, for when nothing is enough
	tiles  int
}

func (s radarBlockScore) betterThan(other radarBlockScore) bool {
	switch {
	case s.sharp != other.sharp:
		return s.sharp
	case s.enough != other.enough:
		return s.enough
	case s.enough:
		if s.tiles != other.tiles {
			return s.tiles < other.tiles
		}
		return s.fill > other.fill
	default:
		// Nothing inside the budget fills the pane: get as close as it can.
		return s.fill > other.fill
	}
}

// radarGrid is the block of tiles a pane is worth. Two things decide it and they
// pull in different directions: a picture should fill its pane, which wants the
// block's shape to match the pane's, and a picture should not be a stretched 512,
// which wants a tile per 512 pixels. So every block the budget allows is scored -
// pixels covered first, shape second, size last - and the best one wins.
func radarGrid(renderer tideui.Renderer, width, height int) (cols, rows int) {
	cellWidth, cellAspect := renderer.CellWidth, renderer.CellAspect
	if cellWidth <= 0 {
		cellWidth = defaultCellWidth
	}
	if cellAspect <= 0 {
		cellAspect = 2 // a monospace cell of unknown shape: taller than wide
	}
	pixelsWide := float64(width) * cellWidth
	pixelsHigh := float64(height) * cellWidth * cellAspect
	if pixelsWide <= 0 || pixelsHigh <= 0 {
		return 1, 1
	}
	best, bestScore := [2]int{1, 1}, radarBlockScore{}
	for candidateCols := 1; candidateCols <= radarTileBudget; candidateCols++ {
		for candidateRows := 1; candidateRows <= radarTileBudget; candidateRows++ {
			if candidateCols*candidateRows > radarTileBudget {
				continue
			}
			pictureWide := float64(candidateCols * provider.RadarTilePixels)
			pictureHigh := float64(candidateRows * provider.RadarTilePixels)
			// How much of the pane the fitted picture covers: the ratio of the
			// smaller scale factor to the larger, which is one when the picture's
			// shape is the pane's own shape.
			widthFit, heightFit := pixelsWide/pictureWide, pixelsHigh/pictureHigh
			fill := math.Min(widthFit, heightFit) / math.Max(widthFit, heightFit)
			score := radarBlockScore{
				sharp:  pictureWide >= pixelsWide && pictureHigh >= pixelsHigh,
				enough: fill >= radarFillEnough,
				fill:   fill,
				tiles:  candidateCols * candidateRows,
			}
			if score.betterThan(bestScore) {
				best, bestScore = [2]int{candidateCols, candidateRows}, score
			}
		}
	}
	return best[0], best[1]
}

func (r *radar) Refresh(ctx context.Context) error {
	r.mu.Lock()
	// The pane may have been resized since the fetch was built - zoomed, the
	// window enlarged - and a fetch built for another pane is a stretched tile.
	r.rebuildLocked()
	fetch := r.fetch
	r.mu.Unlock()
	if fetch == nil {
		return nil
	}
	frame, err := fetch(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.quiet = radarEcho(frame.Image) < radarQuietEcho
	r.mu.Unlock()
	r.Store(frame)
	r.ensureBasemap(ctx)
	return nil
}

// ensureBasemap fetches the ground under the frame, once per place. Stills do not
// change between refreshes, so asking again would be a request for a picture that
// cannot be different - the radar's own five-minute interval is the live one.
func (r *radar) ensureBasemap(ctx context.Context) {
	r.mu.Lock()
	layer, newBasemap := r.basemapFor, r.newBasemap
	wanted := basemapRequest{
		layer: layer, latitude: r.latitude, longitude: r.longitude,
		zoom: r.zoom, cols: r.cols, rows: r.rows,
	}
	have, havePicture := r.basemapOf, r.basemap != nil
	r.mu.Unlock()
	if layer == "" || newBasemap == nil || (havePicture && have == wanted) || wanted.cols == 0 {
		return
	}
	ground, err := newBasemap(provider.BasemapOptions{
		Latitude: wanted.latitude, Longitude: wanted.longitude,
		Zoom: wanted.zoom, Cols: wanted.cols, Rows: wanted.rows, Layer: layer,
	})(ctx)
	if err != nil || ground.Image == nil {
		// A map the panel could not fetch is not news: the radar is the panel's
		// purpose, and a backdrop that is not there draws as the background it
		// replaced.
		return
	}
	r.mu.Lock()
	r.basemap, r.basemapOf = ground.Image, wanted
	r.mu.Unlock()
}

// radarQuietEcho is the echo below which the panel says the sky is empty. It is
// deliberately low - a thin squall line is a few pixels of a 512 px frame and is
// exactly what the panel is for - and the picture is drawn either way, so a frame
// under the bar is never hidden, only explained.
const radarQuietEcho = 0.002

// radarEcho is the fraction of a frame with anything in it. Sampled rather than
// exhaustive: a 512x512 frame is 262k pixels, and this only decides whether the
// panel adds a sentence.
func radarEcho(img image.Image) float64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return 0
	}
	const step = 4
	lit, total := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			_, _, _, alpha := img.At(x, y).RGBA()
			total++
			if alpha > 0 {
				lit++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(lit) / float64(total)
}

func (r *radar) View(ctx tideui.PanelContext) string {
	frame := r.Load()
	if frame.Image == nil {
		// The pane is recorded even with nothing to draw yet: the fetch that fills
		// it is ordered for the pane the panel is in, and the caption plus its
		// notice is the allowance the picture is drawn under.
		r.notePane(ctx, max(1, ctx.Height-2))
		return r.emptyView(ctx)
	}
	r.mu.Lock()
	location, quiet := r.location, r.quiet
	r.mu.Unlock()
	if location == "" {
		location = "local"
	}

	bg := ctx.Renderer.Styles.Workspace.Bg
	// A picture of the weather with no time on it is a picture of a rumour, and
	// the service is credited because it asks to be.
	lines := []string{radarCaption(ctx.Width, frame.Time.Local().Format("15:04"), location, radarScale(frame), r.hasBasemap())}
	// The picture is drawn under the caption, so the space it has is the pane less
	// what the caption and its notice take: a pane is not all picture.
	r.notePane(ctx, max(1, ctx.Height-len(lines)))
	if quiet {
		// Nothing in the frame, and the panel says which kind of nothing: an
		// empty sky and a failed fetch draw the same rectangle otherwise.
		lines = append(lines, "no precipitation in range")
	}
	// The picture is drawn with the reader's own position marked. The middle of
	// the tile is where they are, and without it a lone echo is a smudge on dark
	// glass: no telling weather one county over from weather two states away.
	picture := ctx.Renderer.RenderImage(r.drawnFrame(frame, ctx), ctx.Width, max(1, ctx.Height-len(lines)))
	// A frame that draws nothing is still a frame: the caption goes up either way,
	// because "nothing is falling" and "nothing has loaded" are different things
	// and only the caption and the sentence above tell them apart.
	if strings.TrimSpace(picture) != "" {
		lines = append(lines, strings.Split(picture, "\n")...)
	}
	return ctx.Renderer.RenderLines(lines, ctx.Width, bg)
}

// notePane records the space the picture is drawn into, and the renderer it was
// measured with. A fetch is asked for without a pane, so the block of tiles comes
// from the last pane drawn - and the space that matters is what is left under the
// caption, not the whole pane.
func (r *radar) notePane(ctx tideui.PanelContext, height int) {
	r.mu.Lock()
	r.paneWidth, r.paneHeight, r.renderer = ctx.Width, height, ctx.Renderer
	r.mu.Unlock()
}

// radarDrawing is everything a composed frame depends on. Anything in here
// changing means the picture has to be made again, and nothing else does.
type radarDrawing struct {
	fetchedAt  time.Time
	bounds     image.Rectangle
	centre     image.Point
	kmPerPixel float64
	mark, ring color.RGBA
	// The map under it, by identity: the panel keeps one picture per place, so the
	// same pointer means the same ground and the composed frame can be reused.
	basemap image.Image
}

// drawnFrame is the frame with the reader's own position and the distance ring on
// it, composed the first time it is drawn and then handed out unchanged. It is
// worth the bookkeeping: a terminal is told about a picture once and identifies it
// afterwards by the id written into its cells, so a fresh copy per draw makes the
// panel re-transmit the whole picture every second - and re-copy every pixel of it
// before that.
func (r *radar) drawnFrame(frame tideui.RadarFrame, ctx tideui.PanelContext) image.Image {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := radarDrawing{
		fetchedAt:  frame.Time,
		bounds:     frame.Image.Bounds(),
		centre:     frame.Centre,
		kmPerPixel: frame.KilometresPerPixel,
		mark:       tideui.RGBAOf(ctx.Renderer.Styles.Workspace.BodyFg),
		ring:       tideui.RGBAOf(ctx.Renderer.Styles.Workspace.BodyMutedFg),
		basemap:    r.basemap,
	}
	if r.drawn != nil && r.drawnKey == key {
		return r.drawn
	}
	// The crosshair first, then the ring on the same copy, in the quietest colour
	// the theme has: a scale that competes with the weather is worse than none.
	marked := markCentre(frame.Image, frame.Centre, key.mark)
	drawRing(marked, frame.Centre, frame.KilometresPerPixel, key.ring)
	// Then the map under all of it, where the radar's transparency lets it through.
	// The composed picture is what gets transmitted, so it is the composed picture
	// that has to be the same one from draw to draw.
	r.drawn, r.drawnKey = tideui.Composite(key.basemap, marked), key
	return r.drawn
}

// hasBasemap reports whether there is a map under the frame, which is what the
// caption's credit is about.
func (r *radar) hasBasemap() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.basemap != nil
}

// radarScale is what the picture is worth on the ground: a tile is a fixed number
// of pixels wide whatever the pane does, so this is the pane's scale, and a
// distance on the picture means nothing without it.
func radarScale(frame tideui.RadarFrame) string {
	if frame.Image == nil || frame.KilometresPerPixel <= 0 {
		return ""
	}
	kilometres := int(math.Round(frame.KilometresPerPixel * float64(frame.Image.Bounds().Dx())))
	return fmt.Sprintf("%d km wide", kilometres)
}

// radarCaption is the panel's context line, dropped in order of stubbornness as
// the pane narrows: the time and the source stay, the scale goes first and the
// place before it. A credit the layout truncated away is not a credit.
func radarCaption(width int, when, location, scale string, withMap bool) string {
	credit := "RainViewer"
	if withMap {
		// One part, not two: the caption drops its least important part first, and
		// two credits would turn into one credit and a lost source.
		credit = "RainViewer · NASA GIBS"
	}
	parts := []string{when, location, "", credit}
	parts[2] = scale
	// A frame with no scale is not a reason to print an empty part.
	if scale == "" {
		parts = append(parts[:2], parts[3])
	}
	for len(parts) > 2 && ansi.StringWidth(strings.Join(parts, " · ")) > width {
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
	}
	return strings.Join(parts, " · ")
}

// markCentre draws the reader into the picture at the pixel the frame says they
// are at, on a copy: the frame in state is the one the next draw reuses. The
// middle of the picture is only their position when the block of tiles is
// odd-sized, which is why the frame carries the point instead.
func markCentre(img image.Image, centre image.Point, colour color.RGBA) image.Image {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return img
	}
	marked := image.NewRGBA(bounds)
	draw.Draw(marked, bounds, img, bounds.Min, draw.Src)

	centreX, centreY := centre.X, centre.Y
	// Sized from the frame, so the mark survives being scaled into the pane and
	// stays a crosshair rather than becoming a blob or a single invisible pixel.
	arm := max(2, bounds.Dx()/48)
	thickness := max(1, bounds.Dx()/256)
	for offset := -arm; offset <= arm; offset++ {
		for row := 0; row < thickness; row++ {
			marked.Set(centreX+offset, centreY+row, colour)
			marked.Set(centreX+row, centreY+offset, colour)
		}
	}
	return marked
}

// maxRingKilometres is the ring the panel aims for: a round number a person can
// hold in their head, and one that fits in a normal pane at a normal zoom.
const maxRingKilometres = 100

// ringKilometres is the largest round distance whose ring fits inside the picture
// with a little room to spare, or nothing when none does. It steps down rather
// than picking a distance the pane would clip: a half-drawn ring is a lie.
func ringKilometres(kmPerPixel float64, centre image.Point, bounds image.Rectangle) int {
	if kmPerPixel <= 0 || !centre.In(bounds) {
		return 0
	}
	reach := min(
		min(centre.X-bounds.Min.X, bounds.Max.X-centre.X),
		min(centre.Y-bounds.Min.Y, bounds.Max.Y-centre.Y),
	) - 2
	for _, kilometres := range []int{maxRingKilometres, 50, 25} {
		if radius := int(math.Round(float64(kilometres) / kmPerPixel)); radius > 2 && radius <= reach {
			return kilometres
		}
	}
	return 0
}

// drawRing draws the reader's distance ring into the picture it is given - the
// copy markCentre already made, since a panel redraws this often and the frame in
// state must stay clean. It is dotted rather than solid so it reads as a scale
// rather than as weather.
func drawRing(img image.Image, centre image.Point, kmPerPixel float64, colour color.RGBA) image.Image {
	picture, ok := img.(*image.RGBA)
	if !ok {
		return img
	}
	kilometres := ringKilometres(kmPerPixel, centre, picture.Bounds())
	if kilometres == 0 {
		return img
	}
	radius := math.Round(float64(kilometres) / kmPerPixel)
	for degrees := 0; degrees < 360; degrees += 8 {
		radians := float64(degrees) * math.Pi / 180
		x := centre.X + int(math.Round(radius*math.Cos(radians)))
		y := centre.Y + int(math.Round(radius*math.Sin(radians)))
		if !image.Pt(x, y).In(picture.Bounds()) {
			continue
		}
		picture.Set(x, y, colour)
	}
	return picture
}

// emptyView says what is missing instead of drawing an empty box. A radar with
// nowhere to look is not a picture of nothing; it is a panel waiting for the
// weather panel to be told where you are, so it says that. It mirrors the weather
// panel's own empty view (dash/panels/weather.go:142).
func (r *radar) emptyView(ctx tideui.PanelContext) string {
	r.mu.Lock()
	configured := r.fetch != nil
	r.mu.Unlock()
	message := "No location set · set one in Weather · press s"
	if configured {
		message = "Loading…"
	}
	return ctx.Renderer.RenderLines([]string{message}, ctx.Width,
		ctx.Renderer.Styles.Workspace.Bg)
}
