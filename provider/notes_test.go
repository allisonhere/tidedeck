package provider

import (
	"strings"
	"testing"
)

func TestParseNote(t *testing.T) {
	note := parseNote("remember.md", "# Remember\n- test narrow layouts\n- record demo GIF")
	if note.Title != "Remember" {
		t.Fatalf("title = %q", note.Title)
	}
	if !strings.Contains(note.Body, "test narrow layouts") {
		t.Fatalf("body = %q", note.Body)
	}
	if note.Pinned {
		t.Fatal("note unexpectedly pinned")
	}

	pinned := parseNote("important.md", "# !Important\nbody")
	if !pinned.Pinned || pinned.Title != "Important" {
		t.Fatalf("pinned note = %+v", pinned)
	}

	plain := parseNote("plain.txt", "just text")
	if plain.Title != "plain" || plain.Body != "just text" {
		t.Fatalf("plain note = %+v", plain)
	}
}
