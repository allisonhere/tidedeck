package provider

import (
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

func TestSystemdStatus(t *testing.T) {
	cases := []struct {
		active, sub string
		want        tideui.StatusKind
	}{
		{"active", "running", tideui.StatusHealthy},
		{"active", "exited", tideui.StatusActive},
		{"failed", "failed", tideui.StatusError},
		{"inactive", "dead", tideui.StatusStopped},
		{"activating", "start", tideui.StatusUpdating},
		{"", "", tideui.StatusStale},
	}
	for _, tc := range cases {
		if got := statusForSystemd(tc.active, tc.sub); got != tc.want {
			t.Fatalf("statusForSystemd(%q,%q) = %v, want %v", tc.active, tc.sub, got, tc.want)
		}
	}
}

func TestParseSystemdTime(t *testing.T) {
	parsed, ok := parseSystemdTime("Mon 2026-09-14 09:00:00 UTC")
	if !ok || parsed.Year() != 2026 || parsed.Hour() != 9 {
		t.Fatalf("parseSystemdTime = %v, %v", parsed, ok)
	}
	if _, ok := parseSystemdTime(""); ok {
		t.Fatal("empty timestamp parsed")
	}
}

func TestDockerStatus(t *testing.T) {
	cases := []struct {
		state, status string
		want          tideui.StatusKind
		label         string
	}{
		{"running", "Up 3 days (healthy)", tideui.StatusHealthy, "healthy"},
		{"running", "Up 2 hours (unhealthy)", tideui.StatusWarning, "unhealthy"},
		{"running", "Up 5 minutes (health: starting)", tideui.StatusUpdating, "starting"},
		{"running", "Up 10 minutes", tideui.StatusActive, "running"},
		{"restarting", "Restarting (1) 2 seconds ago", tideui.StatusUpdating, "restarting"},
		{"paused", "Up 1 hour (Paused)", tideui.StatusStale, "paused"},
		{"exited", "Exited (0) 2 days ago", tideui.StatusStopped, "stopped"},
	}
	for _, tc := range cases {
		kind, label := dockerStatus(tc.state, tc.status)
		if kind != tc.want || label != tc.label {
			t.Fatalf("dockerStatus(%q,%q) = %v/%q, want %v/%q", tc.state, tc.status, kind, label, tc.want, tc.label)
		}
	}
}

func TestDockerUptime(t *testing.T) {
	cases := map[string]string{
		"Up 3 days (healthy)": "3d",
		"Up 5 minutes":        "5m",
		"Up 2 hours":          "2h",
		"Exited (0)":          "",
	}
	for status, want := range cases {
		if got := dockerUptime(status); got != want {
			t.Fatalf("dockerUptime(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestScheduleParse(t *testing.T) {
	// A small guard that the age formatting used by systemd/docker is stable.
	if got := humanAge(3 * 24 * time.Hour); got != "3d" {
		t.Fatalf("humanAge = %q", got)
	}
}
