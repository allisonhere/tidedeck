package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempVault creates a vault with the given notes and points Obsidian's config
// and the plugin settings at it, so run() is exercised end to end.
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
	writeObsidianConfig(t, config, map[string]string{"v": vault})
	t.Setenv("TIDEDECK_PLUGIN_VAULT", "")
	t.Setenv("TIDEDECK_PLUGIN_QUERY", "")
	t.Setenv("TIDEDECK_PLUGIN_MODE", "")
	return vault
}

type dashDoc struct {
	SchemaVersion int `json:"schemaVersion"`
	Rows          []struct {
		Type  string `json:"type"`
		Label string `json:"label"`
		Value string `json:"value"`
		ID    string `json:"id"`
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

// With no query the pane is just the search row, and the vault is offered to
// the settings screen.
func TestRunRenderIsCompactWithoutAQuery(t *testing.T) {
	tempVault(t, map[string]string{"Welcome.md": "# Welcome\n", "Projects/Idea.md": "# Idea\n"})
	doc := renderInto(t)
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if len(doc.Rows) != 1 || doc.Rows[0].Label != "search" {
		t.Fatalf("rows = %#v, want the one search row", doc.Rows)
	}
	if len(doc.Options["vault"]) == 0 {
		t.Fatal("render offered no vaults for the settings screen")
	}
}

func TestRunRenderFiltersByQuery(t *testing.T) {
	tempVault(t, map[string]string{"TideFTP.md": "the ftp app\n", "Welcome.md": "hi\n"})
	t.Setenv("TIDEDECK_PLUGIN_QUERY", "tftp")
	doc := renderInto(t)
	found := false
	for _, row := range doc.Rows {
		if row.ID == "TideFTP.md" {
			found = true
		}
		if row.ID == "Welcome.md" {
			t.Fatalf("welcome should not match tftp: %#v", doc.Rows)
		}
	}
	if !found {
		t.Fatalf("query did not surface TideFTP: %#v", doc.Rows)
	}
}

func TestRunEditReachesRipple(t *testing.T) {
	tempVault(t, map[string]string{"Note.md": "old body\n"})
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
	for _, args := range [][]string{nil, {"edit"}, {"frobnicate"}} {
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
