package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The editor has to draw the note without a terminal, which is also the check
// that Ripple and the theme wiring hold together.
func TestEditorRendersTheNote(t *testing.T) {
	vault := &Vault{Name: "v", Path: t.TempDir()}
	model := newEditorModel(vault, "Note.md", []byte("# Title\n\nsome body"), "plain")
	model.width, model.height = 80, 24
	model.ed.SetSize(76, 20)

	out := ansi.Strip(model.View())
	if !strings.Contains(out, "# Title") || !strings.Contains(out, "some body") {
		t.Fatalf("the editor did not draw the note:\n%s", out)
	}
	if !strings.Contains(out, "ctrl+s") {
		t.Fatalf("the status line is missing:\n%s", out)
	}
}

// ctrl+s writes the buffer back to the note, unchanged in path.
func TestSaveNoteWritesTheBuffer(t *testing.T) {
	vault := &Vault{Name: "v", Path: t.TempDir()}
	path := filepath.Join(vault.Path, "Note.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveNote(vault, "Note.md", "new\n"); err != nil {
		t.Fatalf("saveNote: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("note = %q, want the buffer", got)
	}
}
