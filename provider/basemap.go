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

// BasemapLayer is one source of ground to draw under a radar pane.
type BasemapLayer struct {
	// Service is a tile service in the form {service}/{z}/{y}/{x}, serving a pyramid at
	// any zoom. An empty Service means NASA GIBS, whose layers carry one fixed zoom in
	// the path instead.
	Service string
	// Layer is the service's own layer name.
	Layer string
	// Credit is what a panel names when it draws this layer.
	Credit string
	// Paper is true when the map is dark ink on light paper, so a panel drawing it on
	// dark glass re-inks it instead of showing a lit sheet.
	Paper bool
	// Zoom is the zoom a GIBS layer is served at, which that service fixes. Zero for a
	// service with a pyramid, which is asked for the zoom the pane is worth.
	Zoom int
	// USOnly marks a layer whose coverage is the United States.
	USOnly bool
}

// BasemapLayers are the maps this provider can draw, keyed by the name a setting uses.
// Every one is served without a key and without an account, and every one is a still
// picture of the ground: the same tiles come back every time, which is what makes keeping
// them in memory honest rather than merely convenient.
var BasemapLayers = map[string]BasemapLayer{
	// A map: roads, rivers, contours and place names, in the public domain, over the
	// United States, at any zoom. The only one of these that is a map rather than a
	// photograph of the ground.
	"topographic": {Service: usgsTopoService, Layer: "USGSTopo", Credit: "USGS", Paper: true, USOnly: true},
	// Imagery: a photograph of the ground at night, with no names and no lines on it.
	"night lights": {Layer: "VIIRS_CityLights_2012", Credit: "NASA GIBS", Zoom: 8},
	"relief":       {Layer: "BlueMarble_ShadedRelief_Bathymetry", Credit: "NASA GIBS", Zoom: 8},
}

// BasemapAutoLayer is the setting's "choose for me": the best map that covers where the
// pane is looking.
const BasemapAutoLayer = "auto"

// BasemapDefaultLayer is that choice written out, for a settings screen that has to show
// something.
const BasemapDefaultLayer = BasemapAutoLayer

// usgsTopoService is the National Map's topographic tile service: public domain, no key,
// a pyramid at every zoom.
const usgsTopoService = "https://basemap.nationalmap.gov/arcgis/rest/services/USGSTopo/MapServer/tile"

// usCoverage is where the United States' own maps reach, as rough boxes: the lower
// forty-eight, Alaska and Hawaii.
var usCoverage = [][4]float64{
	{-125.0, 24.0, -66.5, 49.5},
	{-170.0, 51.0, -129.0, 71.5},
	{-161.0, 18.5, -154.5, 22.5},
}

// BasemapLayerFor is the best map for a place, or nothing when none covers it. A pane
// with no map draws its radar alone: imagery is not a map, and a map of the wrong place
// is worse than none.
func BasemapLayerFor(lat, lon float64) string {
	for _, box := range usCoverage {
		if lat >= box[1] && lat <= box[3] && lon >= box[0] && lon <= box[2] {
			return "topographic"
		}
	}
	return ""
}

// basemapHost is the GIBS WMTS endpoint - a variable so a test can point it at a server
// of its own.
var basemapHost = "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best"

// basemapTilePixels is a tile's width, for every service here: it is the Web Mercator
// convention and all of them keep to it.
const basemapTilePixels = 256

// basemapMaxTiles is how many tiles one view may cost. A pyramid source is asked for the
// zoom that keeps it inside this; a source with one fixed zoom has no such choice, so a
// view too wide for it is refused rather than fetched in pieces.
const basemapMaxTiles = 16

// BasemapOptions is where and how big the basemap should be. It mirrors RadarOptions
// deliberately: the two pictures are drawn over one another, so a panel hands them the
// same numbers.
type BasemapOptions struct {
	Latitude, Longitude float64
	Zoom                int
	// Width and Height are the pixels the caller will draw: the picture comes back
	// exactly that size, which is what lets it line up with the radar at any zoom.
	Width, Height int
	// Layer is a name from BasemapLayers, or BasemapAutoLayer to be chosen for the place.
	// An unknown name is an error rather than a silent fallback: a wrong layer name is a
	// typo, not a preference.
	Layer string
}

// Basemap fetches the ground a radar panel is looking at, for drawing under it: a picture
// of a place at the size the pane asked for - the same model the radar returns, so a panel
// can compose the two without caring which came from where.
func Basemap(opts BasemapOptions) func(context.Context) (tideui.MapFrame, error) {
	name := strings.ToLower(strings.TrimSpace(opts.Layer))
	if name == BasemapAutoLayer || name == "" {
		name = BasemapLayerFor(opts.Latitude, opts.Longitude)
	}
	layer, known := BasemapLayers[name]
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
		zoom := layer.Zoom
		if layer.Service != "" {
			zoom = basemapPyramidZoom(view)
		}
		grid := basemapGridFor(west, south, east, north, zoom)
		if grid.tiles() > basemapMaxTiles {
			return tideui.MapFrame{}, fmt.Errorf(
				"basemap: %d tiles for a view this wide, more than the %d it will fetch",
				grid.tiles(), basemapMaxTiles)
		}
		canvas := image.NewRGBA(image.Rect(0, 0, grid.cols*basemapTilePixels, grid.rows*basemapTilePixels))
		for row := 0; row < grid.rows; row++ {
			for col := 0; col < grid.cols; col++ {
				tile, err := fetchTile(ctx, layer.tileURL(zoom, grid.x+col, grid.y+row))
				if err != nil {
					return tideui.MapFrame{}, err
				}
				if tileHasNothingInIt(tile) {
					// Ground the service has no picture for comes back as one flat
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
		picture := tideui.ResampleInto(canvas, grid.crop(west, south, east, north, zoom), view.width, view.height)
		return tideui.MapFrame{
			Image:  picture,
			Centre: image.Pt(view.width/2, view.height/2),
			// The view's zoom, because that is the ground the pane asked to see: the
			// map's own zoom is about how finely it is drawn.
			KilometresPerPixel: kilometresPerPixel(opts.Latitude, view.zoom),
		}, nil
	}
}

// tileURL is a tile of this layer, in whichever shape its service uses. Both put the row
// before the column, which is the opposite of the way the radar's tiles read: an order
// mix-up fetches a valid tile of somewhere else rather than failing.
func (l BasemapLayer) tileURL(zoom, x, y int) string {
	if l.Service != "" {
		return fmt.Sprintf("%s/%d/%d/%d", l.Service, zoom, y, x)
	}
	return fmt.Sprintf("%s/%s/default/GoogleMapsCompatible_Level%d/%d/%d/%d.jpg",
		basemapHost, l.Layer, zoom, zoom, y, x)
}

// basemapPyramidZoom is the zoom of a pyramid that suits a view: about one tile per 256
// pixels of the pane, so the map is as fine as the pane can show and no finer, stepped
// down until the block fits the tile budget. A pyramid is the one kind of source that can
// be asked for a sensible amount of work this way.
func basemapPyramidZoom(view radarView) int {
	kilometresPerPixel := kilometresPerPixel(view.latitude, view.zoom)
	if kilometresPerPixel <= 0 {
		return 0
	}
	// The zoom where one tile covers what 256 pixels of the pane cover.
	const earthCircumference = 40075.0
	tilesAcross := earthCircumference * math.Cos(view.latitude*math.Pi/180) /
		(kilometresPerPixel * basemapTilePixels)
	zoom := int(math.Round(math.Log2(math.Max(tilesAcross, 1))))
	zoom = min(max(zoom, 0), 16)
	west, south, east, north := view.bounds()
	for zoom > 0 && basemapGridFor(west, south, east, north, zoom).tiles() > basemapMaxTiles {
		zoom--
	}
	return zoom
}

// basemapGrid is the block of tiles that covers a view at a zoom.
type basemapGrid struct {
	x, y, cols, rows int
}

func (g basemapGrid) tiles() int { return g.cols * g.rows }

// basemapGridFor is the tiles covering a geographic box at a given zoom.
func basemapGridFor(west, south, east, north float64, zoom int) basemapGrid {
	left, top := tilePosition(north, west, zoom)
	right, bottom := tilePosition(south, east, zoom)
	limit := 1 << zoom
	x := clampTile(int(math.Floor(left)), limit)
	y := clampTile(int(math.Floor(top)), limit)
	lastX := clampTile(int(math.Floor(right)), limit)
	lastY := clampTile(int(math.Floor(bottom)), limit)
	return basemapGrid{x: x, y: y, cols: lastX - x + 1, rows: lastY - y + 1}
}

// crop is where the box a panel asked for sits inside the block of tiles: the tiles cover
// more ground than the view, and the view is the part that is wanted.
func (g basemapGrid) crop(west, south, east, north float64, zoom int) image.Rectangle {
	left, top := tilePosition(north, west, zoom)
	right, bottom := tilePosition(south, east, zoom)
	canvas := image.Rect(0, 0, g.cols*basemapTilePixels, g.rows*basemapTilePixels)
	pixel := func(tileX, tileY float64) (int, int) {
		return int(math.Round((tileX - float64(g.x)) * basemapTilePixels)),
			int(math.Round((tileY - float64(g.y)) * basemapTilePixels))
	}
	minX, minY := pixel(left, top)
	maxX, maxY := pixel(right, bottom)
	return image.Rect(minX, minY, maxX, maxY).Intersect(canvas)
}

// basemapNoData is the flat colour GIBS answers for ground it has no imagery for: 42 in
// every channel, verified by asking for a tile outside the data and getting a 200 with
// that one value.
var basemapNoData = color.RGBA{R: 42, G: 42, B: 42, A: 255}

// tileHasNothingInIt reports a tile the service answered with nothing: a fully
// transparent picture, or GIBS's own no-data colour.
//
// It is deliberately not "a tile of one flat colour": the night-lights layer is a
// photograph of the dark half of the planet, and a tile of a rural county is *exactly* one
// flat black value. Treating flat as nothing threw away most of a map of Kentucky and left
// the half of the pane that is countryside transparent. Sampled rather than exhaustive: a
// tile is 65k pixels and this only decides whether to draw it.
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
