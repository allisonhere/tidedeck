package panels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func TestNotesPanelMeta(t *testing.T) {
	meta := Notes().Meta()
	if meta.ID != "notes" || meta.Title != "Notes" {
		t.Fatalf("meta = %#v", meta)
	}
	if meta.MinWidth != 18 || meta.MinHeight != 5 || meta.HideBelow != 80 || meta.Priority != 45 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != 30*time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

func TestNotesConfigureBuildsSourceOrNot(t *testing.T) {
	panel := Notes().(*notes)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("an empty path list should build no fetcher")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}
	values := dash.NewValues()
	values.Set(notesKey, "~/notes.md")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured path should build a fetcher")
	}
}

func TestNotesPanelRendersItsOwnData(t *testing.T) {
	panel := Notes().(*notes)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "notes", Width: 34, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"Remember", "Ideas"} {
		if !strings.Contains(body, want) {
			t.Fatalf("notes body missing %q:\n%s", want, body)
		}
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "notes", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestNotesKeepsLastGoodReading(t *testing.T) {
	good := []tideui.Note{{Title: "Ideas"}}
	calls := 0
	panel := &notes{}
	panel.fetch = func(context.Context) ([]tideui.Note, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return nil, context.DeadlineExceeded
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if got := panel.Load(); len(got) != 1 || got[0].Title != "Ideas" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

func TestNotesThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Notes())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("notes")
	if !ok {
		t.Fatal("notes was not attached to the workspace")
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "e" {
		t.Fatalf("actions = %#v", actions)
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "notes", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "Remember") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
