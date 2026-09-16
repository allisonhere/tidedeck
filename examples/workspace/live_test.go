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
// network: system, network, and storage come from /proc and statfs. Only those
// local sources are configured.
func TestLiveSourceCollectsLocalMetrics(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("live system metrics are linux-only")
	}
	cfg := config{Interface: ""}

	source := newLiveSource(cfg)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		source.refresh(context.Background())
		if snapshot := source.snapshotCopy(); snapshot.System != nil {
			if snapshot.System.CPUPercent < 0 || snapshot.System.CPUPercent > 100 {
				t.Fatalf("cpu percent = %v", snapshot.System.CPUPercent)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no system metrics collected from /proc")
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
