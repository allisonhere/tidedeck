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

// The URL is the service's own shape: the level set carries the zoom, and the numbers
// are row then column - the latitude-ish number first, the opposite order to the
// radar's tiles. Getting that the wrong way round does not fail: it fetches a valid
// tile of somewhere else, which is why it is asserted here.
func TestBasemapTileURLIsRowThenColumn(t *testing.T) {
	got := basemapTileURL("VIIRS_CityLights_2012", 58, 105)
	want := "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/VIIRS_CityLights_2012/default/GoogleMapsCompatible_Level8/8/105/58.jpg"
	if got != want {
		t.Fatalf("basemap tile url =\n  %s\nwant\n  %s", got, want)
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

// A view of a continent is not a view a fixed-resolution map can cover: the panel draws
// the radar alone, which is honest, rather than fetching hundreds of tiles.
func TestBasemapRefusesAViewTooWideToMap(t *testing.T) {
	basemapServer(t, "/nothing-matches-this.jpg")
	fetch := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 1, Width: 1024, Height: 1024,
		Layer: "night lights",
	})
	if _, err := fetch(context.Background()); err == nil {
		t.Fatal("a continent-wide view was fetched rather than refused")
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

// A layer name the provider does not know is an error, not a silent fallback: a typo in
// a setting should be visible.
func TestBasemapRefusesALayerItDoesNotKnow(t *testing.T) {
	fetch := Basemap(BasemapOptions{Latitude: 1, Longitude: 2, Zoom: 7, Layer: "street map"})
	if _, err := fetch(context.Background()); err == nil {
		t.Fatal("an unknown basemap layer was not an error")
	}
}

// Every layer offered is a name a setting can use and a layer the service serves, and
// the default is one of them.
func TestBasemapLayersAreNamedForPeople(t *testing.T) {
	if len(BasemapLayers) == 0 {
		t.Fatal("no basemap layers are offered")
	}
	for name, layer := range BasemapLayers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(layer) == "" {
			t.Fatalf("layer %q -> %q is not usable", name, layer)
		}
		if name != strings.ToLower(name) {
			t.Fatalf("layer name %q is not lowercase, which the lookup would miss", name)
		}
	}
	if _, known := BasemapLayers[BasemapDefaultLayer]; !known {
		t.Fatalf("the default layer %q is not one of the layers offered", BasemapDefaultLayer)
	}
}
