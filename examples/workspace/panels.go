package main

import (
	"github.com/allisonhere/tideui"
)

// Panel views. Each reads the shared demo state, renders a reusable dashboard
// widget, and switches to the widget's detail rendering when the workspace has
// zoomed the panel (the Enter drill-down pattern).

func viewRenderer(state *demoState) tideui.Renderer {
	return tideui.NewRenderer(state.theme, tideui.StyleOptions{
		Density: state.density, PaneCorners: tideui.RoundCorners,
		Gauge: state.gauge, Sparkline: state.spark, ClockFont: state.clockFont,
		IconStyle: state.icons,
	})
}

// panelRenderer prefers the workspace-resolved renderer, so a panel that has
// its own theme (or overrides) renders its content with that theme. The
// fallback keeps direct calls working without a workspace.
func panelRenderer(state *demoState, ctx tideui.PanelContext) tideui.Renderer {
	if ctx.Renderer.Styles.Theme.Name != "" {
		return ctx.Renderer
	}
	return viewRenderer(state)
}

func notesPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderNotes(state.notes, ctx.Width)
	}
}

func marketsPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderMarkets(state.source.Markets(state.now), ctx.Width)
	}
}
