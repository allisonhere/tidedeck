package provider

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/allisonhere/tideui"
)

// BasemapLayers are the NASA GIBS layers this provider can draw, keyed by the name
// a setting uses. Both are static composites in the public domain, served without a
// key and without an account: the same imagery comes back every time, which is what
// makes keeping them in memory honest rather than merely convenient.
var BasemapLayers = map[string]string{
	"night lights": "VIIRS_CityLights_2012",
	"relief":       "BlueMarble_ShadedRelief_Bathymetry",
}

// BasemapDefaultLayer is what a panel draws when nobody has said otherwise: a dark
// map, because the dashboard is dark and a bright backdrop fights the radar.
const BasemapDefaultLayer = "night lights"

// basemapHost is the WMTS endpoint - a variable so a test can point it at a server
// of its own.
var basemapHost = "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best"

// basemapZoom is the only zoom the imagery is served at. GIBS declares a tile matrix
// set per zoom and these layers carry one each (Level8 is zoom eight and nothing
// else: z7 and z9 answer 400), so this is not a preference - it is the ground
// resolution of the imagery. A view at any zoom is cut out of these tiles and
// resampled to the pane.
const basemapZoom = 8

// basemapTilePixels is GIBS's tile size, half the radar's.
const basemapTilePixels = radarTileSize / 2

// basemapMaxTiles is how many tiles one view may cost. The imagery is fixed at one
// zoom, so a view of a continent would want hundreds of them: past this the panel
// draws the radar alone, which is honest - a map helps when a pane is looking at a
// region, and a view this wide is not one.
const basemapMaxTiles = 16

// BasemapOptions is where and how big the basemap should be. It mirrors
// RadarOptions deliberately: the two pictures are drawn over one another, so a panel
// hands them the same numbers.
type BasemapOptions struct {
	Latitude, Longitude float64
	Zoom                int
	// Width and Height are the pixels the caller will draw: the picture comes back
	// exactly that size, which is what lets it line up with the radar at any zoom
	// rather than only when the two tile grids happen to agree.
	Width, Height int
	// Layer is a name from BasemapLayers. An unknown one is an error rather than a
	// silent fallback, because a wrong layer name is a typo, not a preference.
	Layer string
}

// Basemap fetches the ground a radar panel is looking at, for drawing under it: a
// picture of a place at the size the pane asked for, with the reader's own position
// and the ground scale on it - the same model the radar returns, so a panel can
// compose the two without caring which came from where.
func Basemap(opts BasemapOptions) func(context.Context) (tideui.MapFrame, error) {
	layer, known := BasemapLayers[strings.ToLower(strings.TrimSpace(opts.Layer))]
	if !known {
		return func(context.Context) (tideui.MapFrame, error) {
			return tideui.MapFrame{}, fmt.Errorf("basemap: no layer called %q", opts.Layer)
		}
	}
	return func(ctx context.Context) (tideui.MapFrame, error) {
		// The view is the same ground the radar covers, in the same pixels, so the two
		// pictures are the same picture of the same place.
		view := radarViewOf(RadarOptions{
			Latitude: opts.Latitude, Longitude: opts.Longitude, Zoom: opts.Zoom,
			Width: opts.Width, Height: opts.Height,
		})
		west, south, east, north := view.bounds()
		grid := basemapGridFor(west, south, east, north)
		if grid.tiles() > basemapMaxTiles {
			return tideui.MapFrame{}, fmt.Errorf(
				"basemap: %d tiles for a view this wide, more than the %d it will fetch",
				grid.tiles(), basemapMaxTiles)
		}
		canvas := image.NewRGBA(image.Rect(0, 0, grid.cols*basemapTilePixels, grid.rows*basemapTilePixels))
		for row := 0; row < grid.rows; row++ {
			for col := 0; col < grid.cols; col++ {
				tile, err := fetchTile(ctx, basemapTileURL(layer, grid.x+col, grid.y+row))
				if err != nil {
					return tideui.MapFrame{}, err
				}
				if tileHasNothingInIt(tile) {
					// Ground the service has no imagery for comes back as one flat
					// colour. Left out, the panel's own background shows through, which
					// is honest; drawn, it is a grey slab where a map should be.
					continue
				}
				draw.Draw(canvas, image.Rect(
					col*basemapTilePixels, row*basemapTilePixels,
					(col+1)*basemapTilePixels, (row+1)*basemapTilePixels,
				), tile, tile.Bounds().Min, draw.Src)
			}
		}
		picture := tideui.ResampleInto(canvas, grid.crop(west, south, east, north), view.width, view.height)
		return tideui.MapFrame{
			Image:  picture,
			Centre: image.Pt(view.width/2, view.height/2),
			// The view's zoom, because that is the ground the pane asked to see: the
			// imagery's own zoom is about how fine the map is, not how much is shown.
			KilometresPerPixel: kilometresPerPixel(opts.Latitude, view.zoom),
		}, nil
	}
}

// basemapGrid is the block of imagery tiles that covers a view.
type basemapGrid struct {
	x, y, cols, rows int
}

func (g basemapGrid) tiles() int { return g.cols * g.rows }

// basemapGridFor is the imagery tiles covering a geographic box, at the imagery's own
// zoom whatever zoom the view is at.
func basemapGridFor(west, south, east, north float64) basemapGrid {
	left, top := tilePosition(north, west, basemapZoom)
	right, bottom := tilePosition(south, east, basemapZoom)
	limit := 1 << basemapZoom
	x := clampTile(int(math.Floor(left)), limit)
	y := clampTile(int(math.Floor(top)), limit)
	lastX := clampTile(int(math.Floor(right)), limit)
	lastY := clampTile(int(math.Floor(bottom)), limit)
	return basemapGrid{x: x, y: y, cols: lastX - x + 1, rows: lastY - y + 1}
}

// crop is where the box a panel asked for sits inside the block of tiles: the tiles
// cover more ground than the view, and the view is the part that is wanted.
func (g basemapGrid) crop(west, south, east, north float64) image.Rectangle {
	left, top := tilePosition(north, west, basemapZoom)
	right, bottom := tilePosition(south, east, basemapZoom)
	canvas := image.Rect(0, 0, g.cols*basemapTilePixels, g.rows*basemapTilePixels)
	pixel := func(tileX, tileY float64) (int, int) {
		return int(math.Round((tileX - float64(g.x)) * basemapTilePixels)),
			int(math.Round((tileY - float64(g.y)) * basemapTilePixels))
	}
	minX, minY := pixel(left, top)
	maxX, maxY := pixel(right, bottom)
	return image.Rect(minX, minY, maxX, maxY).Intersect(canvas)
}

// basemapTileURL is the service's own shape, and the numbers are row then column -
// latitude-ish first, the opposite order to the radar's tiles. A wrong order is not
// rejected; it fetches a valid tile of somewhere else, so it is worth being exact.
func basemapTileURL(layer string, x, y int) string {
	return fmt.Sprintf("%s/%s/default/GoogleMapsCompatible_Level%d/%d/%d/%d.jpg",
		basemapHost, layer, basemapZoom, basemapZoom, y, x)
}

// basemapNoData is the flat colour the service answers for ground it has no imagery
// for: 42 in every channel, verified by asking for a tile outside the data and getting
// a 200 with that one value.
var basemapNoData = color.RGBA{R: 42, G: 42, B: 42, A: 255}

// tileHasNothingInIt reports a tile the service answered with its no-data colour, or
// with nothing at all.
//
// It is deliberately not "a tile of one flat colour": the night-lights layer is a
// photograph of the dark half of the planet, and a tile of a rural county is *exactly*
// one flat black value. Treating flat as nothing threw away most of a map of Kentucky
// and left the half of the pane that is countryside transparent. Sampled rather than
// exhaustive: a tile is 65k pixels and this only decides whether to draw it.
func tileHasNothingInIt(img image.Image) bool {
	bounds := img.Bounds()
	if bounds.Empty() {
		return true
	}
	const tolerance = 2
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 8 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 8 {
			red, green, blue, alpha := img.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			near := func(channel uint32, want uint8) bool {
				value := int(channel >> 8)
				return value >= int(want)-tolerance && value <= int(want)+tolerance
			}
			if !near(red, basemapNoData.R) || !near(green, basemapNoData.G) || !near(blue, basemapNoData.B) {
				return false
			}
		}
	}
	return true
}
