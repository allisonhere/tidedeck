package main

import (
	"math"
	"strings"
	"time"

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

// agendaNotice reports why a live calendar is empty, so a mistyped or private
// feed is not mistaken for a quiet day. The provider wraps the URL in the
// message, which the panel has no room for, so the trailing hint is preferred.
func (state *demoState) agendaNotice() string {
	if state.live == nil {
		return ""
	}
	err := state.live.snapshotCopy().Errors["agenda"]
	if err == nil {
		return ""
	}
	message := err.Error()
	if open := strings.Index(message, " ("); open >= 0 {
		if close := strings.LastIndex(message, ")"); close > open {
			return strings.TrimSpace(message[open+2 : close])
		}
	}
	return message
}

func agendaPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		items := state.source.Agenda(state.now, state.agendaOffset)
		if len(items) == 0 {
			if notice := state.agendaNotice(); notice != "" {
				return r.RenderNotice("Calendar: "+notice, ctx.Width)
			}
		}
		day := state.now.AddDate(0, 0, state.agendaOffset)
		marked := markedDays(state.source.Agenda(state.now, 0), day)
		if ctx.Zoomed {
			return r.RenderCalendarDetail(day, marked, items, state.now, ctx.Width)
		}
		return r.RenderCalendar(day, marked, items, state.now, ctx.Width)
	}
}

// markedDays collects the days of the displayed month that have an event, so
// the month grid shows where the quiet days aren't.
func markedDays(items []tideui.AgendaItem, day time.Time) map[int]bool {
	marked := map[int]bool{}
	for _, item := range items {
		if item.Start.Year() == day.Year() && item.Start.Month() == day.Month() {
			marked[item.Start.Day()] = true
		}
	}
	return marked
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
		if ctx.Zoomed {
			return r.RenderRepoActivityDetail(state.repos, ctx.Width)
		}
		return r.RenderRepoActivity(state.repos, ctx.Width)
	}
}

func marketsPanel(state *demoState) tideui.PanelView {
	return func(ctx tideui.PanelContext) string {
		r := panelRenderer(state, ctx)
		return r.RenderMarkets(state.source.Markets(state.now), ctx.Width)
	}
}
