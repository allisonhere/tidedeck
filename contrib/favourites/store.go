package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Favourite is one link the reader keeps. The list is small, hand-readable and
// hand-editable on purpose: this file is the whole state, so there is nothing to
// rebuild if it is copied to another machine.
type Favourite struct {
	Title string    `json:"title"`
	URL   string    `json:"url"`
	Tags  []string  `json:"tags,omitempty"`
	Added time.Time `json:"added"`
}

// Store is the file the list lives in. There is no index and no cache: the file
// is the list, so one somebody edited by hand is as valid as one the form wrote.
type Store struct {
	Path string
}

// DefaultPath is where the list lives when nothing says otherwise: data, not
// config, in the XDG data directory the rest of the desktop uses.
func DefaultPath() string {
	base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "tidedeck", "favourites.json")
}

// ResolvePath turns the configured path into a real one. Blank means the default
// location - not the working directory, which is what an empty string names.
func ResolvePath(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return DefaultPath()
	}
	return ExpandHome(configured)
}

// ExpandHome resolves a leading ~ or ~/ against the home directory. A path typed
// into a settings field is written the way a shell writes one, and it arrives
// here as an environment variable, where ~ means itself.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[len("~/"):])
}

// Load reads the list. A file that is not there is an empty list and no error:
// not having a favourite yet is the normal first run, not a failure.
func (s Store) Load() ([]Favourite, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.Path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var list []Favourite
	if err := json.Unmarshal(data, &list); err != nil {
		// The file is left exactly as it was. A list somebody maintains by hand
		// is not worth losing to a stray comma.
		return nil, fmt.Errorf("reading %s: %w", s.Path, err)
	}
	return list, nil
}

// Save writes the list. It goes to a temporary file in the same directory and is
// renamed over the old one, so a crash half way through leaves the previous list
// intact instead of half of a new one - and so a reader never sees a partial file.
func (s Store) Save(list []Favourite) error {
	if list == nil {
		// An empty list is written as [], not as null: the file is read by hand
		// often enough to be worth being explicit about.
		list = []Favourite{}
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("preparing %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the list: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(dir, ".favourites-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.Path, err)
	}
	name := temp.Name()
	defer os.Remove(name) // a no-op once the rename below has happened
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("writing %s: %w", s.Path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", s.Path, err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.Path, err)
	}
	if err := os.Rename(name, s.Path); err != nil {
		return fmt.Errorf("writing %s: %w", s.Path, err)
	}
	return nil
}
