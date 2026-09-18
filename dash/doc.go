package dash

import (
	"fmt"
	"strings"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
)

// DocSchemaVersion is the contract version a document declares. It describes
// this format only; it is not related to any other project's schema version,
// however similar the shape.
const DocSchemaVersion = 1

// Doc is what an external plugin prints: a panel's content as a list of rows.
//
// The row vocabulary deliberately matches the shape tools in this space
// already emit - type, label, value, percent, severity - so that a program
// which already speaks that dialect needs an adapter measured in lines rather
// than a rewrite.
type Doc struct {
	SchemaVersion int       `json:"schemaVersion"`
	Rows          []Row     `json:"rows"`
	Detail        []Row     `json:"detail"` // shown when zoomed; empty reuses Rows
	Badge         *DocBadge `json:"badge"`

	// Options offers the values a setting can usefully take, keyed by the
	// setting's own name as the manifest declares it. A manifest is static
	// JSON written before the machine it runs on existed, so it cannot know
	// what accounts, interfaces or units are actually present; the program
	// can, because it just looked. A setting that reports options is offered
	// as a list rather than as an empty box the reader has to guess at.
	//
	// The empty string is a valid option and means "leave it unset", which is
	// how a setting whose blank value already does something sensible keeps
	// saying so.
	Options map[string][]string `json:"options"`
}

// DocBadge is a badge as a plugin writes it. It is separate from Badge
// because a tone arrives as a name on the wire, not as the integer the
// in-process type uses.
type DocBadge struct {
	Text     string `json:"text"`
	Tone     string `json:"tone"`
	Severity string `json:"severity"`
}

// tone resolves a badge's colour the same way a row's is resolved.
func (b DocBadge) tone() tideui.Tone {
	return Row{Tone: b.Tone, Severity: b.Severity}.rowTone()
}

// Row is one line of a document.
type Row struct {
	// Type is metric, gauge, spark, text, block, divider or spacer. An
	// unknown type is skipped rather than failing the document, so a plugin
	// written against a later schema degrades instead of disappearing.
	Type string `json:"type"`

	Label string `json:"label"`
	Value string `json:"value"`

	// Percent is 0..100, used by metric, gauge and spark.
	Percent float64 `json:"percent"`
	// History is 0..1 samples for a spark.
	History []float64 `json:"history"`
	// Body is the lines of a block.
	Body []string `json:"body"`

	// Tone names a semantic colour directly; Severity is the same idea in the
	// vocabulary other tools use. Tone wins when both are given.
	Tone     string `json:"tone"`
	Severity string `json:"severity"`
}

// rowTone resolves a row's colour, preferring the explicit tone.
func (r Row) rowTone() tideui.Tone {
	if tone, ok := parseTone(r.Tone); ok {
		return tone
	}
	if tone, ok := parseSeverity(r.Severity); ok {
		return tone
	}
	return tideui.ToneNeutral
}

// parseTone maps a document's tone name onto a semantic tone.
func parseTone(name string) (tideui.Tone, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "good", "ok", "success":
		return tideui.ToneGood, true
	case "warning", "warn":
		return tideui.ToneWarning, true
	case "danger", "error", "critical":
		return tideui.ToneDanger, true
	case "muted", "dim":
		return tideui.ToneMuted, true
	case "accent":
		return tideui.ToneAccent, true
	default:
		return tideui.ToneNeutral, false
	}
}

// parseSeverity maps the low/mid/high vocabulary onto the same tones.
func parseSeverity(name string) (tideui.Tone, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "low":
		return tideui.ToneGood, true
	case "mid", "medium":
		return tideui.ToneWarning, true
	case "high", "severe":
		return tideui.ToneDanger, true
	default:
		return tideui.ToneNeutral, false
	}
}

// RenderDoc draws a document at a width. Rows are rendered with the same
// primitives the built-in panels use, and the result is bounded by
// Renderer.RenderLines, so a plugin cannot overflow its pane whatever it
// prints.
func RenderDoc(renderer tideui.Renderer, doc Doc, width int, zoomed bool) string {
	rows := doc.Rows
	if zoomed && len(doc.Detail) > 0 {
		rows = doc.Detail
	}
	bg := renderer.Styles.Workspace.Bg
	labelWidth := docLabelWidth(rows, width)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, renderRow(renderer, row, width, labelWidth, bg)...)
	}
	if len(lines) == 0 {
		lines = []string{lipgloss.NewStyle().Background(bg).
			Foreground(renderer.Styles.Workspace.HintFg).Render("No content")}
	}
	return renderer.RenderLines(lines, width, bg)
}

// docLabelWidth sizes the label column from the document's own labels. A
// built-in panel can hard-code five cells because it chose its own labels;
// a plugin's are arbitrary, and a fixed column would truncate "Session" to
// "Sess…". The column is capped so a wide label cannot squeeze the plot area
// out of existence.
func docLabelWidth(rows []Row, width int) int {
	longest := 0
	for _, row := range rows {
		if length := lipgloss.Width(row.Label); length > longest {
			longest = length
		}
	}
	limit := max(4, width/3)
	if longest > limit {
		return limit
	}
	return max(longest, 3)
}

func renderRow(renderer tideui.Renderer, row Row, width, labelWidth int, bg lipgloss.Color) []string {
	switch strings.ToLower(strings.TrimSpace(row.Type)) {
	case "spacer", "":
		return []string{""}

	case "divider":
		return []string{renderer.RenderSectionDivider(
			tideui.SectionDivider{Label: row.Label, Width: width}, bg)}

	case "metric", "gauge", "spark":
		return []string{renderMetric(renderer, row, width, labelWidth, bg)}

	case "text":
		return []string{docPair(renderer, row.Label, row.Value, labelWidth, row.rowTone(), bg)}

	case "block":
		lines := make([]string, 0, len(row.Body)+1)
		if row.Label != "" {
			lines = append(lines, docPair(renderer, row.Label, row.Value, labelWidth, row.rowTone(), bg))
		}
		body := lipgloss.NewStyle().Background(bg).Foreground(renderer.Styles.Workspace.SubtitleFg)
		for _, line := range row.Body {
			lines = append(lines, body.Render("  "+line))
		}
		return lines

	default:
		// Unknown row types are skipped: a document from a newer plugin
		// should lose a line, not the whole panel.
		return nil
	}
}

// renderMetric draws the three numeric row types, which differ only in
// whether the plot area shows a bar, a history or nothing.
func renderMetric(renderer tideui.Renderer, row Row, width, labelWidth int, bg lipgloss.Color) string {
	kind := strings.ToLower(strings.TrimSpace(row.Type))
	value := row.Value
	if value == "" && row.Percent > 0 {
		value = fmt.Sprintf("%.0f%%", row.Percent)
	}
	metric := tideui.MetricRow{
		Label:      row.Label,
		Value:      value,
		Fraction:   clampFraction(row.Percent / 100),
		Tone:       row.rowTone(),
		LabelWidth: labelWidth,
		ValueWidth: 5,
		TotalWidth: width,
	}
	switch {
	case kind == "spark" || len(row.History) > 0:
		metric.Spark = row.History
	case kind == "gauge", row.Percent > 0:
		// A reading with a percentage is worth drawing: the built-in panels
		// never show a bare number where they have a fraction, and a plugin
		// that wants only the number can leave percent out.
		metric.Bar = true
	}
	return renderer.RenderMetricRow(metric, bg)
}

// docPair renders a label column and a value, the shape the built-in panels
// use for anything that is not a metric.
func docPair(renderer tideui.Renderer, label, value string, labelWidth int, tone tideui.Tone, bg lipgloss.Color) string {
	ws := renderer.Styles.Workspace
	labelStyle := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg)
	valueStyle := lipgloss.NewStyle().Background(bg).Foreground(toneColour(ws, tone))
	if label == "" {
		return valueStyle.Render(value)
	}
	return labelStyle.Render(padLabel(label, labelWidth)) +
		lipgloss.NewStyle().Background(bg).Render("  ") + valueStyle.Render(value)
}

// padLabel pads a label to a column width without truncating it: a long label
// pushes its value along rather than being cut, and RenderLines bounds the
// result either way.
func padLabel(label string, width int) string {
	if pad := width - lipgloss.Width(label); pad > 0 {
		return label + strings.Repeat(" ", pad)
	}
	return label
}

// toneColour resolves a semantic tone to a theme token. tideui resolves tones
// internally through an unexported helper, so this mirrors its mapping rather
// than reaching for a colour of its own: meaning still comes from the theme,
// and a low-colour theme still works.
func toneColour(ws tideui.WorkspaceStyles, tone tideui.Tone) lipgloss.Color {
	switch tone {
	case tideui.ToneGood:
		return ws.MetricGood
	case tideui.ToneWarning:
		return ws.MetricWarning
	case tideui.ToneDanger:
		return ws.MetricBad
	case tideui.ToneMuted:
		return ws.HintFg
	default:
		return ws.BodyFg
	}
}

func clampFraction(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}
