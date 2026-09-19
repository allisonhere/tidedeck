package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui/dash"
)

// The dashboard reads this manifest, and refuses the panel outright when the file
// is wrong - so it is checked with the dashboard's own validator rather than with
// this test's idea of what looks right.
func TestManifestPassesTheDashboardsValidation(t *testing.T) {
	manifest, err := dash.LoadManifest(".")
	if err != nil {
		t.Fatalf("the dashboard could not load this manifest: %v", err)
	}
	if problems := manifest.Validate(); len(problems) > 0 {
		t.Fatalf("the dashboard would reject this manifest: %s", strings.Join(problems, "; "))
	}
	if manifest.ID == "" {
		t.Fatal("the manifest has no id, so the pane could not be remembered or configured")
	}
}

// The two commands the dashboard runs - the one that draws the pane and the one
// that opens a row - are both this program. Anything else is a panel that cannot
// draw itself or a row that cannot open, and neither shows up until it is used.
func TestManifestRunsThisProgramForBothJobs(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	var raw struct {
		EntryPoints map[string][]string `json:"entryPoints"`
		Panel       struct {
			Glyph string   `json:"glyph"`
			Open  []string `json:"open"`
		} `json:"panel"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("the manifest is not JSON: %v", err)
	}

	entry, ok := raw.EntryPoints["panel"]
	if !ok || len(entry) < 2 {
		t.Fatalf("entryPoints.panel is %v, want this program and its render verb", entry)
	}
	if got := filepath.Base(entry[0]); got != "favourites" {
		t.Fatalf("the panel entry point runs %q, want the favourites program built beside this manifest", got)
	}
	if entry[1] != "render" {
		t.Fatalf("the panel entry point runs the verb %q, want render", entry[1])
	}

	if len(raw.Panel.Open) == 0 {
		t.Fatal("no open command: a row would be a destination with nowhere to go")
	}
	if got := filepath.Base(raw.Panel.Open[0]); got != "favourites" {
		t.Fatalf("open runs %q, want the same program", got)
	}
	if !strings.Contains(strings.Join(raw.Panel.Open, " "), "{id}") {
		t.Fatalf("open %v does not name the row's id, so every row would open the same thing", raw.Panel.Open)
	}

	// A pane glyph is a colour emoji, two cells wide, like every other pane the
	// dashboard draws. A one-cell glyph is a monochrome symbol from a text font.
	if width := ansi.StringWidth(raw.Panel.Glyph); width != 2 {
		t.Fatalf("the glyph %q is %d cells wide, want 2", raw.Panel.Glyph, width)
	}
}
