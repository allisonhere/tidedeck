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
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The composite is real reflectivity at full resolution, so it wins where it has
// coverage; everywhere else the global tiles do. The boundary is asserted because it
// is the whole of the choice.
func TestRadarSourceIsChosenByPlace(t *testing.T) {
	cases := []struct {
		name     string
		lat, lon float64
		want     RadarSource
	}{
		{"Austin", 30.2672, -97.7431, RadarSourceNEXRAD},
		{"Kentucky", 36.66, -84.4, RadarSourceNEXRAD},
		{"the north-east corner of the coverage", nexradNorth, nexradEast, RadarSourceNEXRAD},
		{"London", 51.5072, -0.1276, RadarSourceTiles},
		{"Sydney", -33.8688, 151.2093, RadarSourceTiles},
		{"a hair outside the coverage", nexradNorth + 0.01, nexradEast - 0.01, RadarSourceTiles},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RadarSourceFor(tc.lat, tc.lon); got != tc.want {
				t.Fatalf("RadarSourceFor(%v, %v) = %q, want %q", tc.lat, tc.lon, got, tc.want)
			}
		})
	}
	// And the credit names the service that answered: two sources, two credits.
	if RadarSourceNEXRAD.Credit() == RadarSourceTiles.Credit() {
		t.Fatal("both radar sources are credited to the same service")
	}
	if !strings.Contains(RadarSourceNEXRAD.Credit(), "IEM") {
		t.Fatalf("the composite's credit = %q, want the Mesonet named", RadarSourceNEXRAD.Credit())
	}
}

// nexradServer answers the service's GetMap with a picture of the size it was asked
// for, and remembers the request.
func nexradServer(t *testing.T) *url.URL {
	t.Helper()
	asked := &url.URL{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = *r.URL
		width, _ := strconv.Atoi(r.URL.Query().Get("WIDTH"))
		height, _ := strconv.Atoi(r.URL.Query().Get("HEIGHT"))
		if width <= 0 || height <= 0 {
			http.Error(w, "no size", http.StatusBadRequest)
			return
		}
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Set(x, y, color.RGBA{G: 200, A: 255})
			}
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img) //nolint:errcheck
	}))
	t.Cleanup(server.Close)
	restore := nexradEndpoint
	nexradEndpoint = server.URL
	t.Cleanup(func() { nexradEndpoint = restore })
	return asked
}

// The service renders the picture, so it is asked for at the pane's own pixels and for
// a box around the reader: no mosaic, no stretching, and nothing to line up afterwards.
func TestNEXRADAsksForTheViewAtThePanesOwnPixels(t *testing.T) {
	asked := nexradServer(t)
	frame, err := radarNEXRAD(context.Background(), RadarOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Width: 320, Height: 180,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The picture is the pane's size, exactly, and the reader is its middle.
	if got := frame.Image.Bounds(); got.Dx() != 320 || got.Dy() != 180 {
		t.Fatalf("the composite came back %v, want 320x180", got)
	}
	if frame.Centre != image.Pt(160, 90) {
		t.Fatalf("the reader is at %v in a 320x180 picture, want the middle", frame.Centre)
	}
	if frame.KilometresPerPixel != kilometresPerPixel(30.2672, 7) {
		t.Fatalf("a pixel is %v km, want the radar's own scale %v", frame.KilometresPerPixel, kilometresPerPixel(30.2672, 7))
	}

	// The request is the service's own shape: a geographic box, the pixels to render
	// it into, and the frame's time - never a tile path.
	query := asked.Query()
	if query.Get("LAYERS") != nexradLayer || query.Get("SRS") != "EPSG:4326" {
		t.Fatalf("asked for %s in %s, want %s in EPSG:4326", query.Get("LAYERS"), query.Get("SRS"), nexradLayer)
	}
	if query.Get("WIDTH") != "320" || query.Get("HEIGHT") != "180" {
		t.Fatalf("asked for %sx%s pixels, want the pane's 320x180", query.Get("WIDTH"), query.Get("HEIGHT"))
	}
	if query.Get("TIME") == "" || query.Get("REQUEST") != "GetMap" {
		t.Fatalf("the request is not a GetMap with a time: %s", asked.RawQuery)
	}

	// The box is around the reader, and it is as wide as the pane asked for: this is
	// the ground the picture covers, in degrees.
	box := strings.Split(query.Get("BBOX"), ",")
	if len(box) != 4 {
		t.Fatalf("BBOX = %q, want west,south,east,north", query.Get("BBOX"))
	}
	numbers := make([]float64, 4)
	for i, part := range box {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			t.Fatalf("BBOX %q is not numbers: %v", query.Get("BBOX"), err)
		}
		numbers[i] = value
	}
	west, south, east, north := numbers[0], numbers[1], numbers[2], numbers[3]
	if middle := (west + east) / 2; math.Abs(middle-(-97.7431)) > 0.05 {
		t.Fatalf("the box is centred on longitude %v, want the reader's -97.7431", middle)
	}
	if middle := (south + north) / 2; math.Abs(middle-30.2672) > 0.05 {
		t.Fatalf("the box is centred on latitude %v, want the reader's 30.2672", middle)
	}
	want := viewForTest(320, 180, 30.2672, -97.7431, 7)
	wantWest, wantSouth, wantEast, wantNorth := want.bounds()
	for _, pair := range []struct {
		name     string
		got, put float64
	}{{"west", west, wantWest}, {"south", south, wantSouth}, {"east", east, wantEast}, {"north", north, wantNorth}} {
		if math.Abs(pair.got-pair.put) > 1e-6 {
			t.Fatalf("the box's %s = %v, want %v", pair.name, pair.got, pair.put)
		}
	}
}

// viewForTest is the view a request of this shape should produce, written out here so
// the assertion above is about the request rather than about the code that made it.
func viewForTest(width, height int, lat, lon float64, zoom int) radarView {
	return radarView{latitude: lat, longitude: lon, zoom: zoom, width: width, height: height}
}

// The time in the request is the whole reason this source works: the layer declares a
// five-minute cadence with no rounding, so a time between frames is answered with a
// valid, empty picture.
func TestNEXRADFrameTimeIsFlooredAndLags(t *testing.T) {
	cases := []struct {
		now  string
		want string
	}{
		{"2026-09-18T23:47:00Z", "2026-09-18T23:40:00Z"},
		{"2026-09-18T23:44:59Z", "2026-09-18T23:35:00Z"},
		{"2026-09-18T23:50:00Z", "2026-09-18T23:45:00Z"},
		{"2026-09-19T00:02:00Z", "2026-09-18T23:55:00Z"},
	}
	for _, tc := range cases {
		now, err := time.Parse(time.RFC3339, tc.now)
		if err != nil {
			t.Fatal(err)
		}
		got := nexradFrameTime(now)
		if want, _ := time.Parse(time.RFC3339, tc.want); !got.Equal(want) {
			t.Fatalf("the frame for %s is %s, want %s", tc.now, got.Format(time.RFC3339), tc.want)
		}
		if got.Minute()%5 != 0 || got.Second() != 0 {
			t.Fatalf("the frame %s is not on a five-minute mark", got.Format(time.RFC3339))
		}
		if now.Sub(got) < nexradLatency {
			t.Fatalf("the frame for %s is only %v old, want it to lag by the service's own latency", tc.now, now.Sub(got))
		}
	}
}

// The way in routes to the service that covers the place: a coordinate in the
// composite's coverage never touches the tile index, and one outside it never touches
// the composite.
func TestRadarRoutesToTheServiceThatCoversThePlace(t *testing.T) {
	compositeAsked, indexAsked := 0, 0
	composite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compositeAsked++
		width, _ := strconv.Atoi(r.URL.Query().Get("WIDTH"))
		height, _ := strconv.Atoi(r.URL.Query().Get("HEIGHT"))
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, max(width, 1), max(height, 1)))) //nolint:errcheck
	}))
	t.Cleanup(composite.Close)
	restoreEndpoint := nexradEndpoint
	nexradEndpoint = composite.URL
	t.Cleanup(func() { nexradEndpoint = restoreEndpoint })

	tiles := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			indexAsked++
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"host": "http://" + r.Host,
				"radar": map[string]any{"past": []map[string]any{
					{"time": 1789767600, "path": "/v2/radar/newest"},
				}},
			})
			return
		}
		indexAsked++
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, radarTileSize, radarTileSize))) //nolint:errcheck
	}))
	t.Cleanup(tiles.Close)
	restoreIndex := radarIndexURL
	radarIndexURL = tiles.URL + "/public/weather-maps.json"
	t.Cleanup(func() { radarIndexURL = restoreIndex })

	fetch := Radar(RadarOptions{Latitude: 30.2672, Longitude: -97.7431, Zoom: 5, Width: 256, Height: 128})
	if _, err := fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if compositeAsked != 1 || indexAsked != 0 {
		t.Fatalf("a United States coordinate asked the composite %d times and the tiles %d times", compositeAsked, indexAsked)
	}

	fetch = Radar(RadarOptions{Latitude: 51.5072, Longitude: -0.1276, Zoom: 5, Width: 256, Height: 128})
	if _, err := fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if compositeAsked != 1 || indexAsked == 0 {
		t.Fatalf("a coordinate outside the coverage asked the composite %d times and the tiles %d times", compositeAsked, indexAsked)
	}
}
