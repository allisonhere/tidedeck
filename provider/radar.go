package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // the basemap's tiles
	_ "image/png"  // the radar's tiles and every other service's
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

// radarTileSize is the tile to ask for, in pixels. The service offers 256 and
// 512, and a placeholder-capable terminal draws the tile at the panel's own pixel
// size - so 512 is four times the pixels for roughly a kilobyte more per frame,
// while 256 would be stretched across a wide pane.
const radarTileSize = 512

// RadarTilePixels is radarTileSize as something a panel can reason with: how many
// pixels one tile of this provider's picture is, which is the unit in which "is
// this pane bigger than one tile" is asked.
const RadarTilePixels = radarTileSize

// tileUserAgent names this dashboard. A free public service deserves to know who
// is calling it, and a user agent is the only courtesy it gets.
const tileUserAgent = "tideui/1 (+https://github.com/allisonhere/tideui)"

// RadarOptions is where and how closely to look.
type RadarOptions struct {
	Latitude  float64
	Longitude float64
	// Zoom is the slippy-map zoom. The service documents 7 as its deepest, and
	// asks for more serves a canned tile rather than an error: verified by
	// fetching eight and ten, which return byte-identical files for different
	// coordinates. So the clamp is not politeness, it is the difference between
	// a picture of the weather and a picture of nothing.
	Zoom int
	// Cols and Rows are how many tiles the picture is worth, one each way by
	// default. More tiles do not show more weather, they show the same weather
	// with more pixels - which is what a pane wider than one tile needs, and what
	// costs the service more requests, so the caller decides.
	Cols, Rows int
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
		grid := tileGridFor(opts.Latitude, opts.Longitude, zoom, opts.Cols, opts.Rows)
		picture := image.NewRGBA(image.Rect(0, 0, grid.Cols*radarTileSize, grid.Rows*radarTileSize))
		for row := 0; row < grid.Rows; row++ {
			for col := 0; col < grid.Cols; col++ {
				tile, err := fetchTile(ctx, grid.tileURL(host, newest.Path, col, row))
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

// tileFraction is where a coordinate sits inside its own tile, 0..1 in each
// direction.
func tileFraction(lat, lon float64, zoom int) (fx, fy float64) {
	scale := math.Exp2(float64(clampZoom(zoom)))
	x := (lon + 180) / 360 * scale
	radians := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(radians)+1/math.Cos(radians))/math.Pi) / 2 * scale
	return x - math.Floor(x), y - math.Floor(y)
}

// tileGrid is the block of tiles to fetch around the one that contains a
// coordinate, and where in the stitched picture that coordinate falls. It is
// slippy-map geometry rather than anything radar about it: the basemap is drawn on
// the same grid, one zoom in.
type tileGrid struct {
	X, Y       int // the top-left tile of the block
	Cols, Rows int
	Zoom       int
	Centre     image.Point
}

// tileGridFor centres a cols by rows block on the coordinate's own tile. The
// coordinate is the subject, so the block grows the same distance in every
// direction from it, and an even-sided block is off-centre by half a tile - which
// is exactly why Centre is computed rather than assumed.
func tileGridFor(lat, lon float64, zoom, cols, rows int) tileGrid {
	zoom = clampZoom(zoom)
	cols, rows = normaliseGrid(cols), normaliseGrid(rows)
	x, y := tileAt(lat, lon, zoom)
	fx, fy := tileFraction(lat, lon, zoom)
	offsetX, offsetY := blockOffset(cols, fx), blockOffset(rows, fy)
	return tileGrid{
		X: x - offsetX, Y: y - offsetY, Cols: cols, Rows: rows, Zoom: zoom,
		Centre: image.Pt(
			offsetX*radarTileSize+int(fx*radarTileSize),
			offsetY*radarTileSize+int(fy*radarTileSize),
		),
	}
}

// rect is where the tile at col/row of the block goes in the stitched picture.
func (g tileGrid) rect(col, row int) image.Rectangle {
	return image.Rect(col*radarTileSize, row*radarTileSize,
		(col+1)*radarTileSize, (row+1)*radarTileSize)
}

// tileURL is the service's own shape, with the numbers in the order it documents:
// {path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png. An extra segment is not
// rejected, it silently shifts what the numbers mean. A block that runs off the
// world repeats the edge tile: a duplicated edge is honest, a hole is not.
func (g tileGrid) tileURL(host, path string, col, row int) string {
	scale := 1 << g.Zoom
	x := clampTile(g.X+col, scale)
	y := clampTile(g.Y+row, scale)
	return fmt.Sprintf("%s%s/%d/%d/%d/%d/4/1_1.png", host, path, radarTileSize, g.Zoom, x, y)
}

// blockOffset is how many tiles of a block come before the coordinate's own tile.
// An odd block puts the coordinate in its middle; an even one cannot, so it grows
// towards whichever side leaves the coordinate nearest the middle of the picture -
// half a tile out at worst, rather than a whole one.
func blockOffset(tiles int, fraction float64) int {
	offset := (tiles - 1) / 2
	if tiles%2 == 0 && fraction < 0.5 {
		offset++
	}
	return offset
}

// normaliseGrid keeps a block inside what one refresh may ask for: one to three
// tiles each way. What the pane is actually worth is the caller's decision.
func normaliseGrid(n int) int { return min(max(n, 1), 3) }

// kilometresPerPixel is the ground distance one pixel of the picture covers, which
// is what turns a distance ring from decoration into a measurement: at zoom 7 a
// tile spans 40075*cos(lat)/128 km and is radarTileSize pixels wide.
func kilometresPerPixel(lat float64, zoom int) float64 {
	const earthCircumference = 40075.0
	tilesAcross := math.Exp2(float64(clampZoom(zoom)))
	kmPerTile := earthCircumference * math.Cos(lat*math.Pi/180) / tilesAcross
	return kmPerTile / float64(radarTileSize)
}

// fetchRadarIndex reads the frame list.
func fetchRadarIndex(ctx context.Context) (radarIndex, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, radarIndexURL, nil)
	if err != nil {
		return radarIndex{}, err
	}
	request.Header.Set("User-Agent", tileUserAgent)
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

// fetchTile reads and decodes one tile, whatever the service encodes it as: the
// radar serves PNG and the basemap serves JPEG, and both are registered here so
// one fetch serves both sources.
func fetchTile(ctx context.Context, url string) (image.Image, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", tileUserAgent)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile: %s", response.Status)
	}
	img, _, err := image.Decode(response.Body)
	if err != nil {
		return nil, fmt.Errorf("tile: %w", err)
	}
	return img, nil
}

// tileAt is the slippy-map tile containing a coordinate, which is the whole
// of the geography this panel needs: lon maps linearly onto the world, lat does
// not, and nobody has ever enjoyed that fact.
func tileAt(lat, lon float64, zoom int) (int, int) {
	scale := math.Exp2(float64(clampZoom(zoom)))
	x := (lon + 180) / 360 * scale
	radians := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(radians)+1/math.Cos(radians))/math.Pi) / 2 * scale
	return clampTile(int(math.Floor(x)), int(scale)), clampTile(int(math.Floor(y)), int(scale))
}

// clampTile keeps a coordinate inside the world, so a pole or a meridian does not
// ask for a tile that cannot exist.
func clampTile(v, n int) int { return min(max(v, 0), n-1) }

// RadarMaxZoom is the deepest zoom the service serves real radar for. Asking for
// more returns a canned tile rather than an error, which is a lie in the shape of
// a picture, so nothing here may ask for more.
const RadarMaxZoom = 7

// RadarDefaultZoom is where a panel should start: a metro area and its
// surroundings, which is the scale "will it rain here" is asked at.
const RadarDefaultZoom = RadarMaxZoom

// clampZoom keeps a zoom inside what the service actually serves.
func clampZoom(zoom int) int { return min(max(zoom, 0), RadarMaxZoom) }
