# Image rows and a weather radar panel

## Goal

Teach the dashboard to draw a picture inside a panel — as coloured half-block
cells, so it works over ssh and in tmux with no graphics protocol — and ship a
radar panel that uses it to show the precipitation around the place the weather
panel already knows.

## What I think, before the plan (the question you actually asked)

**Image support: yes, but only the cheap version, and be honest about its
ceiling.** Half-block cells (`▀` with the top sample as foreground and the bottom
as background) give one sample across and two down per cell, which is all a
terminal has without a protocol. In a 40-cell-wide panel that is 40×40 samples —
plenty for a radar blob, a sparkline-shaped chart, a QR code, or a heat map;
useless for a photo. The alternatives are worse than the ceiling:

- **The kitty graphics protocol is better than my first pass said, and it is
  cheap _here_.** I ruled it out on the grounds that a pixel protocol forces the
  app to track absolute cell positions and re-emit the image every frame. That is
  true of *cursor-positioned* placement and false of kitty's **Unicode
  placeholders**: the image is transmitted once with `U=1` and then drawn by
  ordinary text cells — `U+10EEEE`, with diacritics naming the row and column and
  the image id carried in the cell's foreground colour. Those cells are just
  characters, so they compose with everything the dashboard already does: panes,
  zoom, padding, and the translucent-background blend in `layout.go:583`. The
  encoder is *already in `go.mod`* — `github.com/charmbracelet/x/ansi/kitty` has
  `EncodeGraphics`, `Options{Action, ID, Format, VirtualPlacement, …}`,
  `Placeholder` and `Diacritic(i)`. And I checked the pipeline instead of assuming:
  a probe through `cellbuf.SetContent` + `cellbuf.Render` — exactly what
  `layout.go:583-620` runs over every panel line — returns the APC transmit
  sequence, the placeholder and its diacritics **unharmed**. You are on kitty
  0.48.2, which supports all of it. So kitty is **Task A3**: half-blocks stay the
  baseline that every terminal gets, kitty is a second transport behind one
  capability check, and what it buys is not a bigger panel but the real picture —
  256×256 of radar at the terminal's pixel resolution, rather than 40×40 averages.
- It still has to be *detected*, never assumed: placeholder cells in a terminal
  that does not understand them are a screenful of tofu, so the transport requires
  kitty without tmux and a truecolor profile, and half-blocks otherwise. That
  check is a unit test, and it is what keeps this debuggable over ssh.
- Inline pixels cannot travel in the plugin document either way: a document is
  capped at `maxPluginOutput` (1 MB), so pixels have to be a *reference* — a path —
  whatever the transport. Which is what this plan does.
- And the fallback degrades through machinery that already exists: `lipgloss`
  quantises colour to the active profile, so a 256-colour terminal gets a
  256-colour half-block picture for free, and only a truly colourless one needs
  the ramp.

**Radar: the best possible first customer, and it is real.** I verified the source
live before writing this: `https://api.rainviewer.com/public/weather-maps.json`
needs no key and returns `host` plus twelve `radar.past` frames
(`{time, path}`), and a tile at
`https://tilecache.rainviewer.com/v2/radar/79877904683f/256/8/58/105/256/4/1_1.png`
comes back `200  image/png`, a 256×256 paletted PNG whose transparency is "no
rain" — which is exactly what the half-block compositing has to handle. The
coordinates for Austin already produce 58/105 at zoom 8, the tile I fetched.

The honest caveats, all of which the plan handles rather than hides: a radar
picture without a base map is blobs on the panel background (fine at zoom 7–9,
where you can read "north-west of me"); the aspect maths only works if the cell is
2:1 tall-to-wide (true of every terminal I know, and a style option later if it
is not); and animation is a separate contract this plan deliberately does not
build (see the end).

## Current context / assumptions

Everything below was verified in this checkout; read the files before starting.

- The module root is `package tideui`: purely presentational, and **it cannot
  import `provider`** (`provider` imports `tideui`, not the other way round — see
  the note at the top of `dash/dash.go`). Data models live in `tideui`
  (`WeatherData` in `dashboard_widgets.go`, `Headline`, …), sources in `provider`,
  and the registry that binds them in `dash`.
- `Renderer` is `type Renderer struct { Styles Styles }` (`layout.go:77`) and every
  widget is a method on it: `RenderWeather(w WeatherData, width int) string`
  (`dashboard_widgets.go:19`), `RenderSparkline` (`chrome.go:760`),
  `RenderLines(lines []string, width int, bg lipgloss.Color) string`
  (`info_widgets.go:296`, pads each line to `width` with `bg`).
- The colour profile is already bridged: `activeColorProfile()`
  (`layout.go:638`) maps lipgloss's global profile onto `colorprofile.Profile`
  and returns `colorprofile.ASCII` for a terminal with no colour. `colorprofile`
  is a direct dependency in `go.mod`. Use this, do not invent a second switch.
- Plugin documents are `dash/doc.go`: `Doc{Rows []Row, Detail []Row, Options,
  Badge}` with row types `metric`, `gauge`, `spark`, `text`, `block`, `divider`,
  `spacer` and, new this week, a row `ID` that makes a row openable. An unknown
  row type is skipped, never fatal. `RenderDoc` delegates to
  `RenderDocSelected(renderer, doc, width, zoomed, selected)`.
- `provider/weather.go` is the template for a source: `WeatherOptions` struct,
  `func Weather(opts WeatherOptions) func(context.Context) (tideui.WeatherData, error)`,
  a package-level `httpClient` (15s) and `openMeteoBase` var so a test can point
  the source at its own server.
- `dash/panels/weather.go` is the template for a panel: `dash.State[T]` for the
  data, a `newFetcher` field as the seam for tests, `Schema()`/`Configure(values)`
  (`dash.Values`), `Interval`, and an `emptyView` that says *why* it has nothing.
  Its coordinate settings are `weather.latitude` / `weather.longitude` /
  `weather.location` (`weather.go:18-20`), and the app's place search writes them
  (`examples/workspace/settings.go:1067 applyPlace`). The radar panel reads those
  keys instead of asking for a second location.
- Panels are registered in one line, `examples/workspace/main.go:171`, and each
  preset lists the panels it hides: `ws.AddPreset("Overview", overviewLayout(),
  "notes", "git", "markets", "updates")` (`main.go:373-393`). A panel that is
  neither placed nor hidden fails `TestNewPanelsArePlacedOrHiddenInEveryPreset`
  (`examples/workspace/main_test.go:29`) — that test is the gate.
- `go.mod` has no image dependency and does not need one: `image`, `image/png`,
  `image/jpeg`, `image/gif` are stdlib.

## Architecture

One primitive and two callers. `tideui.RenderImage(renderer, img, width, height)`
turns any `image.Image` into coloured half-block lines — presentational, stdlib
only, no I/O. `dash` gains an `image` document row (`src` = an absolute path, `alt`
= what to draw when it cannot be read) plus a small mtime-keyed decode cache, so
a *plugin* can show a picture it produced. The radar is a first-party panel
(`provider/radar.go` + `dash/panels/radar.go`) that fetches one RainViewer tile
and draws it through the same primitive — a plugin would need `curl` and `jq` and
could not be tested without faking both, and the coordinates it needs already
live in the app's settings.

## Decisions recorded (do not re-litigate these while implementing)

1. **Half-block cells, no graphics protocol.** Rationale above; the row type and
   the primitive stay the same if a kitty transport is ever added behind them.
2. **A picture is a reference, never pixels in the document.** The row carries an
   absolute path; base64 in a 1 MB document is not a format.
3. **Contain, never stretch.** A cell is one sample wide and two tall, so a square
   image in a 40-cell pane is 40×20 cells. Stretching a radar to fill a wide pane
   would lie about distance.
4. **Transparency shows the panel through it.** A RainViewer tile is transparent
   where it does not rain; compositing happens in the renderer against the panel
   background, not against black.
5. **Colour degrades through lipgloss, and only the colourless case gets a ramp.**
   `ANSI256`/`ANSI` are quantised by lipgloss for free; `colorprofile.ASCII` draws
   a brightness ramp (`" .:-=+*#%@"`) so the panel has shape rather than nothing.
6. **A row states its own height.** `RenderDoc` knows the width and nothing about
   the height, so an image row derives its height from the source aspect and takes
   an optional `rows` cap (default 24 cells) rather than eating a pane it cannot
   measure.
7. **The radar is a built-in panel, not a plugin.** Its data source is a public
   API and its settings are the app's own coordinates; the plugin path would add
   `curl`/`jq` dependencies and untestable shell. The image *row* is still added,
   because a plugin producing a picture is the general case.
8. **One tile, one frame, five minutes.** No mosaic, no animation, no disk cache:
   the file is 1–40 KB and the panel re-runs four times less often than the mail
   panel. Animation is sketched at the end as its own contract.

## Not doing (YAGNI)

- `sixel` and iTerm2 transports. Both draw pixels positioned by the cursor rather
  than by cells, so they would need the absolute-position bookkeeping this plan
  avoids; kitty is the one protocol with a text-compatible mode (Task A3).
- Terminal capability *querying* (`CSI > q` / `xtversion` and friends) — the
  environment variables are enough and are testable without a terminal.
- Images in manifests (a plugin `icon`), or in keys' glyph slots.
- Animation, mosaics, map overlays, click-to-zoom on a picture.
- Rescaling filters (nearest and box-average are the two that matter; a Lanczos
  in a panel is a punchline).
- Reading an image from a URL: the network belongs to `provider` and to plugins,
  not to `dash/doc.go`.

---

## Task A1 - Draw an image as half-block cells

Files: `image.go` (new, module root), `image_test.go` (new).

**A1a. Write the failing tests.** Create `image_test.go`:

```go
package tideui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// solid builds a width×height image of one colour, and a two-tone one for the
// cases that are about which sample went where.
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
	want := lipgloss.NewStyle().Foreground("#ff0000").Background("#0000ff").Render("▀")
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
```

Run:

```
cd /home/allie/Projects/tidedeck && go test -run TestRenderImage .
```

Expected: `undefined: Renderer.RenderImage` — red.

**A1b. Make it pass.** Create `image.go`:

```go
package tideui

import (
	"image"
	"image/color"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/lipgloss"
)

// imageRamp is what a terminal with no colour gets: brightness, coarse enough to
// read a shape in, because a panel that draws nothing at all looks broken.
const imageRamp = " .:-=+*#%@"

// RenderImage draws an image inside a box of cells, as coloured half-blocks:
// each cell is "▀" with the top sample as its foreground and the bottom as its
// background, which is one sample across and two down per cell - everything a
// terminal has without a graphics protocol, and enough for a radar blob, a heat
// map or a chart.
//
// The picture keeps its aspect and is centred in width, because a cell is roughly
// twice as tall as it is wide and a stretched picture lies. Samples that are
// transparent show the panel background through them, and a terminal with no
// colour gets the brightness ramp instead of the picture.
func (r Renderer) RenderImage(img image.Image, width, maxHeight int) string {
	if img == nil || width <= 0 || maxHeight <= 0 {
		return ""
	}
	source := img.Bounds()
	cells, rows := fitCells(source, width, maxHeight)
	if cells <= 0 || rows <= 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	if activeColorProfile() == colorprofile.ASCII {
		return r.renderImageRamp(img, source, cells, rows, width, bg)
	}

	bgColour := rgbaOf(bg)
	left := (width - cells) / 2
	pad := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", max(0, left)))
	lines := make([]string, 0, rows)
	for cy := 0; cy < rows; cy++ {
		var line strings.Builder
		line.WriteString(pad)
		for cx := 0; cx < cells; cx++ {
			top := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 0), bgColour)
			bottom := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 1), bgColour)
			line.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(hexColour(top))).
				Background(lipgloss.Color(hexColour(bottom))).
				Render("▀"))
		}
		lines = append(lines, line.String())
	}
	return r.RenderLines(lines, width, bg)
}

// fitCells is the size in cells an image takes inside a box: as large as fits
// while keeping its aspect. A cell holds one sample across and two down, so an
// image of aspect sw:sh needs width*sh/(2*sw) rows at that width.
func fitCells(source image.Rectangle, width, maxHeight int) (int, int) {
	sw, sh := source.Dx(), source.Dy()
	if sw <= 0 || sh <= 0 || width <= 0 || maxHeight <= 0 {
		return 0, 0
	}
	cells, rows := width, (width*sh+2*sw-1)/(2*sw) // ceil: no half row is dropped
	if rows > maxHeight {
		rows = maxHeight
		cells = (2 * rows * sw) / sh // floor: what that height allows
	}
	if cells > width {
		cells = width
	}
	return max(1, cells), max(1, rows)
}

// halfCell is the source rectangle one half of a cell covers: the top half of
// cell (cx, cy) when half is 0, the bottom half when it is 1. It is clamped to
// the source, so a box larger than the picture repeats edge samples instead of
// reading outside.
func halfCell(source image.Rectangle, cells, rows, cx, cy, half int) image.Rectangle {
	sw, sh := source.Dx(), source.Dy()
	x0 := source.Min.X + cx*sw/cells
	x1 := source.Min.X + (cx+1)*sw/cells
	y0 := source.Min.Y + (cy*2+half)*sh/(rows*2)
	y1 := source.Min.Y + (cy*2+half+1)*sh/(rows*2)
	clamp := func(v, low, high int) int { return min(max(v, low), high) }
	return image.Rect(
		clamp(x0, source.Min.X, source.Max.X),
		clamp(y0, source.Min.Y, source.Max.Y),
		clamp(max(x1, x0+1), source.Min.X, source.Max.X),
		clamp(max(y1, y0+1), source.Min.Y, source.Max.Y),
	)
}

// sampleRegion is the average colour of the source region one half-cell covers,
// composited onto bg. RGBA() is premultiplied, so the colour is the sum divided
// by the alpha it was multiplied by - averaging the premultiplied values would
// darken every partly transparent pixel, which is what a radar tile is made of.
func sampleRegion(img image.Image, region image.Rectangle, bg color.RGBA) color.RGBA {
	var sumR, sumG, sumB, sumA float64
	count := 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			sumR += float64(cr) / 65535
			sumG += float64(cg) / 65535
			sumB += float64(cb) / 65535
			sumA += float64(ca) / 65535
			count++
		}
	}
	if count == 0 {
		return bg
	}
	n := float64(count)
	alpha := sumA / n
	var red, green, blue float64
	if sumA > 0 {
		red, green, blue = sumR/sumA, sumG/sumA, sumB/sumA
	}
	mix := func(front, back float64) uint8 {
		return uint8(255 * clamp01(front*alpha+back*(1-alpha)))
	}
	return color.RGBA{
		R: mix(red, float64(bg.R)/255),
		G: mix(green, float64(bg.G)/255),
		B: mix(blue, float64(bg.B)/255),
		A: 255,
	}
}

// renderImageRamp draws the same cells with no colour at all, which is the one
// case lipgloss cannot downgrade for us.
func (r Renderer) renderImageRamp(img image.Image, source image.Rectangle, cells, rows, width int, bg lipgloss.Color) string {
	style := lipgloss.NewStyle().Background(bg).Foreground(r.Styles.Workspace.BodyFg)
	left := (width - cells) / 2
	lines := make([]string, 0, rows)
	for cy := 0; cy < rows; cy++ {
		var line strings.Builder
		line.WriteString(strings.Repeat(" ", max(0, left)))
		for cx := 0; cx < cells; cx++ {
			top := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 0), color.RGBA{A: 255})
			bottom := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 1), color.RGBA{A: 255})
			luma := (0.2126*float64(top.R) + 0.7152*float64(top.G) + 0.0722*float64(top.B) +
				0.2126*float64(bottom.R) + 0.7152*float64(bottom.G) + 0.0722*float64(bottom.B)) / 2
			// luma is 0..255, and the ramp is shortest at the dark end, so the
			// index is scaled - clamped, because a 255 lands one past the end.
			glyph := imageRamp[min(len(imageRamp)-1, int(luma)*len(imageRamp)/256)]
			line.WriteString(style.Render(string(glyph)))
		}
		lines = append(lines, line.String())
	}
	return r.RenderLines(lines, width, bg)
}

// hexColour is a lipgloss colour for a sampled pixel.
func hexColour(c color.RGBA) string {
	const digits = "0123456789abcdef"
	return string([]byte{
		'#', digits[c.R>>4], digits[c.R&0xF],
		digits[c.G>>4], digits[c.G&0xF],
		digits[c.B>>4], digits[c.B&0xF],
	})
}

// rgbaOf reads a theme colour back as RGBA, so a panel background can be
// composited onto. hexToRGB is the helper the contrast code already uses
// (color.go:45) and it returns 0..1 channels, or ok false for a named colour.
func rgbaOf(c lipgloss.Color) color.RGBA {
	red, green, blue, ok := hexToRGB(c)
	if !ok {
		return color.RGBA{A: 255}
	}
	return color.RGBA{
		R: uint8(math.Round(clamp01(red) * 255)),
		G: uint8(math.Round(clamp01(green) * 255)),
		B: uint8(math.Round(clamp01(blue) * 255)),
		A: 255,
	}
}
```

`image.go` imports `math` for the rounding above. Nothing else in this file is
new: `colorprofile`, `lipgloss` and `image/color` are already in the module (see
`go.mod`), `hexToRGB` and `clamp01` are already in `color.go` (:45 and :195), and
the `min`/`max` builtins are used elsewhere in this package already.

Run:

```
go test -run TestRenderImage -v .
```

Expected: four `--- PASS` lines and `ok  github.com/allisonhere/tideui`.

**A1c. Commit.**

```
git add image.go image_test.go color.go
git commit -m "Draw an image as coloured half-block cells"
```

---

## Task A2 - A model for a radar frame

Files: `image.go`, `image_test.go`.

**A2a. Write the failing test.** Append to `image_test.go`:

```go
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
```

Run: `go test -run TestRadarFrame .` → `undefined: RadarFrame` — red.

**A2b. Make it pass.** Add to `image.go` (with `"time"` imported):

```go
// RadarFrame is one radar frame: the picture and the moment it describes. It
// lives here rather than in provider because it is a rendering model, the same
// way WeatherData does, and because a panel needs it to draw without knowing
// where it came from.
type RadarFrame struct {
	Time  time.Time
	Image image.Image
}
```

Run: `go test -run TestRadarFrame -v .` → `--- PASS` and `ok`.

**A2c. Commit.**

```
git add image.go image_test.go
git commit -m "Give a radar frame a type: a picture and its time"
```

---

## Task A3 - A kitty transport, behind a capability check

Optional, and after B/C/D if you would rather see the radar sooner. It changes no
interfaces and nothing else in the plan depends on it — which is the point:
half-blocks are the baseline every terminal gets, and this is an upgrade for the
one terminal that has a text-compatible pixel mode. What it buys is not a bigger
panel but the real picture, at the terminal's own pixel resolution, inside exactly
the same cell box.

**Step 0 - prove it end to end, before writing any of it.** The one thing that
could not be verified from a read-only inspection is whether *bubbletea's*
renderer (the writer that puts the finished frame on the screen — `cellbuf` is the
blend, not the writer) preserves a transmit sequence that rides inside a panel's
line. Ten minutes, throwaway:

1. In `dash/panels/weather.go`'s `View`, temporarily prefix the returned string
   with the output of `kitty.EncodeGraphics` for a small solid image (id 1, `U=1`)
   and append one line of `kitty.Placeholder` cells.
2. `go run ./examples/workspace` in kitty, with that panel visible.
3. A red square where the placeholders are means the transport rides the normal
   frame and the rest of this task is mechanical. No square means stop: the
   fallback design is a prefix the app writes before the frame, which is worse,
   and the radar should ship on half-blocks only.
4. Revert: `git checkout dash/panels/weather.go`.

### A3a. The capability check - `tideui/image.go`

```go
// kittyPlaceholders reports whether this terminal can be drawn with kitty's
// Unicode placeholder cells. It has to be kitty itself and not something relaying
// kitty - placeholders are cell-level and a multiplexer knows nothing about them,
// so tmux would show a screenful of tofu - and it has to be somewhere a 24-bit
// foreground colour survives, because that colour is where the image id travels.
func kittyPlaceholders(getenv func(string) string, profile colorprofile.Profile) bool {
	if profile != colorprofile.TrueColor {
		return false
	}
	if getenv("TMUX") != "" {
		return false
	}
	return getenv("TERM") == "xterm-kitty" || getenv("KITTY_WINDOW_ID") != ""
}
```

Test in `image_test.go`:

```go
// The transport is detected, never assumed: tofu in every other terminal would be
// worse than a smaller picture.
func TestKittyPlaceholdersNeedsKittyItself(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(key string) string { return kv[key] }
	}
	cases := []struct {
		name    string
		env     map[string]string
		profile colorprofile.Profile
		want    bool
	}{
		{"kitty", map[string]string{"TERM": "xterm-kitty"}, colorprofile.TrueColor, true},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "1"}, colorprofile.TrueColor, true},
		{"kitty inside tmux", map[string]string{"TERM": "xterm-kitty", "TMUX": "/tmp/tmux-1000/default,1,0"}, colorprofile.TrueColor, false},
		{"another terminal", map[string]string{"TERM": "xterm-256color"}, colorprofile.TrueColor, false},
		{"kitty without truecolor", map[string]string{"TERM": "xterm-kitty"}, colorprofile.ANSI256, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := kittyPlaceholders(env(tc.env), tc.profile); got != tc.want {
				t.Fatalf("kittyPlaceholders = %v, want %v", got, tc.want)
			}
		})
	}
}
```

Run: `go test -run TestKittyPlaceholdersNeedsKittyItself .` → `undefined: kittyPlaceholders`, red, then green.

### A3b. Make `RenderImage` a dispatcher

The half-block loop from A1 moves into its own method, unchanged — that move is
proved by running the A1 tests with **no edits to them**:

```go
// renderImageHalfBlocks is the baseline: two samples of vertical resolution per
// cell, in colour, in any terminal that has colour at all.
func (r Renderer) renderImageHalfBlocks(img image.Image, cells, rows, width int, bg lipgloss.Color) string {
	picture := rgbaOf(bg)
	source := img.Bounds()
	left := strings.Repeat(" ", max(0, (width-cells)/2))
	leftCell := lipgloss.NewStyle().Background(bg).Render(left)
	lines := make([]string, 0, rows)
	for cy := 0; cy < rows; cy++ {
		var line strings.Builder
		line.WriteString(leftCell)
		for cx := 0; cx < cells; cx++ {
			top := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 0), picture)
			bottom := sampleRegion(img, halfCell(source, cells, rows, cx, cy, 1), picture)
			line.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(hexColour(top))).
				Background(lipgloss.Color(hexColour(bottom))).
				Render("▀"))
		}
		lines = append(lines, line.String())
	}
	return r.RenderLines(lines, width, bg)
}
```

And `RenderImage` becomes the choice:

```go
func (r Renderer) RenderImage(img image.Image, width, maxHeight int) string {
	if img == nil || width <= 0 || maxHeight <= 0 {
		return ""
	}
	cells, rows := fitCells(img.Bounds(), width, maxHeight)
	if cells <= 0 || rows <= 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	if kittyPlaceholders(os.Getenv, activeColorProfile()) {
		return r.renderImagePlaceholders(img, cells, rows, width, bg)
	}
	if activeColorProfile() == colorprofile.ASCII {
		return r.renderImageRamp(img, img.Bounds(), cells, rows, width, bg)
	}
	return r.renderImageHalfBlocks(img, cells, rows, width, bg)
}
```

Run the A1 tests unchanged: `go test -run TestRenderImage .` → still green. Commit
("Draw a picture with whichever transport the terminal has").

### A3c. The placeholder grid - `tideui/image.go`

```go
// transmitted is the bookkeeping behind "transmit once": kitty is told about a
// picture the first time a frame draws it, and every frame after that sends only
// the cells that place it. An entry holds the image itself, so its address cannot
// be handed to a different picture while a placement for it may still be on
// screen; the oldest go once no terminal could still be showing them.
var transmitted = struct {
	sync.Mutex
	next    uint32
	byImage map[uintptr]*transmission
	order   []uintptr
}{byImage: map[uintptr]*transmission{}}

type transmission struct {
	id   uint32
	sent bool
	// image is held so its address is not reused by another picture.
	image image.Image
}

// maxTransmissions bounds the registry. A panel redrawing a fresh picture every
// five minutes stays far below it, and everything past it is off screen already.
const maxTransmissions = 32

// transmissionFor is the id a picture is transmitted under, and whether this call
// is the one that has to send it.
func transmissionFor(img image.Image) (id uint32, send bool) {
	if reflect.ValueOf(img).Kind() != reflect.Pointer {
		// A value type has no identity to key on, so it is sent every time rather
		// than sharing an id with a picture it is not.
		transmitted.Lock()
		defer transmitted.Unlock()
		transmitted.next++
		return transmitted.next, true
	}
	key := reflect.ValueOf(img).Pointer()

	transmitted.Lock()
	defer transmitted.Unlock()
	entry, ok := transmitted.byImage[key]
	if !ok {
		transmitted.next++
		entry = &transmission{id: transmitted.next, image: img}
		transmitted.byImage[key] = entry
		transmitted.order = append(transmitted.order, key)
		for len(transmitted.order) > maxTransmissions {
			oldest := transmitted.order[0]
			transmitted.order = transmitted.order[1:]
			delete(transmitted.byImage, oldest)
		}
	}
	if entry.sent {
		return entry.id, false
	}
	entry.sent = true
	return entry.id, true
}

// renderImagePlaceholders transmits the picture once and then draws it as
// placeholder cells: U+10EEEE with the diacritics that name a row and a column
// inside the image, and the image id in the cell's foreground colour. Those cells
// are ordinary characters, so everything the dashboard does to text - panes,
// zoom, padding, the background blend - moves the picture with them.
func (r Renderer) renderImagePlaceholders(img image.Image, cells, rows, width int, bg lipgloss.Color) string {
	id, send := transmissionFor(img)
	var out strings.Builder
	if send {
		options := &kitty.Options{
			Action:           kitty.Transmit,
			Quite:            2, // no OK/error replies on the wire
			ID:               int(id),
			Format:           kitty.PNG,
			Transmission:     kitty.Direct,
			Columns:          cells,
			Rows:             rows,
			VirtualPlacement: true, // placed by the cells below, not by the cursor
		}
		if err := kitty.EncodeGraphics(&out, img, options); err != nil {
			// A picture that cannot be transmitted is drawn the way every other
			// terminal gets it, never left as a hole.
			return r.renderImageHalfBlocks(img, cells, rows, width, bg)
		}
	}

	// The id travels as a 24-bit foreground colour: that is how kitty knows which
	// picture a placeholder cell belongs to.
	cell := lipgloss.NewStyle().Foreground(lipgloss.Color(imageIDColour(id)))
	left := strings.Repeat(" ", max(0, (width-cells)/2))
	leftCell := lipgloss.NewStyle().Background(bg).Render(left)
	lines := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		var line strings.Builder
		line.WriteString(leftCell)
		for col := 0; col < cells; col++ {
			line.WriteString(cell.Render(string(kitty.Placeholder) +
				string(kitty.Diacritic(row)) + string(kitty.Diacritic(col))))
		}
		lines = append(lines, line.String())
	}
	// The transmit goes in front of the cells in the same string. It is an APC
	// sequence, so it is zero cells wide: the terminal reads it and everything
	// that measures or moves text - the blend, the pane padding, the renderer -
	// carries it along with the row it belongs to.
	return out.String() + r.RenderLines(lines, width, bg)
}

// imageIDColour is an image id as the colour a placeholder cell carries.
func imageIDColour(id uint32) string { return fmt.Sprintf("#%06x", id&0xFFFFFF) }
```

`image.go` imports `os`, `reflect`, `sync`, `fmt` and
`github.com/charmbracelet/x/ansi/kitty` (all already in the module).

Tests in `image_test.go` — protocol shape, which is the only part of this a test
can honestly assert:

```go
// A picture is transmitted the first time it is drawn and never again: a panel
// that repaints every second must not put a picture on the wire every second.
func TestKittyTransmitsAPictureOnceAndPlacesItWithCells(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TMUX", "")
	transmitted.Lock()
	transmitted.next, transmitted.byImage, transmitted.order = 0, map[uintptr]*transmission{}, nil
	transmitted.Unlock()

	r := NewRenderer(CatppuccinMocha, StyleOptions{})
	picture := solid(8, 8, color.RGBA{R: 255, A: 255})

	first := r.RenderImage(picture, 4, 4)
	second := r.RenderImage(picture, 4, 4)

	if got := strings.Count(first, "\x1b_G"); got != 1 {
		t.Fatalf("first frame transmitted %d times, want 1", got)
	}
	if got := strings.Count(second, "\x1b_G"); got != 0 {
		t.Fatalf("second frame transmitted %d times, want 0", got)
	}
	if !strings.Contains(first, "U=1") || !strings.Contains(first, "f=100") {
		t.Fatalf("transmit options are wrong: %q", first)
	}
	// Four cells wide and four rows: one placeholder per cell, and every cell
	// carries the image id as its foreground colour.
	lines := strings.Split(ansi.Strip(r.RenderImage(picture, 4, 4)), "\n")
	if len(lines) != 4 {
		t.Fatalf("drew %d rows, want 4", len(lines))
	}
	for _, line := range lines {
		if got := strings.Count(line, string(kitty.Placeholder)); got != 4 {
			t.Fatalf("row %q has %d placeholders, want 4", line, got)
		}
	}
	// A different picture is transmitted again, and under a different id.
	other := r.RenderImage(solid(8, 8, color.RGBA{B: 255, A: 255}), 4, 4)
	if !strings.Contains(other, "\x1b_G") {
		t.Fatalf("a new picture was not transmitted: %q", other)
	}
}
```

Run: `go test -run TestKitty .` → red, then green with the code above. Then the
whole gate: `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...`.

Two details in that test, so nobody "fixes" them later: the transmit is counted as
**one** APC because an 8×8 picture fits in a single chunk (`kitty.MaxChunkSize` is
4 KB of payload, and a chunked transmission emits one `\x1b_G` per chunk — a
256×256 radar tile is four or five). And the third `RenderImage` call is
deliberate: it proves a repeated draw of the same picture stays silent.

Commit: **"Draw a picture with placeholder cells on a terminal that has them"**.

### A3d. Keep the half-block path honest

One more test, because the fallback is the one everybody else runs: with the
capability switched off (no `TERM`, or `TMUX` set), the output must contain no APC
sequence at all and must contain half-blocks.

```go
func TestRenderImageStaysOnHalfBlocksWithoutKitty(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("TERM", "xterm-256color")
	// Clear the kitty marker too: these tests are run from inside a kitty
	// terminal often enough that inheriting it would make this pass for the
	// wrong reason - or fail for one.
	t.Setenv("KITTY_WINDOW_ID", "")

	r := NewRenderer(CatppuccinMocha, StyleOptions{})
	got := r.RenderImage(solid(2, 2, color.RGBA{R: 255, A: 255}), 1, 1)
	if strings.Contains(got, "\x1b_G") {
		t.Fatalf("a terminal without kitty graphics was transmitted an image: %q", got)
	}
	if !strings.Contains(got, "▀") {
		t.Fatalf("no half-block cell was drawn: %q", got)
	}
}
```

---

## Task B1 - An image row in a plugin document

Files: `dash/doc.go`, `dash/image.go` (new), `dash/image_test.go` (new),
`dash/doc_test.go`.

**B1a. Write the failing tests.** Create `dash/image_test.go`:

```go
package dash

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writePNG writes a small solid PNG into a fresh directory and returns its path.
func writePNG(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	return writePNGAt(t, filepath.Join(t.TempDir(), "picture.png"), w, h, c)
}

// writePNGAt writes the same picture to a path that already exists, so a test can
// replace a file under a panel and watch it notice.
func writePNGAt(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

// A path is decoded once and kept while the file is unchanged: a panel that
// re-runs every minute should not decode the same picture every minute.
func TestLoadImageCachesUntilTheFileChanges(t *testing.T) {
	path := writePNG(t, 2, 2, color.RGBA{R: 255, A: 255})

	first, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("the same file was decoded twice")
	}

	// Rewriting it must be picked up: this is how a plugin's next frame reaches
	// the screen without the panel knowing it changed.
	writePNG(t, 2, 2, color.RGBA{B: 255, A: 255})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	changed, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("a rewritten file returned the cached decode")
	}
}

// What cannot be read is reported, never guessed at.
func TestLoadImageRefusesWhatItCannotRead(t *testing.T) {
	if _, err := loadImage(filepath.Join(t.TempDir(), "absent.png")); err == nil {
		t.Fatal("a missing file loaded")
	}
	path := filepath.Join(t.TempDir(), "not-an-image.png")
	if err := os.WriteFile(path, []byte("this is not a picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImage(path); err == nil {
		t.Fatal("a file that is not an image loaded")
	}
	// A relative path is refused: a document is data, and data is not resolved
	// against anything.
	if _, err := loadImage("picture.png"); err == nil {
		t.Fatal("a relative path loaded")
	}
}
```

Append to `dash/doc_test.go`:

```go
// A document can point at a picture. The row is drawn from a path, and a path
// that cannot be read draws its alt text rather than a hole - the same promise
// every other row keeps.
func TestRenderDocDrawsAnImageRow(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	renderer := docRenderer()
	path := writePNG(t, 2, 2, color.RGBA{R: 255, A: 255})

	doc := Doc{Rows: []Row{
		{Type: "text", Label: "radar", Value: "18:05"},
		{Type: "image", Src: path, Rows: 2, Alt: "no picture"},
	}}
	out := RenderDoc(renderer, doc, 8, false)
	cell := lipgloss.NewStyle().Foreground("#ff0000").Background("#ff0000").Render("▀")
	if !strings.Contains(out, cell) {
		t.Errorf("the picture was not drawn:\n%q", out)
	}
	// Bounded like every other row: nothing may exceed the pane.
	for _, line := range strings.Split(ansi.Strip(out), "\n") {
		if got := ansi.StringWidth(line); got != 8 {
			t.Errorf("line width = %d, want 8: %q", got, line)
		}
	}

	// A path that is not there draws the alt text, and does not take the rest of
	// the document with it.
	broken := Doc{Rows: []Row{
		{Type: "image", Src: filepath.Join(t.TempDir(), "gone.png"), Alt: "no picture"},
		{Type: "text", Label: "still", Value: "here"},
	}}
	plain := ansi.Strip(RenderDoc(renderer, broken, 24, false))
	if !strings.Contains(plain, "no picture") {
		t.Errorf("a missing picture drew %q, want its alt text", plain)
	}
	if !strings.Contains(plain, "still") {
		t.Errorf("a missing picture lost the row after it: %q", plain)
	}
}
```

`dash/doc_test.go` needs `path/filepath` in its imports.

Run:

```
go test -run 'TestLoadImage|TestRenderDocDrawsAnImageRow' ./dash/
```

Expected: `undefined: loadImage`, `unknown field Src in struct literal of type Row`
— red.

**B1b. Make it pass.** Create `dash/image.go`:

```go
package dash

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"time"

	// The decoders a plugin is likely to hand over, registered so image.Decode
	// can sniff the format from the file itself rather than trusting a suffix.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// maxImagePixels bounds what a document may make the dashboard decode. A plugin
// that points at a 20000×20000 PNG is a mistake or a weapon, and either way the
// panel should say so rather than allocate it.
const maxImagePixels = 4096 * 4096

// imageEntry is one decoded picture, keyed by the file it came from and the state
// it was in when it was read.
type imageEntry struct {
	modTime time.Time
	size    int64
	image   image.Image
	err     error
}

// images is the decode cache, one per process: the dashboard holds one document
// per panel, so a cache per panel would only decode the same file once per panel.
var images = struct {
	mu      sync.Mutex
	entries map[string]imageEntry
}{entries: map[string]imageEntry{}}

// loadImage reads the picture a document row points at. The path must be
// absolute: a document is data, and data is not resolved against the working
// directory of whatever printed it. The stat is taken every call, so a plugin
// that writes the file a moment after printing its document is picked up on the
// next frame instead of being cached as missing forever.
func loadImage(path string) (image.Image, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("image row: %q is not an absolute path", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	images.mu.Lock()
	entry, seen := images.entries[path]
	images.mu.Unlock()
	if seen && entry.modTime.Equal(info.ModTime()) && entry.size == info.Size() {
		return entry.image, entry.err
	}

	img, err := decodeImage(path)
	images.mu.Lock()
	images.entries[path] = imageEntry{modTime: info.ModTime(), size: info.Size(), image: img, err: err}
	images.mu.Unlock()
	return img, err
}

// decodeImage sniffs the format, refuses an absurd size before allocating it, and
// hands back whatever the file actually is.
func decodeImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if config.Width*config.Height > maxImagePixels {
		return nil, fmt.Errorf("image row: %s is %dx%d", path, config.Width, config.Height)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(file)
	return img, err
}
```

`dash/image.go` also needs `io` for `Seek`.

Then the row in `dash/doc.go`:

1. `Row` gains the fields, after `ID`:

```go
	// Src is an absolute path to a picture this row draws. A document carries a
	// reference, never pixels: a document is capped at 1 MB, and a reference is
	// what a plugin that produced the file already has.
	Src string `json:"src"`
	// Rows caps how many cells tall an image row may be drawn (default 24). A
	// document does not know how tall its pane is, so a picture bounds itself.
	Rows int `json:"rows"`
	// Alt is drawn when the picture cannot be read. A row is a promise that
	// something appears, and "something went wrong" is something.
	Alt string `json:"alt"`
```

2. `renderRow`'s switch gains (before `default:`):

```go
	case "image":
		return renderImageRow(renderer, row, width)
```

3. And the renderer, beside `renderRowSelected`:

```go
// renderImageRow draws the picture a row points at, or its alt text. Sized from
// the source's aspect unless the row caps it, because only the row knows roughly
// how much room it is worth.
func renderImageRow(renderer tideui.Renderer, row Row, width int) []string {
	alt := strings.TrimSpace(row.Alt)
	if alt == "" {
		alt = "image unavailable"
	}
	maxRows := row.Rows
	if maxRows <= 0 {
		maxRows = 24
	}
	img, err := loadImage(strings.TrimSpace(row.Src))
	if err != nil {
		return []string{docPair(renderer, row.Label, alt, docLabelWidth([]Row{row}, width), tideui.ToneWarning, renderer.Styles.Workspace.Bg)}
	}
	drawn := renderer.RenderImage(img, width, maxRows)
	if strings.TrimSpace(drawn) == "" {
		return []string{docPair(renderer, row.Label, alt, docLabelWidth([]Row{row}, width), tideui.ToneWarning, renderer.Styles.Workspace.Bg)}
	}
	return strings.Split(drawn, "\n")
}
```

`docPair`'s signature in this file is
`docPair(renderer, label, value string, labelWidth int, tone tideui.Tone, bg lipgloss.Color)`
— check it and pass what it wants.

Run:

```
go build ./... && go vet ./... && go test -run 'TestLoadImage|TestRenderDoc|TestExecPanel' ./dash/
```

Expected: `ok  github.com/allisonhere/tideui/dash`.

**B1c. Commit.**

```
git add dash/doc.go dash/image.go dash/image_test.go dash/doc_test.go
git commit -m "Let a document row point at a picture"
```

---

## Task B2 - A plugin's picture reaches the panel

Files: `dash/exec_test.go`.

**B2a. Write the failing test.** Append to `dash/exec_test.go`:

```go
// A plugin that produces a picture points its document at the file, and the panel
// draws it. The path travels as a declared setting, the way every value a plugin
// needs does - see TestExecPanelPassesDeclaredSettings in this file for the shape.
func TestExecPanelDrawsAnImageRowFromAPlugin(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	frame := filepath.Join(t.TempDir(), "frame.png")
	writePNGAt(t, frame, 2, 2, color.RGBA{R: 255, A: 255})

	manifest := plugin(t,
		`printf '{"rows":[{"type":"image","src":"%s","rows":2}]}\n' "$TIDEDECK_PLUGIN_FRAME"`,
		map[string]any{
			"panel": map[string]any{
				"schema": []map[string]any{
					{"key": "frame", "type": "string", "label": "Frame"},
				},
			},
		})

	panel := Exec(manifest)
	values := NewValues()
	values.Set("plugins.test.plugin.frame", frame)
	if err := panel.(Configurable).Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	out := panel.View(tideui.PanelContext{ID: "test.plugin", Width: 8, Renderer: docRenderer()})
	if !strings.Contains(out, lipgloss.NewStyle().Foreground("#ff0000").Background("#ff0000").Render("▀")) {
		t.Fatalf("the plugin's picture was not drawn:\n%q", out)
	}

	// And the next frame arrives without anyone being told: the loader keys on
	// the file's own state.
	writePNGAt(t, frame, 2, 2, color.RGBA{B: 255, A: 255})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(frame, future, future); err != nil {
		t.Fatal(err)
	}
	out = panel.View(tideui.PanelContext{ID: "test.plugin", Width: 8, Renderer: docRenderer()})
	if !strings.Contains(out, lipgloss.NewStyle().Foreground("#0000ff").Background("#0000ff").Render("▀")) {
		t.Fatalf("a rewritten picture was not picked up:\n%q", out)
	}
}
```

`writePNGAt(t, path, w, h, colour)` is a helper in `dash/image_test.go`; make
`writePNG` call it so there is one PNG writer in the package:

```go
// writePNG writes a small solid PNG into a fresh directory and returns its path;
// writePNGAt writes to a path that already exists.
func writePNG(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	return writePNGAt(t, filepath.Join(t.TempDir(), "picture.png"), w, h, c)
}

func writePNGAt(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
```

(In B1a, define `writePNGAt` this way and `writePNG` as the wrapper; do not write
the encoder body twice.)

`exec_test.go` needs `os` and `time` — check its import block; it already has
`context`, `time` and (from the plugin fixture helpers) `os`.

Run:

```
go test -run TestExecPanelDrawsAnImageRow ./dash/
```

Expected: `--- PASS` with no production change beyond Task B1 — that is the point
of this test: it proves the *plugin* path, not a new code path.

**B2b. Commit.**

```
git add dash/exec_test.go dash/image_test.go
git commit -m "Prove a plugin's picture reaches the panel"
```

---

## Task B3 - Document the row

Files: `README.md`.

In the plugin section, after the `panel.open` paragraph:

````
A plugin can also **point at a picture**, and the panel draws it: `src` is an
absolute path and `alt` is what to draw when it cannot be read, so a plugin that
writes a chart, a heat map or a radar frame gets it on screen without the
dashboard knowing what produced it:

```json
{ "type": "image", "src": "/run/user/1000/radar.png", "rows": 20, "alt": "radar unavailable" }
```

It is drawn as coloured half-blocks — one sample across and two down per cell —
so it works over ssh and inside tmux, needs no graphics protocol, and degrades
through the theme's colour profile: a 256-colour terminal gets a quantised
picture and a colourless one gets a brightness ramp. `rows` caps the height
(default 24); the picture keeps its aspect and is centred rather than stretched,
and transparent samples show the panel background through them.
````

Verify the docs bench (`dash/readme_test.go` renders documents from the README —
check it still passes):

```
go test -run TestReadme ./dash/
```

**Commit.**

```
git add README.md
git commit -m "Document the image row"
```

---

## Task C1 - Fetch a radar tile

Files: `provider/radar.go` (new), `provider/radar_test.go` (new).

**C1a. Write the failing tests.** Create `provider/radar_test.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"image"
	"image/color"
)

// radarServer is RainViewer's shape, small enough to serve in a test: an index
// with two frames, and a tile for whichever frame is asked for.
func radarServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/public/weather-maps.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "tideui") {
			t.Errorf("index request has no tideui user agent, got %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"version": "2.0",
			"host":    server.URL,
			"radar": map[string]any{"past": []map[string]any{
				{"time": 1789767000, "path": "/v2/radar/older"},
				{"time": 1789767600, "path": "/v2/radar/newest"},
			}},
		})
	})
	mux.HandleFunc("/v2/radar/newest/256/8/58/105/256/4/1_1.png", func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// The tile that contains a coordinate, which is the arithmetic the whole panel
// rests on. The second case was checked against the live service: zoom 8 over
// Austin is tile 58/105, and that tile comes back as a PNG.
func TestRadarTileForCoordinates(t *testing.T) {
	for _, c := range []struct {
		lat, lon float64
		zoom     int
		x, y     int
	}{
		{0, 0, 1, 1, 1},
		{30.2672, -97.7431, 8, 58, 105},
		{51.5074, -0.1278, 8, 127, 85},
	} {
		x, y := radarTileFor(c.lat, c.lon, c.zoom)
		if x != c.x || y != c.y {
			t.Errorf("radarTileFor(%v, %v, %d) = %d/%d, want %d/%d", c.lat, c.lon, c.zoom, x, y, c.x, c.y)
		}
	}
}

// The newest frame is the one a live panel wants, and the tile it fetches is
// described by the index rather than guessed at.
func TestRadarFetchesTheNewestFrame(t *testing.T) {
	server := radarServer(t)
	restore := radarIndexURL
	radarIndexURL = server.URL + "/public/weather-maps.json"
	t.Cleanup(func() { radarIndexURL = restore })

	fetch := Radar(RadarOptions{Latitude: 30.2672, Longitude: -97.7431, Zoom: 8})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Unix(1789767600, 0); !frame.Time.Equal(want) {
		t.Errorf("frame time = %v, want %v", frame.Time, want)
	}
	if frame.Image == nil || frame.Image.Bounds().Dx() != 8 {
		t.Fatalf("frame image = %v, want the 8×8 tile", frame.Image)
	}
}

// A service that is down, or answers with something that is not a picture, is an
// error the panel can show rather than a blank panel.
func TestRadarReportsWhatWentWrong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()
	restore := radarIndexURL
	radarIndexURL = server.URL
	t.Cleanup(func() { radarIndexURL = restore })

	if _, err := Radar(RadarOptions{Latitude: 1, Longitude: 2, Zoom: 8})(context.Background()); err == nil {
		t.Fatal("an index that failed was not an error")
	}
}
```

Run:

```
cd /home/allie/Projects/tidedeck && go test -run TestRadar ./provider/
```

Expected: `undefined: Radar`, `undefined: radarIndexURL`, `undefined: radarTileFor`
— red.

**C1b. Make it pass.** Create `provider/radar.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"net/http"
	"time"

	"github.com/allisonhere/tideui"
)

// radarIndexURL is RainViewer's public index: no key, no account, twelve past
// frames and a host to fetch them from. It is a var so a test can point it at its
// own server, the same way openMeteoBase is.
var radarIndexURL = "https://api.rainviewer.com/public/weather-maps.json"

// radarFallbackHost is used when an index does not name one.
const radarFallbackHost = "https://tilecache.rainviewer.com"

// radarUserAgent names this dashboard. A free public service deserves to know who
// is calling it, and a user agent is the only courtesy it gets.
const radarUserAgent = "tideui-radar/1 (+https://github.com/allisonhere/tideui)"

// RadarOptions is where and how closely to look.
type RadarOptions struct {
	Latitude  float64
	Longitude float64
	// Zoom is the slippy-map zoom: 8 covers roughly 150 km, and each step up
	// halves that. RainViewer serves 0-10 at 256 px.
	Zoom int
}

// radarIndex is what the index endpoint answers with.
type radarIndex struct {
	Host  string `json:"host"`
	Radar struct {
		Past []struct {
			Time int64  `json:"time"`
			Path string `json:"path"`
		} `json:"past"`
	} `json:"radar"`
}

// Radar returns a source that fetches the newest frame covering the coordinate.
// One tile, one frame: a panel is 40 cells wide, and a mosaic of nine tiles
// downsampled into it is nine times the traffic for no more picture.
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
		x, y := radarTileFor(opts.Latitude, opts.Longitude, zoom)
		// 256 px tiles, colormap 4 (the familiar green-to-red radar), smooth 1,
		// snow 1: the arguments in the path are the service's own.
		url := fmt.Sprintf("%s%s/256/%d/%d/%d/256/4/1_1.png", host, newest.Path, zoom, x, y)
		img, err := fetchRadarTile(ctx, url)
		if err != nil {
			return tideui.RadarFrame{}, err
		}
		return tideui.RadarFrame{Time: time.Unix(newest.Time, 0), Image: img}, nil
	}
}

// fetchRadarIndex reads the frame list.
func fetchRadarIndex(ctx context.Context) (radarIndex, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, radarIndexURL, nil)
	if err != nil {
		return radarIndex{}, err
	}
	request.Header.Set("User-Agent", radarUserAgent)
	response, err := httpClient.Do(request)
	if err != nil {
		return radarIndex{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return radarIndex{}, fmt.Errorf("radar index: %s", response.Status)
	}
	var index radarIndex
	if err := json.NewDecoder(response.Body).Decode(&index); err != nil {
		return radarIndex{}, fmt.Errorf("radar index: %w", err)
	}
	return index, nil
}

// fetchRadarTile reads and decodes one PNG tile.
func fetchRadarTile(ctx context.Context, url string) (image.Image, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", radarUserAgent)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radar tile: %s", response.Status)
	}
	img, err := png.Decode(response.Body)
	if err != nil {
		return nil, fmt.Errorf("radar tile: %w", err)
	}
	return img, nil
}

// radarTileFor is the slippy-map tile containing a coordinate, which is the whole
// of the geography this panel needs: lon maps linearly onto the world, lat does
// not, and nobody has ever enjoyed that fact.
func radarTileFor(lat, lon float64, zoom int) (int, int) {
	scale := math.Exp2(float64(clampZoom(zoom)))
	x := (lon + 180) / 360 * scale
	radians := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(radians)+1/math.Cos(radians))/math.Pi) / 2 * scale
	return clampTile(int(math.Floor(x)), int(scale)), clampTile(int(math.Floor(y)), int(scale))
}

// clampTile keeps a coordinate inside the world, so a pole or a meridian does not
// ask for a tile that cannot exist.
func clampTile(v, n int) int { return min(max(v, 0), n-1) }

// clampZoom keeps a zoom inside what the service serves.
func clampZoom(zoom int) int { return min(max(zoom, 0), 10) }
```

`clampTile` uses the generic `min`/`max` builtins (Go 1.21+; this module is
`go 1.26.1`), and `math.Exp2` avoids an int shift that would overflow at zoom 10.

Run:

```
go build ./... && go vet ./... && go test -run TestRadar -v ./provider/
```

Expected: three `--- PASS` lines and `ok  github.com/allisonhere/tideui/provider`.

**C1c. Commit.**

```
git add provider/radar.go provider/radar_test.go
git commit -m "Fetch the radar tile covering a coordinate"
```

---

## Task D1 - The radar panel

Files: `dash/panels/radar.go` (new), `dash/panels/radar_test.go` (new).

**D1a. Write the failing tests.** Create `dash/panels/radar_test.go`:

```go
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
)

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
	values.Set(radarZoomKey, float64(8))
	return values
}

// A panel with no coordinates has nowhere to look, and says so rather than
// drawing an empty box: the same promise the weather panel keeps.
func TestRadarSaysWhereToSetALocation(t *testing.T) {
	panel := Radar().(*radar)
	panel.Configure(dash.NewValues())
	view := ansi.Strip(panel.View(tideui.PanelContext{Width: 30, Height: 10}))
	if !strings.Contains(strings.ToLower(view), "weather") {
		t.Fatalf("the empty radar panel = %q, want it to name the Weather panel", view)
	}
}

// With coordinates it draws the frame it fetched, and says when that frame was -
// a radar picture with no time on it is a picture of a rumour.
func TestRadarDrawsTheFrameAndItsTime(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
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
	view := panel.View(tideui.PanelContext{Width: 30, Height: 12, Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})})
	if !strings.Contains(view, lipgloss.NewStyle().Foreground("#ff0000").Background("#ff0000").Render("▀")) {
		t.Fatalf("the frame was not drawn:\n%q", view)
	}
	if !strings.Contains(ansi.Strip(view), "18:05") {
		t.Fatalf("the panel does not say when the frame is from:\n%q", ansi.Strip(view))
	}
	if !strings.Contains(ansi.Strip(view), "RainViewer") {
		t.Fatalf("the panel does not credit its source:\n%q", ansi.Strip(view))
	}
}
```

Run: `go test -run TestRadar ./dash/panels/` → `undefined: Radar` — red.

**D1b. Make it pass.** Create `dash/panels/radar.go`:

```go
package panels

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// Radar builds the radar panel. Like the weather panel it has nowhere to look
// until it is told where - and where is the weather panel's own location, so the
// place search fills both panels at once.
func Radar() dash.Panel {
	return &radar{newFetcher: provider.Radar}
}

type radar struct {
	dash.State[tideui.RadarFrame]

	mu         sync.Mutex
	fetch      func(context.Context) (tideui.RadarFrame, error)
	newFetcher func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error)
	location   string
	zoom       int
}

const (
	radarEnabledKey = "radar.enabled"
	radarZoomKey    = "radar.zoom"
)

func (r *radar) Meta() dash.Meta {
	return dash.Meta{
		ID: "radar", Title: "Radar", Subtitle: "rain",
		Role: tideui.RoleSecondary, Priority: 72,
		MinWidth: 24, MinHeight: 8,
		Interval: 5 * time.Minute,
	}
}

// Schema declares only what is the radar's own. The coordinates are the weather
// panel's: one location, not two copies that drift apart.
func (r *radar) Schema() []dash.Field {
	return []dash.Field{
		{Key: radarEnabledKey, Label: "live radar", Kind: dash.FieldBool, Default: "true",
			Description: "Uses the Weather panel's location."},
		{Key: radarZoomKey, Label: "detail", Kind: dash.FieldFloat, Default: "8",
			Description: "Zoom 5 is a region, 10 is a few blocks.", Min: 5, Max: 10, Step: 1},
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
		r.zoom = 8
	}
	if !boolOr(values, radarEnabledKey, true) || (latitude == 0 && longitude == 0) {
		r.fetch = nil
		return nil
	}
	r.fetch = r.newFetcher(provider.RadarOptions{Latitude: latitude, Longitude: longitude, Zoom: r.zoom})
	return nil
}

func (r *radar) Refresh(ctx context.Context) error {
	r.mu.Lock()
	fetch := r.fetch
	r.mu.Unlock()
	if fetch == nil {
		return nil
	}
	frame, err := fetch(ctx)
	if err != nil {
		return err
	}
	r.Store(frame)
	return nil
}

func (r *radar) View(ctx tideui.PanelContext) string {
	frame := r.Load()
	if frame.Image == nil {
		return r.emptyView(ctx)
	}
	r.mu.Lock()
	location, zoom := r.location, r.zoom
	r.mu.Unlock()
	if location == "" {
		location = "local"
	}

	bg := ctx.Renderer.Styles.Workspace.Bg
	height := max(1, ctx.Height-2)
	picture := ctx.Renderer.RenderImage(frame.Image, ctx.Width, height)
	if strings.TrimSpace(picture) == "" {
		return r.emptyView(ctx)
	}
	// A picture of the weather with no time on it is a picture of a rumour, and
	// the service is credited because it asks to be.
	caption := fmt.Sprintf("%s · %s · zoom %d · RainViewer",
		frame.Time.Local().Format("15:04"), location, zoom)
	lines := append([]string{caption}, strings.Split(picture, "\n")...)
	return ctx.Renderer.RenderLines(lines, ctx.Width, bg)
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
```

Notes for the implementer, all of them small:

- `boolOr` already exists in `dash/panels/weather.go:100` — use it, do not add a
  second one.
- `dash.State[T].Load()` returns `T` (the zero value when nothing has been stored),
  so `frame.Image == nil` is the "nothing yet" test.
- The radar and the weather panel are in the same package, so
  `weatherLatitudeKey`, `weatherLongitudeKey` and `weatherLocationKey` are simply
  in scope — that is the point of reading one location rather than declaring two.

Run:

```
go build ./... && go vet ./... && gofmt -l . ; go test -run TestRadar -v ./dash/panels/
```

Expected: two `--- PASS` lines and `ok  github.com/allisonhere/tideui/dash/panels`.

**D1c. Commit.**

```
git add dash/panels/radar.go dash/panels/radar_test.go
git commit -m "Add a radar panel that draws the frame it fetched"
```

---

## Task D2 - Put it on the dashboard

Files: `examples/workspace/main.go`, `examples/workspace/main_test.go`,
`README.md`.

1. `main.go:171` — add it to the registration line:

```go
	deck.Register(panels.Agenda(), panels.System(), panels.Weather(), panels.Radar(), panels.GPU(), …)
```

2. Each of the five presets gains `"radar"` in its hidden list (`main.go:373-393`),
   because a panel that appears on its own rearranges a dashboard nobody asked to
   rearrange — the same reason a plugin starts hidden:

```go
	ws.AddPreset("Overview", overviewLayout(),
		"notes", "git", "markets", "updates", "radar")
```

   …and the same for `System`, `Productivity`, `Developer` and `Minimal`.

3. `main_test.go:29` — the expectation map gains `"radar": false` in each of the
   five preset entries, so the test states the intent rather than merely passing.

Run:

```
go build ./... && go test -run 'TestNewPanelsArePlacedOrHidden|TestRadar' ./examples/workspace/
```

Expected: `ok  github.com/allisonhere/tideui/examples/workspace`. If it fails with
"visible-but-unplaced", a preset is missing from the hidden list.

4. `README.md` — the panel registry table gains a row beside Weather:

```
| Radar | `Renderer.RenderImage` (the frame), composed with a caption line | — |
```

**Commit.**

```
git add examples/workspace/main.go examples/workspace/main_test.go README.md
git commit -m "Put the radar panel on the dashboard, off by default"
```

---

## Tests and validation

After every task, and again at the end:

```
cd /home/allie/Projects/tidedeck && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...
```

Expected: no `gofmt` output, `ok` for `tideui`, `dash`, `dash/panels`,
`examples/workspace`, `form`, `provider`.

Then the CI simulation, because the plugin tests read a fixture mailbox and the
radar tests must not need the network:

```
T=$(mktemp -d) && env HOME=$T XDG_DATA_HOME=$T/data XDG_CONFIG_HOME=$T/cfg GOCACHE=$HOME/.cache/go-build GOMODCACHE=$HOME/go/pkg/mod GOPATH=$HOME/go go test -count=1 ./... ; rm -rf "$T"
```

Expected: the same `ok` lines. Nothing may be skipped for a missing network, and
no test may reach `api.rainviewer.com`.

By hand, and these are the ones that matter:

1. The source, without the dashboard (this is the exact URL the provider builds,
   for the coordinates the weather panel would hold):

```
curl -s "https://api.rainviewer.com/public/weather-maps.json" | jq -r '.host + .radar.past[-1].path'
```

   then fetch `…/256/7/29/52/4/1_1.png` and look at it in an image viewer —
   a 256×256 PNG with transparent sky and coloured precipitation is the picture
   the panel will draw. Note the shape: `{path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png`,
   with no second size segment and zoom no deeper than 7 (see the notes below).

2. Build and run the dashboard (`go run ./examples/workspace`), open settings
   (`s`), search a place on the Weather page, apply (`ctrl+s`), then enable
   **Radar** in the panel picker (`w`). Expected: a panel with a caption line
   (`17:10 · Austin · zoom 7 · RainViewer`) over a picture with the sky showing
   the panel background and rain in colour.

3. Resize the dashboard (`shift+arrows`, or just a narrower terminal): the picture
   keeps its aspect and stays centred, and never exceeds the pane width.

4. Run it with a colourless terminal (`TERM=vt52` or a theme that forces ASCII):
   the picture becomes a brightness ramp rather than disappearing.

5. A plugin picture, by hand: point `contrib/mail`'s sibling at a PNG? Simpler —
   write a throwaway plugin directory whose script prints
   `{"rows":[{"type":"image","src":"/path/to/a.png","rows":10}]}`, drop it in
   `~/.config/tidedeck/plugins/`, enable it, and confirm the picture appears and a
   missing file draws the alt text.

## Notes from implementing tasks A-B (already in the branch)

Five things the first cut of this plan got wrong, all found by running the tests
rather than by reading them again. They are fixed in the code; the reasons are
here so nobody re-introduces them:

- **The transmit has to be emitted.** `renderImagePlaceholders` built the APC in a
  builder and then returned only the cells, so a kitty terminal got placeholders
  naming an image it had never been sent. Caught by the transmit-count assertion
  (`strings.Count(first, "\x1b_G") != 1`), which is exactly why that assertion is
  written as a count and not as a `Contains`.
- **`a=t` is not in the output.** `kitty.Options.Options()` omits the action when
  it is the zero value, and `Transmit` is the zero value, so the sequence reads
  `\x1b_Gf=100,q=2,i=1,U=1,c=4,r=4;…`. Assert `U=1` and `f=100`; asserting `a=t`
  fails while the code is completely correct.
- **The colour profile is one global for the whole test binary, and `go test`
  detects `Ascii`.** A test that does `defer lipgloss.SetColorProfile(termenv.TrueColor)`
  leaves the package in TrueColor and breaks `layout_test.go`'s
  `TestStatusBarRightStaysThemedWhenLeftCarriesEmbeddedStyling`, which runs later
  in the same binary. Save the previous profile and restore it in `t.Cleanup`
  (`withProfile` in `image_test.go`).
- **`fitCells` keeps the aspect, so the arithmetic has to be done in samples.** A
  16×64 source in a 40×24 box is **12** cells wide and 24 rows (24 rows is 48
  samples, and a 1:4 picture is 12 samples wide), centred with 14 spaces of
  padding - not 24 cells and 8 spaces.
- **`writePNG` returns a path**, so it cannot `return writePNGAt(...)` when
  `writePNGAt` returns nothing: build the path, call the writer, return the path.

### Notes from implementing tasks C-D

- **The tile URL I "verified" was malformed, and a 200 proved nothing.** The
  correct shape is `{path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png`. An
  extra `/256/` segment is *accepted* and silently shifts what the numbers mean,
  so the fetch succeeds and returns a tile of somewhere else. And the documented
  deepest zoom is 7: at 8 and 10 the service returns a **byte-identical canned
  tile for different coordinates** (checked: `2cc6649e`, 1370 bytes for both), so
  a panel asking for more would draw a convincing picture of nothing. Fixed in
  `bdd46d7`; `RadarMaxZoom` is now the single place that knows, `clampZoom` uses
  it, and the panel's schema takes its `Max` and default from it. The visible
  lesson is broader than the bug: when a service answers 200 to anything, a
  successful fetch is not evidence - fetching the *same* thing with different
  parameters and comparing is.
- **The panel has to be `AlwaysLive`.** The dashboard's demo mode is the default
  (`config.Live` is false until the user turns it on), and in demo mode the deck
  skips every panel that is not `AlwaysLive` - so a radar panel that behaved like
  the weather panel would sit on "Loading…" forever in the default configuration.
  A sample storm would be a fabrication, so the panel fetches whatever the mode
  (`7566c88`).
- **The config document is nested, not flat.** `dash.Values.lookup` splits a key
  on dots and walks *maps*, so `{"weather.latitude": 30.26}` is invisible while
  `{"weather": {"latitude": 30.26}}` works. Worth knowing before hand-testing:
  a flat key looks exactly like a working config and produces "No location set".

- **The gate named kitty and the user runs Ghostty.** Ghostty implements the same
  placeholder feature (its `graphics_unicode.zig` has the placeholder codepoint, the
  row/column diacritics and the id-in-the-colour trick), but it identifies itself as
  `TERM_PROGRAM=ghostty` / `TERM=xterm-ghostty` / no `KITTY_WINDOW_ID` - so the panel
  fell to the 40×40 half-block path in the terminal the user actually runs. That is
  what "we need higher rez" meant. `placeholderCells` now accepts both families, and
  every test that asserts the half-block path clears the terminal variables first,
  because otherwise the suite's result depends on the terminal that ran it (`b4127e0`).
- **Ask for the biggest tile the service serves.** Size 256 and 512 are both offered;
  a placeholder-capable terminal draws the tile at the panel's own pixel size, so 512
  is four times the pixels for about a kilobyte more per frame (`e5b4205`).
- **A blank radar panel and a broken one look identical, and the panel is the only
  thing that can tell them apart.** The first real run looked wrong - caption, then
  nothing - and it was right: the frame over Austin had 0.15% of its pixels with any
  echo, while the same service returned 5.8% and 31.9% elsewhere. The panel now
  measures the frame (`radarEcho`, sampled every 4th pixel) and adds "no precipitation
  in range" under the caption when it is under `radarQuietEcho`; the picture is still
  drawn either way, so weather is never hidden behind a sentence (`475e729`). The
  caption also stopped being conditional on the picture having pixels, because "nothing
  is falling" and "nothing has loaded" must not render the same.

- **`a=t,U=1` is not enough: send `a=T,U=1`.** kitty creates the virtual placement for a
  bare transmit; Ghostty 1.3.1 does not - its `graphics_exec.zig` routes both actions to the
  same `transmit()`, and the placement logic that honours `virtual_placement` lives in
  `display()`, which only runs for transmit-and-display. So the panel drew a caption over an
  empty rectangle in the terminal the user actually runs, with every byte-level test green.
  `a=T,U=1` is what kitty's own tools send and both terminals place it (`6f980f3`). The
  general lesson, in the skill: when a compatible terminal draws nothing, read *its* source
  for the command form it implements.

### End-to-end check that was actually run

Real binary, real RainViewer, real terminal, driven through a pty (`w` picker,
`j j j`, space to show the panel):

| `TERM` | APC transmits | placeholder cells | half-blocks | caption |
|---|---|---|---|---|
| `xterm-kitty` + `KITTY_WINDOW_ID=1` | 1 | 40 | 0 | `17:10 · Austin · zoom 7 · RainViewer` |
| `xterm-256color` | 0 | 0 | 32 | `… · RainViewer` |

and the transmitted payload decoded to a real **256×256 PNG** with the parameters
`f=100,q=2,i=1,U=1,c=8,r=4` - the terminal was handed the radar tile itself, not a
picture of one.

## Risks, tradeoffs, and open questions

- **Resolution is the honest ceiling — on everything except kitty.** One sample
  across and two down per cell: at 40 cells a picture is 40×40 samples. A radar
  blob, a heat map, a chart, a QR code — all fine. A photograph is a smear, and the
  plan says so rather than discovering it in the panel. With Task A3 the terminal
  draws the picture itself at its own pixel resolution, so on kitty the same panel
  is a real radar map; everywhere else it is 40×40 samples of one.
- **Kitty's placeholder mode has two live risks, both checked before building.**
  The transmit sequence has to survive the *frame writer*, not just the blend —
  hence Step 0 in A3, which is throwaway and comes before any code. And a
  placeholder in a terminal that does not understand it is a screenful of tofu,
  which is why the transport is detected (`TERM`/`KITTY_WINDOW_ID` minus `TMUX`)
  and unit-tested rather than hoped for. `sixel` and iTerm2 have no equivalent
  cheap mode: they position by cursor, so they stay out (see Not doing).
- **Cell aspect is assumed 2:1.** True of every terminal I know, and false of
  some fonts; the fix, if it is ever needed, is a `StyleOptions` field, not a
  rewrite — `fitCells` is the only place the ratio appears.
- **The decode cache is package-level in `dash`** (`images`), because the
  alternative is threading a loader through `RenderDoc`'s signature, which was
  just changed once. One dashboard is one process; the comment says so. If a
  second process ever renders documents, this becomes a field.
- **A plugin can point at any file the user can read.** That is not new — the
  plugin already runs unsandboxed with the user's permissions — but it is worth
  stating: the row is a read of an absolute path, and what a plugin points at is
  the plugin's business.
- **RainViewer is a free service run by someone else.** Hence: one tile per five
  minutes per panel, a user agent that names the dashboard, and credit in the
  caption. Do not add animation (twelve frames every five minutes) without
  checking their terms first — which is the next bullet.
- **Animation is deliberately not built.** It needs a frame clock the panel
  contract does not have: a plugin re-runs on an interval, and a built-in panel
  re-renders on the dashboard's tick but has no idea a picture is one of twelve.
  The shape it would take: `tideui.RadarFrame` becomes a list with a playhead,
  the panel implements `dash.Ticker` to advance it once a second, and it caches
  the frames it fetched in the same call it already makes (RainViewer's index
  lists all twelve paths). That is a follow-up with its own plan, not a smuggled
  extra here.
- **Open question — radar as Weather's detail view.** `RenderWeatherDetail`
  exists, and "zoom Weather and see the radar" is a nicer drill-down than a second
  panel in a preset. I planned a separate panel because the radar wants its own
  interval, size and settings; say the word and it becomes Weather's detail
  instead, and the panel/registration tasks disappear.
- **Open question — a base map.** Blobs on a background are readable at zoom 7–9
  ("rain to the north-west") but they do not tell you *where* the edge of the rain
  is relative to a town. A coastline overlay means a second source and its terms;
  not now.
- **Open question — plugins with `curl`.** The image row makes a plugin able to
  draw a picture, but a *shell* plugin has no PNG decoder — it can fetch one with
  `curl` and point at it, which is what the mail plugin does with `sqlite3` and
  `jq`. If a plugin ever wants to *generate* an image, the honest answer is a
  script in a language that can (Go, Python), which the plugin contract already
  allows: an entry point is any program.
