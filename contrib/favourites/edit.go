package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/form"
)

// editor is the form that adds, changes and removes one favourite.
//
// It owns the store, and every decision - what a valid entry is, what counts as a
// duplicate, what the title falls back to - is made here rather than in the
// bubbletea model around it, so all of it can be tested without a terminal.
type editor struct {
	store   Store
	entries []Favourite
	index   int // the entry being changed, or -1 when this is a new one

	title *form.Text
	link  *form.Text
	tags  *form.Text
	focus int

	problem string // what is wrong with what has been typed, if anything
	done    bool   // saved, deleted, or cancelled: the screen should exit
	removed bool   // the entry was deleted rather than saved
}

// newEditor loads the list and puts the form on one entry. The id is a
// favourite's link, or addRowID for a blank form.
func newEditor(store Store, id string) (*editor, error) {
	entries, err := store.Load()
	if err != nil {
		return nil, err
	}
	e := &editor{store: store, entries: entries, index: -1}
	if id != "" && id != addRowID {
		e.index = indexOfURL(entries, id)
		if e.index < 0 {
			return nil, fmt.Errorf("no favourite with the link %s", id)
		}
	}
	var current Favourite
	if e.index >= 0 {
		current = entries[e.index]
	}
	e.title = form.NewText(current.Title).WithPlaceholder("what it is")
	e.link = form.NewText(current.URL).WithPlaceholder("https://…")
	e.tags = form.NewText(strings.Join(current.Tags, ", ")).WithPlaceholder("news, tech")
	return e, nil
}

// controls is the fields in the order they are stepped through.
func (e *editor) controls() []*form.Text { return []*form.Text{e.title, e.link, e.tags} }

// editing reports whether a field is taking keys exclusively, which is how the
// form knows not to treat a "j" as a movement.
func (e *editor) editing() bool {
	for _, control := range e.controls() {
		if control.Editing() {
			return true
		}
	}
	return false
}

// key feeds one keystroke to the form and reports whether the screen is finished
// with. Movement lives here rather than in the control: a field that swallowed
// tab would be a trap, and the same keys move between fields and panes everywhere
// else in the dashboard.
func (e *editor) key(msg tea.KeyMsg) bool {
	if e.done {
		return true
	}
	switch msg.String() {
	case "ctrl+s":
		e.save()
		return e.done
	case "ctrl+d":
		if e.index >= 0 {
			e.delete()
			return e.done
		}
		return false
	case "esc":
		if !e.editing() {
			// Leaving a form without saving is a decision, not a failure, so it
			// is quiet and it keeps the file it did not touch.
			e.done = true
			return true
		}
	}

	controls := e.controls()
	control := controls[e.focus]
	action := control.Update(msg)
	if control.Editing() || action == form.ActionEditing {
		return false
	}
	if action == form.ActionIgnored {
		switch msg.String() {
		case "tab", "down":
			e.focus = (e.focus + 1) % len(controls)
		case "shift+tab", "up":
			e.focus = (e.focus + len(controls) - 1) % len(controls)
		}
	}
	return false
}

// save validates what has been typed and writes the list, keeping the form open
// with an explanation when it cannot.
func (e *editor) save() {
	entry, problem := e.entryFromFields()
	if problem != "" {
		e.problem = problem
		return
	}
	if e.index >= 0 {
		e.entries[e.index] = entry
	} else {
		e.entries = append(e.entries, entry)
	}
	if err := e.store.Save(e.entries); err != nil {
		e.problem = err.Error()
		return
	}
	e.problem = ""
	e.done = true
}

// delete removes the entry being edited.
func (e *editor) delete() {
	if e.index < 0 {
		return
	}
	e.entries = append(e.entries[:e.index], e.entries[e.index+1:]...)
	if err := e.store.Save(e.entries); err != nil {
		e.problem = err.Error()
		return
	}
	e.problem = ""
	e.done, e.removed = true, true
}

// entryFromFields is the entry the fields describe, or why they do not describe
// one. A duplicate is caught here rather than merged: silently ending up with two
// rows for one link is worse than being told which entry already has it.
func (e *editor) entryFromFields() (Favourite, string) {
	link := strings.TrimSpace(e.link.Value())
	if link == "" {
		return Favourite{}, "a favourite needs a link"
	}
	if !openable(link) {
		return Favourite{}, fmt.Sprintf("only http and https links open in a browser; %q does not", schemeOrNothing(link))
	}
	if holder, ok := e.holderOf(link); ok {
		return Favourite{}, fmt.Sprintf("%q already saves that link", holder)
	}

	entry := Favourite{URL: link, Tags: splitTags(e.tags.Value()), Added: time.Now().UTC()}
	if e.index >= 0 {
		// Editing keeps the original date: the list is ordered by when a
		// favourite was added, and fixing a typo is not adding it again.
		entry.Added = e.entries[e.index].Added
	}
	entry.Title = strings.TrimSpace(e.title.Value())
	if entry.Title == "" {
		// A blank title is not worth refusing over: the link says what it is,
		// and the pane would fall back to the URL anyway.
		entry.Title = titleFromURL(link)
	}
	return entry, ""
}

// holderOf reports the title of the entry already using a link, ignoring the one
// being edited.
func (e *editor) holderOf(link string) (string, bool) {
	for i, entry := range e.entries {
		if i == e.index {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entry.URL), link) {
			return displayTitle(entry), true
		}
	}
	return "", false
}

// hints are the keys that work right now. A blank form has nothing to delete, so
// it does not offer to.
func (e *editor) hints() []string {
	hints := []string{"enter edit a field", "tab next", "ctrl+s save"}
	if e.index >= 0 {
		hints = append(hints, "ctrl+d delete")
	}
	return append(hints, "esc cancel")
}

// View draws the form: a heading, one row per field with its value cell, what is
// wrong with it if anything, and the keys that work.
func (e *editor) View(r tideui.Renderer, width int) string {
	if width < 20 {
		width = 20
	}
	ws := r.Styles.Workspace
	bg := ws.Bg
	heading := "New favourite"
	if e.index >= 0 {
		heading = "Edit favourite"
	}
	lines := []string{
		lipgloss.NewStyle().Background(bg).Foreground(ws.BodyFg).Bold(true).Render(heading),
		"",
	}

	names := []string{"title", "link", "tags"}
	labelWidth := 0
	for _, name := range names {
		if len(name) > labelWidth {
			labelWidth = len(name)
		}
	}
	controls := e.controls()
	for i, control := range controls {
		marker, markerColour := "  ", ws.HintFg
		if i == e.focus {
			marker, markerColour = "› ", ws.FocusRail
		}
		label := lipgloss.NewStyle().Background(bg).Foreground(ws.BodyMutedFg).
			Render(nameWidth(names[i], labelWidth))
		cellWidth := width - labelWidth - 4
		lines = append(lines, lipgloss.NewStyle().Background(bg).Foreground(markerColour).Render(marker)+
			label+lipgloss.NewStyle().Background(bg).Render("  ")+
			control.View(r, cellWidth))
	}

	if e.problem != "" {
		lines = append(lines, "", lipgloss.NewStyle().Background(bg).Foreground(ws.MetricBad).Render(e.problem))
	}
	lines = append(lines, "", lipgloss.NewStyle().Background(bg).Foreground(ws.HintFg).
		Render(strings.Join(e.hints(), "  ·  ")))
	return r.RenderLines(lines, width, bg)
}

// nameWidth pads a field name to the column width.
func nameWidth(name string, width int) string {
	if pad := width - len(name); pad > 0 {
		return name + strings.Repeat(" ", pad)
	}
	return name
}

// schemeOrNothing names the scheme of a link that is not openable, so the
// complaint says what is wrong with it rather than only that it is wrong.
func schemeOrNothing(link string) string {
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil || parsed.Scheme == "" {
		return "that"
	}
	return parsed.Scheme + ":"
}

// titleFromURL is what a blank title falls back to: the site, without the www.
func titleFromURL(link string) string {
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil || parsed.Host == "" {
		return link
	}
	host := strings.TrimPrefix(parsed.Host, "www.")
	if host == "" {
		return link
	}
	return host
}

// splitTags turns " news , , tech " into ["news","tech"]: the form takes what a
// person types, and an empty tag is a comma they did not finish.
func splitTags(raw string) []string {
	var tags []string
	for _, part := range strings.Split(raw, ",") {
		if tag := strings.TrimSpace(part); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// indexOfURL finds an entry by its link, case-insensitively, as a browser and a
// person both treat a host.
func indexOfURL(entries []Favourite, link string) int {
	for i, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.URL), strings.TrimSpace(link)) {
			return i
		}
	}
	return -1
}

// screen is the bubbletea model around the form. It owns the terminal, the
// renderer and the width, and nothing else: every decision belongs to editor.
type screen struct {
	editor   *editor
	renderer tideui.Renderer
	width    int
	finished bool
}

func (s *screen) Init() tea.Cmd { return nil }

func (s *screen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		return s, nil
	case tea.KeyMsg:
		if s.editor.key(msg) {
			s.finished = true
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *screen) View() string {
	width := s.width
	if width <= 0 {
		width = 72
	}
	return s.editor.View(s.renderer, width)
}
