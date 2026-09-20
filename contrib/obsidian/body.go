package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// previewLines caps the note body the reader view draws. The pane clips to its
// own height anyway; this only stops a very long note bloating the document.
const previewLines = 80

// BodyLines returns a note's lines for the pane to draw: YAML frontmatter is
// dropped (it is metadata, not reading matter), tabs are expanded, and the run
// is capped.
func BodyLines(body []byte, maxLines int) []string {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				lines = lines[i+1:]
				break
			}
		}
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimRight(strings.ReplaceAll(line, "\t", "    "), " \t"))
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	if maxLines > 0 && len(out) > maxLines {
		out = out[:maxLines]
	}
	return out
}

// Excerpt is the first line of a note worth showing beside its name: the first
// non-blank line that is not a heading or a bullet marker, so the list says
// something about each note without opening it.
func Excerpt(body []byte, max int) string {
	for _, line := range BodyLines(body, 200) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "```") {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "-*>0123456789. \t")
		if trimmed == "" {
			continue
		}
		return capText(trimmed, max)
	}
	return ""
}

func capText(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// sanitizeTitle turns a typed title into a file name Obsidian and the filesystem
// both accept: path separators, glob characters and Obsidian's link punctuation
// are dropped, and runs of spaces are collapsed.
func sanitizeTitle(title string) string {
	title = strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", "", "?", "", "\"", "",
		"<", "", ">", "", "|", "", "[", "", "]", "", "#", "", "^", "",
	).Replace(title)
	title = strings.Join(strings.Fields(title), " ")
	return strings.Trim(title, ". ")
}

// CreateNote makes a new empty markdown note for a title in folder (relative to
// the vault) and returns its id. A note that is already there is returned as it
// is rather than clobbered, so typing a name that exists simply opens it.
func CreateNote(vault *Vault, folder, title string) (string, error) {
	title = sanitizeTitle(title)
	if title == "" {
		return "", fmt.Errorf("a new note needs a name")
	}
	rel := filepath.ToSlash(filepath.Join(strings.TrimSpace(folder), title+".md"))
	abs, err := notePath(vault, rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err == nil {
		return rel, nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", rel, err)
	}
	if err := os.WriteFile(abs, nil, 0o644); err != nil {
		return "", fmt.Errorf("creating %s: %w", rel, err)
	}
	return rel, nil
}
