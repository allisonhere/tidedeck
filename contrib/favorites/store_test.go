package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A list written by the form and read back by the panel is the same list, tags
// and timestamps included.
func TestStoreRoundTripsFavorites(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "favorites.json")}
	want := []Favorite{
		{
			Title: "Hacker News", URL: "https://news.ycombinator.com",
			Tags: []string{"news", "tech"}, Added: time.Date(2026, 9, 18, 19, 30, 0, 0, time.UTC),
		},
		{
			Title: "Go", URL: "https://go.dev",
			Added: time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC),
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("saving: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip returned %#v, want %#v", got, want)
	}
}

// A save leaves one file and no litter, and that file is the list: a temporary
// file left behind would be indistinguishable from a list at the wrong path.
func TestStoreSavesAtomically(t *testing.T) {
	dir := t.TempDir()
	store := Store{Path: filepath.Join(dir, "favorites.json")}
	for i := 0; i < 2; i++ {
		if err := store.Save([]Favorite{{Title: "Go", URL: "https://go.dev"}}); err != nil {
			t.Fatalf("saving: %v", err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "favorites.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("after two saves the directory holds %v, want just favorites.json", names)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("the list is mode %v, want 0600: it is nobody else's business", mode)
	}
}

// A list that cannot be parsed is reported and left exactly as it was. The one
// failure worth being paranoid about is losing a list somebody maintains by hand.
func TestStoreRefusesToReadRubbish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	broken := "{\"not\": a list"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatalf("writing the broken file: %v", err)
	}
	store := Store{Path: path}
	if _, err := store.Load(); err == nil {
		t.Fatal("a file that is not JSON loaded without complaint")
	} else if !strings.Contains(err.Error(), path) {
		t.Fatalf("the error does not name the file: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the broken file is gone: %v", err)
	}
	if string(after) != broken {
		t.Fatalf("the broken file was rewritten as %q", after)
	}
}

// Not having a list yet is the first run, not a failure.
func TestStoreLoadsAMissingFileAsNothing(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "favorites.json")}
	list, err := store.Load()
	if err != nil {
		t.Fatalf("a missing file is an error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("a missing file gave %d entries", len(list))
	}
}

// The path setting is typed the way a shell writes a path, so ~ means the home
// directory here too - and blank means the default location, not the working
// directory.
func TestStoreResolvesItsPath(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	t.Setenv("XDG_DATA_HOME", "/data")
	if got := ResolvePath("~/links.json"); got != "/home/someone/links.json" {
		t.Fatalf("ResolvePath(~/links.json) = %q", got)
	}
	if got := ResolvePath("~"); got != "/home/someone" {
		t.Fatalf("ResolvePath(~) = %q", got)
	}
	if got := ResolvePath("  /tmp/links.json "); got != "/tmp/links.json" {
		t.Fatalf("ResolvePath trims nothing: %q", got)
	}
	if got := ResolvePath(""); got != "/data/tidedeck/favorites.json" {
		t.Fatalf("a blank path should be the default location, got %q", got)
	}
}

// A list written under the old British spelling is adopted, not orphaned: the
// next run finds it, moves it to the American name, and the entries survive.
func TestStoreMigratesTheLegacyPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	legacy := legacyDefaultPath()
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`[{"title":"Go","url":"https://go.dev"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	path := ResolvePath("")
	if path != DefaultPath() {
		t.Fatalf("ResolvePath chose %q, want the new default %q", path, DefaultPath())
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("the legacy file is still there after the move: %v", err)
	}
	list, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatalf("loading the migrated list: %v", err)
	}
	if len(list) != 1 || list[0].URL != "https://go.dev" {
		t.Fatalf("the migrated list is %#v", list)
	}
}

// When both names exist the American one wins: a migration never overwrites a
// list that is already being kept under the new name.
func TestStorePrefersTheNewPathOverTheLegacyOne(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(DefaultPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DefaultPath(), []byte(`[{"title":"New","url":"https://new.example"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyDefaultPath(), []byte(`[{"title":"Old","url":"https://old.example"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ResolvePath(""); got != DefaultPath() {
		t.Fatalf("ResolvePath chose %q, want the new default", got)
	}
	list, err := (Store{Path: DefaultPath()}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "New" {
		t.Fatalf("the new list was not preferred: %#v", list)
	}
}
