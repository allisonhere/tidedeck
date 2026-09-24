package tideui

import (
	"math"

	"github.com/charmbracelet/lipgloss"
)

// eighthBlocks are the left-anchored partial blocks, one eighth to seven
// eighths of a cell. They give a bar eight steps per cell instead of one, so a
// gauge in a narrow split still moves when the value moves by a percent.
var eighthBlocks = []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// renderSmoothGauge is a solid bar with sub-cell precision: whole cells of █,
// then the partial block that carries the remainder. The smooth track is the
// muted colour laid as a cell background, and the partial cell sits on it, so
// fill and track meet without a seam and the bar reads as one pill. With heat
// set, each cell takes its colour from its position on the metric gradient, so
// a nearly full bar ends in red and an idle one never leaves green; its track
// is a faint ▁ floor in the same gradient, a scale of what lies ahead.
//
// The two tracks are different glyphs, not only different colours, so each
// family still reads as itself on a terminal with colour turned off.
func (r Renderer) renderSmoothGauge(fraction float64, width int, fill, track lipgloss.Style, bg lipgloss.Color, heat bool) string {
	ws := r.Styles.Workspace
	trackColor := ws.MetricTrack
	// A whole cell of the track colour is far louder than a ░ drawn in it,
	// so the pill's bed is pulled most of the way back to the panel.
	bed := MixColors(bg, trackColor, 0.4)
	eighths := int(math.Round(fraction * float64(width*8)))
	full, rem := eighths/8, eighths%8
	cells := make([]gaugeCell, 0, width)
	cellStyle := func(i int) lipgloss.Style {
		if !heat {
			return fill
		}
		return fill.Foreground(ws.MetricGradient((float64(i) + 0.5) / float64(width)))
	}
	for i := 0; i < width; i++ {
		switch {
		case i < full:
			cells = append(cells, gaugeCell{"█", cellStyle(i)})
		case i == full && rem > 0 && heat:
			cells = append(cells, gaugeCell{eighthBlocks[rem-1], cellStyle(i)})
		case i == full && rem > 0:
			cells = append(cells, gaugeCell{eighthBlocks[rem-1], cellStyle(i).Background(bed)})
		case heat:
			scale := MixColors(trackColor, ws.MetricGradient((float64(i)+0.5)/float64(width)), 0.4)
			cells = append(cells, gaugeCell{"▁", track.Foreground(scale)})
		default:
			cells = append(cells, gaugeCell{" ", track.Background(bed)})
		}
	}
	return renderGaugeCells(cells)
}

// renderLineGauge is a heavy rule on a light one with half-cell precision:
// ━━━━╸────. The filled run brightens toward its head like a comet's tail.
func (r Renderer) renderLineGauge(fraction float64, width int, fill, track lipgloss.Style, tone lipgloss.Color) string {
	trackColor := r.Styles.Workspace.MetricTrack
	halves := int(math.Round(fraction * float64(width*2)))
	full, half := halves/2, halves%2 == 1
	cells := make([]gaugeCell, 0, width)
	for i := 0; i < width; i++ {
		switch {
		case i < full:
			// The tail starts at 45% of the tone over the track and reaches
			// the full tone at the head.
			span := float64(max(1, full-1))
			if half {
				span = float64(full)
			}
			amount := 0.45 + 0.55*float64(i)/span
			cells = append(cells, gaugeCell{"━", fill.Foreground(MixColors(trackColor, tone, math.Min(1, amount)))})
		case i == full && half:
			cells = append(cells, gaugeCell{"╸", fill})
		default:
			cells = append(cells, gaugeCell{"─", track})
		}
	}
	return renderGaugeCells(cells)
}

// Braille dot bits for each of the four rows, top to bottom, in the left and
// right columns of a cell.
var (
	brailleLeft  = [4]rune{0x01, 0x02, 0x04, 0x40}
	brailleRight = [4]rune{0x08, 0x10, 0x20, 0x80}
)

// brailleRow maps a level in 0..1 to one of a cell's four dot rows, counted
// from the top, so 0 sits on the floor and 1 on the ceiling.
func brailleRow(level float64) int {
	return 3 - int(math.Round(clamp01(level)*3))
}

// brailleSpark draws two samples per cell in braille, which doubles the
// horizontal resolution of every other ramp. Each sample is a dot joined to
// the one before it by a vertical stroke, so the trace is continuous. As an
// area (the tide) the water under that crest is stippled, alternate dots in a
// checkerboard, rather than filled: a solid fill lights nearly every dot and
// reads as the block ramp, where a stipple keeps the crest the brightest thing
// in the cell. levels holds 2*width values in 0..1; the result is one glyph
// per cell and the higher of the cell's two levels, for colouring.
func brailleSpark(levels []float64, width int, area bool) ([]string, []float64) {
	glyphs := make([]string, width)
	peaks := make([]float64, width)
	prev := -1
	for cell := 0; cell < width; cell++ {
		var bits rune
		for side := 0; side < 2; side++ {
			level := levels[cell*2+side]
			peaks[cell] = math.Max(peaks[cell], level)
			column := brailleLeft
			if side == 1 {
				column = brailleRight
			}
			row := brailleRow(level)
			top, bottom := row, row
			if prev >= 0 {
				// Join to the previous sample: fill every row between the
				// two, on this column, so steep moves stay connected.
				top, bottom = min(row, prev), max(row, prev)
			}
			for y := top; y <= bottom; y++ {
				bits |= column[y]
			}
			if area {
				for y := bottom + 1; y <= 3; y++ {
					if (cell*2+side+y)%2 == 1 {
						bits |= column[y]
					}
				}
			}
			prev = row
		}
		glyphs[cell] = string(rune(0x2800) + bits)
	}
	return glyphs, peaks
}

// sparkCells turns a run of levels (0..1, already scaled) into one glyph per
// cell for a style, plus the level each cell should be coloured by. It is the
// one place a sparkline's shape is decided, shared by the live render and the
// picker sample.
func (r Renderer) sparkCells(style SparklineStyle, sample func(n int) []float64, width int) ([]string, []float64) {
	style = normalizeSparklineStyle(style)
	if !r.Styles.PlainUI && (style == SparkBraille || style == SparkTide) {
		return brailleSpark(sample(width*2), width, style == SparkTide)
	}
	glyphs := sparkGlyphs(style)
	if r.Styles.PlainUI {
		glyphs = []rune(".:-=+*#@")
	}
	levels := sample(width)
	cells := make([]string, width)
	for i, level := range levels {
		index := int(math.Round(clamp01(level) * float64(len(glyphs)-1)))
		cells[i] = string(glyphs[clampIndex(index, len(glyphs))])
	}
	return cells, levels
}
