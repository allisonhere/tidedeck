package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// previewLines caps the note body the reader view draws. The pane clips to its
// own height anyway; this only stops a very long note bloating the document.
const previewLines = 80

var (
	reImage  = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	reLink   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reWiki   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	reBold   = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__`)
	reStrike = regexp.MustCompile(`~~([^~\n]+)~~`)
	reCode   = regexp.MustCompile("`([^`\n]*)`")
	reTask   = regexp.MustCompile(`^(\s*)[-*+] \[([ xX])\] (.*)$`)
	reBullet = regexp.MustCompile(`^(\s*)[-*+] (.*)$`)
)

// BodyLines returns a note's raw lines with YAML frontmatter dropped (it is
// metadata, not reading matter), tabs expanded, and the run capped. It is the
// base ReadableLines strips markdown from.
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

// ReadableLines turns a note into lines fit for a pane: markdown the terminal
// cannot render is stripped, because a reader should see the note, not its
// source. Headings lose their hashes, emphasis its markers, links their URLs,
// bullets become dots, and fenced code keeps its indentation.
func ReadableLines(body []byte, maxLines int) []string {
	var out []string
	inFence := false
	for _, line := range BodyLines(body, 0) {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			inFence = !inFence
			continue
		case inFence:
			out = append(out, "    "+strings.TrimRight(line, " \t"))
			continue
		case trimmed == "":
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		case isHorizontalRule(trimmed):
			continue
		}
		out = append(out, readableLine(line))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if maxLines > 0 && len(out) > maxLines {
		out = out[:maxLines]
	}
	return out
}

func readableLine(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case reTask.MatchString(line):
		m := reTask.FindStringSubmatch(line)
		box := "☐"
		if strings.EqualFold(m[2], "x") {
			box = "☑"
		}
		return m[1] + box + " " + stripInline(m[3])
	case strings.HasPrefix(trimmed, "#"):
		return stripInline(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
	case strings.HasPrefix(trimmed, ">"):
		return "│ " + stripInline(strings.TrimSpace(strings.TrimLeft(trimmed, ">")))
	case reBullet.MatchString(line):
		m := reBullet.FindStringSubmatch(line)
		return m[1] + "• " + stripInline(m[2])
	default:
		return stripInline(line)
	}
}

// stripInline removes the markdown that decorates a run of text: image and link
// syntax, wikilinks, emphasis and inline code. Emphasis is only stripped for
// the doubled markers, so an identifier like some_snake_case survives.
func stripInline(s string) string {
	s = reImage.ReplaceAllString(s, "$1")
	s = reLink.ReplaceAllString(s, "$1")
	s = reWiki.ReplaceAllStringFunc(s, func(match string) string {
		parts := reWiki.FindStringSubmatch(match)
		if len(parts) == 3 && parts[2] != "" {
			return parts[2]
		}
		return parts[1]
	})
	s = reCode.ReplaceAllString(s, "$1")
	s = reBold.ReplaceAllString(s, "$1$2")
	s = reStrike.ReplaceAllString(s, "$1")
	return strings.TrimRight(s, " \t")
}

func isHorizontalRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	marker := rune(trimmed[0])
	if marker != '-' && marker != '*' && marker != '_' {
		return false
	}
	for _, r := range trimmed {
		if r != marker && r != ' ' {
			return false
		}
	}
	return true
}

// Excerpt is the first readable line of a note that is not the note's own
// title, for the search list: enough to tell two similar names apart.
func Excerpt(body []byte, title string, max int) string {
	title = strings.ToLower(strings.TrimSpace(title))
	for _, line := range ReadableLines(body, 200) {
		line = strings.TrimSpace(line)
		if line == "" || strings.ToLower(line) == title {
			continue
		}
		return capText(line, max)
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
