package tideui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func dashboardNow() time.Time {
	return time.Date(2026, 9, 14, 14, 42, 0, 0, time.UTC)
}

func weatherFixture() WeatherData {
	return WeatherData{
		Location: "Springfield", Temperature: 72, Unit: "F", Condition: "Partly Cloudy",
		High: 76, Low: 61, RainChance: 12, WindSpeed: 9, WindUnit: "mph",
		Hourly:  []ForecastPoint{{Label: "3PM", Temperature: 74}, {Label: "6PM", Temperature: 70}},
		Daily:   []ForecastPoint{{Label: "Tue", Temperature: 71, Condition: "Rain"}},
		Updated: dashboardNow(),
	}
}

func agendaFixture(now time.Time) []AgendaItem {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := func(offset, hour, minute int) time.Time {
		return day.AddDate(0, 0, offset).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	return []AgendaItem{
		{Title: "Standup", Start: at(1, 8, 0), Category: "Work", Tone: ToneAccent},
		{Title: "Focus block", Start: at(0, 14, 0), Category: "Deep work", Tone: ToneAccent},
		{Title: "Project review", Start: at(0, 15, 30), Location: "Meet", Category: "Work"},
	}
}

func TestRenderWeather(t *testing.T) {
	r := chromeRenderer(Compact)
	plain := ansi.Strip(r.RenderWeather(weatherFixture(), 34))
	for _, want := range []string{"72°F", "Partly Cloudy", "H 76°", "L 61°", "Rain", "Wind", "3PM", "74°"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("weather missing %q:\n%s", want, plain)
		}
	}
	detail := ansi.Strip(r.RenderWeatherDetail(weatherFixture(), 40))
	for _, want := range []string{"FORECAST", "Tue", "Springfield", "Updated"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("weather detail missing %q:\n%s", want, detail)
		}
	}
}

func TestWeatherKindMapping(t *testing.T) {
	cases := map[string]WeatherKind{
		"":              WeatherUnknown,
		"Clear":         WeatherClear,
		"Sunny":         WeatherClear,
		"Partly Cloudy": WeatherPartly,
		"Overcast":      WeatherCloudy,
		"Fog":           WeatherFog,
		"Drizzle":       WeatherRain,
		"Showers":       WeatherRain,
		"Rain":          WeatherRain,
		"Snow":          WeatherSnow,
		"Snow Showers":  WeatherSnow,
		"Thunderstorm":  WeatherStorm,
	}
	for condition, want := range cases {
		if got := WeatherKindFromCondition(condition); got != want {
			t.Fatalf("WeatherKindFromCondition(%q) = %d, want %d", condition, got, want)
		}
	}
}

func TestWeatherKindGlyph(t *testing.T) {
	kinds := []WeatherKind{WeatherClear, WeatherPartly, WeatherCloudy, WeatherFog, WeatherRain, WeatherSnow, WeatherStorm}
	seen := map[string]bool{}
	for _, kind := range kinds {
		glyph := kind.Glyph(false)
		if glyph == "" {
			t.Fatalf("kind %d has no glyph", kind)
		}
		if width := ansi.StringWidth(glyph); width != 2 {
			t.Fatalf("kind %d glyph %q width = %d, want 2 (emoji presentation)", kind, glyph, width)
		}
		if seen[glyph] {
			t.Fatalf("duplicate glyph %q", glyph)
		}
		seen[glyph] = true
		if kind.Glyph(true) == "" {
			t.Fatalf("kind %d has no plain fallback", kind)
		}
	}
}

func TestWeatherFeelsLikeAndGlyphRender(t *testing.T) {
	r := chromeRenderer(Compact)
	w := weatherFixture()
	w.FeelsLike = 70
	w.HasFeelsLike = true
	out := ansi.Strip(r.RenderWeather(w, 40))
	if !strings.Contains(out, "Feels 70°") {
		t.Fatalf("feels-like missing:\n%s", out)
	}
	if !strings.Contains(out, WeatherKindFromCondition(w.Condition).Glyph(false)) {
		t.Fatalf("condition glyph missing:\n%s", out)
	}
	if !strings.Contains(out, r.rainGlyph()+" Rain") || !strings.Contains(out, r.windGlyph()+" Wind") {
		t.Fatalf("rain/wind icons missing:\n%s", out)
	}
	if detail := ansi.Strip(r.RenderWeatherDetail(w, 44)); !strings.Contains(detail, "Feels 70°") {
		t.Fatalf("detail missing feels-like:\n%s", detail)
	}
	// A hot feels-like temperature gets the flame; a mild one does not.
	hot := weatherFixture()
	hot.FeelsLike = 102
	hot.HasFeelsLike = true
	hotOut := ansi.Strip(r.RenderWeather(hot, 44))
	if !strings.Contains(hotOut, "Feels 102° "+r.hotGlyph()) {
		t.Fatalf("hot feels-like missing flame:\n%s", hotOut)
	}
	if strings.Contains(ansi.Strip(r.RenderWeather(w, 44)), r.hotGlyph()) {
		t.Fatal("mild feels-like should not show a flame")
	}
}

func TestWeatherColorsAreDistinct(t *testing.T) {
	ws := chromeRenderer(Compact).Styles.Workspace
	kinds := []WeatherKind{WeatherClear, WeatherCloudy, WeatherFog, WeatherRain, WeatherSnow, WeatherStorm}
	colors := map[WeatherKind]lipgloss.Color{}
	for _, kind := range kinds {
		c := ws.WeatherColor(kind)
		if c == "" {
			t.Fatalf("kind %d has no colour", kind)
		}
		colors[kind] = c
	}
	if colors[WeatherClear] == colors[WeatherCloudy] {
		t.Fatal("clear and cloudy should have distinct colours")
	}
	if colors[WeatherRain] == colors[WeatherSnow] {
		t.Fatal("rain and snow should have distinct colours")
	}
}

func TestRenderAgendaSortsAndGroups(t *testing.T) {
	now := dashboardNow()
	out := ansi.Strip(r2().RenderAgenda(agendaFixture(now), now, 44))
	if !strings.Contains(out, "TODAY") || !strings.Contains(out, "TOMORROW") {
		t.Fatalf("agenda missing day groups:\n%s", out)
	}
	focus := strings.Index(out, "Focus block")
	review := strings.Index(out, "Project review")
	standup := strings.Index(out, "Standup")
	if focus < 0 || review < 0 || standup < 0 {
		t.Fatalf("agenda missing items:\n%s", out)
	}
	if !(focus < review && review < standup) {
		t.Fatalf("agenda not sorted by time:\n%s", out)
	}
	if !strings.Contains(out, "next") {
		t.Fatalf("agenda missing next emphasis:\n%s", out)
	}
}

func TestRenderAgendaEmpty(t *testing.T) {
	out := r2().RenderAgenda(nil, dashboardNow(), 30)
	if !strings.Contains(ansi.Strip(out), "No upcoming events") {
		t.Fatalf("empty agenda = %q", ansi.Strip(out))
	}
}

func r2() Renderer { return chromeRenderer(Compact) }

func TestRenderClockAndCalendar(t *testing.T) {
	r := chromeRenderer(Compact)
	now := dashboardNow()
	clock := ClockData{Local: now, Location: "Local", Hour24: true, Zones: []WorldClock{{City: "London", Time: now, Offset: "UTC"}, {City: "Tokyo", Time: now.Add(9 * time.Hour), Offset: "+9"}}}
	// At a narrow width the time is plain text.
	plain := ansi.Strip(r.RenderClock(clock, 14))
	for _, want := range []string{"14:42", "Mon Sep 14", "London", "Tokyo"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("clock missing %q:\n%s", want, plain)
		}
	}
	twelve := ansi.Strip(r.RenderClock(ClockData{Local: now, Hour24: false}, 14))
	if !strings.Contains(twelve, "2:42 PM") {
		t.Fatalf("12-hour clock = %q, want 2:42 PM", twelve)
	}
	// At a wide width the time is drawn big, with a day-progress gauge.
	big := ansi.Strip(r.RenderClock(clock, 30))
	if !strings.Contains(big, "afternoon") || !strings.Contains(big, "% of day") && !strings.Contains(big, "61%") {
		t.Fatalf("wide clock should be rich:\n%s", big)
	}
	if !strings.Contains(big, "_") {
		t.Fatalf("wide clock missing big digits:\n%s", big)
	}
	calendar := ansi.Strip(r.RenderMiniCalendar(MiniCalendar{Year: 2026, Month: time.September, Highlight: 14, Width: 21}, r.Styles.Workspace.Bg))
	if !strings.Contains(calendar, "September 2026") || !strings.Contains(calendar, "14") {
		t.Fatalf("calendar missing content:\n%s", calendar)
	}
	for _, line := range strings.Split(calendar, "\n") {
		if lipgloss.Width(line) != 21 {
			t.Fatalf("calendar line width = %d, want 21", lipgloss.Width(line))
		}
	}
}

func TestClockLook(t *testing.T) {
	r := chromeRenderer(Compact)
	morning := time.Date(2026, 9, 14, 8, 5, 0, 0, time.UTC)
	if label, glyph := r.dayPeriod(morning); label != "morning" || glyph == "" {
		t.Fatalf("dayPeriod(morning) = %q,%q", label, glyph)
	}
	if label, _ := r.dayPeriod(time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)); label != "night" {
		t.Fatalf("dayPeriod(night) = %q", label)
	}
	if f := dayFraction(morning); f <= 0 || f >= 1 {
		t.Fatalf("dayFraction = %v, want between 0 and 1", f)
	}
	big := r.bigTime("14:42") // default dash font is 3 rows
	if len(big) != 3 || lipgloss.Width(big[0]) != 19 {
		t.Fatalf("dash bigTime = %#v", big)
	}
	block := NewRenderer(CatppuccinMocha, StyleOptions{ClockFont: ClockFontBlock}).bigTime("14:42")
	if len(block) != 5 || lipgloss.Width(block[0]) != 19 {
		t.Fatalf("block bigTime = %#v", block)
	}
	face := r.analogClock(morning)
	if len(face) != 7 {
		t.Fatalf("analog face height = %d, want 7", len(face))
	}
	for _, line := range face {
		if lipgloss.Width(line) != 13 {
			t.Fatalf("analog face width = %d, want 13 (%q)", lipgloss.Width(line), line)
		}
	}
	for _, font := range ClockFonts() {
		fr := NewRenderer(CatppuccinMocha, StyleOptions{ClockFont: font})
		if fr.Styles.ClockFont != font {
			t.Fatalf("resolved clock font = %q, want %q", fr.Styles.ClockFont, font)
		}
		rows := fr.bigTime("14:42")
		if lipgloss.Width(rows[0]) != 19 {
			t.Fatalf("%s bigTime = %#v", font, rows)
		}
	}
	if NewRenderer(CatppuccinMocha, StyleOptions{ClockFont: ClockFont("x")}).Styles.ClockFont != ClockFontDash {
		t.Fatal("unknown clock font should fall back to dash")
	}

	_, glyph := r.dayPeriod(morning)
	compact := ansi.Strip(r.RenderClock(ClockData{Local: morning, Location: "Local"}, 30))
	if !strings.Contains(compact, "morning") || !strings.Contains(compact, glyph) {
		t.Fatalf("compact clock missing period:\n%s", compact)
	}
	detail := ansi.Strip(r.RenderClockDetail(ClockData{Local: morning, Location: "Local"}, 30))
	if !strings.Contains(detail, "% of day") {
		t.Fatalf("detail clock missing day progress:\n%s", detail)
	}
}

func TestRenderSystemNetworkStorage(t *testing.T) {
	r := chromeRenderer(Compact)
	system := ansi.Strip(r.RenderSystem(SystemMetrics{
		CPUPercent: 18, CPUSpark: []float64{0.1, 0.2, 0.3}, MemoryPercent: 41,
		TemperatureC: 54, Load: [3]float64{1.4, 1.1, 0.9}, Uptime: 3*24*time.Hour + 14*time.Hour,
	}, 30))
	for _, want := range []string{"CPU", "18%", "MEM", "41%", "TEMP", "54°C", "LOAD", "UP", "3d 14h"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system missing %q:\n%s", want, system)
		}
	}

	network := ansi.Strip(r.RenderNetwork(NetworkMetrics{
		Interface: "wlan0", Download: 87, Upload: 14, Unit: "Mbps",
		DownSpark: []float64{0.1, 0.5, 0.9}, LAN: "940 Mbps", WAN: "87/14",
	}, 30))
	for _, want := range []string{"↓", "↑", "87 Mbps", "14 Mbps", "wlan0"} {
		if !strings.Contains(network, want) {
			t.Fatalf("network missing %q:\n%s", want, network)
		}
	}

	storage := ansi.Strip(r.RenderStorage([]StorageMount{
		{Path: "/", UsedPercent: 72}, {Path: "/home", UsedPercent: 48}, {Path: "/media", UsedPercent: 91},
	}, 30))
	for _, want := range []string{"/", "72%", "/home", "48%", "/media", "91%"} {
		if !strings.Contains(storage, want) {
			t.Fatalf("storage missing %q:\n%s", want, storage)
		}
	}
}

func TestRenderServicesAndHeadlines(t *testing.T) {
	r := chromeRenderer(Compact)
	services := ansi.Strip(r.RenderServices([]ServiceStatus{
		{Name: "jellyfin", Status: StatusHealthy},
		{Name: "forgejo", Status: StatusWarning},
		{Name: "backup", Status: StatusStopped},
	}, 30))
	for _, want := range []string{"jellyfin", "healthy", "warning", "stopped"} {
		if !strings.Contains(services, want) {
			t.Fatalf("services missing %q:\n%s", want, services)
		}
	}

	headlines := ansi.Strip(r.RenderHeadlines([]Headline{
		{Title: "A very long headline that must be truncated somewhere", Source: "src", Age: "5m", Unread: true, Tone: ToneAccent},
	}, 20))
	for _, line := range strings.Split(headlines, "\n") {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("headline line too wide: %q", line)
		}
	}
	if !strings.Contains(headlines, "…") {
		t.Fatalf("headline should truncate:\n%s", headlines)
	}
	if !strings.Contains(headlines, "●") {
		t.Fatalf("unread headline should show a marker:\n%s", headlines)
	}
}

func TestRenderTasksNotesGitMarkets(t *testing.T) {
	r := chromeRenderer(Compact)
	tasks := ansi.Strip(r.RenderTasks([]Task{
		{Title: "Finish TideDeck", Due: "today", Tone: ToneWarning},
		{Title: "Fix spacing", Done: true},
	}, 34))
	for _, want := range []string{"□", "Finish TideDeck", "today", "✓", "Fix spacing"} {
		if !strings.Contains(tasks, want) {
			t.Fatalf("tasks missing %q:\n%s", want, tasks)
		}
	}

	notes := ansi.Strip(r.RenderNotes([]Note{{Title: "Remember", Pinned: true, Body: "- one\n- two"}}, 30))
	for _, want := range []string{"★", "Remember", "• one", "• two"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes missing %q:\n%s", want, notes)
		}
	}

	repos := ansi.Strip(r.RenderRepoActivity([]RepoActivity{
		{Name: "tideui", Branch: "main", Commits: 3, Tone: ToneAccent},
	}, 34))
	for _, want := range []string{"tideui", "main", "3 commits"} {
		if !strings.Contains(repos, want) {
			t.Fatalf("git missing %q:\n%s", want, repos)
		}
	}

	markets := ansi.Strip(r.RenderMarkets([]MarketQuote{
		{Symbol: "AMD", Price: 162.40, ChangePct: 1.8},
		{Symbol: "NVDA", Price: 214.10, ChangePct: -0.4},
	}, 30))
	for _, want := range []string{"AMD", "162.40", "1.8%", "NVDA", "214.10", "0.4%"} {
		if !strings.Contains(markets, want) {
			t.Fatalf("markets missing %q:\n%s", want, markets)
		}
	}
}

func TestDashboardWidgetsBoundedAtTinyWidths(t *testing.T) {
	r := chromeRenderer(Compact)
	now := dashboardNow()
	renderers := map[string]func(int) string{
		"weather":       func(w int) string { return r.RenderWeather(weatherFixture(), w) },
		"weatherDetail": func(w int) string { return r.RenderWeatherDetail(weatherFixture(), w) },
		"agenda":        func(w int) string { return r.RenderAgenda(agendaFixture(now), now, w) },
		"clock": func(w int) string {
			return r.RenderClock(ClockData{Local: now, Zones: []WorldClock{{City: "Tokyo", Time: now}}}, w)
		},
		"system": func(w int) string {
			return r.RenderSystem(SystemMetrics{CPUPercent: 80, CPUSpark: []float64{0.5}, MemoryPercent: 90, Uptime: time.Hour}, w)
		},
		"systemDetail": func(w int) string {
			return r.RenderSystemDetail(SystemMetrics{Cores: []float64{10, 90}, MemoryUsed: "1G", Processes: 3}, w)
		},
		"network": func(w int) string {
			return r.RenderNetwork(NetworkMetrics{Download: 10, Upload: 2, DownSpark: []float64{0.5}}, w)
		},
		"storage": func(w int) string { return r.RenderStorage([]StorageMount{{Path: "/", UsedPercent: 90}}, w) },
		"services": func(w int) string {
			return r.RenderServices([]ServiceStatus{{Name: "x", State: "healthy", Tone: ToneGood}}, w)
		},
		"headlines": func(w int) string {
			return r.RenderHeadlines([]Headline{{Title: "hello world", Source: "src", Age: "1m", Unread: true}}, w)
		},
		"tasks": func(w int) string { return r.RenderTasks([]Task{{Title: "task", Due: "today"}}, w) },
		"notes": func(w int) string { return r.RenderNotes([]Note{{Title: "n", Body: "- line"}}, w) },
		"git": func(w int) string {
			return r.RenderRepoActivity([]RepoActivity{{Name: "r", Branch: "main", Commits: 1}}, w)
		},
		"markets": func(w int) string {
			return r.RenderMarkets([]MarketQuote{{Symbol: "A", Price: 1.23, ChangePct: 1.5}}, w)
		},
	}
	for name, render := range renderers {
		for _, width := range []int{4, 8, 12, 20, 40} {
			view := render(width)
			for i, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("%s width %d: line %d width %d: %q", name, width, i, got, ansi.Strip(line))
				}
			}
		}
	}
}

func TestInfoPrimitives(t *testing.T) {
	withTrueColor(t)
	r := chromeRenderer(Compact)
	bg := r.Styles.Workspace.Bg

	dot := ansi.Strip(r.RenderStatusDot(StatusDot{Label: "ok", Tone: ToneGood}, bg))
	if dot != "● ok" {
		t.Fatalf("status dot = %q", dot)
	}
	dot = ansi.Strip(r.RenderStatusDot(StatusDot{Label: "bad", Tone: ToneDanger}, bg))
	if dot != "● bad" {
		t.Fatalf("status dot = %q", dot)
	}

	trend := ansi.Strip(r.RenderTrend(1.8, "%", bg))
	if !strings.Contains(trend, "▲") || !strings.Contains(trend, "1.8%") {
		t.Fatalf("trend = %q", trend)
	}
	down := ansi.Strip(r.RenderTrend(-0.4, "%", bg))
	if !strings.Contains(down, "▼") {
		t.Fatalf("down trend = %q", down)
	}

	stat := ansi.Strip(r.RenderStatValue(StatValue{Value: "72", Unit: "°F", Label: "Partly Cloudy", Tone: ToneAccent}, bg))
	for _, want := range []string{"72°F", "Partly Cloudy"} {
		if !strings.Contains(stat, want) {
			t.Fatalf("stat value missing %q: %q", want, stat)
		}
	}

	gauge := r.RenderBarGauge(BarGauge{Fraction: 0.5, Width: 10, Tone: ToneGood, Label: "x"}, bg)
	if !strings.Contains(ansi.Strip(gauge), "x") || !strings.Contains(ansi.Strip(gauge), "█") {
		t.Fatalf("bar gauge = %q", ansi.Strip(gauge))
	}
}

func TestStatusDotDoesNotRelyOnColourAlone(t *testing.T) {
	r := chromeRenderer(Compact)
	good := ansi.Strip(r.RenderStatusDot(StatusDot{Label: "healthy", Tone: ToneGood}, r.Styles.Workspace.Bg))
	bad := ansi.Strip(r.RenderStatusDot(StatusDot{Label: "stopped", Tone: ToneDanger}, r.Styles.Workspace.Bg))
	if good == bad {
		t.Fatal("status labels must differ even without colour")
	}
}
