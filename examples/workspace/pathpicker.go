package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

// A path setting - a todo.txt, a notes folder, a repository - can be typed, but
// a path is exactly what nobody remembers letter for letter. o on such a row
// opens a picker in its place: the folder the value points into, filtered by
// what is typed, with Tab completing and Enter opening a folder or choosing.
//
// A list setting (several repositories, several calendars) gains the chosen
// path rather than losing what it had.

// pickerLimit bounds what one folder lists, so a huge directory stays quick.
const pickerLimit = 400

// pathPicker is the picker's state while it is open.
type pathPicker struct {
	field  string // the row's label, which is how the form finds rows
	kind   string // dash.PathFile or dash.PathDir
	list   bool
	query  string
	cursor int
	// entries is what the query lists: folders first, with a trailing slash.
	entries []string
	// useFolder offers the folder being browsed itself as the choice, for a
	// setting that names a folder.
	useFolder bool
	// touched is set by the first key. Until then the query is only where
	// the picker happened to open, so typing ~ or / starts a path afresh
	// rather than being added to it.
	touched bool
}

// home is the user's home directory, or "" when it cannot be found.
func home() string {
	dir, _ := os.UserHomeDir()
	return dir
}

// expand turns a leading ~ into the home directory.
func expand(p string) string {
	if h := home(); h != "" && (p == "~" || strings.HasPrefix(p, "~/")) {
		return h + p[1:]
	}
	return p
}

// openPicker starts the picker on the current row, at the folder its value
// points into.
func (s *settingsForm) openPicker() bool {
	field := s.currentField()
	if field == nil || field.path == "" || field.text == nil {
		return false
	}
	start := strings.TrimSpace(*field.text)
	if field.list {
		parts := strings.Split(start, ",")
		start = strings.TrimSpace(parts[len(parts)-1])
	}
	switch {
	case start == "" || strings.Contains(start, "://"):
		start = "~/"
	default:
		if info, err := os.Stat(expand(start)); err == nil && info.IsDir() {
			start = strings.TrimSuffix(start, "/") + "/"
		} else {
			start = strings.TrimSuffix(filepath.Dir(start), "/") + "/"
		}
	}
	s.picker = &pathPicker{field: field.label, kind: field.path, list: field.list, query: start}
	s.picker.refresh()
	return true
}

// refresh lists the folder the query points into, filtered by what follows
// its last slash.
func (p *pathPicker) refresh() {
	dir, prefix := filepath.Split(expand(p.query))
	if dir == "" {
		dir = "."
	}
	p.entries = p.entries[:0]
	p.useFolder = p.kind == dash.PathDir && prefix == ""
	read, err := os.ReadDir(dir)
	if err == nil {
		var dirs, files []string
		for _, e := range read {
			name := e.Name()
			if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
				continue
			}
			isDir := e.IsDir()
			if e.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
					isDir = info.IsDir()
				}
			}
			if isDir {
				dirs = append(dirs, name+"/")
			} else if p.kind != dash.PathDir {
				files = append(files, name)
			}
		}
		sort.Strings(dirs)
		sort.Strings(files)
		p.entries = append(append(p.entries, dirs...), files...)
		if len(p.entries) > pickerLimit {
			p.entries = p.entries[:pickerLimit]
		}
	}
	p.cursor = min(p.cursor, max(0, p.rows()-1))
}

// rows counts what is listed, including the use-this-folder row.
func (p *pathPicker) rows() int {
	if p.useFolder {
		return len(p.entries) + 1
	}
	return len(p.entries)
}

// at is the path the row under the cursor stands for, and whether it is a
// folder to open rather than a choice.
func (p *pathPicker) at(i int) (string, bool) {
	dir, _ := filepath.Split(p.query)
	if p.useFolder {
		if i == 0 {
			return strings.TrimSuffix(dir, "/"), false
		}
		i--
	}
	if i < 0 || i >= len(p.entries) {
		return "", false
	}
	name := p.entries[i]
	return dir + name, strings.HasSuffix(name, "/")
}

// updatePicker handles a key while the picker is open.
func (s *settingsForm) updatePicker(key string, runes []rune) {
	p := s.picker
	fresh := !p.touched
	p.touched = true
	switch key {
	case "esc":
		s.picker = nil
	case "up", "ctrl+k":
		p.cursor = max(0, p.cursor-1)
	case "down", "ctrl+j":
		p.cursor = min(max(0, p.rows()-1), p.cursor+1)
	case "tab":
		// Complete to the row under the cursor without choosing it.
		if path, _ := p.at(p.cursor); path != "" {
			if p.useFolder && p.cursor == 0 {
				return
			}
			p.query, p.cursor = path, 0
			p.refresh()
		}
	case "enter":
		path, folder := p.at(p.cursor)
		switch {
		case path == "":
			// Nothing listed: the typed path itself may be what was meant.
			if typed := strings.TrimSpace(p.query); typed != "" && !strings.HasSuffix(typed, "/") {
				s.choosePath(typed)
			}
		case folder:
			p.query, p.cursor = path, 0
			p.refresh()
		default:
			s.choosePath(path)
		}
	case "backspace":
		if p.query != "" {
			r := []rune(p.query)
			p.query = string(r[:len(r)-1])
			p.cursor = 0
			p.refresh()
		}
	default:
		if len(runes) > 0 {
			if fresh && (runes[0] == '~' || runes[0] == '/') {
				p.query = ""
			}
			p.query += string(runes)
			p.cursor = 0
			p.refresh()
		}
	}
}

// choosePath writes the chosen path to the row - added to a list, replacing a
// single value - and closes the picker.
func (s *settingsForm) choosePath(path string) {
	p := s.picker
	s.picker = nil
	for i := range s.categories[s.category].fields {
		field := &s.categories[s.category].fields[i]
		if field.label != p.field || field.text == nil {
			continue
		}
		value := path
		if p.list {
			var kept []string
			for _, part := range strings.Split(*field.text, ",") {
				if part = strings.TrimSpace(part); part != "" && part != path {
					kept = append(kept, part)
				}
			}
			value = strings.Join(append(kept, path), ", ")
		}
		if value != *field.text {
			s.writeBack(field, value)
			s.dirty = true
			s.notifyDeck()
		}
		s.report("chose " + path)
		return
	}
}

// pickerLines draws the picker where the rows would be.
func (s settingsForm) pickerLines(r tideui.Renderer, width, rows int) []string {
	p := s.picker
	what := "a file"
	if p.kind == dash.PathDir {
		what = "a folder"
	}
	lines := []string{
		paneHint(r).Render(ansi.Truncate("Choose "+what+" for "+p.field, width, "…")),
		ansi.Truncate("› "+p.query+"▏", width, "…"),
	}
	room := max(1, rows-len(lines))
	if p.rows() == 0 {
		return append(lines, paneHint(r).Render("  nothing here - backspace to go up"))
	}
	first, last := tideui.VisibleRange(p.rows(), p.cursor, room)
	for i := first; i < last; i++ {
		label := ""
		if p.useFolder && i == 0 {
			label = "use this folder"
		} else {
			j := i
			if p.useFolder {
				j--
			}
			label = p.entries[j]
		}
		lines = append(lines, r.RenderSoftRow(tideui.SoftRow{Prefix: "  ", Text: label, Selected: i == p.cursor}, width))
	}
	return lines
}
