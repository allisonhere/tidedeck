package provider

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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
