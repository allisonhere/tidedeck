package main

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestLiveSourceCollectsLocalMetrics exercises the live provider path without
// network: system, network, and storage come from /proc and statfs. Network
// sources are explicitly left unconfigured.
func TestLiveSourceCollectsLocalMetrics(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("live system metrics are linux-only")
	}
	for _, key := range []string{
		"TIDEDECK_LAT", "TIDEDECK_LON", "TIDEDECK_FEEDS", "TIDEDECK_REPOS",
		"TIDEDECK_TODO", "TIDEDECK_NOTES", "TIDEDECK_ICS", "TIDEDECK_SYMBOLS",
		"TIDEDECK_SYSTEMD", "TIDEDECK_DOCKER",
	} {
		t.Setenv(key, "")
	}

	source := newLiveSource()
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
