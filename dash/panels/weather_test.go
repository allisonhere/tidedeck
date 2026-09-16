package panels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
	"github.com/charmbracelet/x/ansi"
)

// testWeather builds the panel with a recording fetcher, so configuring it
// never reaches the network.
func testWeather(data tideui.WeatherData, err error) (*weather, *provider.WeatherOptions) {
	var seen provider.WeatherOptions
	panel := &weather{unit: "F"}
	panel.newFetcher = func(options provider.WeatherOptions) func(context.Context) (tideui.WeatherData, error) {
		seen = options
		return func(context.Context) (tideui.WeatherData, error) { return data, err }
	}
	return panel, &seen
}

func weatherValues(pairs map[string]any) dash.Values {
	values := dash.NewValues()
	for key, value := range pairs {
		values.Set(key, value)
	}
	return values
}

// A configured place builds a source; the settings reach the provider intact.
func TestWeatherSourceBuiltFromSettings(t *testing.T) {
	panel, options := testWeather(tideui.WeatherData{}, nil)
	err := panel.Configure(weatherValues(map[string]any{
		"weather.enabled": true, "weather.latitude": 52.52, "weather.longitude": 13.405,
		"weather.location": "Berlin", "weather.fahrenheit": false, "weather.wind_mph": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured place should build a source")
	}
	want := provider.WeatherOptions{
		Latitude: 52.52, Longitude: 13.405, Location: "Berlin", WindMPH: true,
	}
	if *options != want {
		t.Fatalf("options = %+v, want %+v", *options, want)
	}
}

// Without coordinates there is nowhere to look, so the panel builds no source
// at all rather than asking the API about the Gulf of Guinea.
func TestWeatherSkippedWithoutCoordinatesOrWhenOff(t *testing.T) {
	panel, _ := testWeather(tideui.WeatherData{}, nil)
	if err := panel.Configure(weatherValues(map[string]any{"weather.enabled": true})); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("no coordinates should mean no source")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}

	if err := panel.Configure(weatherValues(map[string]any{
		"weather.enabled": false, "weather.latitude": 52.52,
	})); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("a disabled panel should not fetch")
	}
}

// The settings that default to true must survive an empty document: an absent
// key means the default applies, which Bool alone cannot express.
func TestWeatherDefaultsWhenKeysAreAbsent(t *testing.T) {
	panel, options := testWeather(tideui.WeatherData{}, nil)
	if err := panel.Configure(weatherValues(map[string]any{"weather.latitude": 52.52})); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("an absent weather.enabled should still count as enabled")
	}
	if !options.Fahrenheit || !options.WindMPH {
		t.Fatalf("units defaulted off: %+v", *options)
	}
	if options.Location != "Local" {
		t.Fatalf("location = %q, want Local", options.Location)
	}
}

// A failed fetch keeps the last good reading on screen.
func TestWeatherKeepsLastGoodReading(t *testing.T) {
	good := tideui.WeatherData{Location: "Berlin", Temperature: 12, Unit: "C", Condition: "Cloudy"}
	failing := errors.New("no route to host")
	calls := 0
	panel := &weather{unit: "C"}
	panel.newFetcher = func(provider.WeatherOptions) func(context.Context) (tideui.WeatherData, error) {
		return func(context.Context) (tideui.WeatherData, error) {
			calls++
			if calls == 1 {
				return good, nil
			}
			return tideui.WeatherData{}, failing
		}
	}
	if err := panel.Configure(weatherValues(map[string]any{"weather.latitude": 52.52})); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); !errors.Is(err, failing) {
		t.Fatalf("second refresh error = %v", err)
	}
	if got := panel.Load().Location; got != "Berlin" {
		t.Fatalf("a failed refresh discarded the reading: %q", got)
	}
}

// The unit key converts the reading in place, without touching what is stored.
func TestWeatherUnitToggleConvertsTheReading(t *testing.T) {
	panel, _ := testWeather(tideui.WeatherData{}, nil)
	panel.Store(tideui.WeatherData{Location: "Springfield", Temperature: 72, High: 76,
		Low: 61, Unit: "F", Condition: "Sunny"})
	ctx := tideui.PanelContext{ID: "weather", Width: 40, Renderer: renderer()}

	if body := ansi.Strip(panel.View(ctx)); !strings.Contains(body, "72°F") {
		t.Fatalf("body = %s", body)
	}
	if got := panel.toggleUnit(); got != "units: °C" {
		t.Fatalf("toggle said %q", got)
	}
	body := ansi.Strip(panel.View(ctx))
	if !strings.Contains(body, "22°C") {
		t.Fatalf("toggled body = %s", body)
	}
	// Converting is a display choice: the stored reading is untouched, so a
	// refresh does not have to undo it.
	if panel.Load().Unit != "F" {
		t.Fatalf("stored unit changed to %q", panel.Load().Unit)
	}
}

// An empty panel says what is missing. It used to render a zero-valued
// reading, which looks like a forecast of 0°F rather than like a panel that
// has not been told where it is.
func TestWeatherEmptyViewSaysWhatIsMissing(t *testing.T) {
	panel, _ := testWeather(tideui.WeatherData{}, nil)
	ctx := tideui.PanelContext{ID: "weather", Width: 30, Renderer: renderer()}
	if body := ansi.Strip(panel.View(ctx)); !strings.Contains(body, "No location set") {
		t.Fatalf("unconfigured body = %q", body)
	}
	if err := panel.Configure(weatherValues(map[string]any{"weather.latitude": 52.52})); err != nil {
		t.Fatal(err)
	}
	if body := ansi.Strip(panel.View(ctx)); !strings.Contains(body, "Loading") {
		t.Fatalf("configured but unfetched body = %q", body)
	}
}

// Through the deck: demo data fills the panel, the refresh key really makes it
// due again, and the body stays inside the pane at every width.
func TestWeatherThroughTheDeck(t *testing.T) {
	deck := dash.New()
	panel, _ := testWeather(tideui.WeatherData{Location: "Berlin", Unit: "C", Condition: "Rain"}, nil)
	deck.Register(panel)
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("weather")
	if !ok {
		t.Fatal("weather was not attached to the workspace")
	}
	deck.Tick(time.Unix(1700000000, 0)) // demo mode is the default
	for _, width := range []int{4, 12, 20, 40} {
		body := ansi.Strip(registered.Render(tideui.PanelContext{
			ID: "weather", Width: width, Renderer: renderer(),
		}))
		for _, line := range strings.Split(body, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
	if body := ansi.Strip(registered.Render(tideui.PanelContext{
		ID: "weather", Width: 40, Renderer: renderer(),
	})); !strings.Contains(body, "Partly Cloudy") {
		t.Fatalf("demo data did not reach the panel:\n%s", body)
	}

	// The refresh key clears the panel's due time rather than only claiming a
	// refresh happened, so the next Refresh actually fetches.
	deck.SetMode(dash.ModeLive)
	if err := panel.Configure(weatherValues(map[string]any{"weather.latitude": 52.52})); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	deck.Refresh(context.Background(), now)
	if got := panel.Load().Location; got != "Berlin" {
		t.Fatalf("first refresh = %q", got)
	}
	panel.Store(tideui.WeatherData{})
	deck.Refresh(context.Background(), now) // inside the interval: skipped
	if panel.Load().Location != "" {
		t.Fatal("a panel inside its interval should not have refetched")
	}
	action, ok := actionByKey(registered, "r")
	if !ok {
		t.Fatal("no refresh action")
	}
	action.Handler(ws)
	deck.Refresh(context.Background(), now)
	if got := panel.Load().Location; got != "Berlin" {
		t.Fatalf("the refresh key did not make the panel due again: %q", got)
	}
}

func actionByKey(panel *tideui.Panel, key string) (tideui.PanelAction, bool) {
	for _, action := range panel.ActionList() {
		if action.Key == key {
			return action, true
		}
	}
	return tideui.PanelAction{}, false
}
