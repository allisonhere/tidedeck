package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/allisonhere/tideui/dash"
)

func sampleNotes() []Note {
	return []Note{
		{Rel: "TideFTP.md", Title: "TideFTP"},
		{Rel: "Projects/Tide.md", Title: "Tide"},
	}
}

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

// Without a query the pane is a single search row: no wall of notes.
func TestRenderWithoutAQueryIsOneRow(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, nil, "", nil)
	if len(doc.Rows) != 1 {
		t.Fatalf("rows = %#v, want the one search row", doc.Rows)
	}
	row := doc.Rows[0]
	if row.Label != "search" || row.ID != "" || row.Tone != "muted" {
		t.Fatalf("search row = %#v", row)
	}
	if doc.Badge == nil || doc.Badge.Text != "Vault" {
		t.Fatalf("badge = %#v, want the vault name", doc.Badge)
	}
}

// A query draws the matches, each openable, and counts them.
func TestRenderListsMatchesWhileSearching(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, RankNotes(sampleNotes(), "tftp"), "tftp", nil)

	if doc.Badge == nil || !strings.Contains(doc.Badge.Text, "1") {
		t.Fatalf("badge = %#v, want the match count", doc.Badge)
	}
	row := findRow(t, doc, "TideFTP")
	if row.ID != "TideFTP.md" {
		t.Fatalf("match row id = %q, want the note path", row.ID)
	}
	if row.Value != "vault root" {
		t.Fatalf("row value = %q, want the folder", row.Value)
	}
	if findRow(t, doc, "search").Value != "tftp" {
		t.Fatalf("the query was not echoed: %#v", doc.Rows)
	}
	// The list is only drawn while searching.
	for _, row := range doc.Rows {
		if row.Type == "block" {
			t.Fatalf("a preview was drawn: %#v", row)
		}
	}
}

// A query that matches nothing says so rather than showing an empty pane.
func TestRenderExplainsNoMatches(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, nil, "zzz", nil)
	row := findRow(t, doc, "no note")
	if row.Tone != "warning" || !strings.Contains(row.Value, "zzz") {
		t.Fatalf("no-match row = %#v", row)
	}
}

// A vault that cannot be found is explained, never a blank pane.
func TestRenderExplainsAMissingVault(t *testing.T) {
	doc := Render(nil, nil, nil, "", errors.New("Obsidian has no vaults"))
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
	doc := Render(&vaults[0], vaults, nil, "", nil)
	options := doc.Options["vault"]
	if len(options) != 3 || options[0] != "" || options[1] != "Vault" || options[2] != "Second" {
		t.Fatalf("vault options = %v", options)
	}
}
