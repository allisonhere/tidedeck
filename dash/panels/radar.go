package panels

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// Radar builds the radar panel. Like the weather panel it has nowhere to look
// until it is told where - and where is the weather panel's own location, so the
// place search fills both panels at once.
func Radar() dash.Panel {
	return &radar{newFetcher: provider.Radar}
}

type radar struct {
	dash.State[tideui.RadarFrame]

	mu         sync.Mutex
	fetch      func(context.Context) (tideui.RadarFrame, error)
	newFetcher func(provider.RadarOptions) func(context.Context) (tideui.RadarFrame, error)
	location   string
	zoom       int
}

const (
	radarEnabledKey = "radar.enabled"
	radarZoomKey    = "radar.zoom"
)

func (r *radar) Meta() dash.Meta {
	return dash.Meta{
		ID: "radar", Title: "Radar", Subtitle: "rain",
		Role: tideui.RoleSecondary, Priority: 72,
		MinWidth: 24, MinHeight: 8,
		Interval: 5 * time.Minute,
	}
}

// AlwaysLive keeps the panel fetching in the deck's demo mode too. A radar frame
// is a photograph of the sky at a moment: there is no honest sample of one, and a
// demo would be a fabricated storm. It is the same argument the plugin panels
// make - a real source, whatever mode the dashboard is in.
func (r *radar) AlwaysLive() bool { return true }

// Schema declares only what is the radar's own. The coordinates are the weather
// panel's: one location, not two copies that drift apart.
func (r *radar) Schema() []dash.Field {
	return []dash.Field{
		{Key: radarEnabledKey, Label: "live radar", Kind: dash.FieldBool, Default: "true",
			Description: "Uses the Weather panel's location."},
		{Key: radarZoomKey, Label: "detail", Kind: dash.FieldFloat, Default: strconv.Itoa(provider.RadarDefaultZoom),
			Description: "Zoom 4 is a state, 7 is a metro area and its surroundings. 7 is as deep as the service's data goes.",
			Min:         3, Max: float64(provider.RadarMaxZoom), Step: 1},
	}
}

func (r *radar) Configure(values dash.Values) error {
	// The place the weather panel is looking at, read rather than duplicated:
	// looking up a city once should move both panels.
	latitude := values.Float(weatherLatitudeKey)
	longitude := values.Float(weatherLongitudeKey)
	location := strings.TrimSpace(values.String(weatherLocationKey))

	r.mu.Lock()
	defer r.mu.Unlock()
	r.location = location
	r.zoom = int(values.Float(radarZoomKey))
	if r.zoom == 0 {
		r.zoom = provider.RadarDefaultZoom
	}
	if !boolOr(values, radarEnabledKey, true) || (latitude == 0 && longitude == 0) {
		r.fetch = nil
		return nil
	}
	r.fetch = r.newFetcher(provider.RadarOptions{Latitude: latitude, Longitude: longitude, Zoom: r.zoom})
	return nil
}

func (r *radar) Refresh(ctx context.Context) error {
	r.mu.Lock()
	fetch := r.fetch
	r.mu.Unlock()
	if fetch == nil {
		return nil
	}
	frame, err := fetch(ctx)
	if err != nil {
		return err
	}
	r.Store(frame)
	return nil
}

func (r *radar) View(ctx tideui.PanelContext) string {
	frame := r.Load()
	if frame.Image == nil {
		return r.emptyView(ctx)
	}
	r.mu.Lock()
	location, zoom := r.location, r.zoom
	r.mu.Unlock()
	if location == "" {
		location = "local"
	}

	bg := ctx.Renderer.Styles.Workspace.Bg
	height := max(1, ctx.Height-2)
	picture := ctx.Renderer.RenderImage(frame.Image, ctx.Width, height)
	if strings.TrimSpace(picture) == "" {
		return r.emptyView(ctx)
	}
	// A picture of the weather with no time on it is a picture of a rumour, and
	// the service is credited because it asks to be.
	caption := fmt.Sprintf("%s · %s · zoom %d · RainViewer",
		frame.Time.Local().Format("15:04"), location, zoom)
	lines := append([]string{caption}, strings.Split(picture, "\n")...)
	return ctx.Renderer.RenderLines(lines, ctx.Width, bg)
}

// emptyView says what is missing instead of drawing an empty box. A radar with
// nowhere to look is not a picture of nothing; it is a panel waiting for the
// weather panel to be told where you are, so it says that. It mirrors the weather
// panel's own empty view (dash/panels/weather.go:142).
func (r *radar) emptyView(ctx tideui.PanelContext) string {
	r.mu.Lock()
	configured := r.fetch != nil
	r.mu.Unlock()
	message := "No location set · set one in Weather · press s"
	if configured {
		message = "Loading…"
	}
	return ctx.Renderer.RenderLines([]string{message}, ctx.Width,
		ctx.Renderer.Styles.Workspace.Bg)
}
