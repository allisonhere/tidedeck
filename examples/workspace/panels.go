package main

import (
	"os"

	"github.com/allisonhere/tideui"
)

// viewRenderer builds a renderer from the current presentation styles. The
// workspace gives every panel its own resolved renderer; this is what a panel
// rendered outside a workspace, as the tests do, uses instead.
func viewRenderer(state *demoState) tideui.Renderer {
	return tideui.NewRenderer(state.theme, tideui.StyleOptions{
		Density: state.density, PaneCorners: tideui.RoundCorners,
		Gauge: state.gauge, Sparkline: state.spark, ClockFont: state.clockFont,
		IconStyle:  state.icons,
		CellAspect: tideui.CellAspectOf(os.Stdout),
	})
}
