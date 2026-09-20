package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// State is the plugin's own small memory: which note the reader last loaded, so
// the pane can keep showing it after the program that loaded it has exited.
type State struct {
	Vault string `json:"vault"`
	Note  string `json:"note"`
}

func statePath() string {
	base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "tidedeck", "obsidian.json")
}

// LoadState reads the last-loaded note for the vault given. A missing or
// unreadable file is simply no memory, not an error; a note remembered for a
// different vault is ignored.
func LoadState(vault string) State {
	path := statePath()
	if path == "" {
		return State{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}
	if state.Vault != vault {
		return State{}
	}
	return state
}

// SaveState remembers the note the reader loaded. A failure is returned so the
// open verb can say so, but it is not fatal to the pane.
func SaveState(vault, note string) error {
	path := statePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(State{Vault: vault, Note: note})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
