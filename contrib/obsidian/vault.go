package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Vault is one Obsidian vault: the name Obsidian shows for it, and the folder
// it lives in.
type Vault struct {
	Name string
	Path string
	Open bool
}

// obsidianConfigPath is where Obsidian records the vaults it knows about.
func obsidianConfigPath() string {
	base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "obsidian", "obsidian.json")
}

// Vaults reads Obsidian's own vault list. It is the single source of truth for
// where a vault is, so the plugin never asks the reader to type a path Obsidian
// already knows.
func Vaults() ([]Vault, error) {
	path := obsidianConfigPath()
	if path == "" {
		return nil, fmt.Errorf("no home directory to find Obsidian's config in")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc struct {
		Vaults map[string]struct {
			Path string `json:"path"`
			Open bool   `json:"open"`
		} `json:"vaults"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	vaults := make([]Vault, 0, len(doc.Vaults))
	for _, entry := range doc.Vaults {
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		vaults = append(vaults, Vault{Name: filepath.Base(entry.Path), Path: entry.Path, Open: entry.Open})
	}
	sort.SliceStable(vaults, func(i, j int) bool {
		if vaults[i].Open != vaults[j].Open {
			return vaults[i].Open
		}
		return vaults[i].Name < vaults[j].Name
	})
	return vaults, nil
}

// ResolveVault turns the vault setting into the vault it names. Blank means the
// one Obsidian has open, then the first it knows about; a value may be a vault
// name or a path.
func ResolveVault(configured string) (*Vault, error) {
	vaults, err := Vaults()
	if err != nil {
		return nil, err
	}
	configured = strings.TrimSpace(configured)
	if configured == "" {
		if len(vaults) == 0 {
			return nil, fmt.Errorf("Obsidian has no vaults: open one, or set the vault path")
		}
		return &vaults[0], nil
	}
	want := ExpandHome(configured)
	for i := range vaults {
		if vaults[i].Name == configured || vaults[i].Path == want {
			return &vaults[i], nil
		}
	}
	// A folder Obsidian has not been told about is still a vault if it is one.
	if info, err := os.Stat(want); err == nil && info.IsDir() {
		return &Vault{Name: filepath.Base(want), Path: want}, nil
	}
	return nil, fmt.Errorf("no vault called %q (Obsidian knows %s)", configured, vaultNames(vaults))
}

func vaultNames(vaults []Vault) string {
	if len(vaults) == 0 {
		return "none"
	}
	names := make([]string, len(vaults))
	for i, vault := range vaults {
		names[i] = vault.Name
	}
	return strings.Join(names, ", ")
}

// ExpandHome resolves a leading ~ or ~/ against the home directory. A path
// typed into a settings field arrives here as an environment variable, where ~
// means itself.
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

// Note is one markdown file in a vault. The body is read on demand, not held
// here: the pane only ever previews one note.
type Note struct {
	Path  string
	Rel   string
	Title string
	Mod   time.Time
}

// ScanNotes walks the vault for markdown. Obsidian's own folders (and any other
// dot-directory) are skipped, and a folder that cannot be read is passed over
// rather than failing the whole scan.
func ScanNotes(root string) ([]Note, error) {
	var notes []Note
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		note := Note{
			Path:  path,
			Rel:   filepath.ToSlash(rel),
			Title: strings.TrimSuffix(name, filepath.Ext(name)),
		}
		if info, err := entry.Info(); err == nil {
			note.Mod = info.ModTime()
		}
		notes = append(notes, note)
		return nil
	})
	return notes, err
}

// notePath resolves a note id (a vault-relative path) to an absolute path,
// refusing anything that is not markdown or would escape the vault.
func notePath(vault *Vault, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("no note id")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("note %q is outside the vault", rel)
	}
	if !strings.EqualFold(filepath.Ext(clean), ".md") {
		return "", fmt.Errorf("note %q is not markdown", rel)
	}
	abs := filepath.Join(vault.Path, clean)
	inside, err := filepath.Rel(vault.Path, abs)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("note %q is outside the vault", rel)
	}
	return abs, nil
}

// ReadNote reads one note by its vault-relative id.
func ReadNote(vault *Vault, rel string) ([]byte, error) {
	abs, err := notePath(vault, rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", rel, err)
	}
	return data, nil
}

// WriteNote writes a note back to the file it came from, atomically: a
// temporary file in the same folder, renamed over the original. A crash half
// way through leaves the note the reader was editing intact.
func WriteNote(vault *Vault, rel string, body []byte) error {
	abs, err := notePath(vault, rel)
	if err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(abs); err == nil {
		mode = info.Mode().Perm()
	}
	temp, err := os.CreateTemp(dir, ".obsidian-*.md")
	if err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	name := temp.Name()
	defer os.Remove(name) // a no-op once the rename below has happened
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	if err := os.Rename(name, abs); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	return nil
}

// PreviewLines returns at most max lines of a note, tabs expanded and trailing
// whitespace trimmed, for the pane's preview of the best match.
func PreviewLines(body []byte, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	raw := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	lines := make([]string, 0, min(len(raw), maxLines))
	for _, line := range raw[:min(len(raw), maxLines)] {
		lines = append(lines, strings.TrimRight(strings.ReplaceAll(line, "\t", "    "), " \t"))
	}
	return lines
}

// FuzzyScore reports whether pattern is a case-insensitive subsequence of
// target, and how good the match is. Consecutive runs and matches at a word
// boundary score higher than a scatter of letters, so "tftp" picks
// "TideFTP.md" over an incidental set of letters in a long path.
func FuzzyScore(pattern, target string) (int, bool) {
	if pattern == "" {
		return 0, true
	}
	needle := []rune(strings.ToLower(pattern))
	haystack := []rune(strings.ToLower(target))
	score, at, previous := 0, 0, -2
	for index, r := range haystack {
		if at >= len(needle) {
			break
		}
		if r != needle[at] {
			continue
		}
		score++
		if index == previous+1 {
			score += 4 // a consecutive run
		}
		if index == 0 || isWordBreak(haystack[index-1]) {
			score += 3 // the start of a word
		}
		previous = index
		at++
	}
	if at < len(needle) {
		return 0, false
	}
	// A tighter target beats a sprawling one, and an early match beats a late.
	score += max(0, 12-len(haystack)/8)
	return score, true
}

func isWordBreak(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// RankNotes filters and orders notes for a query: fuzzy score first, then the
// most recently edited, so an empty query is simply the recent notes.
func RankNotes(notes []Note, query string) []Note {
	ranked := append([]Note(nil), notes...)
	query = strings.TrimSpace(query)
	if query != "" {
		type scored struct {
			note  Note
			score int
		}
		var matches []scored
		for _, note := range notes {
			best := -1
			if score, ok := FuzzyScore(query, note.Title); ok {
				best = score + 8 // a title match is worth more than a path one
			}
			if score, ok := FuzzyScore(query, note.Rel); ok && score > best {
				best = score
			}
			if best >= 0 {
				matches = append(matches, scored{note, best})
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].score != matches[j].score {
				return matches[i].score > matches[j].score
			}
			return noteLess(matches[i].note, matches[j].note)
		})
		ranked = make([]Note, len(matches))
		for i, match := range matches {
			ranked[i] = match.note
		}
		return ranked
	}
	sort.SliceStable(ranked, func(i, j int) bool { return noteLess(ranked[i], ranked[j]) })
	return ranked
}

func noteLess(a, b Note) bool {
	if !a.Mod.Equal(b.Mod) {
		return a.Mod.After(b.Mod)
	}
	return a.Rel < b.Rel
}
