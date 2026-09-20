package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
)

// testRenderer is the theme the form draws in during tests: the default theme,
// with no options, so nothing about the drawing depends on the machine.
func testRenderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
}

// visibleWidth is the cells a drawn line occupies, styling removed.
func visibleWidth(line string) int { return ansi.StringWidth(line) }

// newTestEditor puts a form on a store holding list. id is a link to edit, or
// addRowID for a blank form.
func newTestEditor(t *testing.T, list []Favorite, id string) (*editor, Store) {
	t.Helper()
	store := Store{Path: filepath.Join(t.TempDir(), "favorites.json")}
	if err := store.Save(list); err != nil {
		t.Fatalf("seeding the store: %v", err)
	}
	e, err := newEditor(store, id)
	if err != nil {
		t.Fatalf("opening the form: %v", err)
	}
	return e, store
}

// press sends one keystroke by name, the way a test reads best.
func press(e *editor, key string) {
	switch key {
	case "tab":
		e.key(tea.KeyMsg{Type: tea.KeyTab})
	case "shift+tab":
		e.key(tea.KeyMsg{Type: tea.KeyShiftTab})
	case "enter":
		e.key(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		e.key(tea.KeyMsg{Type: tea.KeyEsc})
	case "ctrl+s":
		e.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	case "ctrl+d":
		e.key(tea.KeyMsg{Type: tea.KeyCtrlD})
	case "down":
		e.key(tea.KeyMsg{Type: tea.KeyDown})
	default:
		for _, r := range key {
			e.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
}

// A blank form is blank, and it is a new entry rather than an existing one.
func TestFormStartsBlankForAdd(t *testing.T) {
	e, _ := newTestEditor(t, nil, addRowID)
	if e.title.Value() != "" || e.link.Value() != "" || e.tags.Value() != "" {
		t.Fatalf("the blank form is not blank: %q %q %q", e.title.Value(), e.link.Value(), e.tags.Value())
	}
	if e.index != -1 {
		t.Fatalf("a new entry has index %d, want -1", e.index)
	}
}

// Opening a favorite fills the fields with it, tags and all.
func TestFormLoadsTheEntryItWasGiven(t *testing.T) {
	e, _ := newTestEditor(t, []Favorite{{
		Title: "Hacker News", URL: "https://news.ycombinator.com", Tags: []string{"news", "tech"},
	}}, "https://news.ycombinator.com")
	if e.title.Value() != "Hacker News" {
		t.Fatalf("title = %q", e.title.Value())
	}
	if e.link.Value() != "https://news.ycombinator.com" {
		t.Fatalf("link = %q", e.link.Value())
	}
	if e.tags.Value() != "news, tech" {
		t.Fatalf("tags = %q, want them comma separated for editing", e.tags.Value())
	}
}

// An entry that is not there is an error, not a silent new favorite: the pane
// asked for a link the file no longer holds, which is a stale panel, not a form.
func TestFormRefusesAnUnknownEntry(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "favorites.json")}
	if _, err := newEditor(store, "https://gone.example"); err == nil {
		t.Fatal("editing a favorite that does not exist was accepted")
	}
}

// A link is the one thing a favorite cannot do without.
func TestFormRefusesAnEmptyLink(t *testing.T) {
	e, store := newTestEditor(t, nil, addRowID)
	e.title.SetValue("Something")
	press(e, "ctrl+s")
	if e.done {
		t.Fatal("a favorite with no link was saved")
	}
	if !strings.Contains(e.problem, "link") {
		t.Fatalf("the complaint does not mention the link: %q", e.problem)
	}
	list, _ := store.Load()
	if len(list) != 0 {
		t.Fatalf("a rejected save still wrote %d entries", len(list))
	}
}

// A blank title is not worth refusing over: the site says what it is.
func TestFormNamesABlankTitleFromTheSite(t *testing.T) {
	e, store := newTestEditor(t, nil, addRowID)
	e.link.SetValue("https://www.example.com/some/page")
	press(e, "ctrl+s")
	if e.problem != "" {
		t.Fatalf("a blank title was refused: %q", e.problem)
	}
	list, _ := store.Load()
	if len(list) != 1 {
		t.Fatalf("saving wrote %d entries, want 1", len(list))
	}
	if list[0].Title != "example.com" {
		t.Fatalf("the title fell back to %q, want the site", list[0].Title)
	}
}

// Only the web schemes are saved. A file: or javascript: link is refused by name
// rather than stored for the pane to render as a row nobody can open.
func TestFormRefusesANonWebLink(t *testing.T) {
	e, store := newTestEditor(t, nil, addRowID)
	e.link.SetValue("file:///etc/passwd")
	press(e, "ctrl+s")
	if e.done {
		t.Fatal("a file: link was saved")
	}
	if !strings.Contains(e.problem, "file:") {
		t.Fatalf("the complaint does not name the scheme: %q", e.problem)
	}
	if list, _ := store.Load(); len(list) != 0 {
		t.Fatalf("a rejected save still wrote %d entries", len(list))
	}
}

// Two rows for one link is worse than being told which entry already has it.
func TestFormRefusesADuplicate(t *testing.T) {
	e, store := newTestEditor(t, []Favorite{{
		Title: "Go", URL: "https://go.dev", Tags: []string{"code"},
	}}, addRowID)
	e.title.SetValue("The Go programming language")
	e.link.SetValue("https://go.dev")
	press(e, "ctrl+s")
	if e.done {
		t.Fatal("a duplicate link was saved")
	}
	if !strings.Contains(e.problem, "Go") {
		t.Fatalf("the complaint does not name the entry that has it: %q", e.problem)
	}
	if list, _ := store.Load(); len(list) != 1 {
		t.Fatalf("the list now holds %d entries, want the one it started with", len(list))
	}
}

// Editing an entry keeps its own link as its own: the duplicate check is about
// other entries, not about the one being changed.
func TestFormAllowsSavingAnEntryOverItself(t *testing.T) {
	e, store := newTestEditor(t, []Favorite{{
		Title: "Go", URL: "https://go.dev", Tags: []string{"code"},
	}}, "https://go.dev")
	e.title.SetValue("The Go programming language")
	e.tags.SetValue("code, reference")
	press(e, "ctrl+s")
	if e.problem != "" {
		t.Fatalf("saving an entry over itself was refused: %q", e.problem)
	}
	list, _ := store.Load()
	if len(list) != 1 || list[0].Title != "The Go programming language" {
		t.Fatalf("the edit did not land: %#v", list)
	}
	if list[0].Tags[1] != "reference" {
		t.Fatalf("the tags did not land: %#v", list[0].Tags)
	}
}

// Tags are what a person types: spaces around the commas are theirs, and an empty
// tag is a comma that was not finished.
func TestFormTrimsTagsAndDropsTheEmptyOnes(t *testing.T) {
	got := splitTags("  news , , tech ,")
	if len(got) != 2 || got[0] != "news" || got[1] != "tech" {
		t.Fatalf("splitTags gave %#v, want news and tech", got)
	}
	if tags := splitTags("  ,  "); len(tags) != 0 {
		t.Fatalf("tags that are only commas gave %#v", tags)
	}
}

// The whole flow through the keyboard, because that is how it will be used: enter
// opens a field, tab commits and moves, ctrl+s saves.
func TestFormSavesWhatWasTypedWithKeys(t *testing.T) {
	e, store := newTestEditor(t, nil, addRowID)
	press(e, "enter")
	press(e, "Hacker News")
	press(e, "tab")
	press(e, "enter")
	press(e, "https://news.ycombinator.com")
	press(e, "tab")
	press(e, "enter")
	press(e, "news, tech")
	press(e, "ctrl+s")

	if !e.done {
		t.Fatalf("the form did not finish: %q", e.problem)
	}
	list, _ := store.Load()
	if len(list) != 1 {
		t.Fatalf("saving wrote %d entries, want 1", len(list))
	}
	entry := list[0]
	if entry.Title != "Hacker News" || entry.URL != "https://news.ycombinator.com" {
		t.Fatalf("what was typed is not what was saved: %#v", entry)
	}
	if len(entry.Tags) != 2 || entry.Tags[0] != "news" || entry.Tags[1] != "tech" {
		t.Fatalf("the tags are %#v", entry.Tags)
	}
	if entry.Added.IsZero() {
		t.Fatal("a new favorite has no date, so the list cannot order it")
	}
}

// Editing a favorite keeps the date it was added: the list is ordered by when it
// was added, and fixing a typo is not adding it again.
func TestFormKeepsTheAddedDateWhenEditing(t *testing.T) {
	original := Favorite{Title: "Go", URL: "https://go.dev", Added: stamp(17, 4)}
	e, store := newTestEditor(t, []Favorite{original}, "https://go.dev")
	e.title.SetValue("The Go programming language")
	press(e, "ctrl+s")
	list, _ := store.Load()
	if !list[0].Added.Equal(original.Added) {
		t.Fatalf("the date became %v, want %v", list[0].Added, original.Added)
	}
}

// Deleting removes the entry, and only the entry that was asked for.
func TestFormDeletesOnRequest(t *testing.T) {
	e, store := newTestEditor(t, []Favorite{
		{Title: "Go", URL: "https://go.dev", Added: stamp(17, 4)},
		{Title: "Hacker News", URL: "https://news.ycombinator.com", Added: stamp(18, 9)},
	}, "https://go.dev")
	press(e, "ctrl+d")
	if !e.done || !e.removed {
		t.Fatalf("deleting did not finish the form: done=%v removed=%v", e.done, e.removed)
	}
	list, _ := store.Load()
	if len(list) != 1 || list[0].Title != "Hacker News" {
		t.Fatalf("after deleting Go the list is %#v", list)
	}
}

// A blank form has nothing to delete, so it does not offer to. Offering a key
// that does nothing is how a form teaches people not to trust it.
func TestFormOffersNoDeleteForANewEntry(t *testing.T) {
	fresh, _ := newTestEditor(t, []Favorite{{Title: "Go", URL: "https://go.dev"}}, addRowID)
	if strings.Contains(strings.Join(fresh.hints(), " "), "delete") {
		t.Fatalf("a new entry offers delete: %v", fresh.hints())
	}
	existing, _ := newTestEditor(t, []Favorite{{Title: "Go", URL: "https://go.dev"}}, "https://go.dev")
	if !strings.Contains(strings.Join(existing.hints(), " "), "delete") {
		t.Fatalf("editing an entry does not offer delete: %v", existing.hints())
	}
	// And pressing it on a new entry does nothing rather than deleting someone
	// else's entry.
	press(fresh, "ctrl+d")
	if fresh.done || fresh.removed {
		t.Fatal("ctrl+d on a new entry did something")
	}
}

// Escape leaves without saving, and without touching the file.
func TestFormCancelsWithoutSaving(t *testing.T) {
	e, store := newTestEditor(t, []Favorite{{Title: "Go", URL: "https://go.dev", Added: stamp(17, 4)}}, "https://go.dev")
	press(e, "enter")
	press(e, "Changed")
	press(e, "esc") // ends the edit, keeping the form open
	press(e, "esc") // leaves the form
	if !e.done {
		t.Fatal("escape did not leave the form")
	}
	list, _ := store.Load()
	if list[0].Title != "Go" {
		t.Fatalf("escape saved a change: %#v", list[0])
	}
}

// The form draws inside the width it is given, whichever width that is: a form
// that overflows its pane corrupts the screen around it.
func TestFormDrawsInsideItsWidth(t *testing.T) {
	e, _ := newTestEditor(t, []Favorite{{Title: "Go", URL: "https://go.dev"}}, addRowID)
	renderer := testRenderer()
	for _, width := range []int{20, 30, 80, 200} {
		drawn := e.View(renderer, width)
		for _, line := range strings.Split(drawn, "\n") {
			if got := visibleWidth(line); got > width {
				t.Fatalf("at width %d a line came out %d cells wide: %q", width, got, line)
			}
		}
	}
}
