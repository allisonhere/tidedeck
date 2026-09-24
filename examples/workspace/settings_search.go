package main

import (
	"sort"
	"strings"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/x/ansi"
)

// Settings run to a dozen pages and well over a hundred rows. / searches all of
// them at once - a row's label, its description, and the page it is on - and
// Enter goes to the one picked, on its own page, so "where was the docker
// socket" is a word away rather than a tour of the pages.

// settingsSearch is the search while it is open.
type settingsSearch struct {
	query   string
	cursor  int
	matches []searchMatch
}

// searchMatch is one row the query found.
type searchMatch struct {
	category, field int
	title           string // "Page › row"
	rank            int
}

func (s *settingsForm) openSearch() {
	s.search = &settingsSearch{}
	s.refreshSearch()
}

// refreshSearch finds the rows matching the query: a label that starts with it
// first, then one that contains it, then a match only in the description or
// the page's name.
func (s *settingsForm) refreshSearch() {
	q := strings.ToLower(strings.TrimSpace(s.search.query))
	s.search.matches = s.search.matches[:0]
	if q == "" {
		s.search.cursor = 0
		return
	}
	for ci, category := range s.categories {
		for fi, field := range category.fields {
			label := strings.ToLower(field.label)
			rank := -1
			switch {
			case strings.HasPrefix(label, q):
				rank = 0
			case strings.Contains(label, q):
				rank = 1
			case strings.Contains(strings.ToLower(field.description), q), strings.Contains(strings.ToLower(category.name), q):
				rank = 2
			}
			if rank < 0 {
				continue
			}
			s.search.matches = append(s.search.matches, searchMatch{category: ci, field: fi, title: category.name + " › " + field.label, rank: rank})
		}
	}
	sort.SliceStable(s.search.matches, func(i, j int) bool { return s.search.matches[i].rank < s.search.matches[j].rank })
	s.search.cursor = min(s.search.cursor, max(0, len(s.search.matches)-1))
}

// updateSearch handles a key while the search is open.
func (s *settingsForm) updateSearch(key string, runes []rune) {
	switch key {
	case "esc":
		s.search = nil
	case "up", "ctrl+k":
		s.search.cursor = max(0, s.search.cursor-1)
	case "down", "ctrl+j", "tab":
		s.search.cursor = min(max(0, len(s.search.matches)-1), s.search.cursor+1)
	case "enter":
		if s.search.cursor < len(s.search.matches) {
			m := s.search.matches[s.search.cursor]
			s.category, s.cursor = m.category, m.field
			s.view, s.focus = viewFields, settingsEditor
		}
		s.search = nil
	case "backspace":
		if r := []rune(s.search.query); len(r) > 0 {
			s.search.query = string(r[:len(r)-1])
			s.refreshSearch()
		}
	default:
		if len(runes) > 0 {
			s.search.query += string(runes)
			s.search.cursor = 0
			s.refreshSearch()
		}
	}
}

// searchLines draws the search in place of a page.
func (s settingsForm) searchLines(r tideui.Renderer, width, rows int) []string {
	lines := []string{
		paneHint(r).Render("Search every setting"),
		ansi.Truncate("/ "+s.search.query+"▏", width, "…"),
	}
	switch {
	case strings.TrimSpace(s.search.query) == "":
		return append(lines, paneHint(r).Render("  type part of a setting's name"))
	case len(s.search.matches) == 0:
		return append(lines, paneHint(r).Render("  nothing matches"))
	}
	room := max(1, rows-len(lines))
	first, last := tideui.VisibleRange(len(s.search.matches), s.search.cursor, room)
	for i := first; i < last; i++ {
		lines = append(lines, r.RenderSoftRow(tideui.SoftRow{Prefix: "  ", Text: s.search.matches[i].title, Selected: i == s.search.cursor}, width))
	}
	return lines
}
