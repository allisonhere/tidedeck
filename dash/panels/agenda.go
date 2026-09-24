package panels

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// calendarsKey is the configuration key this panel owns. It names the key
// already in config.json, so migrating the panel does not rewrite anyone's
// file.
const calendarsKey = "calendars"

// agenda shows a month grid beside the upcoming events for the selected day.
// The source is an iCalendar feed built from the configured paths and URLs;
// the panel only windows and draws what it holds.
type agenda struct {
	dash.State[[]tideui.AgendaItem]

	mu     sync.Mutex
	now    time.Time
	offset int
	notice string
	fetch  func(context.Context) ([]tideui.AgendaItem, error)
}

// Agenda builds the calendar panel.
func Agenda() dash.Panel { return &agenda{now: time.Now()} }

func (a *agenda) Meta() dash.Meta {
	return dash.Meta{
		ID: "agenda", Title: "Calendar",
		Role: tideui.RolePrimary, Priority: 100,
		MinWidth: 20, MinHeight: 7,
		Interval: time.Minute,
	}
}

func (a *agenda) Schema() []dash.Field {
	return []dash.Field{{
		Key: calendarsKey, Label: "sources (.ics or URL)", Kind: dash.FieldText, Path: dash.PathFile, List: true,
	}}
}

// Configure rebuilds the source from the configured paths and URLs. With none
// there is nowhere to look, so the panel fetches nothing and lets the demo
// content stand.
func (a *agenda) Configure(values dash.Values) error {
	sources := values.List(calendarsKey)
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(sources) == 0 {
		a.fetch = nil
		return nil
	}
	a.fetch = provider.Calendar(sources...)
	return nil
}

func (a *agenda) Refresh(ctx context.Context) error {
	a.mu.Lock()
	fetch := a.fetch
	a.mu.Unlock()
	if fetch == nil {
		return nil
	}
	items, err := fetch(ctx)
	if err != nil {
		// Keep the last window on screen and remember why, so an empty
		// calendar explains itself rather than looking like a quiet day.
		a.mu.Lock()
		a.notice = noticeHint(err)
		a.mu.Unlock()
		return err
	}
	a.Store(items)
	a.mu.Lock()
	a.notice = ""
	a.mu.Unlock()
	return nil
}

// Tick keeps the selected day current, so a dashboard left open overnight does
// not keep showing yesterday.
func (a *agenda) Tick(now time.Time) {
	a.mu.Lock()
	a.now = now
	a.mu.Unlock()
}

func (a *agenda) View(ctx tideui.PanelContext) string {
	a.mu.Lock()
	now, offset, notice := a.now, a.offset, a.notice
	a.mu.Unlock()
	window := a.Load()
	day := now.AddDate(0, 0, offset)
	items := upcoming(window, day)
	if len(items) == 0 && notice != "" {
		return ctx.Renderer.RenderNotice("Calendar: "+notice, ctx.Width)
	}
	marked := markedDays(window, day)
	if ctx.Zoomed {
		return ctx.Renderer.RenderCalendarDetail(day, marked, items, now, ctx.Width)
	}
	return ctx.Renderer.RenderCalendar(day, marked, items, now, ctx.Width)
}

// Demo synthesises a week's schedule, so the dashboard has a calendar before
// any feed is configured.
func (a *agenda) Demo(now time.Time) {
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := func(day, hour, minute int) time.Time {
		return base.AddDate(0, 0, day).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	a.Store([]tideui.AgendaItem{
		{Title: "Project review", Start: at(0, 9, 30), End: at(0, 10, 15), Location: "Meet", Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Dentist", Start: at(0, 11, 0), Location: "Clinic", Category: "Health", Tone: tideui.ToneGood},
		{Title: "Focus block", Start: at(0, 14, 0), End: at(0, 16, 0), Category: "Deep work", Tone: tideui.ToneAccent},
		{Title: "Team offsite", Start: at(1, 0, 0), AllDay: true, Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Standup", Start: at(1, 8, 0), Location: "Meet", Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Gym", Start: at(1, 18, 30), Category: "Health", Tone: tideui.ToneGood},
		{Title: "Ship TideDeck", Start: at(2, 10, 0), Category: "Work", Tone: tideui.ToneWarning},
	})
	a.mu.Lock()
	a.now = now
	a.mu.Unlock()
}

// Actions move the selected day rather than scrolling the list, so the month
// grid and the agenda always describe the same day.
func (a *agenda) Actions() []dash.Action {
	return []dash.Action{
		{ID: "next", Key: "n", Label: "next", Run: func() string { a.shift(1); return "calendar: +day" }},
		{ID: "prev", Key: "p", Label: "prev", Run: func() string { a.shift(-1); return "calendar: -day" }},
		{ID: "today", Key: "0", Label: "today", Run: func() string { a.today(); return "calendar: today" }},
	}
}

func (a *agenda) shift(delta int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.offset += delta
}

func (a *agenda) today() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.offset = 0
}

// agendaEnd is when an event stops occupying the calendar: its End, its start
// for a timed event without one, or the next midnight for an all-day event,
// whose exclusive End may be absent.
func agendaEnd(item tideui.AgendaItem) time.Time {
	if !item.End.IsZero() {
		return item.End
	}
	if item.AllDay {
		return item.Start.AddDate(0, 0, 1)
	}
	return item.Start
}

// upcoming keeps the events still running on or after the selected day, so a
// past morning drops off while an all-day or multi-day event stays.
func upcoming(items []tideui.AgendaItem, day time.Time) []tideui.AgendaItem {
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	var out []tideui.AgendaItem
	for _, item := range items {
		if agendaEnd(item).After(from) {
			out = append(out, item)
		}
	}
	return out
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

// noticeHint shortens a fetch failure for a panel that has one line to spare.
// The provider wraps the source URL in the message, which the panel has no room
// for, so the trailing hint is preferred.
func noticeHint(err error) string {
	message := err.Error()
	if open := strings.Index(message, " ("); open >= 0 {
		if close := strings.LastIndex(message, ")"); close > open {
			return strings.TrimSpace(message[open+2 : close])
		}
	}
	return message
}
