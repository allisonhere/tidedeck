# Put a map under the radar

## Goal

Draw the radar over a real map — satellite imagery of the same ground, the same
size, the same shape — so the panel reads as weather over a place rather than
bright pixels on dark glass.

## Why the radar looks empty without one

A radar frame is transparent except where it rains: the Austin frame measured
0.15% echo, Kentucky 30%. Everything else is panel background. The overlay is
correct and always has been; there is simply nothing under it. A faint basemap
turns "a smudge on the dark" into "a squall line west of Waco", and it makes the
distance ring mean something, because there is now a coastline and a city to
measure it against.

## Verified before writing this (all live, 2026-09-18)

NASA GIBS (Global Imagery Browse Services) serves Web Mercator tiles of satellite
imagery with **no key and no registration**, in the public domain (US government
work). Probed directly:

| Layer | Tile matrix set | Tiles | Austin z8 (row 105, col 58) |
|---|---|---|---|
| `VIIRS_CityLights_2012` | `GoogleMapsCompatible_Level8` | 256 px JPEG | 200, 19 462 bytes, mean luminance 75, max 251, 1% near-black — dark with a city glow |
| `BlueMarble_ShadedRelief_Bathymetry` | `GoogleMapsCompatible_Level8` | 256 px JPEG | 200, 12 780 bytes, mean 56, max 169 — dim terrain |
| `BlueMarble_ShadedRelief` | `GoogleMapsCompatible_Level8` | 256 px JPEG | 200 |

URL shape, exactly (WMTS REST, `{set}/{z}/{row}/{col}` — row first, which is `y`):

```
https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/{layer}/default/GoogleMapsCompatible_Level8/8/{y}/{x}.jpg
```

Facts that decide the design:

1. **A GIBS level set is one zoom.** `GoogleMapsCompatible_Level8` serves z8 only;
   z7 and z9 both return `400`. So the basemap's zoom is fixed at 8.
2. **That is exactly the radar's resolution.** RainViewer serves 512 px tiles at
   z7; a z8 tile is 256 px and covers a quarter of the ground. So one radar tile
   (512×512 px) is **2×2 GIBS tiles (2×2×256 = 512×512 px)** — the same pixels over
   the same ground. The two mosaics line up cell for cell with **no scaling and no
   reprojection code**.
3. **The layers are static.** City lights is a 2012 composite, Blue Marble is a
   fixed relief map. They do not change between refreshes, so they can be fetched
   once and cached for the life of the process: a 3-tile radar pane costs 12
   basemap requests **once per location/zoom**, and zero afterwards. RainViewer
   keeps its three requests every five minutes.
4. **A missing tile is a flat colour, not a 404.** Asking for a tile outside the
   data returned `200` with a single flat value (mean = min = max = 42). That is
   detectable, and a flat tile should be treated as "no basemap here" rather than
   painted over the radar pane as a grey slab.

## Architecture

`provider/basemap.go` grows a `Basemap(BasemapOptions)` next to `Radar`, returning
the same model — a picture of a place, with the reader's position and the ground
scale on it. To keep one model rather than two, `tideui.RadarFrame` becomes
`tideui.MapFrame` and `RadarFrame` becomes an alias, so the existing provider,
panel and tests are untouched by the rename.

The panel fetches the basemap once per (centre, zoom, block) and composes the radar
over it with a plain source-over blend before drawing, so the radar's transparency
shows the map and its echoes cover it. A package-level cache keyed by
`layer/z/x/y` (bounded, like the image-id registry already is) means a refresh
never re-fetches a basemap tile it already has.

The layer is one setting, `radar.basemap`, an enum: `off`, `night lights`
(default), `relief`. Night lights is the default because the dashboard is dark and
a night map is the same picture the theme is: the daytime relief is bright enough
to fight the radar, and would have to be dimmed to sit under it.

Credit goes in the caption as one part — `RainViewer · NASA GIBS` — so the
existing drop logic never keeps one source and throws the other away.

## Tasks

Each task is a commit, tests first, and the gate is:

```
cd /home/allie/Projects/tidedeck
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
```

Nothing in the test suite touches the network: GIBS is exercised through
`httptest`, like RainViewer already is.

### T1 — One frame model, two sources

1. In `image.go`, rename `RadarFrame` to `MapFrame` (comment updated: "one picture
   of one place: what it shows, when it is from, where the reader is in it and what
   a pixel is worth on the ground").
2. Add `// RadarFrame is a MapFrame: a radar picture is one kind of map picture.`
   and `type RadarFrame = MapFrame`.
3. Verify nothing else changed behaviour: `go test -count=1 ./... 2>&1 | tail -3`
   → six `ok` lines.

### T2 — The basemap provider

New file `provider/basemap.go`:

```go
// BasemapLayers are the NASA GIBS layers this provider offers, by the name a
// setting uses. Every one of them is a static composite in the public domain, in
// Web Mercator, served without a key - which is why they can be cached for the
// life of a process and never re-requested.
var BasemapLayers = map[string]string{
	"night lights": "VIIRS_CityLights_2012",
	"relief":       "BlueMarble_ShadedRelief_Bathymetry",
}

// basemapZoom is the zoom the basemap is fetched at: GIBS serves a level set for
// one zoom only, and Level8 is the one that matters here - a GIBS tile is 256 px
// where a radar tile is 512, so a z8 tile is a quarter of a z7 tile's ground and
// a 2x2 block of them is exactly one radar tile, pixel for pixel.
const basemapZoomOffset = 1
```

with `BasemapOptions{Latitude, Longitude, Zoom, Cols, Rows, Layer}`, a
`Basemap(...)` returning `func(context.Context) (tideui.MapFrame, error)`, and the
tile URL builder. Tests in `provider/basemap_test.go` against `httptest`:
the URL shape (`.../GoogleMapsCompatible_Level8/8/{y}/{x}.jpg`, and that the row
comes before the column), that a 2×2 block per radar tile produces a mosaic of
`Cols*radarTileSize` px, that the reader's centre lands in the right place, and
that a flat tile is reported as no-data rather than drawn.

### T3 — The composite

In `image.go` (or a small `compose.go`), `Composite(base, over image.Image)`
source-over: `out = over*a + base*(1-a)` per channel. Tests: a fully transparent
`over` leaves `base` untouched; a fully opaque one replaces it; a half-alpha
channel blends exactly halfway; different-sized images are a programming error and
are reported, not guessed at.

### T4 — The panel draws the map first

In `dash/panels/radar.go`:

- a `basemapCache` (layer → z → x → y → image, bounded), a `basemapFor(frame)`
  that fetches the block once per (centre, zoom, block) and remembers it,
- `radar.basemap` in `Schema()`: `dash.FieldEnum` with `off`, `night lights`,
  `relief`, default `night lights`,
- `View` composes: basemap, then the fetched frame with its crosshair and ring on
  top — the composed picture stays the cached one (`drawnFrame` already guarantees
  one transmit per picture; the cache extends that to "one fetch per place"),
- the caption credits both sources as one part.

Tests: a stub basemap provider; the composed picture is the size of the radar frame
and a pixel where the radar is transparent shows the basemap; the basemap is asked
for once for two draws of the same place; with `radar.basemap` off the panel draws
exactly what it draws today.

### T5 — Docs

README: the panels table (Radar — "the frame over a NASA GIBS basemap, composed
with a caption line, a crosshair on your position and a distance ring"), the
sources table (`provider.Basemap(BasemapOptions) | NASA GIBS (no key); static
layers, fetched once and cached`), and the settings list. A line in the radar
package doc about why the basemap is cached and why it is two tiles per radar
tile.

## Risks and open questions

- **Politeness is the whole design.** Twelve requests once, then none: no retries,
  no polling, sequential fetches, and a bounded cache. If GIBS is slow or down, the
  panel draws the radar alone — a missing backdrop is not an error worth a notice.
- **The basemap is behind a setting, so it can be turned off**, and turning it off
  must leave today's panel byte-for-byte.
- **Which default layer is a taste question.** Night lights suits the dark theme
  and is my recommendation; relief is the daytime alternative and will look bright
  under a dark theme until it is dimmed (a one-line multiply, not planned yet).
- **Not doing:** a street map (OpenStreetMap's tile policy does not want a
  dashboard polling it), and any source that needs a key or an account.
- Open: should the basemap be dimmed to a fraction of its brightness even at night,
  so echoes stay dominant? Cheap to try once it is on screen.

## Notes from implementing it (2026-09-18, evening)

Built as five commits on `feat/images-and-radar`: `38c6943` (one frame model),
`8920749` (the provider), `d1a1e0a` (the composite), `a9e0b9f` (the panel),
`351c2ce` (the README). Tests green in six packages.

### What the real binary does now

Driven through a pty at the user's own cell size (7x17 px, `TERM=xterm-ghostty`),
Kentucky, zoom 7, 120x40 then 200x60:

```
1 transmit, f=100,q=2,i=1,U=1,c=36,r=5,a=T
picture          : 1536x512 px, 1 416 257 bytes
decoded          : 1536x512, mean luminance 68, 100% opaque, 15.5% coloured
caption          : 18:30 · Kentucky · 753 km wide · RainViewer · NASA GIBS
```

The number that matters is **opaque = 100%**: before this, roughly 70% of that
frame was transparent sky. The map is under it, the echoes are over it, and the
place is credited beside the radar's own service.

### Three things the plan did not foresee

1. **A missing setting key is not the setting's default.** `values.String(key)`
   returns `""` for a key that is not in the file, so a fresh panel read "no layer
   chosen" as "no map" and quietly fetched nothing - while the settings screen
   showed "night lights". `basemapLayerFor` now treats an absent key as the default
   and only an explicit `off` as off. Any panel with a choice field has this trap.
2. **A translucent test colour must be `color.NRGBA`, not `color.RGBA`.** In this
   Go, `color.RGBA{255,255,255,128}.RGBA()` is 65535 - `RGBA` is premultiplied and
   cannot hold "full red at half alpha" - so a hand-written fixture composed to
   white instead of halfway. The composite itself now uses `image/draw`'s own
   `draw.Over`, which knows the difference between a picture carrying straight
   alpha and one carrying premultiplied alpha, which the radar's PNGs and the
   basemap's JPEGs are on either side of.
3. **The composite has to be the cached picture, not a picture made per draw.**
   `drawnFrame` already cached the crosshair-and-ring copy; the map joins its key,
   so the composed picture is still one transmit per frame (one APC in the whole
   twenty-second run) rather than one per redraw.

### The cost, honestly

A photographic backdrop does not compress like a sparse radar frame: the composed
PNG is 1.4 MB where the radar alone was 0.2 MB, and it is re-sent whenever the
frame changes - every five minutes, about 17 MB an hour of terminal traffic. If
that turns out to matter, the fix is to transmit the map once as its own kitty
image and place each radar frame over it, which would put the per-frame cost back
to the radar's own. Not done: it is a bigger change than the backdrop is worth
until someone notices.

Still open: whether night lights should be dimmed further so echoes dominate even
more, and whether `relief` is worth offering at all now that the dark one is on
screen.

## Evening, second pass: a real map, and the bug behind the wrong one

The imagery layers were the wrong answer on a dark dashboard. The user looked at the
pane and said it was "the current solar system photo" - which is exactly what
`VIIRS_CityLights_2012` looks like over a rural county: black, with scattered white
points. Imagery is not a map, and a photograph of the dark half of the planet reads as
noise at 118 cells wide.

What replaced it:

- **`topographic`**: the USGS National Map's topographic tile service (public domain, no
  key, a pyramid at *every* zoom). Roads, rivers, contour shading, place names. The
  default where the United States' own maps reach - the lower 48, Alaska and Hawaii -
  and the setting's `auto` is what resolves that per place. Elsewhere `auto` picks
  nothing: a pane with no map draws its radar alone, which is honest.
- **`tideui.Duotone`**: the topo sheet is dark ink on light paper; the pane is dark
  glass. It is re-inked rather than dimmed - the paper becomes the panel's own
  background and the ink becomes `Mix(background, text, 0.5)`, a line colour *derived*
  from the theme, so the map is drawn in the theme's colours and changes with it.
- **`tideui.Resample`**: a bilinear scaler, because a pyramid at any zoom still has to
  meet a pane of whatever size.

### The bug that made both maps wrong

`tilePosition` and `coordinateAt` - the slippy-map projection helpers - clamped their
zoom with `clampZoom`, which is the *radar's* ceiling of 7. The basemap asks for zoom 8
tiles. So every map request used zoom-7 tile numbers in a zoom-8 URL: a valid tile of
somewhere else, with the right size, the right format and a 200. The night-lights map
the user saw was therefore of the wrong place entirely, and so was the topographic one
before this was found.

It was found by *looking*: the composited picture was flat, the raw map was flat at a
single value (220 everywhere), and a live check of the provider printed the grid it had
asked for - `x=33 y=49` at zoom 8, where the reader's own zoom-8 tile is `67/99`. The
fix is a `slippyZoom` clamp of 0..22 for the projection, with the radar's own ceiling
left in the radar's own code, and a regression test that the block of tiles *contains
the reader's own tile* - the invariant that a wrong-place map violates and a working one
cannot.

### What it looks like now

Verified by rendering the pane at 1000x560 and *looking at it* (native-pixel crops, not
a downscaled view - thin line work disappears when a viewer downscales):

- the map alone reads as a map: "Lake Cumberland", "DANIEL BOONE NATIONAL FOREST",
  "Green River", route shields, dashed state boundaries, terrain shading;
- the composed pane reads as one panel: NEXRAD reflectivity (green through red storm
  cells) over that map, with the crosshair on the reader's position;
- and the dashboard itself, driven through a pty at the user's own cell size, draws
  826x68 and 1386x85 pictures at `c=118` and `c=198` - the pane's own pixels, filling
  it edge to edge, captioned `18:55 · Kentucky · 405 km wide · NEXRAD · IEM · USGS`.
