package panels

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// Configuration keys this panel owns. They are the keys already in the file,
// under the "weather" object it has always used.
const (
	weatherEnabledKey    = "weather.enabled"
	weatherLatitudeKey   = "weather.latitude"
	weatherLongitudeKey  = "weather.longitude"
	weatherLocationKey   = "weather.location"
	weatherFahrenheitKey = "weather.fahrenheit"
	weatherWindMPHKey    = "weather.wind_mph"
)

// weather shows the current conditions and a forecast for one place.
type weather struct {
	dash.State[tideui.WeatherData]

	mu    sync.Mutex
	fetch func(context.Context) (tideui.WeatherData, error)
	// unit is the unit the reading is displayed in, which the panel's own key
	// flips. It is display-only: the configured unit is what gets requested
	// and what survives a restart.
	unit string
	// newFetcher is the constructor, so a test can drive the panel without
	// reaching the network.
	newFetcher func(provider.WeatherOptions) func(context.Context) (tideui.WeatherData, error)
}

// Weather builds the weather panel. It has nothing to fetch until it is told
// where to look, which is why Configure, not this, builds the source.
func Weather() dash.Panel {
	return &weather{unit: "F", newFetcher: provider.Weather}
}

func (w *weather) Meta() dash.Meta {
	return dash.Meta{
		ID: "weather", Title: "Weather",
		Role: tideui.RolePrimary, Priority: 90,
		MinWidth: 18, MinHeight: 7,
		Interval: 10 * time.Minute,
	}
}

func (w *weather) Schema() []dash.Field {
	return []dash.Field{
		{Key: weatherEnabledKey, Label: "live weather", Kind: dash.FieldBool, Default: "true"},
		{Key: weatherLatitudeKey, Label: "latitude", Kind: dash.FieldFloat},
		{Key: weatherLongitudeKey, Label: "longitude", Kind: dash.FieldFloat},
		{Key: weatherLocationKey, Label: "location", Kind: dash.FieldText, Default: "Local"},
		{Key: weatherFahrenheitKey, Label: "fahrenheit", Kind: dash.FieldBool, Default: "true"},
		{Key: weatherWindMPHKey, Label: "wind mph", Kind: dash.FieldBool, Default: "true"},
	}
}

// Configure rebuilds the source. A panel with no coordinates has nowhere to
// look, so it gets no fetcher at all rather than a fetcher that asks the API
// about the Gulf of Guinea.
func (w *weather) Configure(values dash.Values) error {
	options := provider.WeatherOptions{
		Latitude:   values.Float(weatherLatitudeKey),
		Longitude:  values.Float(weatherLongitudeKey),
		Location:   strings.TrimSpace(values.String(weatherLocationKey)),
		Fahrenheit: boolOr(values, weatherFahrenheitKey, true),
		WindMPH:    boolOr(values, weatherWindMPHKey, true),
	}
	if options.Location == "" {
		options.Location = "Local"
	}
	unit := "C"
	if options.Fahrenheit {
		unit = "F"
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.unit = unit
	if !boolOr(values, weatherEnabledKey, true) || (options.Latitude == 0 && options.Longitude == 0) {
		w.fetch = nil
		return nil
	}
	w.fetch = w.newFetcher(options)
	return nil
}

// boolOr reads a boolean that has a default of true. An unset key means the
// default applies, and Bool alone cannot tell that from a deliberate false.
func boolOr(values dash.Values, key string, fallback bool) bool {
	if values.Has(key) {
		return values.Bool(key)
	}
	return fallback
}

func (w *weather) Refresh(ctx context.Context) error {
	w.mu.Lock()
	fetch := w.fetch
	w.mu.Unlock()
	if fetch == nil {
		return nil // nowhere configured to look; the panel says so itself
	}
	data, err := fetch(ctx)
	if err != nil {
		return err
	}
	w.Store(data)
	return nil
}

func (w *weather) View(ctx tideui.PanelContext) string {
	data := w.Load()
	if data.Condition == "" && data.Unit == "" {
		return w.emptyView(ctx)
	}
	w.mu.Lock()
	unit := w.unit
	w.mu.Unlock()
	data = convertWeatherUnit(data, unit)
	if ctx.Zoomed {
		return ctx.Renderer.RenderWeatherDetail(data, ctx.Width)
	}
	return ctx.Renderer.RenderWeather(data, ctx.Width)
}

// emptyView says what is missing instead of drawing a convincing 0°F. Before
// the panel owned its own data it rendered a zero-valued reading, which looks
// like a forecast rather than like a panel waiting to be told where it is.
func (w *weather) emptyView(ctx tideui.PanelContext) string {
	w.mu.Lock()
	configured := w.fetch != nil
	w.mu.Unlock()
	message := "No location set · press s"
	if configured {
		message = "Loading…"
	}
	return ctx.Renderer.RenderLines([]string{message}, ctx.Width,
		ctx.Renderer.Styles.Workspace.Bg)
}

func (w *weather) Actions() []dash.Action {
	return []dash.Action{
		{ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
			Run: func() string { return "weather refreshing…" }},
		{ID: "units", Key: "u", Label: "units", Run: w.toggleUnit},
	}
}

// toggleUnit flips the displayed unit without touching the configuration, so
// a glance in the other scale costs a keystroke rather than a settings trip.
func (w *weather) toggleUnit() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.unit == "F" {
		w.unit = "C"
	} else {
		w.unit = "F"
	}
	return "units: °" + w.unit
}

// convertWeatherUnit converts a reading into the requested unit, whatever unit
// the source reported it in.
func convertWeatherUnit(w tideui.WeatherData, target string) tideui.WeatherData {
	if target == "" || w.Unit == "" || w.Unit == target {
		return w
	}
	switch {
	case target == "C" && w.Unit == "F":
		w = convertTemperatures(w, func(v int) int { return int(math.Round(float64(v-32) * 5 / 9)) }, "C")
	case target == "F" && w.Unit == "C":
		w = convertTemperatures(w, func(v int) int { return int(math.Round(float64(v)*9/5 + 32)) }, "F")
	}
	return w
}

func convertTemperatures(w tideui.WeatherData, convert func(int) int, unit string) tideui.WeatherData {
	w.Temperature = convert(w.Temperature)
	w.High = convert(w.High)
	w.Low = convert(w.Low)
	if w.HasFeelsLike {
		w.FeelsLike = convert(w.FeelsLike)
	}
	w.Unit = unit
	for i := range w.Hourly {
		w.Hourly[i].Temperature = convert(w.Hourly[i].Temperature)
	}
	for i := range w.Daily {
		w.Daily[i].Temperature = convert(w.Daily[i].Temperature)
	}
	return w
}

// Demo synthesises a plausible afternoon, so the dashboard has weather before
// it has coordinates.
func (w *weather) Demo(now time.Time) {
	t := float64(now.UnixNano()) / float64(time.Second)
	data := tideui.WeatherData{
		Location:     "Springfield",
		Temperature:  int(math.Round(71 + 3*wave(t, 900, 0))),
		Unit:         "F",
		Condition:    "Partly Cloudy",
		FeelsLike:    int(math.Round(70 + 3*wave(t, 900, 0))),
		HasFeelsLike: true,
		High:         76,
		Low:          61,
		RainChance:   int(clamp(12+8*wave(t, 420, 1), 0, 100)),
		WindSpeed:    int(clamp(9+4*wave(t, 300, 2), 0, 45)),
		WindUnit:     "mph",
		Hourly: []tideui.ForecastPoint{
			{Label: "3PM", Temperature: 74, Condition: "Partly Cloudy", RainChance: 10},
			{Label: "6PM", Temperature: 70, Condition: "Cloudy", RainChance: 15},
			{Label: "9PM", Temperature: 64, Condition: "Cloudy", RainChance: 20},
			{Label: "12AM", Temperature: 61, Condition: "Fog", RainChance: 12},
		},
		Daily: []tideui.ForecastPoint{
			{Label: "Today", Temperature: 74, Condition: "Partly Cloudy"},
			{Label: "Tue", Temperature: 71, Condition: "Rain"},
			{Label: "Wed", Temperature: 68, Condition: "Cloudy"},
			{Label: "Thu", Temperature: 75, Condition: "Sunny"},
			{Label: "Fri", Temperature: 73, Condition: "Sunny"},
		},
		Updated: now,
	}
	w.Store(data)
}
