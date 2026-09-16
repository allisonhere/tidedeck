package main

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/allisonhere/tideui/provider"
)

// TestLiveSourceCollectsLocalMetrics exercises the live provider path without
// network: network and storage come from /proc and statfs. Only those local
// sources are configured; system is a registry panel now and is covered in
// dash/panels.
func TestLiveSourceCollectsLocalMetrics(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("live network and storage metrics are linux-only")
	}
	source := newLiveSource(config{})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		source.refresh(context.Background())
		if snapshot := source.snapshotCopy(); len(snapshot.Storage) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no storage metrics collected from /proc")
}

func TestAgendaNoticePrefersTheHint(t *testing.T) {
	state := &demoState{live: &liveSource{}}
	state.live.snapshot = provider.Snapshot{Errors: map[string]error{
		"agenda": errors.New("provider: calendar https://example.com/cal.ics: status 404 (a private Google Calendar needs its secret iCal address)"),
	}}
	if got, want := state.agendaNotice(), "a private Google Calendar needs its secret iCal address"; got != want {
		t.Fatalf("agendaNotice = %q, want %q", got, want)
	}
	state.live.snapshot = provider.Snapshot{Errors: map[string]error{"agenda": errors.New("boom")}}
	if got, want := state.agendaNotice(), "boom"; got != want {
		t.Fatalf("agendaNotice = %q, want %q", got, want)
	}
	state.live = nil
	if got := state.agendaNotice(); got != "" {
		t.Fatalf("agendaNotice without live = %q, want empty", got)
	}
}
