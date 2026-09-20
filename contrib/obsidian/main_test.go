package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempVault creates a vault with the given notes and points Obsidian's config,
// the plugin settings and the plugin's own state at throwaway directories, so
// run() is exercised end to end.
func tempVault(t *testing.T, notes map[string]string) string {
	t.Helper()
	vault := t.TempDir()
	for rel, body := range notes {
		path := filepath.Join(vault, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeObsidianConfig(t, config, map[string]string{"v": vault})
	t.Setenv("TIDEDECK_PLUGIN_VAULT", "")
	t.Setenv("TIDEDECK_PLUGIN_QUERY", "")
	t.Setenv("TIDEDECK_PLUGIN_MODE", "")
	t.Setenv("TIDEDECK_PLUGIN_FOLDER", "")
	return vault
}

type dashDoc struct {
	SchemaVersion int `json:"schemaVersion"`
	Rows          []struct {
		Type  string   `json:"type"`
		Label string   `json:"label"`
		Value string   `json:"value"`
		ID    string   `json:"id"`
		Body  []string `json:"body"`
	} `json:"rows"`
	Options map[string][]string `json:"options"`
}

func renderInto(t *testing.T) dashDoc {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"render"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render exited %d: %s", code, stderr.String())
	}
	var doc dashDoc
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("render printed something that is not a document: %v", err)
	}
	return doc
}

func hasID(doc dashDoc, id string) bool {
	for _, row := range doc.Rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

// Without a query the pane reads the note the reader last loaded.
func TestRunRenderReadsTheCurrentNote(t *testing.T) {
	vault := tempVault(t, map[string]string{
		"Welcome.md":       "hello\n",
		"Projects/Idea.md": "# Idea\n\na thought\n",
	})
	if err := SaveState(vault, "Projects/Idea.md"); err != nil {
		t.Fatal(err)
	}
	doc := renderInto(t)
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if !hasID(doc, "Projects/Idea.md") {
		t.Fatalf("the current note was not drawn: %#v", doc.Rows)
	}
	body := false
	for _, row := range doc.Rows {
		if row.Type == "block" && len(row.Body) > 0 {
			body = true
		}
	}
	if !body {
		t.Fatalf("the note body was not drawn: %#v", doc.Rows)
	}
	if len(doc.Options["vault"]) == 0 {
		t.Fatal("render offered no vaults for the settings screen")
	}
}

// A query lists the matches and always ends with the new-note row.
func TestRunRenderSearchListsMatches(t *testing.T) {
	tempVault(t, map[string]string{"TideFTP.md": "the ftp app\n", "Welcome.md": "hi\n"})
	t.Setenv("TIDEDECK_PLUGIN_QUERY", "tftp")
	doc := renderInto(t)
	if !hasID(doc, "TideFTP.md") {
		t.Fatalf("query did not surface TideFTP: %#v", doc.Rows)
	}
	if hasID(doc, "Welcome.md") {
		t.Fatalf("welcome should not match tftp: %#v", doc.Rows)
	}
	if doc.Rows[1].ID != newNoteID {
		t.Fatalf("row 1 = %#v, want the new-note row", doc.Rows[1])
	}
}

// Enter loads a note by remembering it, and the next render shows it.
func TestRunOpenLoadsTheNote(t *testing.T) {
	vault := tempVault(t, map[string]string{"Note.md": "body\n"})
	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", "Note.md"}, &stdout, &stderr); code != 0 {
		t.Fatalf("open exited %d: %s", code, stderr.String())
	}
	if got := LoadState(vault); got.Note != "Note.md" {
		t.Fatalf("state = %+v, want Note.md", got)
	}
	if !hasID(renderInto(t), "Note.md") {
		t.Fatal("the loaded note was not shown by the next render")
	}
}

// e edits the picked note in Ripple and remembers it.
func TestRunEditReachesRipple(t *testing.T) {
	vault := tempVault(t, map[string]string{"Note.md": "old body\n"})
	t.Setenv("TIDEDECK_PLUGIN_MODE", "vim")
	var gotRel, gotMode, gotBody string
	defer swapEditor(func(_ *Vault, rel string, body []byte, mode string) error {
		gotRel, gotBody, gotMode = rel, string(body), mode
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"edit", "Note.md"}, &stdout, &stderr); code != 0 {
		t.Fatalf("edit exited %d: %s", code, stderr.String())
	}
	if gotRel != "Note.md" || gotBody != "old body\n" || gotMode != "vim" {
		t.Fatalf("editor got rel=%q body=%q mode=%q", gotRel, gotBody, gotMode)
	}
	if got := LoadState(vault); got.Note != "Note.md" {
		t.Fatalf("state = %+v, want the edited note remembered", got)
	}
}

// "new" makes a note named by the query and opens it for writing.
func TestRunNewCreatesAndEdits(t *testing.T) {
	vault := tempVault(t, map[string]string{"Welcome.md": "hi\n"})
	t.Setenv("TIDEDECK_PLUGIN_QUERY", "Fresh Idea")
	t.Setenv("TIDEDECK_PLUGIN_FOLDER", "Inbox")
	var gotRel string
	defer swapEditor(func(_ *Vault, rel string, _ []byte, _ string) error {
		gotRel = rel
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", newNoteID}, &stdout, &stderr); code != 0 {
		t.Fatalf("open new exited %d: %s", code, stderr.String())
	}
	if gotRel != "Inbox/Fresh Idea.md" {
		t.Fatalf("editor got %q, want Inbox/Fresh Idea.md", gotRel)
	}
	if _, err := os.Stat(filepath.Join(vault, "Inbox", "Fresh Idea.md")); err != nil {
		t.Fatalf("the note was not created: %v", err)
	}
}

func TestRunVaultsAndPath(t *testing.T) {
	vault := tempVault(t, map[string]string{"Note.md": "x\n"})
	var stdout, stderr bytes.Buffer
	if code := run([]string{"path"}, &stdout, &stderr); code != 0 {
		t.Fatalf("path exited %d: %s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != vault {
		t.Fatalf("path = %q, want %q", stdout.String(), vault)
	}
	stdout.Reset()
	if code := run([]string{"vaults"}, &stdout, &stderr); code != 0 {
		t.Fatalf("vaults exited %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), filepath.Base(vault)) {
		t.Fatalf("vaults printed %q", stdout.String())
	}
}

func TestRunRefusesMisuse(t *testing.T) {
	tempVault(t, map[string]string{"Note.md": "x\n"})
	for _, args := range [][]string{nil, {"open"}, {"edit"}, {"frobnicate"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 {
			t.Fatalf("%v exited 0", args)
		}
		if !strings.Contains(stderr.String(), "obsidian") {
			t.Fatalf("%v said nothing useful: %q", args, stderr.String())
		}
	}
}

// swapEditor lets a test watch what the edit verb does without a terminal.
func swapEditor(replacement func(*Vault, string, []byte, string) error) func() {
	previous := runEditor
	runEditor = replacement
	return func() { runEditor = previous }
}
