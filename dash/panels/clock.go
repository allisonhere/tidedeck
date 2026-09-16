package panels

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
)

// Configuration keys this panel owns. They are the keys already in the file.
const (
	clock24Key       = "clock_24"
	zonesKey         = "zones"
	clockLocationKey = "weather.location"
	// "Sydney" is not an IANA zone name, so the shipped default has always
	// resolved to two clocks rather than three. Spelled properly it is three.
	defaultZones = "Europe/London,Asia/Tokyo,Australia/Sydney"
)

// clock shows the local time, how far through the day it is, and the time in
// other zones.
//
// It is the first panel that ticks rather than fetches. Nothing here does any
// I/O: the time comes from the clock and the zones were resolved when the
// panel was configured, so what used to be a one-minute fetch through the
// provider machinery is a recomputation on the tick the UI was doing anyway.
type clock struct {
	dash.State[tideui.ClockData]

	mu       sync.Mutex
	location string
	hour24   bool
	zones    []zone
}

// zone is a configured world clock, with its location resolved once.
type zone struct {
	city string
	loc  *time.Location
}

// Clock builds the clock panel.
func Clock() dash.Panel {
	panel := &clock{location: "Local", hour24: true}
	panel.setZones(strings.Split(defaultZones, ","))
	return panel
}

func (c *clock) Meta() dash.Meta {
	return dash.Meta{
		ID: "clock", Title: "Clock",
		Role: tideui.RoleSecondary, Priority: 50,
		MinWidth: 16, MinHeight: 7, HideBelow: 80,
		// No Interval: this panel has nothing to fetch. It draws the day
		// progress gauge but no sparkline.
		Gauge: true,
	}
}

func (c *clock) Schema() []dash.Field {
	return []dash.Field{
		{Key: clock24Key, Label: "24-hour", Kind: dash.FieldBool, Default: "true"},
		{Key: zonesKey, Label: "zones", Kind: dash.FieldText, Default: defaultZones},
	}
}

// Configure resolves the configured zones once, so the tick does no lookups.
// The location label follows the weather panel's, which is where the machine's
// place is configured; a clock has no opinion of its own about where it is.
func (c *clock) Configure(values dash.Values) error {
	names := values.List(zonesKey)
	if len(names) == 0 {
		names = strings.Split(defaultZones, ",")
	}
	location := strings.TrimSpace(values.String(clockLocationKey))
	if location == "" {
		location = "Local"
	}
	// An absent key means the declared default applies, which for the clock is
	// 24-hour. Reading it with Bool alone would silently switch a fresh
	// install to 12-hour, because absent and false look the same.
	hour24 := true
	if values.Has(clock24Key) {
		hour24 = values.Bool(clock24Key)
	}
	c.mu.Lock()
	c.location, c.hour24 = location, hour24
	c.mu.Unlock()
	c.setZones(names)
	return nil
}

// setZones resolves IANA names to locations, dropping any that do not exist
// rather than failing the panel: one bad zone should not cost you the others.
func (c *clock) setZones(names []string) {
	resolved := make([]zone, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		resolved = append(resolved, zone{city: cityName(name), loc: loc})
	}
	c.mu.Lock()
	c.zones = resolved
	c.mu.Unlock()
}

// Tick recomputes the displayed time. The zone offsets are read per tick
// rather than cached, so a clock crossing a daylight-saving boundary is right
// the moment it happens.
func (c *clock) Tick(now time.Time) {
	c.mu.Lock()
	data := tideui.ClockData{Local: now, Location: c.location, Hour24: c.hour24}
	for _, z := range c.zones {
		local := now.In(z.loc)
		_, offset := local.Zone()
		data.Zones = append(data.Zones, tideui.WorldClock{
			City: z.city, Time: local, Offset: formatOffset(offset),
		})
	}
	c.mu.Unlock()
	c.Store(data)
}

// Demo shows a spread of zones so the panel looks inhabited before anything
// is configured.
func (c *clock) Demo(now time.Time) {
	utc := now.UTC()
	c.mu.Lock()
	hour24 := c.hour24
	c.mu.Unlock()
	c.Store(tideui.ClockData{
		Local: now, Location: now.Format("MST"), Hour24: hour24,
		Zones: []tideui.WorldClock{
			{City: "London", Time: utc, Offset: "UTC"},
			{City: "Tokyo", Time: utc.Add(9 * time.Hour), Offset: "+9"},
			{City: "Sydney", Time: utc.Add(10 * time.Hour), Offset: "+10"},
			{City: "New York", Time: utc.Add(-5 * time.Hour), Offset: "-5"},
		},
	})
}

func (c *clock) View(ctx tideui.PanelContext) string {
	data := c.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderClockDetail(data, ctx.Width)
	}
	return ctx.Renderer.RenderClock(data, ctx.Width)
}

func (c *clock) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "sync",
		Run: func() string { return "clock synced" },
	}}
}

// cityName turns "Europe/London" into "London", and formatOffset renders a
// UTC offset. Both match what the provider did, so the panel reads the same
// as it did before it owned this.
func cityName(name string) string {
	if index := strings.LastIndexByte(name, '/'); index >= 0 {
		name = name[index+1:]
	}
	return strings.ReplaceAll(name, "_", " ")
}

// formatOffset renders a UTC offset in seconds as "+9", "-5" or "+5:30". It
// duplicates ten lines the provider keeps private rather than widening that
// package's API for a panel that no longer calls into it.
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if minutes == 0 {
		return fmt.Sprintf("%s%d", sign, hours)
	}
	return fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
}
