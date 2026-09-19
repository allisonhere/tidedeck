package main

import (
	"os"

	"github.com/allisonhere/tideui"
)

// viewRenderer builds a renderer from the current presentation styles. The
// workspace gives every panel its own resolved renderer; this is what a panel
// rendered outside a workspace, as the tests do, uses instead.
func viewRenderer(state *demoState) tideui.Renderer {
	cellWidth, cellHeight := tideui.CellSizeOf(os.Stdout)
	options := tideui.StyleOptions{
		Density: state.density, PaneCorners: tideui.RoundCorners,
		Gauge: state.gauge, Sparkline: state.spark, ClockFont: state.clockFont,
		IconStyle: state.icons,
	}
	if cellWidth > 0 && cellHeight > 0 {
		options.CellWidth = cellWidth
		options.CellAspect = cellHeight / cellWidth
	}
	return tideui.NewRenderer(state.theme, options)
}
