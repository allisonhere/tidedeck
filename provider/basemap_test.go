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

// The URL is the service's own shape: the level set carries the zoom, and the
// numbers are row then column - the latitude-ish number first, the opposite order
// to the radar's tiles. Getting that the wrong way round does not fail: it fetches
// a valid tile of somewhere else, which is why it is asserted here.
func TestBasemapTileURLIsRowThenColumn(t *testing.T) {
	got := basemapTileURL("VIIRS_CityLights_2012", 8, 58, 105)
	want := "https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/VIIRS_CityLights_2012/default/GoogleMapsCompatible_Level8/8/105/58.jpg"
	if got != want {
		t.Fatalf("basemap tile url =\n  %s\nwant\n  %s", got, want)
	}
}

// basemapServer serves a tile whose colour says which quarter of a radar tile it
// is, so a test can tell stitching from merely fetching. The tiles are not a flat
// colour, because a flat tile is how the service says "nothing here" - flat is
// marked separately, by path suffix.
func basemapServer(t *testing.T, flatSuffix string) (*httptest.Server, *[]string) {
	t.Helper()
	asked := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.Path)
		img := image.NewRGBA(image.Rect(0, 0, basemapTilePixels, basemapTilePixels))
		if strings.HasSuffix(r.URL.Path, flatSuffix) {
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
			row, _ := strconv.Atoi(segments[len(segments)-2])
			paint := color.RGBA{A: 255}
			switch [2]int{column % 2, row % 2} {
			case [2]int{0, 0}:
				paint.R = 220
			case [2]int{1, 0}:
				paint.G = 220
			case [2]int{0, 1}:
				paint.B = 220
			default:
				paint.R, paint.G = 220, 220
			}
			for y := 0; y < basemapTilePixels; y++ {
				for x := 0; x < basemapTilePixels; x++ {
					// A variation per pixel, so the tile is imagery and not a slab.
					shade := uint8((x + y) % 20)
					img.Set(x, y, color.RGBA{R: paint.R + shade/2, G: paint.G + shade/2, B: paint.B + shade/2, A: 255})
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
	return server, asked
}

// The basemap covers exactly the ground the radar does: a GIBS tile is half the
// width of a radar tile, so each radar tile is a 2x2 block of them and both
// pictures come out the same number of pixels over the same place.
func TestBasemapIsATwoByTwoBlockPerRadarTile(t *testing.T) {
	_, asked := basemapServer(t, "/nothing-matches-this.jpg")

	fetch := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Cols: 2, Rows: 1, Layer: "night lights",
	})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Image.Bounds().Dx(); got != 2*radarTileSize {
		t.Fatalf("the basemap is %d px wide, want %d", got, 2*radarTileSize)
	}
	if got := frame.Image.Bounds().Dy(); got != radarTileSize {
		t.Fatalf("the basemap is %d px tall, want %d", got, radarTileSize)
	}
	if len(*asked) != 8 {
		t.Fatalf("a 2x1 radar block asked for %d tiles, want 8 (four per radar tile)", len(*asked))
	}
	// Each quarter of a radar tile holds its own tile, so the block is stitched in
	// the right order rather than merely fetched.
	quarter := func(quadrantX, quadrantY int) color.RGBA {
		r, g, b, _ := frame.Image.At(
			quadrantX*basemapTilePixels+basemapTilePixels/2,
			quadrantY*basemapTilePixels+basemapTilePixels/2,
		).RGBA()
		return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
	}
	northWest, northEast, southWest, southEast := quarter(0, 0), quarter(1, 0), quarter(0, 1), quarter(1, 1)
	if northWest.R <= northWest.G || northWest.R <= northWest.B {
		t.Fatalf("the north-west quarter = %v, want the red tile", northWest)
	}
	if northEast.G <= northEast.R || northEast.G <= northEast.B {
		t.Fatalf("the north-east quarter = %v, want the green tile", northEast)
	}
	if southWest.B <= southWest.R || southWest.B <= southWest.G {
		t.Fatalf("the south-west quarter = %v, want the blue tile", southWest)
	}
	if southEast.R <= southEast.B || southEast.G <= southEast.B {
		t.Fatalf("the south-east quarter = %v, want the red-and-green tile", southEast)
	}
	// The reader's own place is on it, and a pixel of it is worth the radar's own
	// ground, so a panel can draw its crosshair and its ring on either picture.
	if !frame.Centre.In(frame.Image.Bounds()) {
		t.Fatalf("the basemap's centre %v is not on the picture", frame.Centre)
	}
	if frame.KilometresPerPixel != kilometresPerPixel(30.2672, 7) {
		t.Fatalf("a basemap pixel is %v km, want the radar's own scale", frame.KilometresPerPixel)
	}
}

// Ground the service has no imagery for comes back as one flat colour rather than a
// 404, so it is left out of the picture instead of being painted as though it were
// a map.
func TestBasemapLeavesOutATileWithNothingInIt(t *testing.T) {
	// Austin at zoom 7 is tile 29/52, so its 2x2 block at zoom 8 starts at 58/104:
	// the north-west tile is the one marked flat here.
	_, asked := basemapServer(t, "/8/104/58.jpg")

	fetch := Basemap(BasemapOptions{
		Latitude: 30.2672, Longitude: -97.7431, Zoom: 7, Cols: 1, Rows: 1, Layer: "night lights",
	})
	frame, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 4 {
		t.Fatalf("asked for %d tiles, want four", len(*asked))
	}
	if _, _, _, alpha := frame.Image.At(10, 10).RGBA(); alpha != 0 {
		t.Fatal("a tile with nothing in it was drawn as though it were a map")
	}
	if _, _, _, alpha := frame.Image.At(basemapTilePixels+10, 10).RGBA(); alpha == 0 {
		t.Fatal("a tile with imagery in it was left out")
	}
}

// A layer name the provider does not know is an error, not a silent fallback: a
// typo in a setting should be visible.
func TestBasemapRefusesALayerItDoesNotKnow(t *testing.T) {
	fetch := Basemap(BasemapOptions{Latitude: 1, Longitude: 2, Zoom: 7, Layer: "street map"})
	if _, err := fetch(context.Background()); err == nil {
		t.Fatal("an unknown basemap layer was not an error")
	}
}

// Every layer offered is a name a setting can use and a layer the service serves,
// and the default is one of them.
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
