package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// stamp is a fixed timestamp for fixtures: the order of the list is what these
// tests are about, and it has to be readable at a glance.
func stamp(day, hour int) time.Time {
	return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC)
}

// A list of favourites becomes a row each, in the order the reader cares about:
// the most recently added first. The last row is always the way to add another.
func TestRenderPrintsOneRowPerFavourite(t *testing.T) {
	doc := Render([]Favourite{
		{Title: "Go", URL: "https://go.dev", Added: stamp(17, 4)},
		{Title: "Hacker News", URL: "https://news.ycombinator.com", Tags: []string{"news"}, Added: stamp(18, 9)},
	}, nil)

	if len(doc.Rows) != 3 {
		t.Fatalf("two favourites drew %d rows, want three with the add row", len(doc.Rows))
	}
	if doc.Rows[0].Label != "Hacker News" || doc.Rows[0].ID != "https://news.ycombinator.com" {
		t.Fatalf("the newest favourite is not first: %#v", doc.Rows[0])
	}
	if doc.Rows[1].Label != "Go" {
		t.Fatalf("the older favourite is out of order: %#v", doc.Rows[1])
	}
	if doc.Rows[0].Value != "news" {
		t.Fatalf("the tags are not the row's body: %q", doc.Rows[0].Value)
	}
	if doc.Rows[1].Value != "go.dev" {
		t.Fatalf("a favourite with no tags should show its host, got %q", doc.Rows[1].Value)
	}
	if last := doc.Rows[len(doc.Rows)-1]; last.ID != addRowID {
		t.Fatalf("the last row is %#v, want the add row", last)
	}
}

// An empty list still has to offer the way out of being empty: an empty panel has
// no openable row, so enter could only ever say "select one first".
func TestRenderAlwaysOffersAdding(t *testing.T) {
	doc := Render(nil, nil)
	if len(doc.Rows) == 0 {
		t.Fatal("an empty list drew no rows at all")
	}
	last := doc.Rows[len(doc.Rows)-1]
	if last.ID != addRowID || last.Label == "" {
		t.Fatalf("an empty list's last row is %#v, want the add row", last)
	}
}

// A row that cannot be opened is a row without an id, and it says so in its tone.
// The cursor skips it, so enter can never hand a javascript: URL to a browser-opener.
func TestRenderSkipsNonWebURLs(t *testing.T) {
	doc := Render([]Favourite{
		{Title: "A bookmarklet", URL: "javascript:alert(1)", Added: stamp(18, 9)},
	}, nil)
	row := doc.Rows[0]
	if row.ID != "" {
		t.Fatalf("a non-web URL is openable: %#v", row)
	}
	if !strings.EqualFold(row.Tone, "warning") {
		t.Fatalf("a non-web URL is not flagged, tone = %q", row.Tone)
	}
	if row.Label != "A bookmarklet" {
		t.Fatalf("the row lost its title: %#v", row)
	}
}

// A list that cannot be read is explained rather than shown as empty: a blank
// panel and a broken file look identical otherwise.
func TestRenderExplainsAStoreItCannotRead(t *testing.T) {
	problem := fmt.Errorf("reading /home/someone/.local/share/tidedeck/favourites.json: %w",
		errors.New("invalid character 'a' looking for beginning of object key string"))
	doc := Render(nil, problem)
	if len(doc.Rows) < 2 {
		t.Fatalf("a broken list drew %d rows, want a notice and the add row", len(doc.Rows))
	}
	notice := doc.Rows[0]
	if !strings.EqualFold(notice.Tone, "warning") {
		t.Fatalf("the notice is not a warning: %#v", notice)
	}
	// The document as a whole has to name the file and the parse error: the
	// reader can act on the first and the second is what a search engine needs.
	var said []string
	for _, row := range doc.Rows {
		said = append(said, row.Label, row.Value)
		said = append(said, row.Body...)
	}
	body := strings.Join(said, " ")
	for _, want := range []string{"favourites.json", "invalid character"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the notice does not mention %q: %q", want, body)
		}
	}
	// And it still offers the way forward: an unreadable list is not a dead end
	// for someone who wants to start again at a different path.
	if last := doc.Rows[len(doc.Rows)-1]; last.ID != addRowID {
		t.Fatalf("an unreadable list lost the add row: %#v", doc.Rows[len(doc.Rows)-1])
	}
}

// The document is what the dashboard parses, so its shape is asserted rather than
// assumed: the schema version, and rows the panel knows how to draw.
func TestRenderMarshalsToThePanelSchema(t *testing.T) {
	data, err := json.Marshal(Render([]Favourite{{Title: "Go", URL: "https://go.dev"}}, nil))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	var parsed struct {
		SchemaVersion int `json:"schemaVersion"`
		Rows          []struct {
			Type  string `json:"type"`
			Label string `json:"label"`
			ID    string `json:"id"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if parsed.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", parsed.SchemaVersion)
	}
	if len(parsed.Rows) == 0 || parsed.Rows[0].Type != "text" {
		t.Fatalf("the first row is not a text row: %+v", parsed.Rows)
	}
	if parsed.Rows[0].ID != "https://go.dev" {
		t.Fatalf("the row's id is not the URL: %+v", parsed.Rows[0])
	}
}
