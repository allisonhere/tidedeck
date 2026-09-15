package provider

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Calendar builds an agenda source from local iCalendar (.ics) files exported
// by Google Calendar, Fastmail, Nextcloud, and others. Remote CalDAV is a
// later integration; exporting to a file works today.
func Calendar(paths ...string) func(context.Context) ([]tideui.AgendaItem, error) {
	cloned := append([]string(nil), paths...)
	return func(context.Context) ([]tideui.AgendaItem, error) {
		var items []tideui.AgendaItem
		var firstErr error
		for _, path := range cloned {
			data, err := os.ReadFile(path)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			items = append(items, parseICS(string(data))...)
		}
		if len(items) == 0 && firstErr != nil {
			return nil, firstErr
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
		return items, nil
	}
}

// parseICS extracts VEVENT records from an iCalendar document.
func parseICS(content string) []tideui.AgendaItem {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\n ", "")
	content = strings.ReplaceAll(content, "\n\t", "")

	var items []tideui.AgendaItem
	var fields map[string]string
	inEvent := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "BEGIN:VEVENT":
			inEvent, fields = true, map[string]string{}
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
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if semi := strings.IndexByte(key, ';'); semi >= 0 {
			key = key[:semi]
		}
		fields[strings.ToUpper(strings.TrimSpace(key))] = value
	}
	return items
}

func eventToAgenda(fields map[string]string) (tideui.AgendaItem, bool) {
	start, ok := parseICSDate(fields["DTSTART"])
	if !ok {
		return tideui.AgendaItem{}, false
	}
	item := tideui.AgendaItem{
		Title:    unescapeICS(fields["SUMMARY"]),
		Start:    start,
		Location: unescapeICS(fields["LOCATION"]),
		Category: unescapeICS(fields["CATEGORIES"]),
		Tone:     tideui.ToneAccent,
	}
	if end, ok := parseICSDate(fields["DTEND"]); ok {
		item.End = end
	}
	if item.Title == "" {
		item.Title = "(untitled)"
	}
	return item, true
}

func parseICSDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		"20060102T150405Z", "20060102T150405",
		"20060102T1504Z", "20060102T1504",
		"20060102",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func unescapeICS(value string) string {
	replacer := strings.NewReplacer(`\,`, ",", `\;`, ";", `\n`, " ", `\N`, " ", `\\`, `\`)
	return strings.TrimSpace(replacer.Replace(value))
}
