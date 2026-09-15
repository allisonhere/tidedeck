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

func TestKeyHintsDenseOmitsLabels(t *testing.T) {
	r := chromeRenderer(Dense)
	got := ansi.Strip(r.RenderKeyHints([]KeyHint{Hint("w", "save")}, 80))
	if strings.Contains(got, "save") {
		t.Fatalf("dense hints should omit labels: %q", got)
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
		got := r.RenderProgressBar(ProgressBar{Fraction: 0.5, Width: 10, Tone: ToneGood}, r.Styles.Workspace.Bg)
		if w := lipgloss.Width(got); w != 10 {
			t.Fatalf("%s gauge width = %d, want 10", style, w)
		}
	}
	unknown := NewRenderer(CatppuccinMocha, StyleOptions{Gauge: GaugeStyle("nope")})
	if unknown.Styles.Gauge != GaugeSolid {
		t.Fatalf("unknown gauge = %q, want solid", unknown.Styles.Gauge)
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
	panel.Sparkline(SparkBraille)
	if style, ok := panel.PanelSparkline(); !ok || style != SparkBraille {
		t.Fatalf("PanelSparkline = %q,%v, want braille", style, ok)
	}
	if got := wr.panelRenderer(panel).Styles.Sparkline; got != SparkBraille {
		t.Fatalf("panel sparkline = %q, want braille", got)
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
	a := NewRenderer(CatppuccinMocha, StyleOptions{}).GaugeSample(GaugeSolid, 8)
	b := NewRenderer(CatppuccinMocha, StyleOptions{}).GaugeSample(GaugeCircles, 8)
	if a == b {
		t.Fatalf("solid and circles samples should differ: %q", a)
	}
}

func TestPanelGaugeOverride(t *testing.T) {
	wr := NewWorkspaceRenderer(NewRenderer(CatppuccinMocha, StyleOptions{Gauge: GaugeSolid}))
	panel := newPanel("system", nil)
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeSolid {
		t.Fatalf("default panel gauge = %q, want solid", got)
	}
	panel.Gauge(GaugeCircles)
	if style, ok := panel.PanelGauge(); !ok || style != GaugeCircles {
		t.Fatalf("PanelGauge = %q,%v, want circles", style, ok)
	}
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeCircles {
		t.Fatalf("panel gauge = %q, want circles", got)
	}
	panel.ClearGauge()
	if panel.HasPanelGauge() {
		t.Fatal("ClearGauge did not clear the override")
	}
	if got := wr.panelRenderer(panel).Styles.Gauge; got != GaugeSolid {
		t.Fatalf("cleared panel gauge = %q, want solid", got)
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
