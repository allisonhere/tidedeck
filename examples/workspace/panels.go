package main

import (
	"math"

	"github.com/allisonhere/tideui"
)

// Panel views. Each reads the shared demo state, renders a reusable dashboard
// widget, and switches to the widget's detail rendering when the workspace has
// zoomed the panel (the Enter drill-down pattern).

func viewRenderer(state *demoState) tideui.Renderer {
	return tideui.NewRenderer(state.theme, tideui.StyleOptions{
		Density: state.density, PaneCorners: tideui.RoundCorners,
		Gauge: state.gauge, Sparkline: state.spark, ClockFont: state.clockFont,
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

func weatherPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		w := convertWeatherUnit(state.source.Weather(state.now), state.weatherUnit)
		if ctx.Zoomed {
			return r.RenderWeatherDetail(w, ctx.Width)
		}
		return r.RenderWeather(w, ctx.Width)
	}
}

// convertWeatherUnit converts a reading into the requested unit, regardless of
// the unit the source reported (live weather follows the configured unit).
func convertWeatherUnit(w tideui.WeatherData, target string) tideui.WeatherData {
	if target == "" || w.Unit == "" || w.Unit == target {
		return w
	}
	if target == "C" && w.Unit == "F" {
		w = convertTemperatures(w, func(v int) int { return int(math.Round(float64(v-32) * 5 / 9)) }, "C")
	} else if target == "F" && w.Unit == "C" {
		w = convertTemperatures(w, func(v int) int { return int(math.Round(float64(v)*9/5 + 32)) }, "F")
	}
	return w
}

func convertTemperatures(w tideui.WeatherData, convert func(int) int, unit string) tideui.WeatherData {
	w.Temperature = convert(w.Temperature)
	w.High = convert(w.High)
	w.Low = convert(w.Low)
	if w.HasFeelsLike {
		w.FeelsLike = convert(w.FeelsLike)
	}
	w.Unit = unit
	for i := range w.Hourly {
		w.Hourly[i].Temperature = convert(w.Hourly[i].Temperature)
	}
	for i := range w.Daily {
		w.Daily[i].Temperature = convert(w.Daily[i].Temperature)
	}
	return w
}

func agendaPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		items := state.source.Agenda(state.now, state.agendaOffset)
		if ctx.Zoomed {
			return r.RenderAgendaDetail(items, state.now, ctx.Width)
		}
		return r.RenderAgenda(items, state.now, ctx.Width)
	}
}

func clockPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		c := state.source.Clock(state.now)
		c.Hour24 = state.clock24
		if ctx.Zoomed {
			return r.RenderClockDetail(c, ctx.Width)
		}
		return r.RenderClock(c, ctx.Width)
	}
}

func systemPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		m := state.source.System(state.now)
		if ctx.Zoomed {
			return r.RenderSystemDetail(m, ctx.Width)
		}
		return r.RenderSystem(m, ctx.Width)
	}
}

func networkPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		m := state.source.Network(state.now)
		if ctx.Zoomed {
			return r.RenderNetworkDetail(m, ctx.Width)
		}
		return r.RenderNetwork(m, ctx.Width)
	}
}

func storagePanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderStorage(state.mounts, ctx.Width)
	}
}

func servicesPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		if ctx.Zoomed {
			return r.RenderServicesDetail(state.services, ctx.Width)
		}
		return r.RenderServices(state.services, ctx.Width)
	}
}

func newsPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		if ctx.Zoomed {
			return r.RenderHeadlinesDetail(state.headlines, ctx.Width)
		}
		return r.RenderHeadlines(state.headlines, ctx.Width)
	}
}

func tasksPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderTasks(state.tasks, ctx.Width)
	}
}

func notesPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderNotes(state.notes, ctx.Width)
	}
}

func gitPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderRepoActivity(state.repos, ctx.Width)
	}
}

func marketsPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderMarkets(state.source.Markets(state.now), ctx.Width)
	}
}
