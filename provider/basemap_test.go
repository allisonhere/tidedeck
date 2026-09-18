package provider

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The URL is each service's own shape, and both put the row before the column - the
// latitude-ish number first, the opposite order to the radar's tiles. Getting that the
// wrong way round does not fail: it fetches a valid tile of somewhere else, which is why
// it is asserted here.
func TestBasemapTileURLsAreRowThenColumn(t *testing.T) {
	imagery := BasemapLayers["night lights"].tileURL(8, 58, 105)
	wantImagery := "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/VIIRS_CityLights_2012/default/GoogleMapsCompatible_Level8/8/105/58.jpg"
	if imagery != wantImagery {
		t.Fatalf("imagery tile url =\n  %s\nwant\n  %s", imagery, wantImagery)
	}
	// A map service carries the zoom in the path, not in a level set.
	topographic := BasemapLayers["topographic"].tileURL(9, 135, 199)
	wantTopographic := "https://basemap.nationalmap.gov/arcgis/rest/services/USGSTopo/MapServer/tile/9/199/135"
	if topographic != wantTopographic {
		t.Fatalf("map tile url =\n  %s\nwant\n  %s", topographic, wantTopographic)
	}
}

// basemapServer serves a tile whose colour says which tile it is, so a test can tell
// the block from a single fetch, and records what was asked for. The tiles are not one
// flat colour, because a flat tile is how the service says "nothing here"; flat is
// marked separately, by path suffix.
func basemapServer(t *testing.T, flatSuffix string) *[]string {
	t.Helper()
	asked := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.Path)
		img := image.NewRGBA(image.Rect(0, 0, basemapTilePixels, basemapTilePixels))
		// An empty suffix matches everything, which would make every tile "no data":
		// a helper that quietly turned the whole block flat is a trap, so it is spelt out.
		if flatSuffix != "" && strings.HasSuffix(r.URL.Path, flatSuffix) {
			for y := 0; y < basemapTilePixels; y++ {
				for x := 0; x < basemapTilePixels; x++ {
					img.Set(x, y, color.RGBA{R: 42, G: 42, B: 42, A: 255})
				}
			}
		} else {
			// The last two segments of the path are the row, then the column.
			trimmed := strings.TrimSuffix(r.URL.Path, ".jpg")
			segments := strings.Split(trimmed, "/")
			column, _ := strconv.Atoi(segments[len(segments)-1])
			paint := color.RGBA{R: 200, A: 255}
			if column%2 == 1 {
				paint = color.RGBA{G: 200, A: 255}
			}
			for y := 0; y < basemapTilePixels; y++ {
				for x := 0; x < basemapTilePixels; x++ {
					// A variation per pixel, so the tile is imagery and not a slab.
					shade := uint8((x + y) % 20)
					img.Set(x, y, color.RGBA{R: paint.R + shade/2, G: paint.G + shade/2, B: shade / 2, A: 255})
				}
			}
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img) //nolint:errcheck
	}))
	t.Cleanup(server.Close)
	restore := basemapHost
	basemapHost = server.URL
	t.Cleanup(func() { basemapHost = restore })
	return asked
}

// The map comes back at the pane's own pixels over the pane's own ground: that is what
// lets it line up with the radar at any zoom, rather than only when the two tile grids
// happen to agree.
func TestBasemapCoversTheViewAtThePanesOwnSize(t *testing.T) {
	asked := basemapServer(t, "/nothing-matches-this.jpg")

	fetch := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Width: 320, Height: 180,
		Layer: "night lights",
	})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Image.Bounds(); got.Dx() != 320 || got.Dy() != 180 {
		t.Fatalf("the map came back %v, want the pane's 320x180", got)
	}
	if frame.Centre != image.Pt(160, 90) {
		t.Fatalf("the reader is at %v in a 320x180 picture, want the middle", frame.Centre)
	}
	if frame.KilometresPerPixel != kilometresPerPixel(30.2672, 7) {
		t.Fatalf("a map pixel is %v km, want the view's own scale", frame.KilometresPerPixel)
	}
	// The imagery is asked for at its own zoom whatever the view's zoom is, in the row
	// then column order, and the block is bounded: a view that would need a continent
	// of tiles is refused rather than fetched.
	if len(*asked) == 0 {
		t.Fatal("no tiles were asked for")
	}
	if len(*asked) > basemapMaxTiles {
		t.Fatalf("asked for %d tiles, more than the budget of %d", len(*asked), basemapMaxTiles)
	}
	for _, path := range *asked {
		if !strings.Contains(path, "/GoogleMapsCompatible_Level8/8/") {
			t.Fatalf("asked for %q, want the imagery's own level 8", path)
		}
		if !strings.HasSuffix(path, ".jpg") {
			t.Fatalf("asked for %q, want the imagery's own format", path)
		}
	}
}

// A view of a continent is not a view an imagery layer with one fixed zoom can cover:
// the panel draws the radar alone, which is honest, rather than fetching hundreds of tiles.
func TestBasemapRefusesAViewTooWideForAFixedZoom(t *testing.T) {
	basemapServer(t, "/nothing-matches-this.jpg")
	fetch := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 1, Width: 1024, Height: 1024,
		Layer: "night lights",
	})
	if _, err := fetch(context.Background()); err == nil {
		t.Fatal("a continent-wide view was fetched rather than refused")
	}
}

// A map service serves a pyramid, so the zoom is chosen for the pane and the block stays
// inside the budget whatever the pane is looking at.
func TestBasemapPyramidZoomFitsTheBudget(t *testing.T) {
	cases := []struct {
		name        string
		zoom, width int
		height      int
	}{
		{"a pane over a metro area", 7, 826, 68},
		{"the same pane, twice as wide", 7, 1980, 85},
		{"a whole state", 5, 826, 68},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := radarView{latitude: 36.66, longitude: -84.4, zoom: tc.zoom, width: tc.width, height: tc.height}
			zoom := basemapPyramidZoom(view)
			if zoom < 0 || zoom > 16 {
				t.Fatalf("chose zoom %d, which no service serves", zoom)
			}
			west, south, east, north := view.bounds()
			if tiles := basemapGridFor(west, south, east, north, zoom).tiles(); tiles > basemapMaxTiles {
				t.Fatalf("chose zoom %d, which is %d tiles for this view", zoom, tiles)
			}
		})
	}
}

// The automatic choice is a map where one covers the place, and nothing where none does.
func TestBasemapAutoChoosesAMapForThePlace(t *testing.T) {
	for _, place := range []struct {
		name     string
		lat, lon float64
		want     string
	}{
		{"Kentucky", 36.66, -84.4, "topographic"},
		{"Austin", 30.2672, -97.7431, "topographic"},
		{"London", 51.5072, -0.1276, ""},
		{"Sydney", -33.8688, 151.2093, ""},
	} {
		if got := BasemapLayerFor(place.lat, place.lon); got != place.want {
			t.Fatalf("%s: the map for the place = %q, want %q", place.name, got, place.want)
		}
	}
	// The default setting is the automatic one, and the automatic one resolves for the
	// place - so the panel's default is never an unknown name.
	if BasemapDefaultLayer != BasemapAutoLayer {
		t.Fatalf("the default layer = %q, want the automatic choice", BasemapDefaultLayer)
	}
	topographic, known := BasemapLayers["topographic"]
	if !known {
		t.Fatal("there is no topographic map to choose")
	}
	if !topographic.Paper || !topographic.USOnly || topographic.Credit == "" {
		t.Fatalf("the topographic layer is not described as a paper map of the United States: %+v", topographic)
	}
}

// Ground the service has no imagery for comes back as one flat colour rather than a
// 404, so it is left out of the picture instead of being painted as though it were a
// map. Here every tile has imagery: nothing is transparent.
func TestBasemapLeavesOutATileWithNothingInIt(t *testing.T) {
	basemapServer(t, "/nothing-matches-this.jpg")
	frame, err := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Width: 256, Height: 128,
		Layer: "night lights",
	})(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < frame.Image.Bounds().Dy(); y++ {
		for x := 0; x < frame.Image.Bounds().Dx(); x++ {
			if _, _, _, alpha := frame.Image.At(x, y).RGBA(); alpha == 0 {
				t.Fatalf("a tile with imagery in it was left out at %d,%d", x, y)
			}
		}
	}
}

// And when the service answers a flat colour for every tile, the picture is empty
// rather than a grey slab: what the panel draws there is its own background.
func TestBasemapDrawsNothingWhereTheImageryIsEmpty(t *testing.T) {
	flat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, basemapTilePixels, basemapTilePixels))
		for y := 0; y < basemapTilePixels; y++ {
			for x := 0; x < basemapTilePixels; x++ {
				img.Set(x, y, color.RGBA{R: 42, G: 42, B: 42, A: 255})
			}
		}
		png.Encode(w, img) //nolint:errcheck
	}))
	t.Cleanup(flat.Close)
	restore := basemapHost
	basemapHost = flat.URL
	t.Cleanup(func() { basemapHost = restore })

	frame, err := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Width: 256, Height: 128,
		Layer: "night lights",
	})(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if frame.Image == nil {
		t.Fatal("a view of nothing should still be a picture of nothing, not no picture")
	}
	for y := 0; y < frame.Image.Bounds().Dy(); y++ {
		for x := 0; x < frame.Image.Bounds().Dx(); x++ {
			if _, _, _, alpha := frame.Image.At(x, y).RGBA(); alpha != 0 {
				t.Fatalf("a flat tile was drawn at %d,%d", x, y)
			}
		}
	}
}

// A dark tile is imagery, not an absence: the night-lights layer is full of tiles that
// are one flat black value over countryside, and calling those empty leaves half a map
// transparent. This is the regression test for exactly that.
func TestBasemapDrawsADarkTileRatherThanCallingItEmpty(t *testing.T) {
	dark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, basemapTilePixels, basemapTilePixels))
		for y := 0; y < basemapTilePixels; y++ {
			for x := 0; x < basemapTilePixels; x++ {
				img.Set(x, y, color.RGBA{A: 255}) // one flat black value
			}
		}
		png.Encode(w, img) //nolint:errcheck
	}))
	t.Cleanup(dark.Close)
	restore := basemapHost
	basemapHost = dark.URL
	t.Cleanup(func() { basemapHost = restore })

	frame, err := Basemap(BasemapOptions{
		Latitude: 36.66, Longitude: -84.4, Zoom: 7, Width: 256, Height: 128,
		Layer: "night lights",
	})(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < frame.Image.Bounds().Dy(); y++ {
		for x := 0; x < frame.Image.Bounds().Dx(); x++ {
			if _, _, _, alpha := frame.Image.At(x, y).RGBA(); alpha == 0 {
				t.Fatalf("a dark tile was treated as empty at %d,%d", x, y)
			}
		}
	}
}

// A layer name the provider does not know is an error, not a silent fallback: a typo in
// a setting should be visible.
func TestBasemapRefusesALayerItDoesNotKnow(t *testing.T) {
	fetch := Basemap(BasemapOptions{Latitude: 1, Longitude: 2, Zoom: 7, Layer: "street map"})
	if _, err := fetch(context.Background()); err == nil {
		t.Fatal("an unknown basemap layer was not an error")
	}
}

// Every layer offered is a name a setting can use, a layer its service serves, and a
// credit naming it.
func TestBasemapLayersAreNamedForPeople(t *testing.T) {
	if len(BasemapLayers) == 0 {
		t.Fatal("no basemap layers are offered")
	}
	for name, layer := range BasemapLayers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(layer.Layer) == "" || strings.TrimSpace(layer.Credit) == "" {
			t.Fatalf("layer %q -> %+v is not usable", name, layer)
		}
		if name != strings.ToLower(name) {
			t.Fatalf("layer name %q is not lowercase, which the lookup would miss", name)
		}
		if (layer.Service == "") == (layer.Zoom == 0) {
			t.Fatalf("layer %q is neither a pyramid nor a fixed-zoom layer: %+v", name, layer)
		}
	}
}
