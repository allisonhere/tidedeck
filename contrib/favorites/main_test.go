package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderInto runs the program's render verb against a store at path.
func renderInto(t *testing.T, path string) (string, int) {
	t.Helper()
	t.Setenv("TIDEDECK_PLUGIN_PATH", path)
	var stdout, stderr bytes.Buffer
	code := run([]string{"render"}, &stdout, &stderr)
	if stderr.Len() > 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	return stdout.String(), code
}

// The panel parses what render prints, so its shape is asserted rather than
// assumed: one JSON document, one line, the schema version the dashboard reads.
func TestRunRenderPrintsADocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	if err := (Store{Path: path}).Save([]Favorite{
		{Title: "Go", URL: "https://go.dev", Tags: []string{"code"}},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	out, code := renderInto(t, path)
	if code != 0 {
		t.Fatalf("render exited %d", code)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("render printed more than one line: %q", out)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Rows          []struct {
			Label string `json:"label"`
			ID    string `json:"id"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("render printed something that is not a document: %v", err)
	}
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if len(doc.Rows) != 2 {
		t.Fatalf("one favorite drew %d rows, want it and the add row", len(doc.Rows))
	}
	if doc.Rows[0].ID != "https://go.dev" {
		t.Fatalf("the first row's id is %q, want the link", doc.Rows[0].ID)
	}
	if doc.Rows[len(doc.Rows)-1].ID != addRowID {
		t.Fatalf("the last row is %#v, want the add row", doc.Rows[len(doc.Rows)-1])
	}
}

// A list that cannot be read still renders, and says so: a panel that fails here
// would keep showing its last good content and never explain itself.
func TestRunRenderExplainsABrokenList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	if err := os.WriteFile(path, []byte("{not a list"), 0o600); err != nil {
		t.Fatalf("writing the broken file: %v", err)
	}

	out, code := renderInto(t, path)
	if code != 0 {
		t.Fatalf("render exited %d on a broken list, want a document that explains it", code)
	}
	var doc struct {
		Rows []struct {
			Value string `json:"value"`
			Tone  string `json:"tone"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the explanation is not a document: %v", err)
	}
	if len(doc.Rows) == 0 || !strings.EqualFold(doc.Rows[0].Tone, "warning") {
		t.Fatalf("a broken list did not explain itself: %q", out)
	}
	if !strings.Contains(doc.Rows[0].Value, "could not be read") {
		t.Fatalf("the explanation does not say what is wrong: %q", doc.Rows[0].Value)
	}
}

// A path is what the panel's setting changes, so the program can say where it is
// looking - which is the first question when a form saves somewhere unexpected.
func TestRunPathPrintsTheStore(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_PATH", "~/links.json")
	t.Setenv("HOME", "/home/someone")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"path"}, &stdout, &stderr); code != 0 {
		t.Fatalf("path exited %d: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "/home/someone/links.json" {
		t.Fatalf("path printed %q, want the expanded setting", got)
	}
}

// Misuse is a usage message and a non-zero exit, because the dashboard shows what
// a failed program printed and a silent failure reads as a broken pane.
func TestRunRefusesWhatItCannotDo(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"edit with no link", []string{"edit"}},
		{"an unknown verb", []string{"frobnicate"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(c.args, &stdout, &stderr); code == 0 {
				t.Fatalf("%v exited 0", c.args)
			}
			if !strings.Contains(stderr.String(), "favorites") {
				t.Fatalf("%v said nothing useful: %q", c.args, stderr.String())
			}
		})
	}
}

// Editing something that is not in the list is an error rather than a new
// favorite: a stale pane asking for a link that has gone should say so.
func TestRunEditRefusesAnUnknownLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	t.Setenv("TIDEDECK_PLUGIN_PATH", path)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"edit", "https://gone.example"}, &stdout, &stderr); code != 1 {
		t.Fatalf("editing an unknown link exited %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gone.example") {
		t.Fatalf("the error does not name the link: %q", stderr.String())
	}
}

// Help is help, not an error.
func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	if !strings.Contains(stdout.String(), "favorites render") {
		t.Fatalf("help does not list the verbs: %q", stdout.String())
	}
}

// swapOpener and swapForm let a test watch what a row's keys do without opening a
// browser and without a terminal.
func swapOpener(replacement func(string) error) func() {
	previous := runOpener
	runOpener = replacement
	return func() { runOpener = previous }
}

func swapForm(replacement func(Store, string) error) func() {
	previous := runForm
	runForm = replacement
	return func() { runForm = previous }
}

// A row's enter key hands a web link to the desktop's opener, as one argument.
func TestRunOpenHandsALinkToTheOpener(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_PATH", filepath.Join(t.TempDir(), "favorites.json"))
	var opened []string
	defer swapOpener(func(link string) error {
		opened = append(opened, link)
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", "https://go.dev"}, &stdout, &stderr); code != 0 {
		t.Fatalf("open exited %d: %s", code, stderr.String())
	}
	if len(opened) != 1 || opened[0] != "https://go.dev" {
		t.Fatalf("the opener was given %v, want the link and nothing else", opened)
	}
}

// The add row opens the form: one command serves every row, so the program
// decides, and an empty list is not a dead end.
func TestRunOpenOpensTheFormForTheAddRow(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_PATH", filepath.Join(t.TempDir(), "favorites.json"))
	var edited []string
	defer swapForm(func(store Store, id string) error {
		edited = append(edited, id)
		return nil
	})()
	opened := false
	defer swapOpener(func(link string) error {
		opened = true
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", "add"}, &stdout, &stderr); code != 0 {
		t.Fatalf("open add exited %d: %s", code, stderr.String())
	}
	if len(edited) != 1 || edited[0] != addRowID {
		t.Fatalf("open add ran the form with %v, want the add id", edited)
	}
	if opened {
		t.Fatal("open add handed something to the browser")
	}
}

// A link no browser should be handed never reaches the opener, and the reader is
// told which scheme was refused rather than that something went wrong.
func TestRunOpenRefusesALinkABrowserShouldNotOpen(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_PATH", filepath.Join(t.TempDir(), "favorites.json"))
	opened := false
	defer swapOpener(func(link string) error {
		opened = true
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", "javascript:alert(1)"}, &stdout, &stderr); code != 1 {
		t.Fatalf("opening a bookmarklet exited %d, want 1", code)
	}
	if opened {
		t.Fatal("a javascript: link reached the opener")
	}
	if !strings.Contains(stderr.String(), "javascript") {
		t.Fatalf("the refusal does not name the scheme: %q", stderr.String())
	}
}
