package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/allisonhere/tideui/dash"
)

func findRow(t *testing.T, doc dash.Doc, label string) dash.Row {
	t.Helper()
	for _, row := range doc.Rows {
		if row.Label == label {
			return row
		}
	}
	t.Fatalf("no row labelled %q in %#v", label, doc.Rows)
	return dash.Row{}
}

// Without a query the pane is a reader: the search row, the current note, and
// its body. No list of notes.
func TestRenderReaderShowsTheCurrentNote(t *testing.T) {
	current := &Note{Rel: "Projects/Tide.md", Title: "Tide"}
	doc := Render(&Vault{Name: "Vault"}, nil, "", nil, current, []string{"# Tide", "a note"}, nil)

	if doc.Badge == nil || doc.Badge.Text != "Vault" {
		t.Fatalf("badge = %#v, want the vault name", doc.Badge)
	}
	if row := findRow(t, doc, "search"); row.Tone != "muted" {
		t.Fatalf("search row = %#v", row)
	}
	row := findRow(t, doc, "Tide")
	if row.ID != "Projects/Tide.md" || row.Value != "Projects" || row.Tone != "accent" {
		t.Fatalf("current-note row = %#v", row)
	}
	var body *dash.Row
	for i := range doc.Rows {
		if doc.Rows[i].Type == "block" {
			body = &doc.Rows[i]
		}
	}
	if body == nil || len(body.Body) != 2 || body.Body[0] != "# Tide" {
		t.Fatalf("body = %#v", body)
	}
}

// An empty vault says so rather than showing a blank pane.
func TestRenderReaderWithoutNotes(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, "", nil, nil, nil, nil)
	if row := findRow(t, doc, "no notes"); row.Tone != "warning" {
		t.Fatalf("no-notes row = %#v", row)
	}
}

// A query lists the matches with an excerpt and an offer to make a note.
func TestRenderSearchListsMatchesAndOffersANewNote(t *testing.T) {
	matches := []Match{
		{Note: Note{Rel: "TideFTP.md", Title: "TideFTP"}, Excerpt: "the ftp app"},
		{Note: Note{Rel: "Projects/Tide.md", Title: "Tide"}},
	}
	doc := Render(&Vault{Name: "Vault"}, nil, "tide", matches, nil, nil, nil)

	if doc.Badge == nil || !strings.Contains(doc.Badge.Text, "2") {
		t.Fatalf("badge = %#v, want the match count", doc.Badge)
	}
	first := findRow(t, doc, "TideFTP")
	if first.ID != "TideFTP.md" || first.Value != "the ftp app" || first.Type != "text" {
		t.Fatalf("match row = %#v", first)
	}
	// The new-note row sits above the list so it stays visible.
	if doc.Rows[1].ID != newNoteID {
		t.Fatalf("row 1 = %#v, want the new-note row", doc.Rows[1])
	}
}

// A query that matches nothing still offers to make the note.
func TestRenderSearchWithoutMatches(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, "zzz", nil, nil, nil, nil)
	if row := findRow(t, doc, "no note"); row.Tone != "muted" {
		t.Fatalf("no-match row = %#v", row)
	}
	if doc.Rows[1].ID != newNoteID {
		t.Fatalf("row 1 = %#v, want the new-note row", doc.Rows[1])
	}
}

// A vault that cannot be found is explained, never a blank pane.
func TestRenderExplainsAMissingVault(t *testing.T) {
	doc := Render(nil, nil, "", nil, nil, nil, errors.New("Obsidian has no vaults"))
	if doc.Badge == nil || doc.Badge.Tone != "warning" {
		t.Fatalf("badge = %#v, want a warning", doc.Badge)
	}
	if len(doc.Rows) == 0 || doc.Rows[0].Tone != "warning" {
		t.Fatalf("rows = %#v, want one warning", doc.Rows)
	}
}

// The vault setting is offered as a list, blank first for the default.
func TestRenderOffersTheVaults(t *testing.T) {
	vaults := []Vault{{Name: "Vault"}, {Name: "Second"}}
	doc := Render(&vaults[0], vaults, "", nil, nil, nil, nil)
	options := doc.Options["vault"]
	if len(options) != 3 || options[0] != "" || options[1] != "Vault" || options[2] != "Second" {
		t.Fatalf("vault options = %v", options)
	}
}
