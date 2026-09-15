package main

import (
	"reflect"
	"testing"
	"time"
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

func TestAgendaOffsetShiftsByDay(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := newDemoFeed(1, started)
	base := f.Agenda(started, 0)
	shifted := f.Agenda(started, 1)
	if len(base) != len(shifted) {
		t.Fatalf("lengths differ: %d vs %d", len(base), len(shifted))
	}
	if !shifted[0].Start.Equal(base[0].Start.AddDate(0, 0, 1)) {
		t.Fatalf("offset did not shift the first event: %v vs %v", shifted[0].Start, base[0].Start)
	}
	// The base feed must not be mutated by shifting.
	if !f.Agenda(started, 0)[0].Start.Equal(base[0].Start) {
		t.Fatal("agenda offset mutated the base data")
	}
}
