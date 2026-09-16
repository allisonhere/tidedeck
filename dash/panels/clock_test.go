package panels

import (
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

// The clock ticks rather than fetches. Nothing here does I/O, so the panel
// must not carry an interval: a refresh every minute for a value the UI
// already recomputes on every frame is pure machinery.
func TestClockTicksAndDoesNotFetch(t *testing.T) {
	panel := Clock()
	if got := panel.Meta().Interval; got != 0 {
		t.Fatalf("interval = %v, want none", got)
	}
	if _, ok := panel.(dash.Fetcher); ok {
		t.Fatal("clock should not be a Fetcher")
	}
	ticker, ok := panel.(dash.Ticker)
	if !ok {
		t.Fatal("clock should be a Ticker")
	}

	now := time.Date(2026, 9, 16, 9, 41, 0, 0, time.UTC)
	ticker.Tick(now)
	data := panel.(*clock).Load()
	if !data.Local.Equal(now) {
		t.Fatalf("local = %v, want the tick's time", data.Local)
	}
	if len(data.Zones) != 3 {
		t.Fatalf("default zones = %#v, want three", data.Zones)
	}
}

// An absent key means the declared default applies. The clock defaults to
// 24-hour, so a config with nothing in it must not read as 12-hour - which is
// what Values.Bool alone would have done, since absent and false look alike.
func TestClockStays24HourWhenTheKeyIsAbsent(t *testing.T) {
	now := time.Date(2026, 9, 16, 21, 5, 0, 0, time.UTC)

	fresh := Clock()
	configure(t, fresh, dash.NewValues())
	fresh.(dash.Ticker).Tick(now)
	if !fresh.(*clock).Load().Hour24 {
		t.Fatal("an unset clock_24 switched the clock to 12-hour")
	}

	twelve := Clock()
	values := dash.NewValues()
	values.Set("clock_24", false)
	configure(t, twelve, values)
	twelve.(dash.Ticker).Tick(now)
	if twelve.(*clock).Load().Hour24 {
		t.Fatal("clock_24=false did not switch to 12-hour")
	}
}

// Zones are resolved once, at configure time, and one unusable zone costs
// only itself.
func TestClockResolvesZonesAndSkipsBadOnes(t *testing.T) {
	panel := Clock()
	values := dash.NewValues()
	values.Set("zones", "Europe/London, Nowhere/Nothing, Asia/Tokyo")
	values.Set("weather.location", "Austin")
	configure(t, panel, values)
	panel.(dash.Ticker).Tick(time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))

	data := panel.(*clock).Load()
	if data.Location != "Austin" {
		t.Fatalf("location = %q, want the weather panel's", data.Location)
	}
	var cities []string
	for _, zone := range data.Zones {
		cities = append(cities, zone.City)
	}
	if strings.Join(cities, ",") != "London,Tokyo" {
		t.Fatalf("zones = %v, want the two that resolve", cities)
	}
	// The offsets come from the zone at the displayed instant, so a clock that
	// crosses a daylight-saving boundary is right the moment it does.
	if data.Zones[0].Offset != "+1" || data.Zones[1].Offset != "+9" {
		t.Fatalf("offsets = %q, %q", data.Zones[0].Offset, data.Zones[1].Offset)
	}
}

// The panel declares the keys that are already in config.json: migrating a
// setting onto a panel must not rewrite anyone's file.
func TestClockDeclaresTheKeysAlreadyInConfig(t *testing.T) {
	fields := Clock().(dash.Configurable).Schema()
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		keys = append(keys, field.Key)
	}
	if strings.Join(keys, ",") != "clock_24,zones" {
		t.Fatalf("declared keys = %v", keys)
	}
	if fields[0].Default != "true" {
		t.Fatalf("24-hour default = %q, want true", fields[0].Default)
	}
}

// The deck drives the clock with no special-casing, and the body stays inside
// the pane at every width.
func TestClockThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Clock())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("clock")
	if !ok {
		t.Fatal("clock was not attached to the workspace")
	}
	deck.Tick(time.Date(2026, 9, 16, 9, 41, 0, 0, time.UTC))
	for _, width := range []int{4, 12, 20, 40} {
		body := ansi.Strip(registered.Render(tideui.PanelContext{
			ID: "clock", Width: width, Renderer: renderer(),
		}))
		for _, line := range strings.Split(body, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
	body := ansi.Strip(registered.Render(tideui.PanelContext{
		ID: "clock", Width: 40, Renderer: renderer(),
	}))
	if !strings.Contains(body, "London") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}

func configure(t *testing.T, panel dash.Panel, values dash.Values) {
	t.Helper()
	configurable, ok := panel.(dash.Configurable)
	if !ok {
		t.Fatal("clock should be Configurable")
	}
	if err := configurable.Configure(values); err != nil {
		t.Fatal(err)
	}
}
