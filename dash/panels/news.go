package panels

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// feedsKey is the configuration key this panel reads. It stays owned by the
// app: the curated source catalogue and the free-text "other feeds" row are two
// affordances over one comma-separated value, which a panel's one-key-per-field
// schema cannot express, so the settings category stays hand-written.
const feedsKey = "feeds"

// news shows the latest headlines. It remembers what has been marked read, so a
// refresh cannot resurrect a story the user has already dismissed.
type news struct {
	dash.State[[]tideui.Headline]

	mu    sync.Mutex
	fetch func(context.Context) ([]tideui.Headline, error)
	// read remembers marked stories by key. Every refresh returns fresh
	// Headline values with Unread set, so without this the mark-read action is
	// undone by the next fetch and the badge never clears.
	read map[string]bool
	// cursor is the selected story, driven by the arrow keys and a click, and
	// rows maps each rendered content row to its story (-1 for the blank row
	// between stories) so a click can be resolved to an item.
	cursor int
	rows   []int
}

// News builds the news panel.
func News() dash.Panel { return &news{} }

func (n *news) Meta() dash.Meta {
	return dash.Meta{
		ID: "news", Title: "News", Subtitle: "feeds",
		Role: tideui.RoleSecondary, Priority: 68,
		MinWidth: 22, MinHeight: 6, HideBelow: 78,
		Interval: 5 * time.Minute,
	}
}

// Schema declares no fields: the feeds setting is app-owned, as the comment on
// feedsKey explains. Configure still reads it, so the panel fetches the
// configured sources.
func (n *news) Schema() []dash.Field { return nil }

func (n *news) Configure(values dash.Values) error {
	feeds := values.List(feedsKey)
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(feeds) == 0 {
		n.fetch = nil
		return nil
	}
	n.fetch = provider.Feed(feeds...)
	return nil
}

func (n *news) Refresh(ctx context.Context) error {
	n.mu.Lock()
	fetch := n.fetch
	n.mu.Unlock()
	if fetch == nil {
		return nil
	}
	headlines, err := fetch(ctx)
	if err != nil {
		return err
	}
	n.Store(n.applyRead(headlines))
	return nil
}

func (n *news) View(ctx tideui.PanelContext) string {
	headlines := n.Load()
	n.mu.Lock()
	cursor := n.cursor
	n.mu.Unlock()
	// Mark the selected story on a copy, so the cursor is a rendering concern
	// and never leaks into the stored data. It shows only while the pane has the
	// keyboard - entered with space, or owning the screen - because a cursor the
	// arrows cannot move is a cursor that lies about what the reader can do.
	if ctx.Entered && cursor >= 0 && cursor < len(headlines) {
		marked := append([]tideui.Headline(nil), headlines...)
		marked[cursor].Selected = true
		headlines = marked
	}
	out, rows := ctx.Renderer.RenderHeadlinesRows(headlines, ctx.Width, ctx.Zoomed)
	n.mu.Lock()
	n.rows = rows
	n.mu.Unlock()
	return out
}

// Move changes the selected story, clamped to the list. It reports whether the
// panel took the key, so an empty list still lets the arrows move focus.
func (n *news) Move(delta int) bool {
	headlines := n.Load()
	if len(headlines) == 0 {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cursor += delta
	if n.cursor < 0 {
		n.cursor = 0
	}
	if n.cursor >= len(headlines) {
		n.cursor = len(headlines) - 1
	}
	return true
}

// Copy offers the selected story's link, so the copy key works on it.
func (n *news) Copy() (string, bool) {
	link, _ := n.copySelected()
	return link, link != ""
}

// Activate is the selected story's primary action - copy its link - the same
// thing a click does.
func (n *news) Activate() (string, string) {
	return n.copySelected()
}

// copySelected returns the selected story's link and a status message.
func (n *news) copySelected() (string, string) {
	headlines := n.Load()
	n.mu.Lock()
	index := n.cursor
	n.mu.Unlock()
	if index < 0 || index >= len(headlines) {
		return "", "no story selected"
	}
	link := headlines[index].Link
	if link == "" {
		return "", "this story has no link"
	}
	return link, "link copied"
}

// Click selects the story on a content row and copies it. It reports whether
// the row held a story.
func (n *news) Click(x, y int) (string, string, bool) {
	n.mu.Lock()
	rows := n.rows
	n.mu.Unlock()
	if y < 0 || y >= len(rows) || rows[y] < 0 {
		return "", "", false
	}
	index := rows[y]
	if index < 0 || index >= len(n.Load()) {
		return "", "", false
	}
	n.mu.Lock()
	n.cursor = index
	n.mu.Unlock()
	link, status := n.copySelected()
	return link, status, true
}

// markReadSelected marks the selected story read. Copying a link does not mean
// you read the story, so this stays an explicit action.
func (n *news) markReadSelected() string {
	n.mu.Lock()
	index := n.cursor
	n.mu.Unlock()
	return n.markReadAt(index)
}

// Demo synthesises a plausible morning's stories.
func (n *news) Demo(now time.Time) {
	n.Store([]tideui.Headline{
		{Title: "Linux 6.12 released", Source: "kernel.org", Age: "18m", Link: "https://kernel.org/", Unread: true, Tone: tideui.ToneAccent},
		{Title: "New Rust TUI framework", Source: "GitHub", Age: "42m", Link: "https://github.com/", Unread: true, Tone: tideui.ToneAccent},
		{Title: "Arch update lands", Source: "archlinux.org", Age: "1h", Link: "https://archlinux.org/news/", Tone: tideui.ToneMuted},
		{Title: "SQLite 3.47 ships", Source: "sqlite.org", Age: "2h", Link: "https://sqlite.org/", Tone: tideui.ToneMuted},
		{Title: "Go 1.24 beta available", Source: "go.dev", Age: "3h", Link: "https://go.dev/", Tone: tideui.ToneMuted},
		{Title: "Rust 1.83 released", Source: "blog.rust-lang.org", Age: "4h", Link: "https://blog.rust-lang.org/", Tone: tideui.ToneMuted},
		{Title: "Wayland 1.24 planned", Source: "phoronix", Age: "5h", Link: "https://phoronix.com/", Tone: tideui.ToneMuted},
		{Title: "Kubernetes 1.32 ships", Source: "k8s.io", Age: "7h", Link: "https://k8s.io/", Tone: tideui.ToneMuted},
	})
}

// Badge shows the unread count, distinguishing "nothing unread" from the panel
// simply not being populated.
func (n *news) Badge() (string, tideui.Tone) {
	unread := 0
	for _, headline := range n.Load() {
		if headline.Unread {
			unread++
		}
	}
	if unread == 0 {
		return "", tideui.ToneMuted
	}
	return fmt.Sprintf("%d", unread), tideui.ToneMuted
}

// Actions advertises the refresh key, the primary action Enter and a click
// both run, and an explicit mark-read. The copy key is a hint only: the
// application owns Enter and the clipboard, so its handler does nothing.
func (n *news) Actions() []dash.Action {
	return []dash.Action{
		{ID: "move", Key: "up/down", Label: "move"},
		{ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
			Run: func() string { return "feeds refreshed" }},
		{ID: "copy", Key: "enter", Label: "copy link"},
		{ID: "read", Key: "x", Label: "mark read", Run: n.markReadSelected},
	}
}

// markReadAt marks one story read and records it, so a later refresh leaves it
// read, and returns a status message.
func (n *news) markReadAt(index int) string {
	headlines := n.Load()
	if index < 0 || index >= len(headlines) {
		return "no story selected"
	}
	headlines[index].Unread = false
	n.mu.Lock()
	if n.read == nil {
		n.read = make(map[string]bool)
	}
	n.read[headlineKey(headlines[index])] = true
	n.mu.Unlock()
	n.Store(headlines)
	return "marked read"
}

// applyRead re-applies what has been marked read to a freshly fetched list.
func (n *news) applyRead(headlines []tideui.Headline) []tideui.Headline {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.read) == 0 {
		return headlines
	}
	for i := range headlines {
		if n.read[headlineKey(headlines[i])] {
			headlines[i].Unread = false
		}
	}
	return headlines
}

// headlineKey identifies a story across refreshes. Headline carries no link,
// so title and source are what there is to go on; a repeat of both is the same
// story for this purpose.
func headlineKey(h tideui.Headline) string {
	return h.Source + "\x00" + h.Title
}
