package provider

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/allisonhere/tideui"
)

// nexradEndpoint is the Iowa Environmental Mesonet's time-aware WMS: the NWS NEXRAD
// level III base reflectivity composite for the continental United States, rendered
// by the service into whatever box and pixel size is asked for. One request per
// frame - no tiles, no mosaic, no grid to line up - and the picture comes back the
// size of the pane, which is why a pane of any shape is filled rather than fitted
// into.
//
// It is a var so a test can point it at a server of its own.
var nexradEndpoint = "https://mesonet.agron.iastate.edu/cgi-bin/wms/nexrad/n0q-t.cgi"

// nexradLayer is the service's time-aware composite layer. It declares one, with the
// time extent 2011-02-16/2026-12-31 at PT5M and nearestValue=0 - so a time has to be
// an exact five-minute mark, and a time that is not one is answered with a valid,
// empty picture rather than an error.
const nexradLayer = "nexrad-n0q-wmst"

// nexradStep is the composite's cadence and nexradLatency is how far the newest frame
// lags real time: the composite is published about a step behind, so asking for "now"
// is asking for a frame that has not been composited yet.
const (
	nexradStep    = 5 * time.Minute
	nexradLatency = 5 * time.Minute
)

// nexradFrameTime is the newest frame the service is likely to have, floored to its
// cadence. Both halves matter: unfloored, the request asks for a moment between
// frames; unlagged, for one that does not exist yet. Either way the service answers a
// perfectly valid, perfectly empty PNG.
func nexradFrameTime(now time.Time) time.Time {
	return now.UTC().Add(-nexradLatency).Truncate(nexradStep)
}

// nexradURL is the service's own shape: GetMap with a geographic box, the
// pixels to render it into, and the moment to render.
func nexradURL(bounds [4]float64, width, height int, at time.Time) string {
	return fmt.Sprintf("%s?SERVICE=WMS&VERSION=1.1.1&REQUEST=GetMap&LAYERS=%s"+
		"&STYLES=&FORMAT=image/png&TRANSPARENT=true&SRS=EPSG:4326"+
		"&BBOX=%.6f,%.6f,%.6f,%.6f&WIDTH=%d&HEIGHT=%d&TIME=%s",
		nexradEndpoint, nexradLayer,
		bounds[0], bounds[1], bounds[2], bounds[3], width, height,
		at.UTC().Format("2006-01-02T15:04:05Z"))
}

// radarNEXRAD fetches one composite for the view, at the view's own size.
func radarNEXRAD(ctx context.Context, opts RadarOptions) (tideui.RadarFrame, error) {
	view := radarViewOf(opts)
	west, south, east, north := view.bounds()
	at := nexradFrameTime(time.Now())
	picture, err := fetchTile(ctx, nexradURL([4]float64{west, south, east, north}, view.width, view.height, at))
	if err != nil {
		return tideui.RadarFrame{}, err
	}
	bounds := picture.Bounds()
	return tideui.RadarFrame{
		Time: at,
		// A composite with no weather is a fully transparent picture rather than an
		// error: an empty sky is a fact, and the panel has its own way of saying so.
		Image: picture,
		// The box was asked for around the reader, so the reader is the middle of
		// whatever came back.
		Centre:             image.Pt(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2),
		KilometresPerPixel: kilometresPerPixel(opts.Latitude, view.zoom),
	}, nil
}
