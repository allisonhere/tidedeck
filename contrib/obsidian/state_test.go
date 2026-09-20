package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTripsPerVault(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := SaveState("/vault/a", "Projects/Tide.md"); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got := LoadState("/vault/a")
	if got.Vault != "/vault/a" || got.Note != "Projects/Tide.md" {
		t.Fatalf("state = %+v", got)
	}
	// A note remembered for one vault is not another vault's memory.
	if other := LoadState("/vault/b"); other.Note != "" {
		t.Fatalf("state leaked across vaults: %+v", other)
	}
}

func TestLoadStateWithoutAFileIsEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := LoadState("/vault"); got.Note != "" {
		t.Fatalf("state = %+v, want empty", got)
	}
}

func TestSaveStateIsPrivate(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := SaveState("/vault", "Note.md"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(os.Getenv("XDG_DATA_HOME"), "tidedeck", "obsidian.json"))
	if err != nil {
		t.Fatalf("the state file is not there: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("state mode = %v, want 0600", mode)
	}
}
