package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

// addRowID is the id the editor reads as "a blank form". It cannot collide with a
// favorite's id because the renderer only ever puts a web URL in an id.
const addRowID = "add"

// addRowLabel is how the invitation reads in the list itself.
const addRowLabel = "＋ add a favorite"

// Render turns the list into the document the panel draws. problem is what went
// wrong reading the list, if anything - a list that cannot be read is explained
// rather than shown as empty, because a blank panel and a broken file look
// identical otherwise, and one of them is the reader's to fix.
func Render(list []Favorite, problem error) dash.Doc {
	doc := dash.Doc{SchemaVersion: dash.DocSchemaVersion}
	switch {
	case problem != nil:
		// The error already names the file: the store wraps every failure with
		// the path it failed on.
		doc.Rows = append(doc.Rows,
			dash.Row{Type: "text", Label: "favorites", Value: "the list could not be read", Tone: "warning"},
			dash.Row{Type: "block", Body: []string{problem.Error()}, BodyTone: "muted"})
		doc.Badge = &dash.DocBadge{Text: "unreadable", Tone: "warning"}

	case len(list) == 0:
		doc.Rows = append(doc.Rows,
			dash.Row{Type: "text", Label: "favorites", Value: "nothing saved yet", Tone: "muted"})

	default:
		sorted := append([]Favorite(nil), list...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Added.After(sorted[j].Added) })
		for _, favorite := range sorted {
			doc.Rows = append(doc.Rows, rowFor(favorite))
		}
		doc.Badge = &dash.DocBadge{Text: fmt.Sprintf("%d saved", len(sorted)), Tone: "muted"}
	}

	// Every document ends with the way to add another. An empty list's last
	// openable row would otherwise be nothing at all, and enter on a pane with
	// nothing to select can only ever say "select one first".
	doc.Rows = append(doc.Rows, addRow())
	return doc
}

// rowFor is one favorite's row: the title as the label, the tags or the site as
// the body, and the URL as the id when a browser can open it.
func rowFor(favorite Favorite) dash.Row {
	row := dash.Row{Type: "text", Label: displayTitle(favorite), Value: detail(favorite)}
	if !openable(favorite.URL) {
		// A row without an id is not a destination - the cursor skips it - so
		// enter can never hand a javascript: or file: URL to the opener. The
		// warning tone is what tells the reader why this row is different.
		row.Tone = "warning"
		return row
	}
	row.ID = favorite.URL
	return row
}

// displayTitle falls back to the URL, because a row with an empty label looks
// like nothing rather than like an entry with no title.
func displayTitle(favorite Favorite) string {
	if title := strings.TrimSpace(favorite.Title); title != "" {
		return title
	}
	return favorite.URL
}

// detail is what the row says beside its title: the tags if it has any, and the
// site otherwise, so no row is a bare title.
func detail(favorite Favorite) string {
	if len(favorite.Tags) > 0 {
		return strings.Join(favorite.Tags, " · ")
	}
	parsed, err := url.Parse(strings.TrimSpace(favorite.URL))
	if err != nil {
		return favorite.URL
	}
	if parsed.Host != "" {
		return strings.TrimPrefix(parsed.Host, "www.")
	}
	if parsed.Scheme != "" {
		return parsed.Scheme + " link"
	}
	return favorite.URL
}

// openable reports whether a URL is something to hand a browser. Only the web
// schemes are: a bookmarklet or a local file is not a link the opener should see.
func openable(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
}

// addRow is the invitation the list always ends with.
func addRow() dash.Row {
	return dash.Row{Type: "text", Label: addRowLabel, Value: "a title, a link, and tags", ID: addRowID}
}
