package provider

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	mux.HandleFunc("/v2/radar/newest/512/7/29/52/4/1_1.png", func(w http.ResponseWriter, r *http.Request) {
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
// rests on. The Austin and London cases were checked against the live service:
// both tiles come back as PNGs at zoom 7, the deepest zoom it serves real radar
// for.
func TestRadarTileForCoordinates(t *testing.T) {
	for _, c := range []struct {
		lat, lon float64
		zoom     int
		x, y     int
	}{
		{0, 0, 1, 1, 1},
		{30.2672, -97.7431, 7, 29, 52},
		{51.5074, -0.1278, 7, 63, 42},
	} {
		x, y := radarTileFor(c.lat, c.lon, c.zoom)
		if x != c.x || y != c.y {
			t.Errorf("radarTileFor(%v, %v, %d) = %d/%d, want %d/%d", c.lat, c.lon, c.zoom, x, y, c.x, c.y)
		}
	}
}

// Asking deeper than the service serves returns a canned tile rather than an
// error - eight and ten were byte-identical files for different coordinates when
// this was written - so the clamp is what keeps a panel honest.
func TestRadarClampsTheZoomToWhatTheServiceServes(t *testing.T) {
	if got := clampZoom(11); got != RadarMaxZoom {
		t.Fatalf("clampZoom(11) = %d, want %d", got, RadarMaxZoom)
	}
	if got := clampZoom(-3); got != 0 {
		t.Fatalf("clampZoom(-3) = %d, want 0", got)
	}
	if got := clampZoom(7); got != 7 {
		t.Fatalf("clampZoom(7) = %d, want 7", got)
	}
}

// The newest frame is the one a live panel wants, and the tile it fetches is
// described by the index rather than guessed at.
func TestRadarFetchesTheNewestFrame(t *testing.T) {
	server := radarServer(t)
	restore := radarIndexURL
	radarIndexURL = server.URL + "/public/weather-maps.json"
	t.Cleanup(func() { radarIndexURL = restore })

	fetch := Radar(RadarOptions{Latitude: 30.2672, Longitude: -97.7431, Zoom: 7})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Unix(1789767600, 0); !frame.Time.Equal(want) {
		t.Errorf("frame time = %v, want %v", frame.Time, want)
	}
	if frame.Image == nil || frame.Image.Bounds().Dx() != radarTileSize {
		t.Fatalf("frame image is %v, want one %dpx tile", frame.Image.Bounds(), radarTileSize)
	}
	// And the tile the index pointed at is in it, drawn at the size the service
	// promises rather than stretched to fit.
	if r, _, _, _ := frame.Image.At(4, 4).RGBA(); r>>8 != 255 {
		t.Fatalf("the picture's own tile = %v, want the red one the index pointed at", frame.Image.At(4, 4))
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

	if _, err := Radar(RadarOptions{Latitude: 1, Longitude: 2, Zoom: 11})(context.Background()); err == nil {
		t.Fatal("an index that failed was not an error")
	}
}

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
	if grid.Centre.Y < radarTileSize || grid.Centre.Y >= 2*radarTileSize {
		t.Fatalf("the coordinate landed at y=%d, outside its own tile", grid.Centre.Y)
	}
	// One tile means the coordinate's own tile, and the centre is inside it.
	single := radarGridFor(30.2672, -97.7431, 7, 1, 1)
	if single.X != 29 || single.Y != 52 {
		t.Fatalf("a single-tile grid = %d/%d, want 29/52", single.X, single.Y)
	}
	if single.Centre.X < 0 || single.Centre.X >= radarTileSize || single.Centre.Y < 0 || single.Centre.Y >= radarTileSize {
		t.Fatalf("a single-tile centre = %v, outside the tile", single.Centre)
	}
	// And it is the coordinate, not the middle: Austin is not the centre of its
	// own tile, and a panel that drew it at 256,256 would be a few km out.
	if single.Centre == image.Pt(radarTileSize/2, radarTileSize/2) {
		t.Fatal("the centre came out as the middle of the tile, which the coordinate is not")
	}

	// An even block cannot put the coordinate in the middle, and must not make it
	// worse: half a tile out is the best there is, a whole one is a bug.
	even := radarGridFor(30.2672, -97.7431, 7, 2, 2)
	if got := math.Abs(float64(even.Centre.X - even.Cols*radarTileSize/2)); got > radarTileSize/2 {
		t.Fatalf("the coordinate is %.0f px from the middle of a 2x2 block, want at most %d", got, radarTileSize/2)
	}
	if got := math.Abs(float64(even.Centre.Y - even.Rows*radarTileSize/2)); got > radarTileSize/2 {
		t.Fatalf("the coordinate is %.0f px from the middle of a 2x2 block, want at most %d", got, radarTileSize/2)
	}
}

// Ground distance per pixel is what makes a distance ring honest. At zoom 7 the
// world is 128 tiles across and a tile is radarTileSize pixels wide, so a tile
// spans 40075*cos(lat)/128 km of ground.
func TestRadarKilometresPerPixel(t *testing.T) {
	got := kilometresPerPixel(39, 7)
	want := 40075 * math.Cos(39*math.Pi/180) / 128 / radarTileSize
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("kilometresPerPixel(39, 7) = %v, want %v", got, want)
	}
	if got := kilometresPerPixel(0, 7); got < 0.6 || got > 0.62 {
		t.Fatalf("at the equator a pixel at zoom 7 = %v km, want about 0.61", got)
	}
	// Half the area at half the latitude: a degree of longitude is worth less
	// ground the further north you are.
	if kilometresPerPixel(60, 7) >= kilometresPerPixel(0, 7) {
		t.Fatal("a northern pixel covers at least as much ground as an equatorial one")
	}
}

// A block that runs off the edge of the world repeats the edge tile instead of
// asking for a tile that cannot exist.
func TestRadarGridClampsToTheWorld(t *testing.T) {
	grid := radarGridFor(0, -179.9, 1, 3, 3)
	url := grid.tileURL("https://example.test", "/v2/radar/x", 0, 0)
	if !strings.Contains(url, "/1/0/0/") {
		t.Fatalf("a tile off the west edge = %q, want x clamped to 0", url)
	}
	if got := grid.tileURL("https://example.test", "/v2/radar/x", 0, 0); got != "https://example.test/v2/radar/x/512/1/0/0/4/1_1.png" {
		t.Fatalf("tile URL = %q", got)
	}
}

// A mosaic is stitched in the order it was asked for, and the picture is as big as
// the block: four tiles is 2*radarTileSize square.
func TestRadarMosaicStitchesTheBlock(t *testing.T) {
	block := radarGridFor(30.2672, -97.7431, 7, 2, 2)
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".json") {
			asked = append(asked, r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/public/weather-maps.json") {
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"host": "http://" + r.Host,
				"radar": map[string]any{"past": []map[string]any{
					{"time": 1789767600, "path": "/v2/radar/newest"},
				}},
			})
			return
		}
		// One distinct colour per tile, so a test can say which came from where.
		// The URLs come from the grid itself, so this asserts the stitching rather
		// than repeating the arithmetic that chose the block.
		colours := map[string]color.RGBA{
			block.tileURL("", "/v2/radar/newest", 0, 0): {R: 255, A: 255},
			block.tileURL("", "/v2/radar/newest", 1, 0): {G: 255, A: 255},
			block.tileURL("", "/v2/radar/newest", 0, 1): {B: 255, A: 255},
			block.tileURL("", "/v2/radar/newest", 1, 1): {R: 255, G: 255, A: 255},
		}[r.URL.Path]
		// Full-size tiles, because that is what the service serves and what the
		// stitcher copies one to one: a smaller fixture would be a picture of a
		// different promise.
		img := image.NewRGBA(image.Rect(0, 0, radarTileSize, radarTileSize))
		for y := 0; y < radarTileSize; y++ {
			for x := 0; x < radarTileSize; x++ {
				img.Set(x, y, colours)
			}
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img) //nolint:errcheck
	}))
	t.Cleanup(server.Close)
	restore := radarIndexURL
	radarIndexURL = server.URL + "/public/weather-maps.json"
	t.Cleanup(func() { radarIndexURL = restore })

	fetch := Radar(RadarOptions{Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Cols: 2, Rows: 2})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Image.Bounds().Dx(); got != 2*radarTileSize {
		t.Fatalf("the mosaic is %d px wide, want %d", got, 2*radarTileSize)
	}
	if got := frame.Image.Bounds().Dy(); got != 2*radarTileSize {
		t.Fatalf("the mosaic is %d px tall, want %d", got, 2*radarTileSize)
	}
	// Each quadrant holds its own tile: the block is stitched into the right place
	// rather than in the order the tiles happened to arrive.
	quadrant := func(col, row int) (uint32, uint32, uint32) {
		r, g, b, _ := frame.Image.At(col*radarTileSize+radarTileSize/2, row*radarTileSize+radarTileSize/2).RGBA()
		return r >> 8, g >> 8, b >> 8
	}
	if r, g, b := quadrant(0, 0); r != 255 || g != 0 || b != 0 {
		t.Fatalf("the top-left tile = %d,%d,%d, want red; asked for %v", r, g, b, asked)
	}
	if r, g, b := quadrant(1, 0); g != 255 || r != 0 {
		t.Fatalf("the top-right tile = %d,%d,%d, want green", r, g, b)
	}
	if r, g, b := quadrant(0, 1); b != 255 || r != 0 {
		t.Fatalf("the bottom-left tile = %d,%d,%d, want blue", r, g, b)
	}
	if r, g, b := quadrant(1, 1); r != 255 || g != 255 || b != 0 {
		t.Fatalf("the bottom-right tile = %d,%d,%d, want red and green", r, g, b)
	}
	// The pixel the frame marks is inside the coordinate's own tile of the block,
	// which is the thing a panel draws a crosshair on.
	if tileX := block.X + frame.Centre.X/radarTileSize; tileX != 29 {
		t.Fatalf("the centre %v sits in tile x=%d, want the coordinate's own 29", frame.Centre, tileX)
	}
	if tileY := block.Y + frame.Centre.Y/radarTileSize; tileY != 52 {
		t.Fatalf("the centre %v sits in tile y=%d, want the coordinate's own 52", frame.Centre, tileY)
	}
	if frame.KilometresPerPixel <= 0 {
		t.Fatal("the frame did not say what a pixel is worth on the ground")
	}
}
