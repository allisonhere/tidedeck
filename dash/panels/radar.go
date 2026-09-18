package panels

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	// quiet is true when the newest frame has nothing in it. An empty sky and a
	// broken panel look identical, so the panel is the only thing that can tell
	// the reader which one this is.
	quiet bool
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
	r.mu.Lock()
	r.quiet = radarEcho(frame.Image) < radarQuietEcho
	r.mu.Unlock()
	r.Store(frame)
	return nil
}

// radarQuietEcho is the echo below which the panel says the sky is empty. It is
// deliberately low - a thin squall line is a few pixels of a 512 px frame and is
// exactly what the panel is for - and the picture is drawn either way, so a frame
// under the bar is never hidden, only explained.
const radarQuietEcho = 0.002

// radarEcho is the fraction of a frame with anything in it. Sampled rather than
// exhaustive: a 512x512 frame is 262k pixels, and this only decides whether the
// panel adds a sentence.
func radarEcho(img image.Image) float64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return 0
	}
	const step = 4
	lit, total := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			_, _, _, alpha := img.At(x, y).RGBA()
			total++
			if alpha > 0 {
				lit++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(lit) / float64(total)
}

func (r *radar) View(ctx tideui.PanelContext) string {
	frame := r.Load()
	if frame.Image == nil {
		return r.emptyView(ctx)
	}
	r.mu.Lock()
	location, zoom, quiet := r.location, r.zoom, r.quiet
	r.mu.Unlock()
	if location == "" {
		location = "local"
	}

	bg := ctx.Renderer.Styles.Workspace.Bg
	// A picture of the weather with no time on it is a picture of a rumour, and
	// the service is credited because it asks to be.
	lines := []string{radarCaption(ctx.Width, frame.Time.Local().Format("15:04"), location, zoom)}
	if quiet {
		// Nothing in the frame, and the panel says which kind of nothing: an
		// empty sky and a failed fetch draw the same rectangle otherwise.
		lines = append(lines, "no precipitation in range")
	}
	// The picture is drawn with the reader's own position marked. The middle of
	// the tile is where they are, and without it a lone echo is a smudge on dark
	// glass: no telling weather one county over from weather two states away.
	marked := markCentre(frame.Image, tideui.RGBAOf(ctx.Renderer.Styles.Workspace.BodyFg))
	picture := ctx.Renderer.RenderImage(marked, ctx.Width, max(1, ctx.Height-len(lines)))
	// A frame that draws nothing is still a frame: the caption goes up either way,
	// because "nothing is falling" and "nothing has loaded" are different things
	// and only the caption and the sentence above tell them apart.
	if strings.TrimSpace(picture) != "" {
		lines = append(lines, strings.Split(picture, "\n")...)
	}
	return ctx.Renderer.RenderLines(lines, ctx.Width, bg)
}

// radarCaption is the panel's context line, dropped in order of stubbornness as
// the pane narrows: the time and the source stay, the zoom level goes first and
// the place before it. A credit the layout truncated away is not a credit.
func radarCaption(width int, when, location string, zoom int) string {
	parts := []string{when, location, fmt.Sprintf("zoom %d", zoom), "RainViewer"}
	for len(parts) > 2 && ansi.StringWidth(strings.Join(parts, " · ")) > width {
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
	}
	return strings.Join(parts, " · ")
}

// markCentre draws the reader into the picture, on a copy: the frame in state is
// the one the next draw reuses.
func markCentre(img image.Image, colour color.RGBA) image.Image {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return img
	}
	marked := image.NewRGBA(bounds)
	draw.Draw(marked, bounds, img, bounds.Min, draw.Src)

	centreX, centreY := bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2
	// Sized from the frame, so the mark survives being scaled into the pane and
	// stays a crosshair rather than becoming a blob or a single invisible pixel.
	arm := max(2, bounds.Dx()/48)
	thickness := max(1, bounds.Dx()/256)
	for offset := -arm; offset <= arm; offset++ {
		for row := 0; row < thickness; row++ {
			marked.Set(centreX+offset, centreY+row, colour)
			marked.Set(centreX+row, centreY+offset, colour)
		}
	}
	return marked
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
