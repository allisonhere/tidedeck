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
	if !reflect.DeepEqual(a.Clock(now), b.Clock(now)) {
		t.Fatal("clock is not deterministic")
	}
	if !reflect.DeepEqual(a.Storage(), b.Storage()) || !reflect.DeepEqual(a.Tasks(), b.Tasks()) {
		t.Fatal("static fixtures are not deterministic")
	}
}

func TestAgendaOnDay(t *testing.T) {
	day := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	timed := tideui.AgendaItem{Title: "standup", Start: day.Add(8 * time.Hour)}
	if !agendaOnDay(timed, day) {
		t.Fatal("timed event not on its own day")
	}
	if agendaOnDay(timed, day.AddDate(0, 0, 1)) {
		t.Fatal("timed event on the following day")
	}
	allDay := tideui.AgendaItem{Title: "holiday", Start: day, End: day.AddDate(0, 0, 1), AllDay: true}
	if !agendaOnDay(allDay, day) {
		t.Fatal("all-day event not on its own day")
	}
	if agendaOnDay(allDay, day.AddDate(0, 0, 1)) {
		t.Fatal("all-day event leaked onto the next day")
	}
	if open := (tideui.AgendaItem{Title: "offsite", Start: day, AllDay: true}); !agendaOnDay(open, day) {
		t.Fatal("all-day event without an end not on its day")
	}
	multi := tideui.AgendaItem{Title: "conference", Start: day.Add(10 * time.Hour), End: day.AddDate(0, 0, 2).Add(10 * time.Hour)}
	if !agendaOnDay(multi, day.AddDate(0, 0, 1)) {
		t.Fatal("multi-day event not on its middle day")
	}
}

func TestFeedUpdatesOverTime(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := newDemoFeed(7, started)
	t0 := started
	t1 := started.Add(7 * time.Second)

	if f.Clock(t0).Local.Equal(f.Clock(t1).Local) {
		t.Fatal("clock did not advance")
	}
	if reflect.DeepEqual(f.Network(t0).DownSpark, f.Network(t1).DownSpark) {
		t.Fatal("network sparkline did not advance")
	}
	if f.Weather(t0).Updated.Equal(f.Weather(t1).Updated) {
		t.Fatal("weather timestamp did not advance")
	}
}

func TestAgendaOffsetSelectsDay(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := newDemoFeed(1, started)
	today := f.Agenda(started, 0)
	tomorrow := f.Agenda(started, 1)
	if len(today) == 0 || len(tomorrow) == 0 {
		t.Fatalf("empty day: today=%d tomorrow=%d", len(today), len(tomorrow))
	}
	for _, item := range today {
		if !sameDay(item.Start, started) {
			t.Fatalf("today event on the wrong day: %+v", item)
		}
	}
	for _, item := range tomorrow {
		if item.AllDay && item.Start.Hour() != 0 {
			t.Fatalf("all-day event not at midnight: %+v", item)
		}
		if !sameDay(item.Start, started.AddDate(0, 0, 1)) {
			t.Fatalf("tomorrow event on the wrong day: %+v", item)
		}
	}
	if got := f.Agenda(started, 9); len(got) != 0 {
		t.Fatalf("far day = %d events, want 0", len(got))
	}
	// Selecting a day must not mutate the underlying schedule.
	if !sameDay(f.Agenda(started, 0)[0].Start, started) {
		t.Fatal("agenda offset mutated the base data")
	}
}
