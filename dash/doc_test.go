package dash

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func docRenderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{Density: tideui.Compact})
}

// sampleDoc uses every row type, including the four a real tool already
// emits: metric, text, block and spacer.
func sampleDoc() Doc {
	return Doc{
		SchemaVersion: DocSchemaVersion,
		Badge:         &DocBadge{Text: "3", Tone: "warning"},
		Rows: []Row{
			{Type: "metric", Label: "Session", Value: "43%", Percent: 43, Severity: "low"},
			{Type: "gauge", Label: "MEM", Value: "41%", Percent: 41},
			{Type: "spark", Label: "CPU", Value: "18%", Percent: 18, History: []float64{0.1, 0.4, 0.2}},
			{Type: "text", Label: "Balance", Value: "$7.02"},
			{Type: "block", Label: "Credits", Body: []string{"balance: 0", "topped up $7.02"}},
			{Type: "divider", Label: "DETAIL"},
			{Type: "spacer"},
		},
		Detail: []Row{{Type: "text", Label: "Plan", Value: "Claude Pro"}},
	}
}

func TestRenderDocDrawsEveryRowType(t *testing.T) {
	out := ansi.Strip(RenderDoc(docRenderer(), sampleDoc(), 40, false))
	for _, want := range []string{"Session", "43%", "MEM", "CPU", "Balance", "$7.02", "Credits", "balance: 0", "DETAIL"} {
		if !strings.Contains(out, want) {
			t.Fatalf("document missing %q:\n%s", want, out)
		}
	}
	// Zoomed swaps in the detail rows when a document supplies them.
	zoomed := ansi.Strip(RenderDoc(docRenderer(), sampleDoc(), 40, true))
	if !strings.Contains(zoomed, "Claude Pro") {
		t.Fatalf("zoomed document did not use detail rows:\n%s", zoomed)
	}
	if strings.Contains(zoomed, "Balance") {
		t.Fatalf("zoomed document still showed the compact rows:\n%s", zoomed)
	}
	// Without detail rows, zooming reuses the compact ones rather than
	// emptying the panel.
	doc := sampleDoc()
	doc.Detail = nil
	if out := ansi.Strip(RenderDoc(docRenderer(), doc, 40, true)); !strings.Contains(out, "Balance") {
		t.Fatalf("zoom without detail rows lost the content:\n%s", out)
	}
}

// The bound every built-in widget is held to applies to a plugin's output too:
// whatever a program prints, it cannot overflow its pane.
func TestRenderDocIsBoundedAtEveryWidth(t *testing.T) {
	long := Doc{Rows: []Row{
		{Type: "text", Label: "A very long label indeed", Value: strings.Repeat("value ", 20)},
		{Type: "metric", Label: "LongMetricLabel", Value: "1234567890%", Percent: 99},
		{Type: "block", Label: "Block", Body: []string{strings.Repeat("body ", 30)}},
		{Type: "divider", Label: strings.Repeat("DIVIDER ", 10)},
		{Type: "gauge", Label: "G", Value: "50%", Percent: 50},
	}}
	for _, width := range []int{4, 8, 12, 20, 40} {
		out := RenderDoc(docRenderer(), long, width, false)
		for _, line := range strings.Split(ansi.Strip(out), "\n") {
			if got := ansi.StringWidth(line); got != width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

// A document from a newer plugin should lose a row, not the whole panel.
func TestRenderDocSkipsUnknownRows(t *testing.T) {
	doc := Doc{Rows: []Row{
		{Type: "text", Label: "Kept", Value: "yes"},
		{Type: "hologram", Label: "Future", Value: "???"},
		{Type: "text", Label: "Also", Value: "kept"},
	}}
	out := ansi.Strip(RenderDoc(docRenderer(), doc, 30, false))
	if !strings.Contains(out, "Kept") || !strings.Contains(out, "Also") {
		t.Fatalf("known rows were lost:\n%s", out)
	}
	if strings.Contains(out, "???") {
		t.Fatalf("an unknown row was drawn:\n%s", out)
	}
}

// An empty document says so rather than rendering as a blank hole that looks
// like a broken panel.
func TestRenderDocEmpty(t *testing.T) {
	out := ansi.Strip(RenderDoc(docRenderer(), Doc{}, 24, false))
	if !strings.Contains(out, "No content") {
		t.Fatalf("empty document = %q", out)
	}
	if got := ansi.StringWidth(strings.Split(out, "\n")[0]); got != 24 {
		t.Fatalf("empty document line width = %d, want 24", got)
	}
}

func TestDocToneVocabulary(t *testing.T) {
	// Both vocabularies resolve, and an explicit tone wins over a severity.
	for _, c := range []struct {
		row  Row
		want tideui.Tone
	}{
		{Row{Tone: "good"}, tideui.ToneGood},
		{Row{Tone: "warning"}, tideui.ToneWarning},
		{Row{Tone: "danger"}, tideui.ToneDanger},
		{Row{Tone: "muted"}, tideui.ToneMuted},
		{Row{Tone: "accent"}, tideui.ToneAccent},
		{Row{Severity: "low"}, tideui.ToneGood},
		{Row{Severity: "mid"}, tideui.ToneWarning},
		{Row{Severity: "high"}, tideui.ToneDanger},
		{Row{Tone: "danger", Severity: "low"}, tideui.ToneDanger},
		{Row{Tone: "nonsense"}, tideui.ToneNeutral},
		{Row{}, tideui.ToneNeutral},
		{Row{Tone: "  WARNING  "}, tideui.ToneWarning},
	} {
		if got := c.row.rowTone(); got != c.want {
			t.Fatalf("row %+v resolved to tone %v, want %v", c.row, got, c.want)
		}
	}
}

// A metric with no value string shows its percentage, so the simplest useful
// row is {"type":"metric","label":"CPU","percent":42}.
func TestRenderDocMetricDerivesItsValue(t *testing.T) {
	doc := Doc{Rows: []Row{{Type: "metric", Label: "CPU", Percent: 42}}}
	if out := ansi.Strip(RenderDoc(docRenderer(), doc, 30, false)); !strings.Contains(out, "42%") {
		t.Fatalf("metric did not derive its value:\n%s", out)
	}
	// Out-of-range percentages are clamped rather than overflowing the gauge.
	for _, percent := range []float64{-50, 150} {
		doc := Doc{Rows: []Row{{Type: "gauge", Label: "G", Value: "x", Percent: percent}}}
		out := RenderDoc(docRenderer(), doc, 20, false)
		if got := ansi.StringWidth(ansi.Strip(out)); got != 20 {
			t.Fatalf("percent %v produced a %d-cell line", percent, got)
		}
	}
}

// A row may carry an id, which is what makes it openable: it is an opaque value
// the plugin chose - a mail plugin puts the message's row id here - and the
// panel hands it back to the command the manifest declares.
func TestRowCarriesAnID(t *testing.T) {
	var doc Doc
	if err := json.Unmarshal([]byte(`{"rows":[
		{"type":"block","id":"41","label":"ana@example.com","body":["standup notes"]},
		{"type":"spacer"}]}`), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(doc.Rows))
	}
	if doc.Rows[0].ID != "41" {
		t.Errorf("id = %q, want %q", doc.Rows[0].ID, "41")
	}
	// A row without an id is text, not a destination.
	if doc.Rows[1].ID != "" {
		t.Errorf("a spacer has id %q", doc.Rows[1].ID)
	}
}

// A block's body is the row's content, so it is drawn in the panel's normal
// text colour; a body that is only detail under its label names a bodyTone.
// When every body was drawn in the subtitle colour, a mail subject read as a
// footnote under its sender rather than as the thing the row was showing.
func TestBlockBodyIsContentUnlessItAsksForATone(t *testing.T) {
	// Without a TTY there is no colour to assert on, and every SGR test passes
	// vacuously.
	lipgloss.SetColorProfile(termenv.TrueColor)
	renderer := docRenderer()
	ws := renderer.Styles.Workspace

	body := func(doc Doc) string {
		return RenderDoc(renderer, doc, 40, false)
	}
	plain := body(Doc{Rows: []Row{{Type: "block", Label: "sam@example.com", Body: []string{"dinner?"}}}})
	if want := lipgloss.NewStyle().Background(ws.Bg).Foreground(ws.BodyFg).Render("  dinner?"); !strings.Contains(plain, want) {
		t.Errorf("a block's body is not the normal text colour:\n%q", plain)
	}
	detail := body(Doc{Rows: []Row{{Type: "block", Body: []string{"balance: 0"}, BodyTone: "muted"}}})
	if want := lipgloss.NewStyle().Background(ws.Bg).Foreground(ws.HintFg).Render("  balance: 0"); !strings.Contains(detail, want) {
		t.Errorf("bodyTone did not colour the body:\n%q", detail)
	}
}
