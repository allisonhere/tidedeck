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

	if !reflect.DeepEqual(a.Markets(now), b.Markets(now)) {
		t.Fatal("markets are not deterministic")
	}
	if !reflect.DeepEqual(a.Storage(), b.Storage()) || !reflect.DeepEqual(a.Tasks(), b.Tasks()) {
		t.Fatal("static fixtures are not deterministic")
	}
}
