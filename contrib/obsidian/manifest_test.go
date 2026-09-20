package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui/dash"
)

// The dashboard reads this manifest and refuses the panel outright when the
// file is wrong, so it is checked with the dashboard's own validator.
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

// The commands the dashboard runs - render, open and edit - are all this
// program, and the pane takes typing for the query.
func TestManifestRunsThisProgramForEveryJob(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	var raw struct {
		EntryPoints map[string][]string `json:"entryPoints"`
		Panel       struct {
			Glyph      string   `json:"glyph"`
			Open       []string `json:"open"`
			Edit       []string `json:"edit"`
			Input      string   `json:"input"`
			InputChars string   `json:"inputChars"`
			Schema     []struct {
				Key  string `json:"key"`
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"panel"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("the manifest is not JSON: %v", err)
	}

	entry, ok := raw.EntryPoints["panel"]
	if !ok || len(entry) < 2 {
		t.Fatalf("entryPoints.panel is %v, want this program and its render verb", entry)
	}
	if got := filepath.Base(entry[0]); got != "run.sh" {
		t.Fatalf("the panel entry point runs %q, want the entry script beside this manifest", got)
	}
	if entry[1] != "render" {
		t.Fatalf("the panel entry point runs the verb %q, want render", entry[1])
	}

	for _, command := range [][]string{raw.Panel.Open, raw.Panel.Edit} {
		if len(command) == 0 {
			t.Fatal("a row command is missing: a row would have nowhere to go")
		}
		if got := filepath.Base(command[0]); got != "run.sh" {
			t.Fatalf("a row command runs %q, want the same entry script", got)
		}
		if !strings.Contains(strings.Join(command, " "), "{id}") {
			t.Fatalf("%v does not name the row's id, so every row would do the same thing", command)
		}
	}
	if strings.Join(raw.Panel.Open, " ") == strings.Join(raw.Panel.Edit, " ") {
		t.Fatal("open and edit run the same thing: one of the two keys would be a duplicate")
	}

	// The query is the pane's typing, so the setting it names has to exist.
	declared := false
	for _, field := range raw.Panel.Schema {
		if field.Key == raw.Panel.Input {
			declared = true
		}
	}
	if raw.Panel.Input == "" || !declared {
		t.Fatalf("panel.input = %q, which is not a declared setting", raw.Panel.Input)
	}
	if raw.Panel.InputChars == "" {
		t.Fatal("a panel that takes typing must list the runes it accepts")
	}

	if width := ansi.StringWidth(raw.Panel.Glyph); width != 2 {
		t.Fatalf("the glyph %q is %d cells wide, want 2: a colour emoji", raw.Panel.Glyph, width)
	}
}

// The panel is a Go program, so what the manifest names is a script that runs
// the built binary - and building it is a second script.
func TestTheEntryPointAndTheBuildScriptAreRunnable(t *testing.T) {
	for _, name := range []string{"run.sh", "build.sh"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatalf("%s is missing, so an installed copy has nothing to run: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s is not executable (mode %v)", name, info.Mode().Perm())
		}
	}
	if out, err := exec.Command("sh", "-n", "run.sh").CombinedOutput(); err != nil {
		t.Fatalf("run.sh is not valid shell: %v: %s", err, out)
	}
}

// Before it is built, the entry point explains itself rather than printing
// nothing: a blank pane and a broken pane look identical.
func TestRunScriptSaysWhatToDoBeforeItIsBuilt(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile("run.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, body, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", script, "render").Output()
	if err != nil {
		t.Fatalf("the entry point failed before the build: %v", err)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Rows          []struct {
			Tone string   `json:"tone"`
			Body []string `json:"body"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("what it printed is not a document: %v: %s", err, out)
	}
	if len(doc.Rows) == 0 || doc.Rows[0].Tone != "warning" {
		t.Fatalf("an unbuilt plugin did not explain itself: %s", out)
	}
	if err := exec.Command("sh", script, "edit", "x.md").Run(); err == nil {
		t.Fatal("edit succeeded with no binary to run")
	}
}
