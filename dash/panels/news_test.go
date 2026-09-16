package panels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func TestNewsPanelMeta(t *testing.T) {
	meta := News().Meta()
	if meta.ID != "news" || meta.Title != "News" || meta.Subtitle != "feeds" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 22 || meta.MinHeight != 6 || meta.HideBelow != 78 || meta.Priority != 68 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != 5*time.Minute {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

// The feeds key stays app-owned, but Configure still reads it so the panel
// fetches the configured sources.
func TestNewsConfigureBuildsSourceOrNot(t *testing.T) {
	panel := News().(*news)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("an empty feed list should build no fetcher")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}
	values := dash.NewValues()
	values.Set(feedsKey, "https://example.com/rss.xml")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured feed should build a fetcher")
	}
}

func TestNewsPanelRendersItsOwnData(t *testing.T) {
	panel := News().(*news)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "news", Width: 40, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"Linux 6.12 released", "kernel.org"} {
		if !strings.Contains(body, want) {
			t.Fatalf("news body missing %q:\n%s", want, body)
		}
	}
	// Every line stays within the panel, at any width.
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "news", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestNewsBadgeCountsUnread(t *testing.T) {
	panel := News().(*news)
	if text, _ := panel.Badge(); text != "" {
		t.Fatalf("empty panel badge = %q, want empty", text)
	}
	panel.Store([]tideui.Headline{
		{Title: "One", Unread: true},
		{Title: "Two", Unread: true},
		{Title: "Three"},
	})
	if text, tone := panel.Badge(); text != "2" || tone != tideui.ToneMuted {
		t.Fatalf("badge = %q %v, want 2 muted", text, tone)
	}
}

// A refresh returns fresh Headline values with Unread set, so read state has
// to be re-applied or the mark-read action is undone a second later.
func TestNewsReadStateSurvivesRefresh(t *testing.T) {
	panel := &news{}
	panel.Store([]tideui.Headline{
		{Title: "First story", Source: "BBC World", Unread: true},
		{Title: "Second story", Source: "BBC World", Unread: true},
	})
	if got := panel.markReadAt(0); got != "marked read" {
		t.Fatalf("markReadAt = %q", got)
	}
	if panel.Load()[0].Unread {
		t.Fatal("markReadAt did not clear the selected story")
	}

	// A later fetch hands back both stories as unread, plus a new one.
	fresh := []tideui.Headline{
		{Title: "Brand new", Source: "BBC World", Unread: true},
		{Title: "First story", Source: "BBC World", Unread: true},
		{Title: "Second story", Source: "BBC World", Unread: true},
	}
	applied := panel.applyRead(fresh)
	if applied[1].Unread {
		t.Fatal("a story marked read came back unread after a refresh")
	}
	if !applied[0].Unread || !applied[2].Unread {
		t.Fatalf("unrelated stories should stay unread: %+v", applied)
	}
	// The same title from a different source is a different story.
	other := panel.applyRead([]tideui.Headline{{Title: "First story", Source: "NPR News", Unread: true}})
	if !other[0].Unread {
		t.Fatal("read state leaked across sources")
	}
}

// The cursor moves over the stories and clamps at both ends.
func TestNewsCursorMovesAndClamps(t *testing.T) {
	panel := &news{}
	panel.Store([]tideui.Headline{
		{Title: "One", Link: "https://one"},
		{Title: "Two", Link: "https://two"},
	})
	if !panel.Move(1) {
		t.Fatal("Move should take the key when there are stories")
	}
	if got, ok := panel.Copy(); !ok || got != "https://two" {
		t.Fatalf("copy after down = %q %v", got, ok)
	}
	panel.Move(1) // already at the last: stays
	panel.Move(-5)
	if got, _ := panel.Copy(); got != "https://one" {
		t.Fatalf("copy after clamping up = %q", got)
	}
	// An empty list does not take the key, so the arrows still move focus.
	if (&news{}).Move(1) {
		t.Fatal("an empty list should not take the key")
	}
}

// Activating a story copies its link, and copying is not reading: the story
// stays unread. The mark-read action is separate and survives a refresh.
func TestNewsActivateCopiesWithoutReading(t *testing.T) {
	panel := &news{}
	panel.Store([]tideui.Headline{
		{Title: "First", Source: "BBC", Link: "https://bbc/1", Unread: true},
		{Title: "Second", Source: "BBC", Link: "https://bbc/2", Unread: true},
	})
	panel.Move(1)
	copied, status := panel.Activate()
	if copied != "https://bbc/2" || status != "link copied" {
		t.Fatalf("activate = %q %q", copied, status)
	}
	if !panel.Load()[1].Unread {
		t.Fatal("copying a link should not mark the story read")
	}

	// The explicit mark-read action clears it, and a refresh leaves it read.
	if got := panel.markReadSelected(); got != "marked read" {
		t.Fatalf("markReadSelected = %q", got)
	}
	if panel.Load()[1].Unread {
		t.Fatal("mark read did not clear the selected story")
	}
	fresh := panel.applyRead([]tideui.Headline{
		{Title: "First", Source: "BBC", Link: "https://bbc/1", Unread: true},
		{Title: "Second", Source: "BBC", Link: "https://bbc/2", Unread: true},
	})
	if fresh[1].Unread {
		t.Fatal("a refresh resurrected a story marked read")
	}
	if !fresh[0].Unread {
		t.Fatal("an unselected story should stay unread")
	}
}

// A click resolves to the story on that content row and activates it.
func TestNewsClickSelectsAndActivates(t *testing.T) {
	panel := &news{}
	panel.Store([]tideui.Headline{
		{Title: "First story", Source: "BBC", Link: "https://bbc/1", Unread: true},
		{Title: "Second story", Source: "BBC", Link: "https://bbc/2", Unread: true},
	})
	_ = panel.View(tideui.PanelContext{ID: "news", Width: 40, Renderer: renderer()})

	rows := tideui.HeadlineRows(panel.Load(), 40, false)
	target := -1
	for i, story := range rows {
		if story == 1 {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("no row for the second story: %v", rows)
	}
	copied, status, hit := panel.Click(0, target)
	if !hit {
		t.Fatal("click missed the story")
	}
	if copied != "https://bbc/2" || status != "link copied" {
		t.Fatalf("click = %q %q", copied, status)
	}
	if !panel.Load()[1].Unread {
		t.Fatal("clicking a story should not mark it read")
	}
	if _, _, hit := panel.Click(0, len(rows)+5); hit {
		t.Fatal("a click past the list should miss")
	}
}

// A failed refresh keeps the last headlines on screen.
func TestNewsKeepsLastGoodReading(t *testing.T) {
	good := []tideui.Headline{{Title: "Linux 6.12 released", Source: "kernel.org"}}
	calls := 0
	panel := &news{}
	panel.fetch = func(context.Context) ([]tideui.Headline, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return nil, errors.New("no route to host")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if got := panel.Load(); len(got) != 1 || got[0].Title != "Linux 6.12 released" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

// The deck must be able to drive the panel: demo data, the mark-read key, and
// the unread badge all come from the panel itself.
func TestNewsThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(News())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("news")
	if !ok {
		t.Fatal("news was not attached to the workspace")
	}
	if registered.TitleText() != "News" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	keys := map[string]bool{}
	for _, action := range registered.ActionList() {
		keys[action.Key] = true
	}
	for _, want := range []string{"r", "enter", "x"} {
		if !keys[want] {
			t.Fatalf("missing %q action; got %v", want, keys)
		}
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "news", Width: 40, Renderer: renderer()}))
	if !strings.Contains(body, "Linux 6.12 released") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
	if badge, ok := deck.Badges()["news"]; !ok || badge.Text != "2" {
		t.Fatalf("news badge = %#v, want 2", badge)
	}
	// The primary action copies the selected link without changing what is
	// unread; the explicit mark-read action is what clears the badge.
	panel, ok := deck.Lookup("news")
	if !ok {
		t.Fatal("news is not on the deck")
	}
	activator, ok := panel.(dash.Activator)
	if !ok {
		t.Fatal("news does not implement Activator")
	}
	if copied, _ := activator.Activate(); copied == "" {
		t.Fatal("activate should offer the selected story's link")
	}
	if badge, _ := deck.Badges()["news"]; badge.Text != "2" {
		t.Fatalf("copying changed the unread badge: %#v", badge)
	}
	read, ok := actionByKey(registered, "x")
	if !ok {
		t.Fatal("no mark-read action")
	}
	read.Handler(ws)
	if badge, _ := deck.Badges()["news"]; badge.Text != "1" {
		t.Fatalf("news badge after mark read = %#v, want 1", badge)
	}
}
