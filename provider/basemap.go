package provider

import (
	"context"
	"fmt"
	"image"
	"image/draw"
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

// basemapTilePixels is GIBS's tile size. It is half the radar's, which is the whole
// reason the two pictures line up: one radar tile is a 2x2 block of these, over the
// same ground in the same 512 pixels.
const basemapTilePixels = radarTileSize / 2

// basemapZoomOffset is how many zooms deeper the basemap is fetched than the radar.
// Not a preference: GIBS serves a tile matrix set per zoom (Level8 is zoom eight and
// nothing else), and a 256 px tile one zoom in is exactly a radar pixel's worth of
// ground - the one choice that needs no scaling.
const basemapZoomOffset = 1

// BasemapOptions is where and how big the basemap should be. It mirrors
// RadarOptions deliberately: the two pictures have to cover the same ground, so a
// panel hands them the same numbers.
type BasemapOptions struct {
	Latitude, Longitude float64
	Zoom                int
	Cols, Rows          int
	// Layer is a name from BasemapLayers. An unknown one is an error rather than a
	// silent fallback, because a wrong layer name is a typo, not a preference.
	Layer string
}

// Basemap fetches the ground a radar panel is looking at, for drawing under it: a
// picture of a place with the reader's own position and the ground scale on it, the
// same model the radar returns so a panel can compose the two without caring which
// came from where.
func Basemap(opts BasemapOptions) func(context.Context) (tideui.MapFrame, error) {
	layer, known := BasemapLayers[strings.ToLower(strings.TrimSpace(opts.Layer))]
	if !known {
		return func(context.Context) (tideui.MapFrame, error) {
			return tideui.MapFrame{}, fmt.Errorf("basemap: no layer called %q", opts.Layer)
		}
	}
	return func(ctx context.Context) (tideui.MapFrame, error) {
		// The radar's own block, so both pictures stand on the same ground, and one
		// zoom in, where a tile covers a quarter of the ground and a quarter of the
		// pixels - which is to say, the same ground per pixel.
		grid := tileGridFor(opts.Latitude, opts.Longitude, clampZoom(opts.Zoom), opts.Cols, opts.Rows)
		zoom := grid.Zoom + basemapZoomOffset
		picture := image.NewRGBA(image.Rect(0, 0, grid.Cols*radarTileSize, grid.Rows*radarTileSize))
		for row := 0; row < grid.Rows; row++ {
			for col := 0; col < grid.Cols; col++ {
				for dy := 0; dy < 2; dy++ {
					for dx := 0; dx < 2; dx++ {
						x := clampTile(2*(grid.X+col)+dx, 1<<zoom)
						y := clampTile(2*(grid.Y+row)+dy, 1<<zoom)
						tile, err := fetchTile(ctx, basemapTileURL(layer, zoom, x, y))
						if err != nil {
							return tideui.MapFrame{}, err
						}
						if tileHasNothingInIt(tile) {
							// Ground the service has no imagery for comes back as one
							// flat colour. Left out, the panel's own background shows
							// through, which is honest; drawn, it is a grey slab where
							// a map should be.
							continue
						}
						draw.Draw(picture, image.Rect(
							col*radarTileSize+dx*basemapTilePixels, row*radarTileSize+dy*basemapTilePixels,
							col*radarTileSize+(dx+1)*basemapTilePixels, row*radarTileSize+(dy+1)*basemapTilePixels,
						), tile, tile.Bounds().Min, draw.Src)
					}
				}
			}
		}
		return tideui.MapFrame{
			Image:  picture,
			Centre: grid.Centre,
			// The radar's zoom, because a basemap pixel covers exactly as much ground
			// as a radar pixel: that is what "one zoom in, half the tile" buys.
			KilometresPerPixel: kilometresPerPixel(opts.Latitude, grid.Zoom),
		}, nil
	}
}

// basemapTileURL is the service's own shape, and the numbers are row then column -
// latitude-ish first, the opposite order to the radar's tiles. A wrong order is not
// rejected; it fetches a valid tile of somewhere else, so it is worth being exact.
func basemapTileURL(layer string, zoom, x, y int) string {
	return fmt.Sprintf("%s/%s/default/GoogleMapsCompatible_Level%d/%d/%d/%d.jpg",
		basemapHost, layer, zoom, zoom, y, x)
}

// tileHasNothingInIt reports a tile of a single flat colour, which is how the
// service answers for ground it has no imagery for - verified by asking for a tile
// outside the data and getting a 200 with one value. Sampled rather than
// exhaustive: a tile is 65k pixels and this only decides whether to draw it.
func tileHasNothingInIt(img image.Image) bool {
	bounds := img.Bounds()
	if bounds.Empty() {
		return true
	}
	firstRed, firstGreen, firstBlue, _ := img.At(bounds.Min.X, bounds.Min.Y).RGBA()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 8 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 8 {
			red, green, blue, _ := img.At(x, y).RGBA()
			if red != firstRed || green != firstGreen || blue != firstBlue {
				return false
			}
		}
	}
	return true
}
