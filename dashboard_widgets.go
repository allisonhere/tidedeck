package tideui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// First-party dashboard widgets. Each renders a data model to a bounded,
// themed block; none of them fetch data. Applications supply the model from a
// provider (or a demo feed), so acquisition and rendering stay decoupled.

// RenderWeather renders the compact weather summary.
func (r Renderer) RenderWeather(w WeatherData, width int) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	lines := []string{
		r.weatherHeadline(w, bg),
		lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).
			Render(r.weatherRangeText(w)),
		r.dashPair(r.rainGlyph()+" Rain", fmt.Sprintf("%d%%", w.RainChance), 7, bg),
		r.dashPair(r.windGlyph()+" Wind", fmt.Sprintf("%d %s", w.WindSpeed, w.WindUnit), 7, bg),
	}
	if len(w.Hourly) > 0 {
		lines = append(lines, "", r.renderHourly(w.Hourly, bg))
	}
	return r.dashBlock(lines, width, bg)
}

// weatherHeadline renders the current temperature with a condition glyph and
// label, colouring the glyph by the coarse kind.
func (r Renderer) weatherHeadline(w WeatherData, bg lipgloss.Color) string {
	ws := r.Styles.Workspace
	kind := w.EffectiveKind()
	headline := r.RenderStatValue(StatValue{Value: fmt.Sprintf("%d°", w.Temperature), Unit: w.Unit, Tone: ToneAccent}, bg)
	if glyph := kind.Glyph(r.Styles.PlainUI); glyph != "" {
		headline += lipgloss.NewStyle().Background(bg).Render(" ") +
			lipgloss.NewStyle().Background(bg).Foreground(ws.WeatherColor(kind)).Bold(true).Render(glyph+" ")
	}
	if w.Condition != "" {
		headline += lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).Render(w.Condition)
	}
	return headline
}

// weatherRangeText renders the high/low line, appending feels-like when known
// and a flame once the feels-like temperature is hot.
func (r Renderer) weatherRangeText(w WeatherData) string {
	text := fmt.Sprintf("H %d°   L %d°", w.High, w.Low)
	if w.HasFeelsLike {
		text += fmt.Sprintf("   Feels %d°", w.FeelsLike)
		if w.FeelsLike > 90 {
			text += " " + r.hotGlyph()
		}
	}
	return text
}

// hotGlyph marks a hot feels-like temperature. The emoji is two cells wide and
// the width table agrees, so it does not shift the rest of the line.
func (r Renderer) hotGlyph() string {
	if r.Styles.PlainUI {
		return "!"
	}
	return "\U0001F525" // fire
}

// rainGlyph and windGlyph label the rain and wind lines.
func (r Renderer) rainGlyph() string {
	if r.Styles.PlainUI {
		return "*"
	}
	return "\U0001F4A7" // droplet
}

func (r Renderer) windGlyph() string {
	if r.Styles.PlainUI {
		return "~"
	}
	return "\U0001F4A8" // dash
}

// forecastCondition prefixes a condition with its glyph when one is known.
func (r Renderer) forecastCondition(condition string) string {
	if condition == "" {
		return "—"
	}
	kind := WeatherKindFromCondition(condition)
	if kind == WeatherUnknown {
		return condition
	}
	return kind.Glyph(r.Styles.PlainUI) + " " + condition
}

// renderHourly lays the short forecast out as one grouped strip so it reads as
// a single element rather than a stack of rows.
func (r Renderer) renderHourly(points []ForecastPoint, bg lipgloss.Color) string {
	ws := r.Styles.Workspace
	segments := make([]string, 0, len(points))
	for i, point := range points {
		if i >= 5 {
			break
		}
		label := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).Render(point.Label)
		temp := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Bold(true).
			Render(fmt.Sprintf("%d°", point.Temperature))
		segment := label + " " + temp
		if kind := WeatherKindFromCondition(point.Condition); kind != WeatherUnknown {
			segment += lipgloss.NewStyle().Background(bg).Render(" ") +
				lipgloss.NewStyle().Background(bg).Foreground(ws.WeatherColor(kind)).Render(kind.Glyph(r.Styles.PlainUI))
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, "   ")
}

// RenderWeatherDetail renders the extended forecast.
func (r Renderer) RenderWeatherDetail(w WeatherData, width int) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	lines := []string{
		r.weatherHeadline(w, bg),
		lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).
			Render(fmt.Sprintf("%s   Rain %d%%   Wind %d %s", r.weatherRangeText(w), w.RainChance, w.WindSpeed, w.WindUnit)),
	}
	if w.Location != "" {
		lines = append(lines, r.dashPair("Place", w.Location, 6, bg))
	}
	if !w.Updated.IsZero() {
		lines = append(lines, r.dashPair("Updated", w.Updated.Format("15:04"), 6, bg))
	}
	if len(w.Daily) > 0 {
		lines = append(lines, r.RenderSectionDivider(SectionDivider{Label: "FORECAST", Width: width}, bg))
		for _, day := range w.Daily {
			lines = append(lines, r.dashPair(day.Label, fmt.Sprintf("%d°  %s", day.Temperature, r.forecastCondition(day.Condition)), 6, bg))
		}
	}
	return r.dashBlock(lines, width, bg)
}

// RenderAgenda renders upcoming events grouped by day with the next event
// emphasised.
func (r Renderer) RenderAgenda(items []AgendaItem, now time.Time, width int) string {
	return r.renderAgenda(items, now, width, false)
}

// RenderAgendaDetail renders the agenda with locations and categories.
func (r Renderer) RenderAgendaDetail(items []AgendaItem, now time.Time, width int) string {
	return r.renderAgenda(items, now, width, true)
}

func (r Renderer) renderAgenda(items []AgendaItem, now time.Time, width int, detail bool) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	if len(items) == 0 {
		return r.dashBlock([]string{lipgloss.NewStyle().Background(bg).Foreground(ws.HintFg).Render("No upcoming events")}, width, bg)
	}
	sorted := append([]AgendaItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	next := ""
	if !detail {
		for _, item := range sorted {
			if !item.Done && item.Start.After(now) {
				next = item.Start.Format(time.RFC3339) + item.Title
				break
			}
		}
	}

	maxItems := 10
	if r.Styles.Density.IsDense() {
		maxItems = 8
	}
	var lines []string
	shown := 0
	for _, group := range groupAgenda(sorted, now) {
		lines = append(lines, r.RenderSectionDivider(SectionDivider{Label: group.label, Width: width}, bg))
		for _, item := range group.items {
			if shown >= maxItems {
				break
			}
			key := item.Start.Format(time.RFC3339) + item.Title
			lines = append(lines, r.renderAgendaItem(item, key == next, detail, width, bg))
			shown++
		}
	}
	return r.dashBlock(lines, width, bg)
}

func (r Renderer) renderAgendaItem(item AgendaItem, isNext, detail bool, width int, bg lipgloss.Color) string {
	ws := r.Styles.Workspace
	timeText := dashTime(item.Start)
	timeStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg)
	titleStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg)
	dot := r.RenderStatusDot(StatusDot{Tone: item.Tone}, bg)
	if item.Done {
		titleStyle = titleStyle.Foreground(ws.BodyDimmedFg)
		timeStyle = timeStyle.Foreground(ws.HintFg)
	}
	if isNext {
		titleStyle = titleStyle.Foreground(ws.FrameActive).Bold(true)
		dot = r.RenderStatusDot(StatusDot{Tone: ToneAccent}, bg)
	}
	left := timeStyle.Render(timeText) + lipgloss.NewStyle().Background(bg).Render("  ") +
		dot + lipgloss.NewStyle().Background(bg).Render(" ") + titleStyle.Render(item.Title)
	suffix := ""
	if isNext {
		suffix = r.RenderBadgeOn(NewBadge("next").WithTone(ToneAccent), bg)
	}
	if detail {
		meta := item.Category
		if item.Location != "" {
			if meta != "" {
				meta += " · "
			}
			meta += item.Location
		}
		if meta != "" {
			left += lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).Render("  " + meta)
		}
	}
	return alignRow(left, "", suffix, width)
}

type agendaGroup struct {
	label string
	items []AgendaItem
}

func groupAgenda(items []AgendaItem, now time.Time) []agendaGroup {
	var groups []agendaGroup
	index := map[string]int{}
	for _, item := range items {
		label := dayLabel(item.Start, now)
		position, ok := index[label]
		if !ok {
			position = len(groups)
			index[label] = position
			groups = append(groups, agendaGroup{label: label})
		}
		groups[position].items = append(groups[position].items, item)
	}
	return groups
}

func dayLabel(t, now time.Time) string {
	switch daysBetween(now, t) {
	case 0:
		return "TODAY"
	case 1:
		return "TOMORROW"
	case -1:
		return "YESTERDAY"
	default:
		return strings.ToUpper(t.Format("Mon Jan 2"))
	}
}

func daysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	a0 := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	b0 := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(b0.Sub(a0).Hours() / 24)
}

// RenderClock renders local time and world clocks. When the pane is wide enough
// the time is drawn as big digital rows; otherwise it falls back to text. The
// date carries the day-period and a sun/moon glyph, a day-progress gauge sits
// under it, and each world clock carries its own glyph.
func (r Renderer) RenderClock(c ClockData, width int) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	period, glyph := r.dayPeriod(c.Local)
	glyphColor := ws.WeatherSun
	if period == "night" {
		glyphColor = ws.SubtitleFg
	}
	timeStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.FrameActive).Bold(true)
	center := lipgloss.NewStyle().Background(bg).Width(width).Align(lipgloss.Center)
	var lines []string
	bigShown := false
	if big := bigTime(clockDigits(c.Local)); lipgloss.Width(big[0]) <= width {
		bigShown = true
		bigStyle := timeStyle.Width(width).Align(lipgloss.Center)
		for _, line := range big {
			lines = append(lines, bigStyle.Render(strings.TrimRight(line, " ")))
		}
		lines = append(lines, "") // spacer below the large clock
	} else {
		lines = append(lines, timeStyle.Render(clockTime(c.Local, c.Hour24)))
	}
	dateLine := lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).
		Render(c.Local.Format("Mon Jan 2")+" · "+period) +
		lipgloss.NewStyle().Background(bg).Foreground(glyphColor).Render(" "+glyph)
	if bigShown {
		lines = append(lines, center.Render(dateLine))
	} else {
		lines = append(lines, dateLine)
	}
	if barWidth := min(width-2, 18); barWidth >= 6 {
		fraction := dayFraction(c.Local)
		bar := r.RenderProgressBar(ProgressBar{Fraction: fraction, Width: barWidth, Tone: ToneAccent}, bg)
		pct := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
			Render(fmt.Sprintf("  %d%%", int(math.Round(fraction*100))))
		if bigShown {
			lines = append(lines, center.Render(bar+pct))
		} else {
			lines = append(lines, bar+pct)
		}
	}
	if c.Location != "" {
		loc := lipgloss.NewStyle().Background(bg).Foreground(ws.HintFg).Render(c.Location)
		if bigShown {
			loc = center.Render(loc)
		}
		lines = append(lines, loc)
	}
	if len(c.Zones) > 0 {
		lines = append(lines, "")
		for _, zone := range c.Zones {
			label := zone.City
			if zone.Offset != "" {
				label = fmt.Sprintf("%s %s", zone.City, zone.Offset)
			}
			_, zoneGlyph := r.dayPeriod(zone.Time)
			lines = append(lines, r.dashPair(label, clockTime(zone.Time, c.Hour24)+" "+zoneGlyph, 12, bg))
		}
	}
	return r.dashBlock(lines, width, bg)
}

// RenderClockDetail renders the clock as a big digital time over a day-progress
// gauge, an analog face, the world clocks, and a month calendar.
func (r Renderer) RenderClockDetail(c ClockData, width int) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	period, _ := r.dayPeriod(c.Local)
	var parts []string

	timeStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.FrameActive).Bold(true)
	if big := bigTime(clockDigits(c.Local)); lipgloss.Width(big[0]) <= width {
		for _, line := range big {
			parts = append(parts, timeStyle.Render(line))
		}
	} else {
		parts = append(parts, timeStyle.Render(clockTime(c.Local, c.Hour24)))
	}
	parts = append(parts, lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).
		Render(c.Local.Format("Monday, Jan 2")+" · "+period))

	if barWidth := min(max(width-14, 0), 24); barWidth >= 6 {
		fraction := dayFraction(c.Local)
		bar := r.RenderProgressBar(ProgressBar{Fraction: fraction, Width: barWidth, Tone: ToneAccent}, bg)
		pct := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
			Render(fmt.Sprintf("  %d%% of day", int(math.Round(fraction*100))))
		parts = append(parts, bar+pct)
	}

	if width >= 15 {
		for _, line := range r.analogClock(c.Local) {
			parts = append(parts, lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Render(line))
		}
	}

	if len(c.Zones) > 0 {
		parts = append(parts, "")
		for _, zone := range c.Zones {
			label := zone.City
			if zone.Offset != "" {
				label = fmt.Sprintf("%s %s", zone.City, zone.Offset)
			}
			_, zoneGlyph := r.dayPeriod(zone.Time)
			parts = append(parts, r.dashPair(label, clockTime(zone.Time, c.Hour24)+" "+zoneGlyph, 12, bg))
		}
	}

	calendarWidth := min(width, 21)
	if calendarWidth >= 15 {
		parts = append(parts, r.RenderMiniCalendar(MiniCalendar{
			Year: c.Local.Year(), Month: c.Local.Month(), Highlight: c.Local.Day(), Width: calendarWidth,
		}, bg))
	}
	return strings.Join(parts, "\n")
}

// clockTime formats a time as 24-hour ("15:04") or 12-hour ("3:04 PM").
func clockTime(t time.Time, hour24 bool) string {
	if hour24 {
		return t.Format("15:04")
	}
	return t.Format("3:04 PM")
}

// clockDigits formats a time without a meridiem, for the big digital face.
func clockDigits(t time.Time) string {
	hour := t.Hour()
	if hour > 12 {
		hour -= 12
	}
	if hour == 0 {
		hour = 12
	}
	return fmt.Sprintf("%d:%02d", hour, t.Minute())
}

// dayPeriod returns a short label and a sun/moon glyph for a time of day.
func (r Renderer) dayPeriod(t time.Time) (string, string) {
	label := "night"
	glyph := "\U0001F319\uFE0F" // crescent moon
	switch h := t.Hour(); {
	case h >= 5 && h < 12:
		label, glyph = "morning", "\U0001F31E\uFE0F" // sun with face
	case h >= 12 && h < 17:
		label, glyph = "afternoon", "\U0001F31E\uFE0F"
	case h >= 17 && h < 21:
		label, glyph = "evening", "\U0001F31E\uFE0F"
	}
	if r.Styles.PlainUI {
		if label == "night" {
			glyph = "z"
		} else {
			glyph = "*"
		}
	}
	return label, glyph
}

// dayFraction is how far through the day a time is, 0..1.
func dayFraction(t time.Time) float64 {
	return clamp01(float64(t.Hour()*60+t.Minute()) / (24 * 60))
}

// bigClockFont is a 3-row seven-segment-ish font for the large digital time.
var bigClockFont = map[rune][3]string{
	'0': {" _ ", "| |", "|_|"},
	'1': {"   ", "  |", "  |"},
	'2': {" _ ", " _|", "|_ "},
	'3': {" _ ", " _|", " _|"},
	'4': {"   ", "|_|", "  |"},
	'5': {" _ ", "|_ ", " _|"},
	'6': {" _ ", "|_ ", "|_|"},
	'7': {" _ ", "  |", "  |"},
	'8': {" _ ", "|_|", "|_|"},
	'9': {" _ ", "|_|", " _|"},
	':': {"   ", " . ", " . "},
}

// bigTime renders "14:42" as three rows of large digits.
func bigTime(text string) []string {
	rows := []string{"", "", ""}
	for i, ch := range text {
		glyph, ok := bigClockFont[ch]
		if !ok {
			glyph = [3]string{"   ", "   ", "   "}
		}
		for row := 0; row < 3; row++ {
			if i > 0 {
				rows[row] += " "
			}
			rows[row] += glyph[row]
		}
	}
	return rows
}

// analogClock draws a small clock face: a rim, quarter marks, and hour and
// minute hands. Cells are about twice as tall as wide, so the radius is wider
// horizontally to keep the face round.
func (r Renderer) analogClock(t time.Time) []string {
	const cx, cy = 6, 3
	const rx, ry = 6, 3
	grid := make([][]rune, cy*2+1)
	for i := range grid {
		grid[i] = make([]rune, cx*2+1)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	put := func(x, y int, ch rune) {
		if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[0]) {
			grid[y][x] = ch
		}
	}
	// Rim: mark every cell near the ellipse boundary, so it comes out even.
	for y := range grid {
		for x := range grid[y] {
			dx := float64(x-cx) / float64(rx)
			dy := float64(y-cy) / float64(ry)
			if d := math.Sqrt(dx*dx + dy*dy); d > 0.82 && d < 1.18 {
				grid[y][x] = '·'
			}
		}
	}
	for _, hour := range []int{0, 3, 6, 9} {
		a := float64(hour) * math.Pi / 6
		put(cx+int(math.Round(rx*math.Sin(a))), cy-int(math.Round(ry*math.Cos(a))), '•')
	}
	hand := func(angle, length float64, ch rune) {
		for step := 1; step <= 12; step++ {
			f := length * float64(step) / 12
			put(cx+int(math.Round(f*rx*math.Sin(angle))), cy-int(math.Round(f*ry*math.Cos(angle))), ch)
		}
	}
	hourAngle := (float64(t.Hour()%12) + float64(t.Minute())/60) / 12 * 2 * math.Pi
	minuteAngle := float64(t.Minute()) / 60 * 2 * math.Pi
	hand(minuteAngle, 0.85, '•')
	hand(hourAngle, 0.55, '●')
	put(cx, cy, '○')
	lines := make([]string, len(grid))
	for i, row := range grid {
		lines[i] = string(row)
	}
	return lines
}

// RenderSystem renders the system-health summary.
func (r Renderer) RenderSystem(m SystemMetrics, width int) string {
	bg := r.Styles.Workspace.Bg
	total := metricTotal(width, 5, 5, 20)
	rows := []MetricRow{
		{Label: "CPU", Value: fmt.Sprintf("%.0f%%", m.CPUPercent), Fraction: m.CPUPercent / 100,
			Spark: m.CPUSpark, Tone: toneForPercent(m.CPUPercent), LabelWidth: 5, ValueWidth: 5, TotalWidth: total},
		{Label: "MEM", Value: fmt.Sprintf("%.0f%%", m.MemoryPercent), Fraction: m.MemoryPercent / 100,
			Bar: true, Tone: toneForPercent(m.MemoryPercent), LabelWidth: 5, ValueWidth: 5, TotalWidth: total},
	}
	lines := make([]string, 0, len(rows)+3)
	for _, row := range rows {
		lines = append(lines, r.RenderMetricRow(row, bg))
	}
	lines = append(lines, r.dashPair("TEMP", fmt.Sprintf("%d°C", m.TemperatureC), 5, bg))
	lines = append(lines, r.dashPair("LOAD", fmt.Sprintf("%.1f %.1f %.1f", m.Load[0], m.Load[1], m.Load[2]), 5, bg))
	lines = append(lines, r.dashPair("UP", formatUptime(m.Uptime), 5, bg))
	return r.dashBlock(lines, width, bg)
}

// RenderSystemDetail renders per-core load, memory totals, and processes.
func (r Renderer) RenderSystemDetail(m SystemMetrics, width int) string {
	bg := r.Styles.Workspace.Bg
	lines := strings.Split(r.RenderSystem(m, width), "\n")
	if len(m.Cores) > 0 {
		lines = append(lines, r.RenderSectionDivider(SectionDivider{Label: "CORES", Width: width}, bg))
		barWidth := max(4, width-10)
		for i, core := range m.Cores {
			lines = append(lines, r.dashPair(fmt.Sprintf("cpu%d", i), "", 5, bg)+
				r.RenderProgressBar(ProgressBar{Fraction: core / 100, Width: barWidth, Tone: toneForPercent(core)}, bg))
		}
	}
	memory := m.MemoryUsed
	if m.MemoryTotal != "" {
		memory = fmt.Sprintf("%s / %s", m.MemoryUsed, m.MemoryTotal)
	}
	if memory != "" {
		lines = append(lines, r.dashPair("Mem", memory, 5, bg))
	}
	if m.Processes > 0 {
		lines = append(lines, r.dashPair("Proc", fmt.Sprintf("%d", m.Processes), 5, bg))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderNetwork renders throughput and a short-term graph.
func (r Renderer) RenderNetwork(m NetworkMetrics, width int) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	unit := m.Unit
	if unit == "" {
		unit = "Mbps"
	}
	lines := []string{
		lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).Render("↓ ") +
			lipgloss.NewStyle().Background(bg).Foreground(ws.FrameActive).Bold(true).
				Render(fmt.Sprintf("%.0f %s", m.Download, unit)),
		lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).Render("↑ ") +
			lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Bold(true).
				Render(fmt.Sprintf("%.0f %s", m.Upload, unit)),
	}
	if len(m.DownSpark) > 0 {
		graphWidth := min(width, 26)
		lines = append(lines, "", r.RenderSparkline(Sparkline{Values: m.DownSpark, Width: graphWidth, Tone: ToneAccent}, bg))
	}
	if m.Interface != "" {
		lines = append(lines, "", lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
			Render(m.Interface))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderNetworkDetail adds LAN/WAN summaries and an upload graph.
func (r Renderer) RenderNetworkDetail(m NetworkMetrics, width int) string {
	bg := r.Styles.Workspace.Bg
	lines := strings.Split(r.RenderNetwork(m, width), "\n")
	if len(m.UpSpark) > 0 {
		lines = append(lines, "", r.RenderSparkline(Sparkline{Values: m.UpSpark, Width: width, Tone: ToneGood}, bg))
	}
	if m.LAN != "" {
		lines = append(lines, r.dashPair("LAN", m.LAN, 6, bg))
	}
	if m.WAN != "" {
		lines = append(lines, r.dashPair("WAN", m.WAN, 6, bg))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderStorage renders mounted filesystems with gauges.
func (r Renderer) RenderStorage(mounts []StorageMount, width int) string {
	bg := r.Styles.Workspace.Bg
	if len(mounts) == 0 {
		return ""
	}
	labelWidth := 0
	for _, mount := range mounts {
		labelWidth = max(labelWidth, lipgloss.Width(mount.Path))
	}
	labelWidth = min(labelWidth, max(4, width/3))
	lines := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		tone := mount.Tone
		if tone == ToneNeutral {
			tone = toneForPercent(mount.UsedPercent)
		}
		lines = append(lines, r.RenderMetricRow(MetricRow{
			Label: mount.Path, Value: fmt.Sprintf("%.0f%%", mount.UsedPercent),
			Fraction: mount.UsedPercent / 100, Bar: true, Tone: tone,
			LabelWidth: labelWidth, ValueWidth: 4, TotalWidth: metricTotal(width, labelWidth, 4, 28),
		}, bg))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderServices renders service/container statuses.
func (r Renderer) RenderServices(items []ServiceStatus, width int) string {
	return r.renderServices(items, width, false)
}

// RenderServicesDetail adds each service's detail and uptime.
func (r Renderer) RenderServicesDetail(items []ServiceStatus, width int) string {
	return r.renderServices(items, width, true)
}

func (r Renderer) renderServices(items []ServiceStatus, width int, detail bool) string {
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	if len(items) == 0 {
		return ""
	}
	plain := r.Styles.PlainUI
	nameWidth, stateWidth, ageWidth := 0, 0, 0
	for _, item := range items {
		_, label, _ := item.resolved()
		nameWidth = max(nameWidth, lipgloss.Width(item.Name))
		stateWidth = max(stateWidth, lipgloss.Width(label))
		ageWidth = max(ageWidth, lipgloss.Width(serviceAge(item)))
	}
	nameWidth = min(nameWidth, max(4, width/3))
	stateWidth = min(stateWidth, 9)
	ageWidth = min(ageWidth, 5)

	var lines []string
	for _, item := range items {
		kind, label, tone := item.resolved()
		dot := lipgloss.NewStyle().Background(bg).Foreground(r.toneColor(tone)).
			Render(kind.Glyph(plain))
		name := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).
			Render(padRight(ansi.Truncate(item.Name, nameWidth, "…"), nameWidth))
		state := lipgloss.NewStyle().Background(bg).Foreground(r.toneColor(tone)).
			Render(padRight(ansi.Truncate(label, stateWidth, "…"), stateWidth))
		age := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
			Render(padRight(ansi.Truncate(serviceAge(item), ageWidth, "…"), ageWidth))
		// Keep name, state, and age as one grouped column set rather than
		// flinging the age to the far edge of a wide panel.
		lines = append(lines, dot+" "+name+" "+state+"  "+age)
		if detail && item.Detail != "" {
			extra := item.Detail
			if item.Uptime != "" {
				extra += "  ·  up " + item.Uptime
			}
			lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).
				Render("  "+extra))
		}
	}
	return r.dashBlock(lines, width, bg)
}

// serviceAge returns the compact age column, defaulting to "--".
func serviceAge(item ServiceStatus) string {
	if item.Age != "" {
		return item.Age
	}
	return "--"
}

// RenderHeadlines renders a compact headline list.
func (r Renderer) RenderHeadlines(items []Headline, width int) string {
	return r.renderHeadlines(items, width, false)
}

// RenderHeadlinesDetail renders more headlines with sources.
func (r Renderer) RenderHeadlinesDetail(items []Headline, width int) string {
	return r.renderHeadlines(items, width, true)
}

func (r Renderer) renderHeadlines(items []Headline, width int, detail bool) string {
	if len(items) == 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	limit := 6
	if detail {
		limit = 12
	}
	twoLine := !r.Styles.Density.IsDense()
	var lines []string
	for i, item := range items {
		if i >= limit {
			break
		}
		// Headlines stay neutral; only the unread marker carries accent so the
		// panel never becomes a wall of colour.
		titleStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg)
		if !item.Unread {
			titleStyle = titleStyle.Foreground(ws.BodyDimmedFg)
		}
		marker := " "
		if item.Unread {
			marker = lipgloss.NewStyle().Background(bg).Foreground(r.toneColor(ToneAccent)).
				Render(StatusHealthy.Glyph(r.Styles.PlainUI))
		}
		title := titleStyle.Render(item.Title)
		if twoLine {
			lines = append(lines, marker+lipgloss.NewStyle().Background(bg).Render(" ")+title)
			meta := item.Source
			if item.Age != "" {
				meta += "  ·  " + item.Age
			}
			lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).Render("   "+meta))
		} else {
			meta := item.Source
			if item.Age != "" {
				meta += " " + item.Age
			}
			lines = append(lines, alignRow(marker+" "+title, "", lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).Render(meta), width))
		}
	}
	return r.dashBlock(lines, width, bg)
}

// RenderTasks renders a task list with checkboxes and due dates.
func (r Renderer) RenderTasks(tasks []Task, width int) string {
	if len(tasks) == 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	plain := r.Styles.PlainUI
	var lines []string
	for _, task := range tasks {
		box := "□"
		tone := ToneMuted
		if task.Done {
			box = "✓"
			tone = ToneGood
		} else if task.Tone != ToneNeutral {
			tone = task.Tone
		}
		if plain {
			if task.Done {
				box = "[x]"
			} else {
				box = "[ ]"
			}
		}
		boxStyle := lipgloss.NewStyle().Background(bg).Foreground(r.toneColor(tone))
		titleStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg)
		if task.Done {
			titleStyle = titleStyle.Foreground(ws.BodyDimmedFg)
		}
		left := boxStyle.Render(box) + lipgloss.NewStyle().Background(bg).Render(" ") + titleStyle.Render(task.Title)
		if len(task.Tags) > 0 && !r.Styles.Density.IsDense() {
			left += lipgloss.NewStyle().Background(bg).Foreground(ws.SubtitleFg).Render("  " + strings.Join(task.Tags, " "))
		}
		right := ""
		if task.Due != "" {
			dueStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg)
			if !task.Done && strings.EqualFold(task.Due, "today") {
				dueStyle = dueStyle.Foreground(r.toneColor(ToneWarning))
			}
			right = dueStyle.Render(task.Due)
		}
		lines = append(lines, alignRow(left, "", right, width))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderNotes renders pinned and short notes.
func (r Renderer) RenderNotes(items []Note, width int) string {
	if len(items) == 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	var lines []string
	for i, note := range items {
		if i > 0 {
			lines = append(lines, "")
		}
		title := note.Title
		if note.Pinned {
			title = "★ " + title
		}
		if title != "" {
			lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Bold(true).Render(title))
		}
		for _, bodyLine := range strings.Split(note.Body, "\n") {
			lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
				Render("  • "+strings.TrimSpace(strings.TrimPrefix(bodyLine, "- "))))
		}
	}
	return r.dashBlock(lines, width, bg)
}

// RenderRepoActivity renders a compact repository activity list.
func (r Renderer) RenderRepoActivity(items []RepoActivity, width int) string {
	if len(items) == 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	labelWidth := 0
	for _, item := range items {
		labelWidth = max(labelWidth, lipgloss.Width(item.Name))
	}
	labelWidth = min(labelWidth, max(4, width/3))
	var lines []string
	for _, item := range items {
		tone := item.Tone
		if tone == ToneNeutral {
			tone = ToneAccent
		}
		left := lipgloss.NewStyle().Background(bg).Foreground(r.toneColor(tone)).Bold(true).
			Render(padRight(item.Name, labelWidth))
		summary := item.Summary
		if !r.Styles.Density.IsDense() && item.Branch != "" {
			mark := "✓"
			if r.Styles.PlainUI {
				mark = "ok"
			}
			summary = strings.TrimSpace(item.Branch + " " + mark + "  " + summary)
		}
		left += lipgloss.NewStyle().Background(bg).Render(" ") +
			lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).Render(summary)
		right := ""
		if item.Commits > 0 {
			right = lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).
				Render(fmt.Sprintf("%d commits", item.Commits))
		}
		lines = append(lines, alignRow(left, "", right, width))
	}
	return r.dashBlock(lines, width, bg)
}

// RenderMarkets renders an aligned watchlist.
func (r Renderer) RenderMarkets(items []MarketQuote, width int) string {
	if len(items) == 0 {
		return ""
	}
	bg := r.Styles.Workspace.Bg
	ws := r.Styles.Workspace
	symbolWidth := 0
	for _, item := range items {
		symbolWidth = max(symbolWidth, lipgloss.Width(item.Symbol))
	}
	var lines []string
	for _, item := range items {
		price := fmt.Sprintf("%.2f", item.Price)
		if item.Currency != "" {
			price += " " + item.Currency
		}
		left := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Bold(true).
			Render(padRight(item.Symbol, symbolWidth))
		right := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Render(price) +
			lipgloss.NewStyle().Background(bg).Render("  ") + r.RenderTrend(item.ChangePct, "%", bg)
		lines = append(lines, alignRow(left, "", right, width))
	}
	return r.dashBlock(lines, width, bg)
}

// metricTotal caps a metric row's width so its gauge or sparkline does not
// stretch across a very wide panel.
func metricTotal(width, labelWidth, valueWidth, maxPlot int) int {
	return min(width, labelWidth+valueWidth+4+maxPlot)
}

// toneForPercent maps a utilisation percentage to a metric tone.
func toneForPercent(percent float64) Tone {
	switch {
	case percent >= 85:
		return ToneDanger
	case percent >= 70:
		return ToneWarning
	default:
		return ToneGood
	}
}

// formatUptime renders a duration in a compact dashboard form.
func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
