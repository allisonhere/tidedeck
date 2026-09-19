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
func TestStoreRoundTripsFavourites(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "favourites.json")}
	want := []Favourite{
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
	store := Store{Path: filepath.Join(dir, "favourites.json")}
	for i := 0; i < 2; i++ {
		if err := store.Save([]Favourite{{Title: "Go", URL: "https://go.dev"}}); err != nil {
			t.Fatalf("saving: %v", err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "favourites.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("after two saves the directory holds %v, want just favourites.json", names)
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
	path := filepath.Join(t.TempDir(), "favourites.json")
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
	store := Store{Path: filepath.Join(t.TempDir(), "favourites.json")}
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
	if got := ResolvePath(""); got != "/data/tidedeck/favourites.json" {
		t.Fatalf("a blank path should be the default location, got %q", got)
	}
}
