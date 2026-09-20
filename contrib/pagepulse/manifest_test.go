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

// The dashboard reads this manifest, and refuses the panel outright when the
// file is wrong - so it is checked with the dashboard's own validator rather
// than with this test's idea of what looks right.
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
// draw itself or a row that cannot open.
func TestManifestRunsThisProgramForBothJobs(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	var raw struct {
		EntryPoints map[string][]string `json:"entryPoints"`
		Panel       struct {
			Glyph  string   `json:"glyph"`
			Open   []string `json:"open"`
			Schema []struct {
				Key     string   `json:"key"`
				Type    string   `json:"type"`
				Options []string `json:"options"`
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

	if len(raw.Panel.Open) == 0 {
		t.Fatal("no open command: a row would be a destination with nowhere to go")
	}
	if got := filepath.Base(raw.Panel.Open[0]); got != "run.sh" {
		t.Fatalf("open runs %q, want the same entry script", got)
	}
	if !strings.Contains(strings.Join(raw.Panel.Open, " "), "{id}") {
		t.Fatalf("open %v does not name the row's id, so every row would open the same thing", raw.Panel.Open)
	}

	// The site setting must be a plain string so the program's options can widen
	// it into a list; a choice with fixed options could not.
	fields := map[string]string{}
	for _, field := range raw.Panel.Schema {
		fields[field.Key] = field.Type
	}
	if fields["site"] != "string" {
		t.Fatalf("the site setting is type %q, want string so options may fill it", fields["site"])
	}
	if fields["preset"] != "choice" {
		t.Fatalf("the period setting is type %q, want a choice", fields["preset"])
	}
	for _, field := range raw.Panel.Schema {
		if field.Type == "choice" && len(field.Options) == 0 {
			t.Fatalf("%s is a choice with no options", field.Key)
		}
	}

	// A pane glyph is a colour emoji, two cells wide, like every other pane the
	// dashboard draws. A one-cell glyph is a monochrome symbol from a text font.
	if width := ansi.StringWidth(raw.Panel.Glyph); width != 2 {
		t.Fatalf("the glyph %q is %d cells wide, want 2", raw.Panel.Glyph, width)
	}
}

// The panel is a Go program, so what the manifest names is a script that runs the
// built binary - and building it is a second script. A published plugin whose
// program has to be compiled is only installable if both are there and runnable.
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
// nothing: a blank pane and a broken pane look identical, and this one is the
// reader's to fix.
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
			Value string   `json:"value"`
			Tone  string   `json:"tone"`
			Body  []string `json:"body"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("what it printed is not a document: %v: %s", err, out)
	}
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if len(doc.Rows) == 0 || !strings.EqualFold(doc.Rows[0].Tone, "warning") {
		t.Fatalf("an unbuilt plugin did not explain itself: %s", out)
	}
	var said []string
	for _, row := range doc.Rows {
		said = append(said, row.Value)
		said = append(said, row.Body...)
	}
	if !strings.Contains(strings.Join(said, " "), "build.sh") {
		t.Fatalf("the notice does not name the command to run: %q", strings.Join(said, " "))
	}

	if err := exec.Command("sh", script, "open", "https://example.com").Run(); err == nil {
		t.Fatal("open succeeded with no binary to run")
	}
}

// Run anywhere without the module above it, the build script explains why rather
// than failing with a compiler error nobody can act on.
func TestBuildScriptExplainsItselfOutsideACheckout(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile("build.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, body, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("sh", script).CombinedOutput()
	if err == nil {
		t.Fatalf("build.sh claimed to build outside a checkout: %s", out)
	}
	if !strings.Contains(string(out), "go.mod") {
		t.Fatalf("the refusal does not explain itself: %s", out)
	}
}
