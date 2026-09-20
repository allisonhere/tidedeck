package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeObsidianConfig puts a vault list where Obsidian would, so the discovery
// path is exercised rather than bypassed.
func writeObsidianConfig(t *testing.T, dir string, vaults map[string]string) {
	t.Helper()
	entries := make([]string, 0, len(vaults))
	for id, path := range vaults {
		entries = append(entries, `"`+id+`":{"path":"`+path+`","open":true}`)
	}
	body := `{"vaults":{` + strings.Join(entries, ",") + `}}`
	configDir := filepath.Join(dir, "obsidian")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "obsidian.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVaultsReadsObsidianConfig(t *testing.T) {
	config := t.TempDir()
	vault := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	writeObsidianConfig(t, config, map[string]string{"abc": vault})

	vaults, err := Vaults()
	if err != nil {
		t.Fatalf("Vaults: %v", err)
	}
	if len(vaults) != 1 || vaults[0].Path != vault || vaults[0].Name != filepath.Base(vault) {
		t.Fatalf("vaults = %+v", vaults)
	}
	if !vaults[0].Open {
		t.Fatal("the open flag was lost")
	}
}

func TestVaultsWithoutConfigIsNotAnError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vaults, err := Vaults()
	if err != nil {
		t.Fatalf("a missing Obsidian config should be empty, not an error: %v", err)
	}
	if len(vaults) != 0 {
		t.Fatalf("vaults = %+v, want none", vaults)
	}
}

func TestResolveVaultByNamePathAndDefault(t *testing.T) {
	config := t.TempDir()
	vault := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	writeObsidianConfig(t, config, map[string]string{"abc": vault})

	byName, err := ResolveVault(filepath.Base(vault))
	if err != nil || byName.Path != vault {
		t.Fatalf("by name: %+v, %v", byName, err)
	}
	byPath, err := ResolveVault(vault)
	if err != nil || byPath.Path != vault {
		t.Fatalf("by path: %+v, %v", byPath, err)
	}
	blank, err := ResolveVault("")
	if err != nil || blank.Path != vault {
		t.Fatalf("blank: %+v, %v", blank, err)
	}
	if _, err := ResolveVault("nope"); err == nil {
		t.Fatal("an unknown vault name was accepted")
	}
}

func TestResolveVaultRejectsAnEmptyList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := ResolveVault(""); err == nil {
		t.Fatal("no vaults should be an error the pane explains")
	}
}

func TestScanNotesSkipsObsidiansOwnFolders(t *testing.T) {
	vault := t.TempDir()
	for _, rel := range []string{"Welcome.md", "Projects/Idea.md", "Servers/host.md"} {
		path := filepath.Join(vault, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# "+rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Obsidian's cache and trash hold markdown that is not the reader's notes.
	for _, rel := range []string{".obsidian/workspace.md", ".trash/Old.md"} {
		path := filepath.Join(vault, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(vault, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	notes, err := ScanNotes(vault)
	if err != nil {
		t.Fatalf("ScanNotes: %v", err)
	}
	if len(notes) != 3 {
		rels := make([]string, len(notes))
		for i, note := range notes {
			rels[i] = note.Rel
		}
		t.Fatalf("notes = %v, want the three real ones", rels)
	}
}

func TestFuzzyScoreAndRanking(t *testing.T) {
	if _, ok := FuzzyScore("tftp", "TideFTP"); !ok {
		t.Fatal("tftp should match TideFTP")
	}
	if _, ok := FuzzyScore("zzz", "TideFTP"); ok {
		t.Fatal("zzz should not match TideFTP")
	}
	if _, ok := FuzzyScore("", "anything"); !ok {
		t.Fatal("an empty pattern matches everything")
	}

	now := time.Now()
	notes := []Note{
		{Rel: "Misc/Untitled.md", Title: "Untitled", Mod: now},
		{Rel: "TideFTP.md", Title: "TideFTP", Mod: now.Add(-time.Hour)},
		{Rel: "Projects/Tide.md", Title: "Tide", Mod: now.Add(-2 * time.Hour)},
	}
	ranked := RankNotes(notes, "tftp")
	if len(ranked) == 0 || ranked[0].Title != "TideFTP" {
		t.Fatalf("ranked = %+v, want TideFTP first", ranked)
	}
	// An empty query is the recent notes.
	recent := RankNotes(notes, "")
	if len(recent) != 3 || recent[0].Title != "Untitled" {
		t.Fatalf("recent = %+v, want the newest first", recent)
	}
}

func TestNotePathRefusesToEscapeTheVault(t *testing.T) {
	vault := &Vault{Name: "v", Path: t.TempDir()}
	for _, bad := range []string{"../secret.md", "/etc/passwd.md", "note.txt", "sub/../../x.md", ""} {
		if _, err := notePath(vault, bad); err == nil {
			t.Errorf("notePath(%q) was accepted", bad)
		}
	}
	good, err := notePath(vault, "Projects/Idea.md")
	if err != nil {
		t.Fatalf("a note in the vault was refused: %v", err)
	}
	if good != filepath.Join(vault.Path, "Projects", "Idea.md") {
		t.Fatalf("notePath = %q", good)
	}
}

func TestBodyLinesDropsFrontmatterAndCaps(t *testing.T) {
	body := []byte("---\ntags: [x]\n---\n\n# Title\n\nfirst line\nsecond\nthird\n")
	lines := BodyLines(body, 3)
	if len(lines) != 3 {
		t.Fatalf("lines = %q, want 3", lines)
	}
	if lines[0] != "# Title" || lines[1] != "" || lines[2] != "first line" {
		t.Fatalf("lines = %q, want the frontmatter dropped and the body kept", lines)
	}
}

func TestExcerptSkipsHeadings(t *testing.T) {
	body := []byte("# Title\n\n## Section\n\nthe real first line\nmore\n")
	if got := Excerpt(body, 40); got != "the real first line" {
		t.Fatalf("excerpt = %q", got)
	}
	if got := Excerpt([]byte("# Only A Title\n"), 40); got != "" {
		t.Fatalf("excerpt = %q, want empty when there is only a heading", got)
	}
}

func TestCreateNoteSanitizesAndDoesNotClobber(t *testing.T) {
	vault := &Vault{Name: "v", Path: t.TempDir()}
	rel, err := CreateNote(vault, "Inbox", "My/Idea: notes?")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if rel != "Inbox/My Idea notes.md" {
		t.Fatalf("rel = %q", rel)
	}
	if _, err := os.Stat(filepath.Join(vault.Path, "Inbox", "My Idea notes.md")); err != nil {
		t.Fatalf("the note was not created: %v", err)
	}
	// Typing a name that already exists opens it rather than overwriting it.
	if err := os.WriteFile(filepath.Join(vault.Path, "Inbox", "My Idea notes.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := CreateNote(vault, "Inbox", "My Idea notes")
	if err != nil || again != rel {
		t.Fatalf("CreateNote again = %q, %v", again, err)
	}
	body, _ := os.ReadFile(filepath.Join(vault.Path, "Inbox", "My Idea notes.md"))
	if string(body) != "keep me" {
		t.Fatalf("an existing note was clobbered: %q", body)
	}
	if _, err := CreateNote(vault, "", "..."); err == nil {
		t.Fatal("a note with no usable name was created")
	}
}

func TestReadWriteNoteRoundTripsAndLeavesNoLitter(t *testing.T) {
	vault := &Vault{Name: "v", Path: t.TempDir()}
	if err := os.WriteFile(filepath.Join(vault.Path, "Note.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteNote(vault, "Note.md", []byte("new\nbody\n")); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}
	got, err := ReadNote(vault, "Note.md")
	if err != nil {
		t.Fatalf("ReadNote: %v", err)
	}
	if string(got) != "new\nbody\n" {
		t.Fatalf("body = %q", got)
	}
	entries, err := os.ReadDir(vault.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Note.md" {
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		t.Fatalf("after a save the vault holds %v, want one note", names)
	}
}
