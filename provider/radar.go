package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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

// radarUserAgent names this dashboard. A free public service deserves to know who
// is calling it, and a user agent is the only courtesy it gets.
const radarUserAgent = "tideui-radar/1 (+https://github.com/allisonhere/tideui)"

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
		// Colormap 4 (the familiar green-to-red radar), smooth 1, snow 1. The
		// shape is the service's own and is exact:
		// {path}/{size}/{z}/{x}/{y}/{color}/{smooth}_{snow}.png - an extra
		// segment is not rejected, it silently shifts what the numbers mean.
		url := fmt.Sprintf("%s%s/%d/%d/%d/%d/4/1_1.png", host, newest.Path, radarTileSize, zoom, x, y)
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

// RadarMaxZoom is the deepest zoom the service serves real radar for. Asking for
// more returns a canned tile rather than an error, which is a lie in the shape of
// a picture, so nothing here may ask for more.
const RadarMaxZoom = 7

// RadarDefaultZoom is where a panel should start: a metro area and its
// surroundings, which is the scale "will it rain here" is asked at.
const RadarDefaultZoom = RadarMaxZoom

// clampZoom keeps a zoom inside what the service actually serves.
func clampZoom(zoom int) int { return min(max(zoom, 0), RadarMaxZoom) }
