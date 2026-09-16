package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

const sampleICS = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:1
DTSTART;TZID=Europe/London:20260914T093000
DTEND;TZID=Europe/London:20260914T101500
SUMMARY:Project review
LOCATION:Meet
CATEGORIES:Work
END:VEVENT
BEGIN:VEVENT
UID:2
DTSTART:20260915T080000Z
SUMMARY:Standup
END:VEVENT
END:VCALENDAR
`

const sampleAllDayICS = `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:1
DTSTART;VALUE=DATE:20260920
DTEND;VALUE=DATE:20260921
SUMMARY:Company holiday
END:VEVENT
BEGIN:VEVENT
UID:2
DTSTART:20260921
SUMMARY:Conference
END:VEVENT
END:VCALENDAR
`

func TestParseICS(t *testing.T) {
	items := parseICS(sampleICS)
	if len(items) != 2 {
		t.Fatalf("events = %d, want 2", len(items))
	}
	first := items[0]
	if first.Title != "Project review" || first.Location != "Meet" || first.Category != "Work" {
		t.Fatalf("first event = %+v", first)
	}
	if first.AllDay {
		t.Fatalf("timed event marked AllDay: %+v", first)
	}
	if first.Start.Hour() != 9 || first.Start.Minute() != 30 {
		t.Fatalf("start = %v, want 09:30", first.Start)
	}
	if first.Start.Day() != 14 {
		t.Fatalf("start day = %d, want 14", first.Start.Day())
	}
	if first.End.Hour() != 10 || first.End.Minute() != 15 {
		t.Fatalf("end = %v, want 10:15", first.End)
	}
	if items[1].Title != "Standup" {
		t.Fatalf("second event = %+v", items[1])
	}
}

func TestParseICSIgnoresNonEvents(t *testing.T) {
	if items := parseICS("BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR\n"); len(items) != 0 {
		t.Fatalf("expected no events, got %d", len(items))
	}
}

func TestParseICSAllDay(t *testing.T) {
	items := parseICS(sampleAllDayICS)
	if len(items) != 2 {
		t.Fatalf("events = %d, want 2", len(items))
	}
	holiday := items[0]
	if !holiday.AllDay {
		t.Fatalf("event not marked AllDay: %+v", holiday)
	}
	if holiday.Start.Hour() != 0 || holiday.Start.Minute() != 0 {
		t.Fatalf("all-day start = %v, want midnight", holiday.Start)
	}
	if y, m, d := holiday.Start.Date(); y != 2026 || m != time.September || d != 20 {
		t.Fatalf("all-day date = %d-%v-%d, want 2026-09-20", y, m, d)
	}
	if want := holiday.Start.AddDate(0, 0, 1); !holiday.End.Equal(want) {
		t.Fatalf("all-day end = %v, want %v", holiday.End, want)
	}
	conference := items[1]
	if !conference.AllDay || !conference.End.IsZero() {
		t.Fatalf("bare date event = %+v", conference)
	}
}

func TestParseICSUTC(t *testing.T) {
	items := parseICS(sampleICS)
	standup := items[1]
	if name, _ := standup.Start.Zone(); name != "UTC" {
		t.Fatalf("Z time zone = %q, want UTC", name)
	}
	if standup.Start.Hour() != 8 {
		t.Fatalf("Z start = %v, want 08:00", standup.Start)
	}
}

func TestParseICSTZID(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skip("tzdata unavailable")
	}
	const zoned = `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:1
DTSTART;TZID=America/New_York:20260914T090000
DTEND;TZID=America/New_York:20260914T101500
SUMMARY:Call
END:VEVENT
END:VCALENDAR
`
	items := parseICS(zoned)
	if len(items) != 1 {
		t.Fatalf("events = %d, want 1", len(items))
	}
	call := items[0]
	if name := call.Start.Location().String(); name != "America/New_York" {
		t.Fatalf("zone = %q, want America/New_York", name)
	}
	if call.Start.Hour() != 9 || call.End.Hour() != 10 || call.End.Minute() != 15 {
		t.Fatalf("call = %v..%v, want 09:00..10:15 local", call.Start, call.End)
	}
}

func TestWindowEventsKeepsRunningAndDropsStale(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	items := []tideui.AgendaItem{
		{Title: "stale", Start: now.Add(-72 * time.Hour), End: now.Add(-71 * time.Hour)},
		{Title: "running", Start: now.Add(-time.Hour), End: now.Add(time.Hour)},
		{Title: "soon", Start: now.Add(2 * time.Hour)},
		{Title: "far", Start: now.Add(120 * 24 * time.Hour)},
	}
	got := map[string]bool{}
	for _, item := range windowEvents(items, now) {
		got[item.Title] = true
	}
	if got["stale"] || got["far"] {
		t.Fatalf("window kept %v", got)
	}
	if !got["running"] || !got["soon"] {
		t.Fatalf("window dropped %v", got)
	}
}

func TestReadCalendarLocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cal.ics")
	if err := os.WriteFile(path, []byte(sampleICS), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := readCalendar(context.Background(), path)
	if err != nil {
		t.Fatalf("readCalendar: %v", err)
	}
	if !strings.Contains(data, "BEGIN:VCALENDAR") {
		t.Fatalf("data = %q", data)
	}
}

func TestReadCalendarMissingFile(t *testing.T) {
	if _, err := readCalendar(context.Background(), filepath.Join(t.TempDir(), "nope.ics")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestNormalizeCalendarSource(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		isURL bool
	}{
		{"  https://example.com/a.ics ", "https://example.com/a.ics", true},
		{"http://example.com/a.ics", "http://example.com/a.ics", true},
		{"webcal://example.com/a.ics", "https://example.com/a.ics", true},
		{"/home/me/cal.ics", "/home/me/cal.ics", false},
		{"~/cal.ics", "~/cal.ics", false},
	}
	for _, c := range cases {
		got, isURL := normalizeCalendarSource(c.in)
		if got != c.want || isURL != c.isURL {
			t.Fatalf("normalize(%q) = %q,%v want %q,%v", c.in, got, isURL, c.want, c.isURL)
		}
	}
}
