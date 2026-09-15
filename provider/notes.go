package provider

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allisonhere/tideui"
)

// Notes builds a notes source from Markdown or text files. A path may be a
// file or a directory; directories are scanned for *.md and *.txt.
func Notes(paths ...string) func(context.Context) ([]tideui.Note, error) {
	cloned := append([]string(nil), paths...)
	return func(context.Context) ([]tideui.Note, error) {
		var files []string
		for _, path := range cloned {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if !info.IsDir() {
				files = append(files, path)
				continue
			}
			for _, pattern := range []string{"*.md", "*.txt"} {
				matches, _ := filepath.Glob(filepath.Join(path, pattern))
				files = append(files, matches...)
			}
		}
		sort.Strings(files)

		var notes []tideui.Note
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			notes = append(notes, parseNote(filepath.Base(file), string(data)))
		}
		return notes, nil
	}
}

func parseNote(name string, content string) tideui.Note {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	note := tideui.Note{Title: strings.TrimSuffix(name, filepath.Ext(name))}
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if strings.HasPrefix(first, "#") {
			title := strings.TrimSpace(strings.TrimLeft(first, "#"))
			if strings.HasPrefix(title, "!") {
				note.Pinned = true
				title = strings.TrimSpace(strings.TrimPrefix(title, "!"))
			}
			if title != "" {
				note.Title = title
			}
			lines = lines[1:]
		}
	}
	note.Body = strings.TrimSpace(strings.Join(lines, "\n"))
	if len(note.Body) > 600 {
		note.Body = note.Body[:600]
	}
	return note
}
