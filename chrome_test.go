package tideui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func chromeRenderer(density Density) Renderer {
	return NewRenderer(CatppuccinMocha, StyleOptions{Density: density, PaneCorners: RoundCorners})
}

// withTrueColor forces a colour-capable profile for tests that compare raw
// styled output (colour is otherwise stripped in a non-TTY test process).
func withTrueColor(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func TestBadgeTonesAndWidth(t *testing.T) {
	withTrueColor(t)
	r := chromeRenderer(Compact)
	tones := []Tone{ToneNeutral, ToneAccent, ToneGood, ToneWarning, ToneDanger, ToneMuted}
	seen := map[string]bool{}
	for _, tone := range tones {
		rendered := r.RenderBadge(NewBadge("9").WithTone(tone))
		if got := lipgloss.Width(rendered); got != 3 {
			t.Fatalf("tone %d badge width = %d, want 3", tone, got)
		}
		if got := ansi.Strip(rendered); got != " 9 " {
			t.Fatalf("tone %d badge text = %q", tone, got)
		}
		seen[rendered] = true
	}
	if len(seen) < 3 {
		t.Fatalf("badge tones are not visually distinct (%d variants)", len(seen))
	}
	if got := r.RenderBadge(NewBadge("")); got != "" {
		t.Fatalf("empty badge = %q, want empty", got)
	}
}

func TestKeyHintsLabelsAndNarrowFallback(t *testing.T) {
	r := chromeRenderer(Compact)
	hints := []KeyHint{Hint("w", "save"), Hint("r", "run"), Hint("f", "format")}

	wide := r.RenderKeyHints(hints, 80)
	if lipgloss.Width(wide) > 80 {
		t.Fatalf("wide hints exceed budget: %d", lipgloss.Width(wide))
	}
	if !strings.Contains(ansi.Strip(wide), "save") {
		t.Fatalf("wide hints lost labels: %q", ansi.Strip(wide))
	}

	narrow := r.RenderKeyHints(hints, 12)
	if lipgloss.Width(narrow) > 12 {
		t.Fatalf("narrow hints exceed budget: %d", lipgloss.Width(narrow))
	}
	// Labels drop per-hint as space runs out, so the last hint loses its label
	// or is omitted entirely while the leading keys survive.
	narrowPlain := ansi.Strip(narrow)
	if strings.Contains(narrowPlain, "format") {
		t.Fatalf("narrow hints kept a trailing label: %q", narrowPlain)
	}
	if !strings.Contains(narrowPlain, "w") {
		t.Fatalf("narrow hints lost keys: %q", narrowPlain)
	}

	if got := r.RenderKeyHints(nil, 20); got != "" {
		t.Fatalf("no hints = %q, want empty", got)
	}
	if got := r.RenderKeyHints(hints, 0); got != "" {
		t.Fatalf("zero width = %q, want empty", got)
	}
}

// Key hints draw a key as the symbol its icon style calls for, and an ASCII
// theme falls back to the key's name.
func TestKeyGlyphFollowsIconStyle(t *testing.T) {
	cases := map[IconStyle]map[string]string{
		IconPlain: {
			"enter": "↵", "esc": "␛", "tab": "↹", "shift+tab": "⇤", "space": "␣",
			"up": "▲", "down": "▼", "left": "◀", "right": "▶",
			"shift+space": "⇧␣", "ctrl+p": "⌃p",
			"m": "m", "⇧arrows": "⇧arrows",
		},
		IconEmoji: {
			"enter": "\u21a9\ufe0f",
			"up":    "\u2b06\ufe0f", "down": "\u2b07\ufe0f",
			"left": "\u2b05\ufe0f", "right": "\u27a1\ufe0f",
		},
		IconNerd: {
			"enter": "\uf0311", "esc": "\uf12b7", "tab": "\uf0312",
			"backspace": "\uf030d", "up": "\uf005e",
		},
	}
	for style, wants := range cases {
		r := NewRenderer(CatppuccinMocha, StyleOptions{IconStyle: style})
		for key, want := range wants {
			if got := r.keyGlyph(key); got != want {
				t.Fatalf("%s %q = %q, want %q", style, key, got, want)
			}
		}
	}
	if got := NewRenderer(VT52, StyleOptions{}).keyGlyph("enter"); got != "enter" {
		t.Fatalf("ascii enter = %q, want the key name", got)
	}
}

// Dense keeps a footer label while it fits, because a bare key is not
// discoverable; the key with no label is the narrow-panel fallback.
func TestKeyHintsDenseKeepsLabelsWhileTheyFit(t *testing.T) {
	r := chromeRenderer(Dense)
	wide := ansi.Strip(r.RenderKeyHints([]KeyHint{Hint("w", "save")}, 80))
	if !strings.Contains(wide, "save") {
		t.Fatalf("dense hints dropped a label that fits: %q", wide)
	}
	narrow := ansi.Strip(r.RenderKeyHints([]KeyHint{Hint("w", "save")}, 6))
	if !strings.Contains(narrow, "w") || strings.Contains(narrow, "save") {
		t.Fatalf("narrow dense hints = %q, want the bare key", narrow)
	}
}

func TestProgressBarBounds(t *testing.T) {
	r := chromeRenderer(Compact)
	bg := r.Styles.Workspace.Bg
	cases := []struct {
		fraction float64
		filled   int
	}{
		{0, 0}, {0.5, 5}, {1, 10}, {-1, 0}, {2, 10},
	}
	for _, tc := range cases {
		bar := ansi.Strip(r.RenderProgressBar(ProgressBar{Fraction: tc.fraction, Width: 10, Tone: ToneGood}, bg))
		if got := lipgloss.Width(bar); got != 10 {
			t.Fatalf("bar width = %d, want 10", got)
		}
		if got := strings.Count(bar, "█"); got != tc.filled {
			t.Fatalf("fraction %v filled = %d, want %d", tc.fraction, got, tc.filled)
		}
	}
}

func TestMetricGradientScale(t *testing.T) {
	ws := chromeRenderer(Compact).Styles.Workspace
	if got := ws.MetricGradient(0); got != ws.MetricGood {
		t.Fatalf("gradient(0) = %s, want MetricGood %s", got, ws.MetricGood)
	}
	if got := ws.MetricGradient(1); got != ws.MetricBad {
		t.Fatalf("gradient(1) = %s, want MetricBad %s", got, ws.MetricBad)
	}
	if ws.MetricGradient(0.5) == ws.MetricGood || ws.MetricGradient(0.5) == ws.MetricBad {
		t.Fatal("mid gradient should sit between the extremes")
	}
	low, high := valueRange([]float64{0.4, 0.1, 0.9, 0.2})
	if low != 0.1 || high != 0.9 {
		t.Fatalf("valueRange = %v..%v, want 0.1..0.9", low, high)
	}
}

func TestGaugeStyles(t *testing.T) {
	for _, style := range GaugeStyles() {
		r := NewRenderer(CatppuccinMocha, StyleOptions{Gauge: style})
		if r.Styles.Gauge != style {
			t.Fatalf("resolved gauge = %q, want %q", r.Styles.Gauge, style)
		}
		// Every family, at every width from a sliver to a wide pane, is exactly
		// the width asked for: a gauge that overflows or drifts breaks a row.
		for _, width := range []int{1, 2, 3, 5, 8, 10, 14, 15, 20, 40} {
			got := r.RenderProgressBar(ProgressBar{Fraction: 0.5, Width: width, Tone: ToneGood}, r.Styles.Workspace.Bg)
			if w := lipgloss.Width(got); w != width {
				t.Fatalf("%s gauge at width %d rendered %d cells (%q)", style, width, w, ansi.Strip(got))
			}
		}
	}
	unknown := NewRenderer(CatppuccinMocha, StyleOptions{Gauge: GaugeStyle("nope")})
	if unknown.Styles.Gauge != GaugeBlock {
		t.Fatalf("unknown gauge = %q, want block", unknown.Styles.Gauge)
	}
}

// The glyph sets that predate the two standard shapes keep working: a config
// written before the change resolves to the shape that reads most like it.
func TestLegacyGaugeStylesMapToAFamily(t *testing.T) {
	cases := map[GaugeStyle]GaugeStyle{
		"solid": GaugeBlock, "bars": GaugeBlock,
		"blocks": GaugeSegment, "circles": GaugeSegment,
		"fisheye": GaugeSegment, "marker": GaugeSegment,
	}
	for legacy, want := range cases {
		r := NewRenderer(CatppuccinMocha, StyleOptions{Gauge: legacy})
		if r.Styles.Gauge != want {
			t.Errorf("legacy %q resolved to %q, want %q", legacy, r.Styles.Gauge, want)
		}
	}
}

// Each shape is its own, and the fill follows the value.
func TestGaugeFamiliesDifferAndTrackTheValue(t *testing.T) {
	r := NewRenderer(CatppuccinMocha, StyleOptions{})
	bg := r.Styles.Workspace.Bg
	drawn := map[string]string{}
	for _, style := range GaugeStyles() {
		empty := ansi.Strip(r.RenderProgressBar(ProgressBar{Fraction: 0, Width: 17, Style: style}, bg))
		full := ansi.Strip(r.RenderProgressBar(ProgressBar{Fraction: 1, Width: 17, Style: style}, bg))
		if empty == full {
			t.Errorf("%s draws the same at 0 and 1: %q", style, empty)
		}
		if seen, dup := drawn[empty]; dup {
			t.Errorf("%s and %s draw the same empty gauge: %q", style, seen, empty)
		}
		drawn[empty] = string(style)
	}
}

func TestSparklineStyles(t *testing.T) {
	for _, style := range SparklineStyles() {
		r := NewRenderer(CatppuccinMocha, StyleOptions{Sparkline: style})
		if r.Styles.Sparkline != style {
			t.Fatalf("resolved sparkline = %q, want %q", r.Styles.Sparkline, style)
		}
		sample := r.SparkSample(style, 9)
		if w := lipgloss.Width(sample); w != 9 {
			t.Fatalf("%s sample width = %d, want 9 (%q)", style, w, sample)
		}
		line := r.RenderSparkline(Sparkline{Values: []float64{0.2, 0.8, 0.4}, Width: 6, Tone: ToneAccent}, r.Styles.Workspace.Bg)
		if w := lipgloss.Width(line); w != 6 {
			t.Fatalf("%s sparkline width = %d, want 6", style, w)
		}
	}
	unknown := NewRenderer(CatppuccinMocha, StyleOptions{Sparkline: SparklineStyle("nope")})
	if unknown.Styles.Sparkline != SparkBlocks {
		t.Fatalf("unknown sparkline = %q, want blocks", unknown.Styles.Sparkline)
	}
}

func TestPanelSparklineOverride(t *testing.T) {
	wr := NewWorkspaceRenderer(NewRenderer(CatppuccinMocha, StyleOptions{Sparkline: SparkBlocks}))
	panel := newPanel("system", nil)
	if got := wr.panelRenderer(panel).Styles.Sparkline; got != SparkBlocks {
		t.Fatalf("default panel sparkline = %q, want blocks", got)
	}
	panel.Sparkline(SparkDots)
	if style, ok := panel.PanelSparkline(); !ok || style != SparkDots {
		t.Fatalf("PanelSparkline = %q,%v, want dots", style, ok)
	}
	if got := wr.panelRenderer(panel).Styles.Sparkline; got != SparkDots {
		t.Fatalf("panel sparkline = %q, want dots", got)
	}
	panel.ClearSparkline()
	if got := wr.panelRenderer(panel).Styles.Sparkline; got != SparkBlocks {
		t.Fatalf("cleared panel sparkline = %q, want blocks", got)
	}
}

func TestGaugeSample(t *testing.T) {
	for _, style := range GaugeStyles() {
		r := NewRenderer(CatppuccinMocha, StyleOptions{Gauge: style})
		sample := r.GaugeSample(style, 8)
		if w := lipgloss.Width(sample); w != 8 {
			t.Fatalf("%s sample width = %d, want 8 (%q)", style, w, sample)
		}
	}
	// The sample differs between styles, so the picker previews are distinct.
	a := NewRenderer(CatppuccinMocha, StyleOptions{}).GaugeSample(GaugeBlock, 8)
	b := NewRenderer(CatppuccinMocha, StyleOptions{}).GaugeSample(GaugeSegment, 8)
	if a == b {
		t.Fatalf("block and segment samples should differ: %q", a)
	}
}

func TestPanelGaugeOverride(t *testing.T) {
	wr := NewWorkspaceRenderer(NewRenderer(CatppuccinMocha, StyleOptions{Gauge: GaugeSegment}))
	panel := newPanel("system", nil)
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeSegment {
		t.Fatalf("default panel gauge = %q, want segment", got)
	}
	panel.Gauge(GaugeBlock)
	if style, ok := panel.PanelGauge(); !ok || style != GaugeBlock {
		t.Fatalf("PanelGauge = %q,%v, want block", style, ok)
	}
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeBlock {
		t.Fatalf("panel gauge = %q, want block", got)
	}
	panel.ClearGauge()
	if panel.HasPanelGauge() {
		t.Fatal("ClearGauge did not clear the override")
	}
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeSegment {
		t.Fatalf("cleared panel gauge = %q, want segment", got)
	}
}

func TestSparklineWidth(t *testing.T) {
	r := chromeRenderer(Compact)
	got := r.RenderSparkline(Sparkline{Values: []float64{0.1, 0.9, 0.4}, Width: 8, Tone: ToneAccent}, r.Styles.Workspace.Bg)
	if w := lipgloss.Width(got); w != 8 {
		t.Fatalf("sparkline width = %d, want 8", w)
	}
	empty := r.RenderSparkline(Sparkline{Width: 6}, r.Styles.Workspace.Bg)
	if w := lipgloss.Width(empty); w != 6 {
		t.Fatalf("empty sparkline width = %d, want 6", w)
	}
}

func TestMetricRowAlignment(t *testing.T) {
	r := chromeRenderer(Compact)
	bg := r.Styles.Workspace.Bg
	row := MetricRow{Label: "CPU", Value: "42%", Fraction: 0.42, Bar: true,
		Tone: ToneGood, LabelWidth: 5, ValueWidth: 5, TotalWidth: 30}
	rendered := r.RenderMetricRow(row, bg)
	if got := lipgloss.Width(rendered); got != 30 {
		t.Fatalf("metric row width = %d, want 30", got)
	}
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "CPU") || !strings.Contains(plain, "42%") {
		t.Fatalf("metric row missing content: %q", plain)
	}

	unaligned := r.RenderMetricRow(MetricRow{Label: "NET", Value: "8M"}, bg)
	if !strings.Contains(ansi.Strip(unaligned), "8M") {
		t.Fatalf("metric row without totals missing value")
	}
}

func TestTabStripActiveInactive(t *testing.T) {
	withTrueColor(t)
	r := chromeRenderer(Compact)
	items := []TabItem{{Title: "Inspector", Badge: "3"}, {Title: "Metrics"}}
	strip, used := r.RenderTabStrip(items, 1, true, 40)
	if used <= 0 {
		t.Fatal("tab strip consumed no width")
	}
	if got := lipgloss.Width(strip); got != used {
		t.Fatalf("tab strip width = %d, consumed %d", got, used)
	}
	plain := ansi.Strip(strip)
	if !strings.Contains(plain, "Inspector") || !strings.Contains(plain, "Metrics") {
		t.Fatalf("tab strip missing titles: %q", plain)
	}
	if !strings.Contains(plain, "3") {
		t.Fatalf("tab strip missing badge: %q", plain)
	}

	active, _ := r.RenderTabStrip(items, 0, true, 40)
	inactive, _ := r.RenderTabStrip(items, 0, false, 40)
	if active == inactive {
		t.Fatal("active and inactive tab strips should differ")
	}
}

func TestPanelHeaderTruncationAndBadge(t *testing.T) {
	r := chromeRenderer(Compact)
	badge := NewBadge("saved").WithTone(ToneGood)
	wide := r.RenderPanelHeader(PanelHeader{
		Title: "Editor", Subtitle: "main.go", Badge: &badge,
		Border: lipgloss.NormalBorder(), Density: Compact, Focused: true,
	}, 40)
	if got := lipgloss.Width(wide); got != 40 {
		t.Fatalf("header width = %d, want 40", got)
	}
	plain := ansi.Strip(wide)
	for _, want := range []string{"Editor", "main.go", "saved"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("header missing %q: %q", want, plain)
		}
	}

	narrow := r.RenderPanelHeader(PanelHeader{
		Title: "A very long panel title indeed", Badge: &badge,
		Border: lipgloss.NormalBorder(), Density: Compact,
	}, 14)
	if got := lipgloss.Width(narrow); got != 14 {
		t.Fatalf("narrow header width = %d, want 14", got)
	}
	if !strings.Contains(ansi.Strip(narrow), "…") {
		t.Fatalf("narrow header should truncate: %q", ansi.Strip(narrow))
	}

	tiny := r.RenderPanelHeader(PanelHeader{Title: "Title", Border: lipgloss.NormalBorder(), Density: Compact}, 3)
	if got := lipgloss.Width(tiny); got != 3 {
		t.Fatalf("tiny header width = %d, want 3", got)
	}
}

// A pane header carrying a colour glyph - the mail pane's envelope, and every other pane
// the host gives an emoji - is still exactly the width the frame asked for. A colour emoji
// is two cells wide and some are several runes, so this is the case where counting runes
// instead of cells would push the pane's own border out by a column.
func TestPanelHeaderKeepsItsWidthWithAColourGlyph(t *testing.T) {
	r := chromeRenderer(Compact)
	for _, glyph := range []string{"📧", "🌤️", "📅", "⚙️", "🔌"} {
		for _, width := range []int{3, 4, 6, 12, 40, 118} {
			header := r.RenderPanelHeader(PanelHeader{
				Title:  glyph + " Mail",
				Border: lipgloss.NormalBorder(), Density: Compact,
			}, width)
			if got := lipgloss.Width(header); got != width {
				t.Errorf("a header with %q at width %d came out %d cells wide", glyph, width, got)
			}
		}
	}
}

func TestPanelFooterHintsAndMode(t *testing.T) {
	r := chromeRenderer(Compact)
	hints := r.RenderPanelFooter(PanelFooter{
		Hints:  []KeyHint{Hint("s", "stage"), Hint("o", "open")},
		Border: lipgloss.NormalBorder(), Density: Compact, Focused: true,
	}, 30)
	if got := lipgloss.Width(hints); got != 30 {
		t.Fatalf("footer width = %d, want 30", got)
	}
	if !strings.Contains(ansi.Strip(hints), "stage") {
		t.Fatalf("footer missing hint: %q", ansi.Strip(hints))
	}

	mode := r.RenderPanelFooter(PanelFooter{Mode: "arrange", Border: lipgloss.NormalBorder(), Density: Compact, Focused: true}, 30)
	if !strings.Contains(ansi.Strip(mode), "ARRANGE") {
		t.Fatalf("footer missing mode: %q", ansi.Strip(mode))
	}

	unfocused := r.RenderPanelFooter(PanelFooter{Hints: []KeyHint{Hint("s", "stage")}, Border: lipgloss.NormalBorder(), Density: Compact}, 30)
	if strings.Contains(ansi.Strip(unfocused), "stage") {
		t.Fatalf("unfocused footer should not show hints: %q", ansi.Strip(unfocused))
	}
}

func TestFocusChromeDecisions(t *testing.T) {
	r := chromeRenderer(Compact)
	styles := r.Styles
	fc := FocusChrome{Presentation: DefaultFocusPresentation(), Density: Compact}
	if got := fc.FrameColor(styles, true, false, ""); got != styles.Workspace.FrameActive {
		t.Fatalf("focused frame color = %v", got)
	}
	if got := fc.FrameColor(styles, false, true, ""); got != styles.Workspace.FrameDimmed {
		t.Fatalf("dimmed frame color = %v", got)
	}
	if got := fc.FrameColor(styles, false, false, ""); got != styles.Workspace.FrameIdle {
		t.Fatalf("idle frame color = %v", got)
	}
	if !fc.ShowRail(true) || fc.ShowRail(false) {
		t.Fatal("rail should show only when focused")
	}
	off := FocusChrome{Presentation: FocusPresentation{}}
	if off.ShowRail(true) {
		t.Fatal("rail should respect presentation")
	}
}

func TestWorkspaceFocusRailRendering(t *testing.T) {
	r := chromeRenderer(Compact)
	ws := NewWorkspace(WithGap(1))
	ws.Panel("a", Text("alpha")).Title("Alpha").MinWidth(6).MinHeight(3)
	ws.Panel("b", Text("beta")).Title("Beta").MinWidth(6).MinHeight(3)
	ws.Layout(HStack(Leaf("a"), Leaf("b")))
	ws.Focus("a")

	withRail := ansi.Strip(NewWorkspaceRenderer(r).Render(ws, 40, 12))
	if !strings.Contains(withRail, "▎") {
		t.Fatalf("expected focus rail:\n%s", withRail)
	}

	ws.SetFocusPresentation(FocusPresentation{ActiveBorder: true, ActiveTitle: true, KeyHints: true})
	withoutRail := ansi.Strip(NewWorkspaceRenderer(r).Render(ws, 40, 12))
	if strings.Contains(withoutRail, "▎") {
		t.Fatalf("rail should be disabled:\n%s", withoutRail)
	}
}

func TestDensityModes(t *testing.T) {
	for _, density := range []Density{Comfortable, Compact, Dense} {
		styles := BuildStyles(CatppuccinMocha, StyleOptions{Density: density})
		if styles.Density != density {
			t.Fatalf("density = %q, want %q", styles.Density, density)
		}
	}
	if Compact.RowStride() != 1 || Dense.RowStride() != 1 || Comfortable.RowStride() != 2 {
		t.Fatal("row stride per density is wrong")
	}
	if Dense.ShowsSecondary() || !Compact.ShowsSecondary() {
		t.Fatal("secondary metadata visibility per density is wrong")
	}

	compactItem := ansi.Strip(chromeRenderer(Compact).RenderListItem(ListItem{Text: "file", Counter: "12"}, 24))
	if !strings.Contains(compactItem, "12") {
		t.Fatalf("compact list should show counters: %q", compactItem)
	}
	denseItem := ansi.Strip(chromeRenderer(Dense).RenderListItem(ListItem{Text: "file", Counter: "12"}, 24))
	if strings.Contains(denseItem, "12") {
		t.Fatalf("dense list should hide counters: %q", denseItem)
	}

	denseHeader := ansi.Strip(chromeRenderer(Dense).RenderPanelHeader(PanelHeader{
		Title: "Editor", Subtitle: "main.go", Border: lipgloss.NormalBorder(), Density: Dense,
	}, 40))
	if strings.Contains(denseHeader, "main.go") {
		t.Fatalf("dense header should hide subtitle: %q", denseHeader)
	}
	compactHeader := ansi.Strip(chromeRenderer(Compact).RenderPanelHeader(PanelHeader{
		Title: "Editor", Subtitle: "main.go", Border: lipgloss.NormalBorder(), Density: Compact,
	}, 40))
	if !strings.Contains(compactHeader, "main.go") {
		t.Fatalf("compact header should show subtitle: %q", compactHeader)
	}
}

func TestThemeTokenFallbacks(t *testing.T) {
	for _, theme := range BuiltinThemes {
		styles := BuildStyles(theme, StyleOptions{})
		ws := styles.Workspace
		required := map[string]lipgloss.Color{
			"bg": ws.Bg, "surface": ws.SurfaceBg, "frameActive": ws.FrameActive,
			"frameIdle": ws.FrameIdle, "body": ws.BodyFg, "titleIdle": ws.TitleIdleFg,
			"selection": ws.SelectionBg, "separator": ws.Separator, "keyBg": ws.KeyBg,
			"metricGood": ws.MetricGood, "metricBad": ws.MetricBad, "dock": ws.DockColor,
		}
		for name, color := range required {
			if color == "" {
				t.Fatalf("theme %q: token %q is empty", theme.Name, name)
			}
		}
		if ratio := contrastRatio(ws.BodyFg, ws.Bg); ratio < 4.5 {
			t.Fatalf("theme %q: body contrast %.2f < 4.5", theme.Name, ratio)
		}
		if ratio := contrastRatio(ws.FrameActive, ws.Bg); ratio < 4.5 {
			t.Fatalf("theme %q: focused frame contrast %.2f < 4.5", theme.Name, ratio)
		}
	}
}

func TestRenderListItemStates(t *testing.T) {
	withTrueColor(t)
	r := chromeRenderer(Compact)
	active := r.RenderListItem(ListItem{Text: "row", Selected: true, Active: true}, 20)
	inactive := r.RenderListItem(ListItem{Text: "row", Selected: true}, 20)
	if active == inactive {
		t.Fatal("active and inactive selection should differ")
	}
	for _, rendered := range []string{active, inactive} {
		if got := lipgloss.Width(rendered); got != 20 {
			t.Fatalf("list item width = %d, want 20", got)
		}
	}
	plain := ansi.Strip(r.RenderListItem(ListItem{Text: "row", Counter: "9", Meta: "meta"}, 20))
	if !strings.Contains(plain, "row") || !strings.Contains(plain, "9") || !strings.Contains(plain, "meta") {
		t.Fatalf("list item missing content: %q", plain)
	}
}

func TestSectionDividerWidth(t *testing.T) {
	r := chromeRenderer(Compact)
	got := r.RenderSectionDivider(SectionDivider{Label: "SOURCE", Width: 24}, r.Styles.Workspace.Bg)
	if w := lipgloss.Width(got); w != 24 {
		t.Fatalf("divider width = %d, want 24", w)
	}
	if !strings.Contains(ansi.Strip(got), "SOURCE") {
		t.Fatalf("divider missing label: %q", ansi.Strip(got))
	}
}

func TestStatusRegionsDegradeByPriority(t *testing.T) {
	r := chromeRenderer(Compact)
	identity := "tidedeck  ·  catppuccin-mocha  ·  Editor"
	mode := "ARRANGE"
	commands := "tab focus  m arrange  ctrl+p commands"

	wide := ansi.Strip(r.RenderStatusRegions(identity, mode, commands, 100))
	if w := lipgloss.Width(wide); w != 100 {
		t.Fatalf("wide status width = %d, want 100", w)
	}
	for _, want := range []string{"tidedeck", "ARRANGE", "commands"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide status missing %q: %q", want, wide)
		}
	}

	mid := ansi.Strip(r.RenderStatusRegions(identity, mode, commands, 40))
	if lipgloss.Width(mid) != 40 {
		t.Fatalf("mid status width = %d, want 40", lipgloss.Width(mid))
	}
	if !strings.Contains(mid, "ARRANGE") {
		t.Fatalf("mid status dropped the mode capsule: %q", mid)
	}
	if strings.Contains(mid, "commands") {
		t.Fatalf("mid status should drop commands before mode: %q", mid)
	}

	narrow := ansi.Strip(r.RenderStatusRegions(identity, mode, commands, 14))
	if lipgloss.Width(narrow) != 14 {
		t.Fatalf("narrow status width = %d, want 14", lipgloss.Width(narrow))
	}
	if !strings.Contains(narrow, "tidedeck") {
		t.Fatalf("narrow status lost identity: %q", narrow)
	}

	if got := r.RenderStatusRegions(identity, mode, commands, 0); got != "" {
		t.Fatalf("zero-width status = %q, want empty", got)
	}
}

func TestListItemAndHeaderAtTinyWidths(t *testing.T) {
	r := chromeRenderer(Compact)
	for width := 1; width <= 8; width++ {
		item := r.RenderListItem(ListItem{Text: "content", Counter: "9", Selected: true, Active: true}, width)
		if got := lipgloss.Width(item); got > width {
			t.Fatalf("list item width %d exceeds %d", got, width)
		}
		header := r.RenderPanelHeader(PanelHeader{Title: "Panel", Badge: ptrBadge(NewBadge("x")), Border: lipgloss.NormalBorder(), Density: Compact}, width)
		if got := lipgloss.Width(header); got != width {
			t.Fatalf("header width %d != %d", got, width)
		}
		footer := r.RenderPanelFooter(PanelFooter{Hints: []KeyHint{Hint("s", "stage")}, Border: lipgloss.NormalBorder(), Density: Compact, Focused: true}, width)
		if got := lipgloss.Width(footer); got != width {
			t.Fatalf("footer width %d != %d", got, width)
		}
	}
}

func ptrBadge(b Badge) *Badge { return &b }

func TestWorkspaceRenderArrangeLiveMoveBounded(t *testing.T) {
	r := chromeRenderer(Compact)
	for _, size := range [][2]int{{20, 6}, {30, 10}, {80, 24}, {120, 30}} {
		ws := NewWorkspace(WithGap(1))
		ws.Panel("a", Text("a")).Title("Alpha").MinWidth(5).MinHeight(3)
		ws.Panel("b", Text("b")).Title("Beta").MinWidth(5).MinHeight(3)
		ws.Layout(HStack(Leaf("a"), Leaf("b")))
		ws.Focus("a")
		ws.Solve(size[0], size[1])
		ws.EnterArrange()
		if !ws.ArrangeMove(DirRight) {
			t.Fatalf("%v: ArrangeMove failed", size)
		}
		view := NewWorkspaceRenderer(r).Render(ws, size[0], size[1])
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%v: %d lines, want %d", size, len(lines), size[1])
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got > size[0] {
				t.Fatalf("%v: line %d width %d", size, i, got)
			}
		}
	}
}

// The glyph and the colour must be picked from the same number. They were not:
// the glyph came from the absolute value while the colour was scaled to the
// run, so an idle GPU drew eight identical smallest marks and coloured the
// highest of them red — the reddest cell was also the smallest.
func TestSparklinePeakIsTheLargestGlyph(t *testing.T) {
	for _, style := range SparklineStyles() {
		r := NewRenderer(CatppuccinMocha, StyleOptions{Sparkline: style})
		ramp := sparkGlyphs(style)
		// A run that never leaves the bottom of the absolute scale, which is
		// exactly the idle-GPU case that showed the bug.
		values := []float64{0.06, 0.07, 0.06, 0.22}
		out := ansi.Strip(r.RenderSparkline(Sparkline{Values: values, Width: 4}, ""))
		glyphs := []rune(out)
		if len(glyphs) != 4 {
			t.Fatalf("%s: rendered %d cells, want 4 (%q)", style, len(glyphs), out)
		}
		if glyphs[3] != ramp[len(ramp)-1] {
			t.Fatalf("%s: peak drew %q, want the top of the ramp %q (%q)",
				style, string(glyphs[3]), string(ramp[len(ramp)-1]), out)
		}
		if glyphs[0] != ramp[0] {
			t.Fatalf("%s: trough drew %q, want the bottom of the ramp %q (%q)",
				style, string(glyphs[0]), string(ramp[0]), out)
		}
	}
}

// Scaling a run to its own min and max turns sampling noise into a full-height
// swing, so a run that barely moves is drawn at its actual level instead.
func TestSparklineFlatRunsUseTheirActualLevel(t *testing.T) {
	r := NewRenderer(CatppuccinMocha, StyleOptions{Sparkline: SparkDots})
	ramp := sparkGlyphs(SparkDots)

	idle := ansi.Strip(r.RenderSparkline(Sparkline{Values: []float64{0.06, 0.07, 0.06, 0.07}, Width: 4}, ""))
	for _, g := range idle {
		if g != ramp[0] {
			t.Fatalf("a flat idle run should stay at the bottom of the ramp, got %q", idle)
		}
	}
	busy := ansi.Strip(r.RenderSparkline(Sparkline{Values: []float64{0.92, 0.93, 0.92, 0.93}, Width: 4}, ""))
	for _, g := range busy {
		if g != ramp[len(ramp)-1] {
			t.Fatalf("a flat busy run should stay at the top of the ramp, got %q", busy)
		}
	}
}

// Every ramp needs steps a terminal font actually draws differently. The
// bullets ramp was "∙•●", whose first two glyphs render at the same size in
// most monospace fonts, leaving it with two visible levels rather than three.
func TestSparkRampsAreDistinctAndSingleWidth(t *testing.T) {
	for _, style := range SparklineStyles() {
		ramp := sparkGlyphs(style)
		if len(ramp) < 3 {
			t.Fatalf("%s ramp has %d steps, too few to read as a trend", style, len(ramp))
		}
		seen := map[rune]bool{}
		for _, glyph := range ramp {
			if seen[glyph] {
				t.Fatalf("%s ramp repeats %q", style, string(glyph))
			}
			seen[glyph] = true
			if w := lipgloss.Width(string(glyph)); w != 1 {
				t.Fatalf("%s ramp glyph %q is %d cells wide, which would break alignment",
					style, string(glyph), w)
			}
		}
	}
	// The bullets ramp is gone: only the block and dot ramps remain.
	if len(SparklineStyles()) != 2 {
		t.Fatalf("SparklineStyles = %v, want the standard blocks and dots", SparklineStyles())
	}
}

// Icons are drawn one per cell, so a wider glyph would shift everything after
// it; and an empty field would silently drop a state from the row.
func TestRepoIconSetsAreSingleWidthAndComplete(t *testing.T) {
	sets := map[string]RepoIcons{
		"default": DefaultRepoIcons(),
		"emoji":   EmojiRepoIcons(),
		"nerd":    NerdRepoIcons(),
		"ascii":   ASCIIRepoIcons(),
	}
	for name, icons := range sets {
		fields := map[string]string{
			"Ahead": icons.Ahead, "Behind": icons.Behind, "Changed": icons.Changed,
			"Stash": icons.Stash, "NoUpstream": icons.NoUpstream, "Clean": icons.Clean,
			"Attention": icons.Attention, "Conflict": icons.Conflict, "Missing": icons.Missing,
		}
		for field, glyph := range fields {
			if glyph == "" {
				t.Fatalf("%s icon set has no %s", name, field)
			}
			// Each family has its own cell budget: emoji are two cells wide
			// (as the weather widget's already are), ASCII may use two
			// characters ("ok", "!!"), and the rest are exactly one.
			if want := iconCells(name); want > 0 && lipgloss.Width(glyph) != want {
				t.Fatalf("%s %s = %q is %d cells, want %d", name, field, glyph, lipgloss.Width(glyph), want)
			}
		}
		// Branch is optional - the default set has none, because every plain
		// Unicode branch symbol tested renders blank in common fonts.
		if want := iconCells(name); icons.Branch != "" && want > 0 && lipgloss.Width(icons.Branch) != want {
			t.Fatalf("%s Branch = %q is %d cells, want %d", name, icons.Branch, lipgloss.Width(icons.Branch), want)
		}
	}
	// Overrides apply per field and leave the rest of the set alone.
	styles := BuildStyles(CatppuccinMocha, StyleOptions{RepoIcons: RepoIcons{Ahead: "»"}})
	if styles.RepoIcons.Ahead != "»" {
		t.Fatalf("override ignored: %q", styles.RepoIcons.Ahead)
	}
	if styles.RepoIcons.Changed != EmojiRepoIcons().Changed {
		t.Fatalf("overriding one icon changed another: %q", styles.RepoIcons.Changed)
	}
	// An ASCII theme takes the ASCII set whatever was requested, because it
	// can render neither emoji nor private-use glyphs.
	for _, style := range []IconStyle{IconEmoji, IconNerd} {
		if got := BuildStyles(VT52, StyleOptions{IconStyle: style}).RepoIcons; got.Ahead != ASCIIRepoIcons().Ahead {
			t.Fatalf("ascii theme with %s icons used %q for ahead", style, got.Ahead)
		}
	}
	// Each style selects its own set, and an unknown one falls back to plain.
	for style, want := range map[IconStyle]RepoIcons{
		IconPlain:       DefaultRepoIcons(),
		IconEmoji:       EmojiRepoIcons(),
		IconNerd:        NerdRepoIcons(),
		IconStyle("??"): EmojiRepoIcons(),
	} {
		if got := BuildStyles(CatppuccinMocha, StyleOptions{IconStyle: style}).RepoIcons; got.Ahead != want.Ahead {
			t.Fatalf("%s icons used %q for ahead, want %q", style, got.Ahead, want.Ahead)
		}
	}
}

// iconCells is the width every glyph in a family must have.
func iconCells(family string) int {
	switch family {
	case "emoji":
		return 2
	case "ascii":
		return 0 // variable: "ok" and "!!" are two characters by design
	default:
		return 1
	}
}
