package panels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func agendaNow() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

func TestAgendaPanelMeta(t *testing.T) {
	meta := Agenda().Meta()
	if meta.ID != "agenda" || meta.Title != "Calendar" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 20 || meta.MinHeight != 7 || meta.Priority != 100 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Minute {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

// A configured source builds a fetcher; with no source there is nowhere to
// look, so the panel leaves the demo content in place.
func TestAgendaConfigureBuildsSourceOrNot(t *testing.T) {
	panel := Agenda().(*agenda)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("an empty source list should build no fetcher")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}
	values := dash.NewValues()
	values.Set(calendarsKey, "https://example.com/cal.ics")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured source should build a fetcher")
	}
}

func TestAgendaPanelRendersMonthAndEvents(t *testing.T) {
	now := agendaNow()
	panel := Agenda().(*agenda)
	panel.Demo(now)
	ctx := tideui.PanelContext{ID: "agenda", Width: 60, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"September 2026", "Project review", "all-day", "Team offsite"} {
		if !strings.Contains(body, want) {
			t.Fatalf("calendar body missing %q:\n%s", want, body)
		}
	}
	detail := ansi.Strip(panel.View(tideui.PanelContext{ID: "agenda", Width: 60, Zoomed: true, Renderer: renderer()}))
	if !strings.Contains(detail, "Meet") {
		t.Fatalf("zoomed body missing locations:\n%s", detail)
	}
	// Every line stays within the panel, at any width.
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "agenda", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

// next/prev/today move the selected day, so the month grid and the agenda
// always describe the same day.
func TestAgendaActionsMoveTheDay(t *testing.T) {
	panel := Agenda().(*agenda)
	panel.Demo(agendaNow())

	run := func(key string) {
		for _, action := range panel.Actions() {
			if action.Key == key {
				action.Run()
				return
			}
		}
		t.Fatalf("no %q action", key)
	}
	offset := func() int {
		panel.mu.Lock()
		defer panel.mu.Unlock()
		return panel.offset
	}

	run("n")
	run("n")
	if got := offset(); got != 2 {
		t.Fatalf("after two next presses offset = %d, want 2", got)
	}
	run("p")
	if got := offset(); got != 1 {
		t.Fatalf("after prev offset = %d, want 1", got)
	}
	run("0")
	if got := offset(); got != 0 {
		t.Fatalf("after today offset = %d, want 0", got)
	}
}

// A failed fetch keeps the last window on screen and remembers why, so the
// empty calendar reports the failure instead of looking like a quiet day.
func TestAgendaKeepsLastWindowAndNoticesFailure(t *testing.T) {
	now := agendaNow()
	failing := errors.New("provider: calendar https://example.com/cal.ics: status 404 (a private Google Calendar needs its secret iCal address)")
	calls := 0
	panel := &agenda{now: now}
	panel.fetch = func(context.Context) ([]tideui.AgendaItem, error) {
		calls++
		if calls == 1 {
			return []tideui.AgendaItem{{Title: "Review", Start: now}}, nil
		}
		return nil, failing
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); !errors.Is(err, failing) {
		t.Fatalf("second refresh error = %v", err)
	}
	if got := len(panel.Load()); got != 1 {
		t.Fatalf("a failed refresh discarded the window: %d events", got)
	}

	// With nothing to show, the panel says why rather than rendering an empty
	// month.
	empty := &agenda{now: now}
	empty.fetch = func(context.Context) ([]tideui.AgendaItem, error) { return nil, failing }
	if err := empty.Refresh(context.Background()); err == nil {
		t.Fatal("expected the refresh to fail")
	}
	body := ansi.Strip(empty.View(tideui.PanelContext{ID: "agenda", Width: 80, Renderer: renderer()}))
	if !strings.Contains(body, "a private Google Calendar needs its secret iCal address") {
		t.Fatalf("empty calendar did not explain itself:\n%s", body)
	}
}

func TestAgendaEnd(t *testing.T) {
	day := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if got := agendaEnd(tideui.AgendaItem{Start: day.Add(8 * time.Hour)}); !got.Equal(day.Add(8 * time.Hour)) {
		t.Fatalf("timed end = %v, want its start", got)
	}
	if got := agendaEnd(tideui.AgendaItem{Start: day, End: day.Add(2 * time.Hour)}); !got.Equal(day.Add(2 * time.Hour)) {
		t.Fatalf("end = %v, want the event end", got)
	}
	// An all-day event without an end runs to the next midnight.
	if got := agendaEnd(tideui.AgendaItem{Start: day, AllDay: true}); !got.Equal(day.AddDate(0, 0, 1)) {
		t.Fatalf("all-day end = %v, want next midnight", got)
	}
}

func TestNoticeHintPrefersTheHint(t *testing.T) {
	full := errors.New("provider: calendar https://example.com/cal.ics: status 404 (a private Google Calendar needs its secret iCal address)")
	if got, want := noticeHint(full), "a private Google Calendar needs its secret iCal address"; got != want {
		t.Fatalf("noticeHint = %q, want %q", got, want)
	}
	if got, want := noticeHint(errors.New("boom")), "boom"; got != want {
		t.Fatalf("noticeHint = %q, want %q", got, want)
	}
}

// The deck must be able to drive the panel with no special-casing.
func TestAgendaThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Agenda())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("agenda")
	if !ok {
		t.Fatal("agenda was not attached to the workspace")
	}
	if registered.TitleText() != "Calendar" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	keys := map[string]bool{}
	for _, action := range registered.ActionList() {
		keys[action.Key] = true
	}
	for _, want := range []string{"n", "p", "0"} {
		if !keys[want] {
			t.Fatalf("missing %q action; got %v", want, keys)
		}
	}
	// Demo mode fills the panel through the deck's tick alone.
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "agenda", Width: 50, Renderer: renderer()}))
	if !strings.Contains(body, "Project review") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
