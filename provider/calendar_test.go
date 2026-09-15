package provider

import "testing"

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

func TestParseICS(t *testing.T) {
	items := parseICS(sampleICS)
	if len(items) != 2 {
		t.Fatalf("events = %d, want 2", len(items))
	}
	first := items[0]
	if first.Title != "Project review" || first.Location != "Meet" || first.Category != "Work" {
		t.Fatalf("first event = %+v", first)
	}
	if first.Start.Hour() != 9 || first.Start.Minute() != 30 {
		t.Fatalf("start = %v, want 09:30", first.Start)
	}
	if first.Start.Day() != 14 {
		t.Fatalf("start day = %d, want 14", first.Start.Day())
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
