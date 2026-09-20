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

// A query draws one openable row per match, and previews the best one.
func TestRenderListsMatchesAndPreviewsTheBest(t *testing.T) {
	vault := &Vault{Name: "Vault"}
	doc := Render(vault, nil, RankNotes(sampleNotes(), "tftp"), "tftp", []string{"# TideFTP", "a note"}, "TideFTP", nil)

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
	var preview *dash.Row
	for i := range doc.Rows {
		if doc.Rows[i].Type == "block" {
			preview = &doc.Rows[i]
		}
	}
	if preview == nil || len(preview.Body) != 2 || preview.Body[0] != "# TideFTP" {
		t.Fatalf("preview = %#v", preview)
	}
	if !strings.Contains(preview.Label, "") && preview.Label != "" {
		t.Fatalf("preview block carried a label: %q", preview.Label)
	}
}

// With no query the pane is the recent notes, not an empty box.
func TestRenderWithoutAQueryShowsTheVault(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, sampleNotes(), "", nil, "", nil)
	row := findRow(t, doc, "search")
	if row.Tone != "muted" || !strings.Contains(row.Value, "Vault") {
		t.Fatalf("hint row = %#v", row)
	}
	if len(doc.Rows) < 3 {
		t.Fatalf("rows = %#v, want the notes", doc.Rows)
	}
}

// A query that matches nothing says so rather than showing an empty pane.
func TestRenderExplainsNoMatches(t *testing.T) {
	doc := Render(&Vault{Name: "Vault"}, nil, nil, "zzz", nil, "", nil)
	row := findRow(t, doc, "no note")
	if row.Tone != "warning" || !strings.Contains(row.Value, "zzz") {
		t.Fatalf("no-match row = %#v", row)
	}
}

// A vault that cannot be found is explained, never a blank pane.
func TestRenderExplainsAMissingVault(t *testing.T) {
	doc := Render(nil, nil, nil, "", nil, "", errors.New("Obsidian has no vaults"))
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
	doc := Render(&vaults[0], vaults, sampleNotes(), "", nil, "", nil)
	options := doc.Options["vault"]
	if len(options) != 3 || options[0] != "" || options[1] != "Vault" || options[2] != "Second" {
		t.Fatalf("vault options = %v", options)
	}
}
