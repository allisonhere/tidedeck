# Fill the radar panel with the radar picture

## Goal

Make the radar panel's picture fill its pane — the fetched ground area matching the
pane's shape and pixel density, with the parts that are not rain drawn as a map
(scale, distance, your position) rather than as empty background.

---

## The short answer to "why don't we fill the panel?"

Because three different things are being conflated, and only one of them is a bug.

**1. The picture already spans the pane.** `tideui.RenderImage` fits the source
inside the pane's cell box and pads whatever is left with the panel background.
The padding is real but small in a normal pane: a pane of 33 cells × 14 rows at
this machine's measured cell (~7.2 × 17 px, aspect 2.4) is 238 × 238 px, and a
square 512 px tile fills it to within a few percent. In a *wide, short* pane the
gap is bigger — a 40 × 8-cell pane is 288 × 137 px, so a square picture can only
be 137 px wide, leaving ~150 px of background on both sides. That is the aspect
rule doing its job: stretching the tile instead would make distance a lie.

**2. Most of a radar tile is transparent.** A tile is transparent where it is not
raining. Measured over Austin on 2026-09-18: 0.15% of the frame had any echo.
Over Kentucky, same frame: 30% echo, 70% clear. So "fill the panel with the radar
image" mostly means filling it with transparency — the image is there, the weather
is not. Nothing in the code can fix that; what it *can* fix is that an empty panel
is indistinguishable from a broken one, which is what the picture furniture below
is for.

**3. One tile's ground area is fixed, and it is not the pane's job to decide it.**
At zoom 7 one tile covers ≈ 243 km at 39°N (see `kilometresPerPixel` below),
whatever size the pane is. A bigger pane does not show more of the world, it shows
the same 243 km with more pixels — until the 512 px source runs out and the
picture goes soft. That is the one place where "not filling the panel" is literally
true: a pane wider than ~512 px is drawn from a source smaller than the pane, and
the only fix is fetching more tiles (a mosaic) rather than scaling one up.

So the plan does three things: fetch enough tiles that the picture matches the
pane's pixels, size the fetch so the picture's shape matches the pane's shape, and
draw the scale and the reader's position so the filled area reads as a map.

---

## Current context / assumptions

Everything here was verified in this checkout (`feat/images-and-radar`, HEAD
`751aac8`). Read the files before starting.

- The module root is `package tideui` (presentational: `layout.go`, `styles.go`,
  `image.go`), `provider` fetches, `dash` binds them, `dash/panels` holds the
  built-in panels, and `examples/workspace` is the app that runs.
- `tideui.RenderImage(img image.Image, width, maxHeight int) string` (`image.go:38`)
  draws via `kitty` placeholders when the terminal supports them and half-blocks
  otherwise, sizing the picture with
  `fitCells(source, width, maxHeight, cellAspect)` (`image.go:73`): the picture
  keeps its source aspect and is centred in the width.
- `Renderer` carries `CellAspect` (cell height ÷ cell width, `layout.go:82`),
  measured by the app with `tideui.CellAspectOf(os.Stdout)` (`cell_size.go:17`,
  wired at `examples/workspace/main.go:1283` and `panels.go:11`). **The absolute
  pixel size of a cell is not measured today** — the new tasks need it, because
  "does this pane need more tiles" is a question about pixels.
- `provider/radar.go` fetches exactly one tile: `RadarOptions{Latitude, Longitude,
  Zoom int}`, `Radar(opts) func(context.Context) (tideui.RadarFrame, error)`,
  `radarTileSize = 512`, `RadarMaxZoom = 7`, `radarTileFor(lat, lon, zoom)` (slippy
  map), `radarTileFraction` does not exist yet, the URL shape is
  `{host}{path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png` and must stay
  exactly that (an extra segment is accepted by the service and silently returns a
  different place).
- `tideui.RadarFrame` (`image.go:19`) is `{Time time.Time; Image image.Image}`.
- `dash/panels/radar.go` reads the Weather panel's coordinates
  (`weatherLatitudeKey` etc.), keeps `location`, `zoom` and `quiet`, configures the
  fetcher at `radar.go:92`, and draws in `View`: a caption from `radarCaption`,
  a "no precipitation in range" line when the frame is under `radarQuietEcho`, then
  `markCentre(...)` and `ctx.Renderer.RenderImage(marked, ctx.Width, max(1,
  ctx.Height-len(lines)))`.
- **A panel does not know its size when it fetches.** `Fetcher.Refresh(ctx)` has no
  `PanelContext`; only `View` does. The radar panel will therefore remember the
  last pane size it drew at and use it on the next refresh (Task T3), which is one
  refresh behind a resize — the picture is soft for at most one interval, never
  wrong, and the panel's own `r` action (see the weather panel's `Actions()`) can
  force it sooner.

## Architecture / proposed approach

The **provider** learns to fetch a block of tiles and stitch them (data), the
**renderer** stays unchanged apart from carrying the cell's pixel width (measure),
and the **panel** decides how many tiles its pane is worth, then draws the map
furniture — the reader's position, a distance ring and the ground scale in the
caption — into the picture before rendering it (presentation).

## Not doing (YAGNI)

- Stretching or squeezing the picture to the pane. It lies about distance and
  makes a radar panel actively harmful.
- A base map (coastlines, state lines). That is a second tile source with its own
  terms and its own plan; the ring and the caption scale are the cheap 90%.
- Animating the twelve frames the index already lists. Separate contract (a frame
  clock the panel does not have).
- Asking the *deck* to pass the pane size into `Refresh`. It would change the
  `Fetcher` interface for every panel in the tree; the remembered-size approach
  costs one soft frame per resize instead.

---

## Task T1 - Measure the cell in pixels, not just its shape

Files: `cell_size.go`, `styles.go`, `layout.go`, `image_test.go` (or a new
`cell_size_test.go`).

Today the app learns the cell's *aspect* (2.4) but not its *size* (7.2 × 17 px), so
nothing can decide whether a 512 px tile covers a pane.

**T1a. Write the failing test.** Create `cell_size_test.go`:

```go
package tideui

import "testing"

// A measurement no terminal could produce is treated as no measurement: a pane
// sized from a broken ioctl must not ask for nine tiles.
func TestNormalisedCellWidthRefusesTheImpossible(t *testing.T) {
	if got := normalisedCellWidth(0); got != defaultCellWidth {
		t.Fatalf("an unmeasured cell = %v, want the default %v", got, defaultCellWidth)
	}
	if got := normalisedCellWidth(-3); got != defaultCellWidth {
		t.Fatalf("a negative cell = %v, want the default %v", got, defaultCellWidth)
	}
	if got := normalisedCellWidth(120); got != defaultCellWidth {
		t.Fatalf("a 120 px cell = %v, want the default %v", got, defaultCellWidth)
	}
	if got := normalisedCellWidth(7.2); got != 7.2 {
		t.Fatalf("a measured cell = %v, want 7.2", got)
	}
}
```

Run: `go test -run TestNormalisedCellWidth ./...` → `undefined:
normalisedCellWidth`, red.

**T1b. Make it pass.** `cell_size.go` gains `CellSizeOf` and keeps `CellAspectOf`
as a wrapper (one ioctl, two callers):

```go
// CellSizeOf measures a terminal's cell in pixels, or reports 0, 0 when the
// terminal will not say: the ioctl reports the window in pixels alongside its
// size in cells, and a pty with no emulator behind it reports zeros.
func CellSizeOf(file *os.File) (width, height float64) {
	if file == nil {
		return 0, 0
	}
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil || size == nil || size.Col == 0 || size.Row == 0 {
		return 0, 0
	}
	if size.Xpixel == 0 || size.Ypixel == 0 {
		return 0, 0
	}
	return float64(size.Xpixel) / float64(size.Col), float64(size.Ypixel) / float64(size.Row)
}

// CellAspectOf is CellSizeOf as the ratio an image is sized by.
func CellAspectOf(file *os.File) float64 {
	width, height := CellSizeOf(file)
	if width <= 0 || height <= 0 {
		return 0
	}
	return height / width
}
```

`styles.go` gains the option, next to `CellAspect`:

```go
	// CellWidth is one cell's width in pixels, measured with CellSizeOf. Zero
	// means nobody measured it, and anything that needs a pane's pixel size
	// assumes 8 - the width almost every monospace font has at a sane size.
	CellWidth float64
```

`layout.go` gains the field and the normaliser (same shape as
`normalisedCellAspect`, in the same place):

```go
	// CellWidth is one cell's width in pixels, already checked (see
	// StyleOptions.CellWidth). Only things that reason in pixels read it.
	CellWidth float64
```

```go
// defaultCellWidth is what a pane is measured with when nobody measured the
// terminal: about the width of a monospace cell at a sane font size.
const defaultCellWidth = 8.0

// normalisedCellWidth keeps a measurement a terminal could plausibly have - one
// pixel per cell to forty - and treats anything else as no measurement: a pane
// sized from a broken ioctl would otherwise ask for a mosaic it does not need.
func normalisedCellWidth(width float64) float64 {
	if width < 1 || width > 40 {
		return defaultCellWidth
	}
	return width
}
```

and `NewRenderer` sets both:

```go
	return Renderer{
		Styles:     BuildStyles(theme, options),
		CellAspect: normalisedCellAspect(options.CellAspect),
		CellWidth:  normalisedCellWidth(options.CellWidth),
	}
```

Then wire the app: in `examples/workspace/main.go` and
`examples/workspace/panels.go`, replace `CellAspect: tideui.CellAspectOf(os.Stdout)`
with a measurement of both, which means measuring once:

```go
	cellWidth, cellHeight := tideui.CellSizeOf(os.Stdout)
```

…and passing `CellWidth: cellWidth` and `CellAspect:` the ratio when the
measurement is real:

```go
	// One ioctl gives both, so the pair is measured together: a picture is sized
	// by the shape of a cell and a fetch is sized by its pixels.
	cellWidth, cellHeight := tideui.CellSizeOf(os.Stdout)
	options := tideui.StyleOptions{
		Density: m.state.density, PaneCorners: tideui.RoundCorners,
		Gauge: m.state.gauge, Sparkline: m.state.spark, ClockFont: m.state.clockFont,
		IconStyle: m.state.icons, ModalShadow: true,
	}
	if cellWidth > 0 && cellHeight > 0 {
		options.CellWidth = cellWidth
		options.CellAspect = cellHeight / cellWidth
	}
	renderer := tideui.NewRenderer(m.state.theme, options)
```

Run: `go test -count=1 ./... && go build ./...` →
`ok` for every package, no build output.

**T1c. Commit.**

```
git add cell_size.go cell_size_test.go styles.go layout.go examples/workspace/main.go examples/workspace/panels.go
git commit -m "Measure a cell's pixels, not just its shape"
```

---

## Task T2 - A mosaic, so the picture can be as big as the pane

Files: `provider/radar.go`, `provider/radar_test.go`, `image.go`.

**T2a. Write the failing tests.** Append to `provider/radar_test.go`:

```go
// The block of tiles is centred on the coordinate's own tile, and the picture says
// where in itself the coordinate is - a panel that assumes the middle puts the
// reader on the wrong county.
func TestRadarGridCentresTheCoordinate(t *testing.T) {
	// Austin at zoom 7 is tile 29/52 (checked against the live service).
	grid := radarGridFor(30.2672, -97.7431, 7, 3, 3)
	if grid.X != 28 || grid.Y != 51 {
		t.Fatalf("grid origin = %d/%d, want 28/51", grid.X, grid.Y)
	}
	if grid.Centre.X < radarTileSize || grid.Centre.X >= 2*radarTileSize {
		t.Fatalf("the coordinate landed at x=%d, outside its own tile", grid.Centre.X)
	}
	// One tile means the coordinate's own tile, and the centre is inside it.
	single := radarGridFor(30.2672, -97.7431, 7, 1, 1)
	if single.X != 29 || single.Y != 52 {
		t.Fatalf("a single-tile grid = %d/%d, want 29/52", single.X, single.Y)
	}
	if single.Centre.X < 0 || single.Centre.X >= radarTileSize || single.Centre.Y < 0 || single.Centre.Y >= radarTileSize {
		t.Fatalf("a single-tile centre = %v, outside the tile", single.Centre)
	}
}

// Ground distance per pixel is what makes a distance ring honest. At zoom 7 the
// world is 128 tiles across and a tile is radarTileSize pixels, so a tile is
// 40075*cos(lat)/128 km wide.
func TestRadarKilometresPerPixel(t *testing.T) {
	got := kilometresPerPixel(39, 7)
	want := 40075 * math.Cos(39*math.Pi/180) / 128 / radarTileSize
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("kilometresPerPixel(39, 7) = %v, want %v", got, want)
	}
	if got := kilometresPerPixel(0, 7); got < 0.6 || got > 0.62 {
		t.Fatalf("at the equator a pixel at zoom 7 = %v km, want about 0.61", got)
	}
}

// A block that runs off the edge of the world repeats the edge tile instead of
// asking for a tile that does not exist.
func TestRadarGridClampsToTheWorld(t *testing.T) {
	url := radarGridFor(0, -179.9, 1, 3, 3).tileURL("https://example.test", "/v2/radar/x", 0, 0)
	if !strings.Contains(url, "/1/0/0/") {
		t.Fatalf("a tile off the west edge = %q, want x clamped to 0", url)
	}
}
```

Append to `image_test.go` (the model is `tideui`'s):

```go
// A frame carries where the reader is in the picture and what a pixel is worth on
// the ground, because a panel needs both and can compute neither.
func TestRadarFrameCarriesItsScale(t *testing.T) {
	frame := RadarFrame{Time: time.Now(), Image: solid(4, 4, color.RGBA{A: 255})}
	if frame.Centre != (image.Point{}) || frame.KilometresPerPixel != 0 {
		t.Fatal("a zero frame should have a zero scale")
	}
	frame.Centre = image.Pt(3, 4)
	frame.KilometresPerPixel = 0.475
	if frame.Centre.X != 3 || frame.KilometresPerPixel != 0.475 {
		t.Fatalf("frame = %+v", frame)
	}
}
```

Run: `go test -run 'TestRadarGrid|TestRadarKilometres|TestRadarFrameCarriesItsScale' ./provider/ .` →
`undefined: radarGridFor`, `unknown field Centre`, red.

**T2b. Make it pass.** In `image.go`, extend the model:

```go
// RadarFrame is one radar frame: the picture and the moment it describes, plus
// where in that picture the reader's own coordinate is and what one pixel of it
// is worth on the ground. The panel needs all four and can compute none of them
// from the picture alone.
type RadarFrame struct {
	Time  time.Time
	Image image.Image
	// Centre is the pixel the coordinate sits at. It is only the middle of the
	// picture when the block of tiles is odd, which is why it is carried.
	Centre image.Point
	// KilometresPerPixel is the ground distance one pixel covers.
	KilometresPerPixel float64
}
```

In `provider/radar.go` (add `image`, `image/draw`, `math` to the imports it
already has):

```go
// radarTileFraction is where a coordinate sits inside its own tile, 0..1 in each
// direction.
func radarTileFraction(lat, lon float64, zoom int) (fx, fy float64) {
	scale := math.Exp2(float64(clampZoom(zoom)))
	x := (lon + 180) / 360 * scale
	radians := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(radians)+1/math.Cos(radians))/math.Pi) / 2 * scale
	return x - math.Floor(x), y - math.Floor(y)
}

// radarGrid is the block of tiles to fetch around the one that contains a
// coordinate, and where in the stitched picture that coordinate falls.
type radarGrid struct {
	X, Y       int // the top-left tile of the block
	Cols, Rows int
	Zoom       int
	Centre     image.Point
}

// radarGridFor centres a cols x rows block on the coordinate's own tile. An
// off-centre block is deliberate rather than rounded outwards: the reader is the
// subject, so the block grows the same distance in every direction.
func radarGridFor(lat, lon float64, zoom, cols, rows int) radarGrid {
	zoom = clampZoom(zoom)
	cols, rows = normaliseGrid(cols), normaliseGrid(rows)
	x, y := radarTileFor(lat, lon, zoom)
	fx, fy := radarTileFraction(lat, lon, zoom)
	offsetX, offsetY := (cols-1)/2, (rows-1)/2
	return radarGrid{
		X: x - offsetX, Y: y - offsetY, Cols: cols, Rows: rows, Zoom: zoom,
		Centre: image.Pt(
			offsetX*radarTileSize+int(fx*radarTileSize),
			offsetY*radarTileSize+int(fy*radarTileSize),
		),
	}
}

// rect is where a tile of the block goes in the stitched picture.
func (g radarGrid) rect(col, row int) image.Rectangle {
	return image.Rect(col*radarTileSize, row*radarTileSize,
		(col+1)*radarTileSize, (row+1)*radarTileSize)
}

// tileURL is the service's own shape, with the numbers in the order it documents:
// {path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png. A block that runs off the
// world repeats the edge tile: a duplicated edge is honest, a hole is not.
func (g radarGrid) tileURL(host, path string, col, row int) string {
	scale := 1 << g.Zoom
	x := clampTile(g.X+col, scale)
	y := clampTile(g.Y+row, scale)
	return fmt.Sprintf("%s%s/%d/%d/%d/%d/4/1_1.png", host, path, radarTileSize, g.Zoom, x, y)
}

// normaliseGrid keeps a grid inside what a panel may ask for: one to three tiles
// each way.
func normaliseGrid(n int) int { return min(max(n, 1), 3) }

// kilometresPerPixel is the ground distance one pixel of the picture covers, which
// is what turns a distance ring from decoration into a measurement.
func kilometresPerPixel(lat float64, zoom int) float64 {
	const earthCircumference = 40075.0
	tilesAcross := math.Exp2(float64(clampZoom(zoom)))
	kmPerTile := earthCircumference * math.Cos(lat*math.Pi/180) / tilesAcross
	return kmPerTile / float64(radarTileSize)
}
```

`RadarOptions` gains the block, and `Radar` stitches:

```go
type RadarOptions struct {
	Latitude  float64
	Longitude float64
	// Zoom is the slippy-map zoom. The service documents 7 as its deepest, and
	// asks for more serves a canned tile rather than an error: verified by
	// fetching eight and ten, which return byte-identical files for different
	// coordinates. So the clamp is not politeness, it is the difference between
	// a picture of the weather and a picture of nothing.
	Zoom int
	// Cols and Rows are how many tiles the pane is worth, one each way by
	// default. More tiles do not show more weather, they show the same weather
	// with more pixels - which is what a zoomed pane needs.
	Cols, Rows int
}
```

```go
func Radar(opts RadarOptions) func(context.Context) (tideui.RadarFrame, error) {
	return func(ctx context.Context) (tideui.RadarFrame, error) {
		index, err := fetchRadarIndex(ctx)
		if err != nil {
			return tideui.RadarFrame{}, err
		}
		if len(index.Radar.Past) == 0 {
			return tideui.RadarFrame{}, fmt.Errorf("radar: the index lists no frames")
		}
		newest := index.Radar.Past[len(index.Radar.Past)-1]
		host := index.Host
		if host == "" {
			host = radarFallbackHost
		}

		zoom := clampZoom(opts.Zoom)
		grid := radarGridFor(opts.Latitude, opts.Longitude, zoom, opts.Cols, opts.Rows)
		picture := image.NewRGBA(image.Rect(0, 0, grid.Cols*radarTileSize, grid.Rows*radarTileSize))
		for row := 0; row < grid.Rows; row++ {
			for col := 0; col < grid.Cols; col++ {
				tile, err := fetchRadarTile(ctx, grid.tileURL(host, newest.Path, col, row))
				if err != nil {
					return tideui.RadarFrame{}, err
				}
				draw.Draw(picture, grid.rect(col, row), tile, tile.Bounds().Min, draw.Src)
			}
		}
		return tideui.RadarFrame{
			Time:               time.Unix(newest.Time, 0),
			Image:              picture,
			Centre:             grid.Centre,
			KilometresPerPixel: kilometresPerPixel(opts.Latitude, zoom),
		}, nil
	}
}
```

The old single-tile tests still pass unchanged: a 1 × 1 grid is what they exercise,
and `radarTileSize` is still 512 in the URL. The fake server in `radarServer` needs
its handler path left alone for the 1 × 1 case; add a second test for a 2 × 2 fetch
that registers four paths and asserts each quadrant of the returned picture came
from the right tile:

```go
// A mosaic is stitched in the order it was asked for, and the picture is as big
// as the block: four tiles is 2*radarTileSize square.
func TestRadarMosaicStitchesTheBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/public/weather-maps.json") {
			json.NewEncoder(w).Encode(map[string]any{
				"host": "http://" + r.Host,
				"radar": map[string]any{"past": []map[string]any{
					{"time": 1789767600, "path": "/v2/radar/newest"},
				}},
			})
			return
		}
		// One distinct colour per tile, so a test can say which came from where.
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		colour := map[string]color.RGBA{
			"/v2/radar/newest/512/7/28/51/4/1_1.png": {R: 255, A: 255},
			"/v2/radar/newest/512/7/29/51/4/1_1.png": {G: 255, A: 255},
			"/v2/radar/newest/512/7/28/52/4/1_1.png": {B: 255, A: 255},
			"/v2/radar/newest/512/7/29/52/4/1_1.png": {R: 255, G: 255, A: 255},
		}[r.URL.Path]
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				img.Set(x, y, colour)
			}
		}
		png.Encode(w, img)
	}))
	t.Cleanup(server.Close)
	restore := radarIndexURL
	radarIndexURL = server.URL + "/public/weather-maps.json"
	t.Cleanup(func() { radarIndexURL = restore })

	frame, err := Radar(RadarOptions{Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Cols: 2, Rows: 2})(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Image.Bounds().Dx(); got != 2*radarTileSize {
		t.Fatalf("the mosaic is %d px wide, want %d", got, 2*radarTileSize)
	}
	// The coordinate's own tile is the bottom-right one of this block.
	if _, _, _, alpha := frame.Image.At(frame.Centre.X, frame.Centre.Y).RGBA(); alpha == 0 {
		t.Fatal("the centre of the mosaic is empty, so the tiles are not stitched")
	}
}
```

Run: `go test -count=1 ./provider/ .` → `ok` for both.

**T2c. Commit.**

```
git add image.go provider/radar.go provider/radar_test.go image_test.go
git commit -m "Fetch a block of radar tiles, and say where the reader is in it"
```

---

## Task T3 - Fetch the block this pane is worth

Files: `dash/panels/radar.go`, `dash/panels/radar_test.go`.

**T3a. Write the failing tests.** Append to `dash/panels/radar_test.go`:

```go
// A pane smaller than one tile asks for one tile: a dashboard glance is not a
// download. A pane bigger than one tile asks for enough to cover it.
func TestRadarAsksForTheTilesItsPaneIsWorth(t *testing.T) {
	small := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 7, CellAspect: 2.4})
	if cols, rows := radarGrid(small, 33, 13); cols != 1 || rows != 1 {
		t.Fatalf("a 33x13 pane asked for %dx%d tiles, want 1x1", cols, rows)
	}
	big := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{CellWidth: 8, CellAspect: 2})
	if cols, rows := radarGrid(big, 130, 30); cols*rows <= 1 {
		t.Fatalf("a 130x30 pane asked for %dx%d tiles, want more than one", cols, rows)
	}
	if cols, rows := radarGrid(big, 130, 30); cols*rows > radarTileBudget {
		t.Fatalf("a 130x30 pane asked for %d tiles, over the budget of %d", cols*rows, radarTileBudget)
	}
}
```

Run: `go test -run TestRadarAsksForTheTiles ./dash/panels/` → `undefined: radarGrid`,
red.

**T3b. Make it pass.** In `dash/panels/radar.go`:

```go
// defaultCellWidth matches tideui's: what a pane is measured with when the
// terminal never said how wide its cells are.
const defaultCellWidth = 8.0

// radarTileBudget is the most tiles one refresh will fetch, and nobody else's
// money should be spent on a panel that is a glance: four tiles is a 1024 px
// picture, wider than any pane this runs in.
const radarTileBudget = 4

// radarGrid is how many tiles a pane is worth. The pane's size in cells says
// nothing on its own - 40 cells is a small pane at one font size and a huge one at
// another - so the question is asked in pixels: enough tiles to cover the pane, no
// more than the budget.
func radarGrid(renderer tideui.Renderer, width, height int) (cols, rows int) {
	cellWidth := renderer.CellWidth
	if cellWidth <= 0 {
		cellWidth = defaultCellWidth
	}
	pixelsWide := float64(width) * cellWidth
	pixelsHigh := float64(height) * cellWidth * renderer.CellAspect

	cols = min(int(math.Ceil(pixelsWide/provider.RadarTilePixels)), 3)
	rows = min(int(math.Ceil(pixelsHigh/provider.RadarTilePixels)), 3)
	for cols*rows > radarTileBudget {
		// Give up height first: a panel is wider than it is tall.
		if rows > 1 {
			rows--
		} else if cols > 1 {
			cols--
		} else {
			break
		}
	}
	return max(1, cols), max(1, rows)
}
```

`provider/radar.go` exports the tile size it fetches, so the panel is not repeating
a magic number:

```go
// RadarTilePixels is the pixel size of every tile this provider asks the service
// for. It is exported because a panel deciding how many tiles it needs is asking
// the same question in the same units.
const RadarTilePixels = radarTileSize
```

The panel remembers the pane it last drew in and configures from that (it has no
size at fetch time; see the assumption above):

```go
type radar struct {
	...
	location   string
	zoom       int
	// paneWidth, paneHeight and cell are remembered from the last View, because
	// a fetch happens without a PanelContext and "how many tiles" is a question
	// about the pane. A resize is picked up on the next refresh; until then the
	// picture is soft, never wrong.
	paneWidth, paneHeight int
	cell                  tideui.Renderer
```

`Configure` passes the remembered grid (a zero pane asks for one tile):

```go
	cols, rows := 1, 1
	if r.paneWidth > 0 && r.paneHeight > 0 {
		cols, rows = radarGrid(r.cell, r.paneWidth, r.paneHeight)
	}
	r.fetch = r.newFetcher(provider.RadarOptions{
		Latitude: latitude, Longitude: longitude, Zoom: r.zoom, Cols: cols, Rows: rows,
	})
```

and `View` records the pane before drawing (at the top of the frame path, next to
the caption):

```go
	r.mu.Lock()
	r.paneWidth, r.paneHeight, r.cell = ctx.Width, ctx.Height, ctx.Renderer
	r.mu.Unlock()
```

Note the `r.mu` is already held in places in this file: read the existing locking
before adding to it, and do not take the lock twice in one path.

Run: `go test -count=1 ./dash/panels/` → `ok`.

**T3c. Commit.**

```
git add dash/panels/radar.go dash/panels/radar_test.go provider/radar.go
git commit -m "Ask for the tiles the pane is worth"
```

---

## Task T4 - Draw the picture as a map: position, distance, scale

Files: `dash/panels/radar.go`, `dash/panels/radar_test.go`.

**T4a. Write the failing tests.** Append to `dash/panels/radar_test.go`:

```go
// The mark is where the coordinate is, which is only the middle of the picture
// when the block is odd: this is the bug a single-tile assumption hides.
func TestRadarMarksTheCoordinateWhereItActuallyIs(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 64, 64))
	marked, ok := markCentre(blank, image.Pt(20, 30), color.RGBA{R: 255, A: 255}).(*image.RGBA)
	if !ok {
		t.Fatal("markCentre did not return a drawable image")
	}
	if _, _, _, alpha := marked.At(20, 30).RGBA(); alpha == 0 {
		t.Fatal("the coordinate was left unmarked")
	}
	if _, _, _, alpha := marked.At(46, 30).RGBA(); alpha != 0 {
		t.Fatal("the mark was drawn at the middle instead of at the coordinate")
	}
}

// A ring is a measurement, not decoration: at a known km per pixel it lands where
// the ground distance says it should, and it never runs outside the picture.
func TestRadarRingIsDrawnAtTheDistanceItClaims(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 512, 512))
	marked, ok := drawRing(blank, image.Pt(256, 256), 0.475, maxRingKilometres, color.RGBA{G: 255, A: 255}).(*image.RGBA)
	if !ok {
		t.Fatal("drawRing did not return a drawable image")
	}
	// 100 km at 0.475 km per pixel is 210.5 px: the ring is on that radius.
	radius := int(math.Round(100 / 0.475))
	if _, _, _, alpha := marked.At(256+radius, 256).RGBA(); alpha == 0 {
		t.Fatalf("no ring at the radius %d px that 100 km is worth", radius)
	}
	if _, _, _, alpha := marked.At(256+radius/2, 256).RGBA(); alpha != 0 {
		t.Fatal("the ring was filled instead of drawn")
	}
	// Past the edge there is no ring to draw rather than a clipped one.
	if got := ringKilometres(0.475, image.Pt(10, 10), image.Rect(0, 0, 512, 512)); got != 0 {
		t.Fatalf("a ring for a coordinate 10 px from the edge = %d km, want none", got)
	}
}

// The caption says what the picture is worth on the ground, which is the one
// number a radar glance is missing.
func TestRadarCaptionNamesTheScale(t *testing.T) {
	wide := radarCaption(60, "17:50", "Kansas City", "240 km")
	if !strings.Contains(wide, "240 km") {
		t.Fatalf("a wide caption = %q, want the ground scale", wide)
	}
	if got := radarCaption(26, "17:50", "Kansas City", "240 km"); !strings.Contains(got, "RainViewer") {
		t.Fatalf("a narrow caption = %q, want the source kept", got)
	}
}
```

Run: `go test -run 'TestRadarMarks|TestRadarRing|TestRadarCaptionNames' ./dash/panels/`
→ `too many arguments in call to markCentre`, `undefined: drawRing`, red.

**T4b. Make it pass.** Replace `markCentre` and add the ring (both draw on a copy,
because the frame in state is reused by the next draw):

```go
// markCentre draws the reader's position into the picture. It is not the middle of
// the picture: that is only true when the block of tiles is odd, and a block that
// is even puts them half a tile off - on the wrong county.
func markCentre(img image.Image, centre image.Point, colour color.RGBA) image.Image {
	marked, ok := copyOf(img)
	if !ok {
		return img
	}
	bounds := marked.Bounds()
	centreX, centreY := clampInt(centre.X, bounds.Min.X, bounds.Max.X-1), clampInt(centre.Y, bounds.Min.Y, bounds.Max.Y-1)
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

// ringKilometres picks the roundest ground distance that fits inside the picture
// around the coordinate: 100 km, or 50, or 25, or nothing at all when the
// coordinate is too near an edge for a ring to mean anything.
func ringKilometres(kmPerPixel float64, centre image.Point, bounds image.Rectangle) int {
	if kmPerPixel <= 0 {
		return 0
	}
	reach := min(
		min(centre.X-bounds.Min.X, bounds.Max.X-centre.X),
		min(centre.Y-bounds.Min.Y, bounds.Max.Y-centre.Y),
	) - 2
	for _, kilometres := range []int{maxRingKilometres, 50, 25} {
		if float64(kilometres)/kmPerPixel <= float64(reach) {
			return kilometres
		}
	}
	return 0
}

// maxRingKilometres is the widest ring worth drawing: at the zoom this panel
// uses, a bigger one leaves the picture.
const maxRingKilometres = 100

// drawRing draws a dotted circle of a ground distance around the coordinate, in
// the picture's own pixels, so the terminal scales it with everything else.
func drawRing(img image.Image, centre image.Point, kmPerPixel float64, kilometres int, colour color.RGBA) image.Image {
	marked, ok := copyOf(img)
	if !ok {
		return img
	}
	if kilometres <= 0 || kmPerPixel <= 0 {
		return marked
	}
	radius := int(math.Round(float64(kilometres) / kmPerPixel))
	if radius < 3 {
		return marked
	}
	bounds := marked.Bounds()
	// A dotted circle: every eighth degree of the sweep, so it reads as a guide
	// and not as a border.
	for step := 0; step < 360; step += 8 {
		radians := float64(step) * math.Pi / 180
		x := centre.X + int(math.Round(float64(radius)*math.Cos(radians)))
		y := centre.Y + int(math.Round(float64(radius)*math.Sin(radians)))
		if x >= bounds.Min.X && x < bounds.Max.X && y >= bounds.Min.Y && y < bounds.Max.Y {
			marked.Set(x, y, colour)
		}
	}
	return marked
}

// copyOf hands back a drawable copy, or false when the picture is not one.
func copyOf(img image.Image) (*image.RGBA, bool) {
	if img == nil {
		return nil, false
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, false
	}
	marked := image.NewRGBA(bounds)
	draw.Draw(marked, bounds, img, bounds.Min, draw.Src)
	return marked, true
}

func clampInt(value, low, high int) int { return min(max(value, low), high) }
```

`radarCaption` takes the scale as a string, and the scale part is dropped before
the source exactly as the zoom was:

```go
// radarCaption is the panel's context line, dropped in order of stubbornness as
// the pane narrows: the time and the source stay, the ground scale goes first and
// the place before it. A credit the layout truncated away is not a credit.
func radarCaption(width int, when, location, scale string) string {
	parts := []string{when, location, scale, "RainViewer"}
	for len(parts) > 2 && ansi.StringWidth(strings.Join(parts, " · ")) > width {
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
	}
	return strings.Join(parts, " · ")
}
```

and `View` builds the picture with all of it — the ring in a muted colour, the
mark in the workspace's text colour, the caption carrying the scale:

```go
	width := frame.Image.Bounds().Dx()
	scale := fmt.Sprintf("%d km", int(math.Round(frame.KilometresPerPixel*float64(width))))
	lines := []string{radarCaption(ctx.Width, frame.Time.Local().Format("15:04"), location, scale)}
	if quiet {
		lines = append(lines, "no precipitation in range")
	}
	picture := markCentre(frame.Image, frame.Centre, tideui.RGBAOf(ctx.Renderer.Styles.Workspace.BodyFg))
	picture = drawRing(picture, frame.Centre, frame.KilometresPerPixel,
		ringKilometres(frame.KilometresPerPixel, frame.Centre, frame.Image.Bounds()),
		tideui.RGBAOf(ctx.Renderer.Styles.Workspace.BodyMutedFg))
	picture = ctx.Renderer.RenderImage(picture, ctx.Width, max(1, ctx.Height-len(lines)))
```

(`BodyMutedFg` is the quiet text colour in `WorkspaceStyles` (`styles.go:380`),
which is what a guide line should be: present and ignorable. The mark stays the
workspace's `BodyFg`, so it reads as *the reader* rather than as another guide.)

The existing `TestRadarMarksWhereTheReaderIs` and
`TestRadarCaptionDropsTheZoomBeforeTheSource` are replaced by the tests above; the
`TestRadarDrawsTheFrameAndItsTime` and `TestRadarSaysWhenTheSkyIsEmpty` fixtures
need `Centre`/`KilometresPerPixel` set on their frames so the caption has a scale
(the zero value would say "0 km", which is honest but useless in a test).

Run: `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...` →
`ok` for all six packages, no other output.

**T4c. Commit.**

```
git add dash/panels/radar.go dash/panels/radar_test.go
git commit -m "Draw the radar as a map: your place, a distance ring, the ground scale"
```

---

## Task T5 - Document it

Files: `README.md`.

The provider table row and the renderer table row exist already
("Radar | `provider.Radar(RadarOptions)` | RainViewer (no key); one 512 px tile,
newest frame, zoom ≤ 7"). Update the provider row to say a block of tiles, and add
one sentence to the row-type/panel prose:

```
| Radar | `provider.Radar(RadarOptions)` | RainViewer (no key); the newest frame as one or more 512 px tiles, zoom ≤ 7 |
```

Verify the docs bench still passes, since it renders documents out of the README
and counts them:

```
go test -run TestReadme ./dash/
```

Expected: `ok  	github.com/allisonhere/tideui/dash` — one manifest and one document
found in the README, which is what that bench asserts.

**Commit.**

```
git add README.md
git commit -m "Say what the radar panel fetches now"
```

---

## Tests and validation

After every task, and again at the end:

```
cd /home/allie/Projects/tidedeck && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...
```

Expected: no `gofmt` output, `ok` for `tideui`, `dash`, `dash/panels`,
`examples/workspace`, `form`, `provider`.

Then the same suite as a machine with no terminal at all, because every test that
touches the transport must not care where it runs:

```
T=$(mktemp -d) && env -u TERM -u TERM_PROGRAM -u KITTY_WINDOW_ID -u TMUX HOME=$T GOCACHE=$HOME/.cache/go-build GOMODCACHE=$HOME/go/pkg/mod GOPATH=$HOME/go go test -count=1 ./... ; rm -rf "$T"
```

Expected: six `ok` lines. Nothing may reach `api.rainviewer.com` from a test.

By hand, in a real terminal (this is the part that matters, because the whole
feature is a picture):

1. `go run ./examples/workspace`, panel picker (`w`), show Radar, and read the
   caption: it should name the ground scale in kilometres, and the picture should
   have a ring and a crosshair where the location is.
2. Zoom the radar pane (`space` then `space` again, or the zoom key in the status
   bar): the picture should stay sharp rather than being stretched, because the
   panel now asks for a block of tiles when the pane is bigger than one.
3. Resize the terminal: the picture re-fits; the tile block follows on the next
   refresh.
4. Compare `printf '\033_Gi=1,a=q,s=1,v=1,t=d,f=24;AAAA\033\\'` behaviour if a
   terminal ever draws nothing: that is the graphics query, and it is the fastest
   way to tell a terminal that cannot do this from a program that is asking wrong.

## Risks, tradeoffs, and open questions

- **Traffic.** Four tiles per five minutes is ~100 requests an hour to a free
  service, up from 12. The budget is in the panel and the default pane asks for
  one tile, so the extra traffic happens only when the pane is big enough to
  deserve it. If that is still too much, the honest lever is a longer interval
  (`Meta().Interval`), not a smaller picture.
- **One refresh behind a resize.** A fetch has no pane size, so the tile block is
  decided from the last drawn pane. A resize leaves the picture soft for at most
  one interval; the panel's own refresh action fixes it sooner. The alternative is
  changing the `Fetcher` interface for every panel, which is not worth it here.
- **The mark is a crosshair, not a pin.** At zoom 7 a pixel is ~0.5 km, so a
  two-pixel crosshair is a kilometre wide - about right, and it disappears if the
  pane is tiny. If it proves too subtle, the next step is drawing it in the cell
  grid instead of the picture (a marked cell cannot be scaled away).
- **No base map, still.** The ring and the scale make an empty pane readable, not
  *located*: "100 km north-west" is a bearing without a landmark. A coastline or
  state-line source would fix it and is a separate plan with its own terms of use.
- **Open question - the default zoom.** The panel starts at 7, which shows ~243 km
  and therefore shows nothing at all on a quiet day (measured: 0.15% echo over
  Austin versus 30% over Kentucky in the same frame). A default of 6 shows four
  times the area for the same traffic and would fill the pane with weather far more
  often; the trade is that each cell becomes ~15 km of ground. This is a taste
  question and the setting is one keystroke away, so the plan leaves 7 unless the
  reader says otherwise.
