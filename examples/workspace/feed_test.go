package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

func TestFeedIsDeterministic(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	a := newDemoFeed(7, started)
	b := newDemoFeed(7, started)
	now := started.Add(42 * time.Second)

	if !reflect.DeepEqual(a.Weather(now), b.Weather(now)) {
		t.Fatal("weather is not deterministic")
	}
	if !reflect.DeepEqual(a.System(now), b.System(now)) {
		t.Fatal("system is not deterministic")
	}
	if !reflect.DeepEqual(a.Network(now), b.Network(now)) {
		t.Fatal("network is not deterministic")
	}
	if !reflect.DeepEqual(a.Markets(now), b.Markets(now)) {
		t.Fatal("markets are not deterministic")
	}
	if !reflect.DeepEqual(a.Agenda(now, 0), b.Agenda(now, 0)) {
		t.Fatal("agenda is not deterministic")
	}
	if !reflect.DeepEqual(a.Storage(), b.Storage()) || !reflect.DeepEqual(a.Tasks(), b.Tasks()) {
		t.Fatal("static fixtures are not deterministic")
	}
}

func TestAgendaEndTime(t *testing.T) {
	day := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if got := agendaEndTime(tideui.AgendaItem{Start: day.Add(8 * time.Hour)}); !got.Equal(day.Add(8 * time.Hour)) {
		t.Fatalf("timed end = %v, want its start", got)
	}
	if got := agendaEndTime(tideui.AgendaItem{Start: day, End: day.Add(2 * time.Hour)}); !got.Equal(day.Add(2 * time.Hour)) {
		t.Fatalf("end = %v, want the event end", got)
	}
	// An all-day event without an end runs to the next midnight.
	if got := agendaEndTime(tideui.AgendaItem{Start: day, AllDay: true}); !got.Equal(day.AddDate(0, 0, 1)) {
		t.Fatalf("all-day end = %v, want next midnight", got)
	}
}

func TestFeedUpdatesOverTime(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := newDemoFeed(7, started)
	t0 := started
	t1 := started.Add(7 * time.Second)

	if reflect.DeepEqual(f.Network(t0).DownSpark, f.Network(t1).DownSpark) {
		t.Fatal("network sparkline did not advance")
	}
	if f.Weather(t0).Updated.Equal(f.Weather(t1).Updated) {
		t.Fatal("weather timestamp did not advance")
	}
}

func TestAgendaOffsetStartsAtDay(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := newDemoFeed(1, started)
	all := f.Agenda(started, 0)
	if len(all) == 0 {
		t.Fatal("no events from today")
	}
	target := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	fromTomorrow := f.Agenda(started, 1)
	if len(fromTomorrow) == 0 || len(fromTomorrow) >= len(all) {
		t.Fatalf("tomorrow = %d events, all = %d", len(fromTomorrow), len(all))
	}
	for _, item := range fromTomorrow {
		if item.Start.Before(target) {
			t.Fatalf("event before the start day: %+v", item)
		}
	}
	if got := f.Agenda(started, 9); len(got) != 0 {
		t.Fatalf("far day = %d events, want 0", len(got))
	}
	// Moving the start day must not mutate the underlying schedule.
	if !f.Agenda(started, 0)[0].Start.Equal(all[0].Start) {
		t.Fatal("agenda offset mutated the base data")
	}
}
