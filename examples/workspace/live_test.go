package main

import (
	"context"
	"runtime"
	"testing"
	"time"
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
