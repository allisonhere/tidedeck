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
	if ctx.Zoomed {
		return ctx.Renderer.RenderHeadlinesDetail(headlines, ctx.Width)
	}
	return ctx.Renderer.RenderHeadlines(headlines, ctx.Width)
}

// Demo synthesises a plausible morning's stories.
func (n *news) Demo(now time.Time) {
	n.Store([]tideui.Headline{
		{Title: "Linux 6.12 released", Source: "kernel.org", Age: "18m", Unread: true, Tone: tideui.ToneAccent},
		{Title: "New Rust TUI framework", Source: "GitHub", Age: "42m", Unread: true, Tone: tideui.ToneAccent},
		{Title: "Arch update lands", Source: "archlinux.org", Age: "1h", Tone: tideui.ToneMuted},
		{Title: "SQLite 3.47 ships", Source: "sqlite.org", Age: "2h", Tone: tideui.ToneMuted},
		{Title: "Go 1.24 beta available", Source: "go.dev", Age: "3h", Tone: tideui.ToneMuted},
		{Title: "Rust 1.83 released", Source: "blog.rust-lang.org", Age: "4h", Tone: tideui.ToneMuted},
		{Title: "Wayland 1.24 planned", Source: "phoronix", Age: "5h", Tone: tideui.ToneMuted},
		{Title: "Kubernetes 1.32 ships", Source: "k8s.io", Age: "7h", Tone: tideui.ToneMuted},
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

func (n *news) Actions() []dash.Action {
	return []dash.Action{
		{ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
			Run: func() string { return "feeds refreshed" }},
		{ID: "mark", Key: "m", Label: "mark read", Run: n.markRead},
	}
}

// markRead clears the first unread story and records it, so a later refresh
// leaves it read.
func (n *news) markRead() string {
	headlines := n.Load()
	for i := range headlines {
		if !headlines[i].Unread {
			continue
		}
		headlines[i].Unread = false
		n.mu.Lock()
		if n.read == nil {
			n.read = make(map[string]bool)
		}
		n.read[headlineKey(headlines[i])] = true
		n.mu.Unlock()
		n.Store(headlines)
		return "marked read"
	}
	return "nothing unread"
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
