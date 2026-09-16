package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// calendarHorizon is how far ahead events are kept. A calendar file holds
// years of history; an agenda panel wants the next few weeks.
const calendarHorizon = 90 * 24 * time.Hour

// Calendar builds an agenda source from iCalendar data. Each source is either
// a local .ics file or an https URL - the "secret address in iCal format" that
// Google Calendar, Fastmail and Nextcloud publish - so the agenda can follow a
// live calendar rather than a stale export.
func Calendar(sources ...string) func(context.Context) ([]tideui.AgendaItem, error) {
	cloned := append([]string(nil), sources...)
	return func(ctx context.Context) ([]tideui.AgendaItem, error) {
		var items []tideui.AgendaItem
		var firstErr error
		for _, source := range cloned {
			data, err := readCalendar(ctx, source)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			items = append(items, parseICS(data)...)
		}
		if len(items) == 0 && firstErr != nil {
			return nil, firstErr
		}
		items = windowEvents(items, time.Now())
		sort.SliceStable(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
		return items, nil
	}
}

// normalizeCalendarSource maps webcal:// - an https URL wearing a different
// scheme - to https://, and reports whether the source is a URL rather than a
// local path.
func normalizeCalendarSource(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if rest, ok := strings.CutPrefix(source, "webcal://"); ok {
		source = "https://" + rest
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return source, true
	}
	return source, false
}

// readCalendar fetches a calendar URL or reads a local file.
func readCalendar(ctx context.Context, source string) (string, error) {
	source, isURL := normalizeCalendarSource(source)
	if !isURL {
		data, err := os.ReadFile(source)
		return string(data), err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "tidedeck/1.0 (+https://github.com/allisonhere/tidedeck)")
	response, err := httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("provider: calendar %s: status %d", source, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	return string(body), err
}

// windowEvents keeps what an agenda can use: everything still running or yet
// to start, up to the horizon. Without this, years of past events crowd out
// the next few days.
func windowEvents(items []tideui.AgendaItem, now time.Time) []tideui.AgendaItem {
	cutoff := now.Add(-24 * time.Hour)
	limit := now.Add(calendarHorizon)
	kept := items[:0]
	for _, item := range items {
		end := item.End
		if end.IsZero() {
			end = item.Start
		}
		if end.Before(cutoff) || item.Start.After(limit) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// icsLine is one unfolded content line: its value plus the parameters that
// qualify it. The parameters carry the timezone and whether a date is a
// whole day, so discarding them - as this parser used to - silently shifts
// every timed event by the UTC offset and moves all-day events a day.
type icsLine struct {
	value  string
	params map[string]string
}

// parseICS extracts VEVENT records from an iCalendar document.
func parseICS(content string) []tideui.AgendaItem {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\n ", "")
	content = strings.ReplaceAll(content, "\n\t", "")

	var items []tideui.AgendaItem
	var fields map[string]icsLine
	inEvent := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "BEGIN:VEVENT":
			inEvent, fields = true, map[string]icsLine{}
			continue
		case line == "END:VEVENT":
			if inEvent {
				if item, ok := eventToAgenda(fields); ok {
					items = append(items, item)
				}
			}
			inEvent, fields = false, nil
			continue
		case !inEvent, line == "":
			continue
		}
		if key, entry, ok := parseICSLine(line); ok {
			fields[key] = entry
		}
	}
	return items
}

// parseICSLine splits "KEY;PARAM=V:value" into its three parts.
func parseICSLine(line string) (string, icsLine, bool) {
	head, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", icsLine{}, false
	}
	parts := strings.Split(head, ";")
	key := strings.ToUpper(strings.TrimSpace(parts[0]))
	entry := icsLine{value: value, params: map[string]string{}}
	for _, param := range parts[1:] {
		name, argument, ok := strings.Cut(param, "=")
		if !ok {
			continue
		}
		entry.params[strings.ToUpper(strings.TrimSpace(name))] = strings.Trim(strings.TrimSpace(argument), `"`)
	}
	return key, entry, true
}

func eventToAgenda(fields map[string]icsLine) (tideui.AgendaItem, bool) {
	start, allDay, ok := parseICSTime(fields["DTSTART"])
	if !ok {
		return tideui.AgendaItem{}, false
	}
	item := tideui.AgendaItem{
		Title:    unescapeICS(fields["SUMMARY"].value),
		Start:    start,
		AllDay:   allDay,
		Location: unescapeICS(fields["LOCATION"].value),
		Category: unescapeICS(fields["CATEGORIES"].value),
		Tone:     tideui.ToneAccent,
	}
	if end, _, ok := parseICSTime(fields["DTEND"]); ok {
		item.End = end
	}
	if item.Title == "" {
		item.Title = "(untitled)"
	}
	return item, true
}

// parseICSTime resolves a date-time in the zone it was written in. A value
// ending in Z is UTC; a TZID names its zone; a whole-day date and a floating
// time both mean local time, which is what a calendar shows its owner.
func parseICSTime(entry icsLine) (time.Time, bool, bool) {
	value := strings.TrimSpace(entry.value)
	if value == "" {
		return time.Time{}, false, false
	}
	if entry.params["VALUE"] == "DATE" || len(value) == 8 {
		if parsed, err := time.ParseInLocation("20060102", value, time.Local); err == nil {
			return parsed, true, true
		}
	}
	location := time.Local
	if tzid := entry.params["TZID"]; tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			location = loaded
		}
	}
	if strings.HasSuffix(value, "Z") {
		location = time.UTC
	}
	for _, layout := range []string{
		"20060102T150405Z", "20060102T150405",
		"20060102T1504Z", "20060102T1504",
	} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, false, true
		}
	}
	return time.Time{}, false, false
}

func unescapeICS(value string) string {
	replacer := strings.NewReplacer(`\,`, ",", `\;`, ";", `\n`, " ", `\N`, " ", `\\`, `\`)
	return strings.TrimSpace(replacer.Replace(value))
}
