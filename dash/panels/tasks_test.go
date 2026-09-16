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

func TestTasksPanelMeta(t *testing.T) {
	meta := Tasks().Meta()
	if meta.ID != "tasks" || meta.Title != "Tasks" {
		t.Fatalf("meta = %#v", meta)
	}
	if meta.MinWidth != 20 || meta.MinHeight != 6 || meta.HideBelow != 88 || meta.Priority != 72 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != 30*time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

func TestTasksConfigureBuildsSourceOrNot(t *testing.T) {
	panel := Tasks().(*tasks)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("an empty path should build no fetcher")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}
	values := dash.NewValues()
	values.Set(todoKey, "~/todo.txt")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured path should build a fetcher")
	}
}

func TestTasksPanelRendersItsOwnData(t *testing.T) {
	panel := Tasks().(*tasks)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "tasks", Width: 30, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"Finish TideDeck", "Record demo GIF"} {
		if !strings.Contains(body, want) {
			t.Fatalf("tasks body missing %q:\n%s", want, body)
		}
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "tasks", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestTasksBadgeToggleAndAdd(t *testing.T) {
	panel := Tasks().(*tasks)
	panel.Demo(time.Unix(1700000000, 0))
	if text, tone := panel.Badge(); text != "3" || tone != tideui.ToneMuted {
		t.Fatalf("badge = %q %v, want 3 open", text, tone)
	}
	if message := panel.toggle(); !strings.Contains(message, "Finish TideDeck") {
		t.Fatalf("toggle = %q", message)
	}
	if text, _ := panel.Badge(); text != "2" {
		t.Fatalf("badge after toggle = %q, want 2", text)
	}
	if message := panel.add(); message != "task added" {
		t.Fatalf("add = %q", message)
	}
	if text, _ := panel.Badge(); text != "3" {
		t.Fatalf("badge after add = %q, want 3", text)
	}
}

func TestTasksKeepsLastGoodReading(t *testing.T) {
	good := []tideui.Task{{Title: "Finish TideDeck"}}
	calls := 0
	panel := &tasks{}
	panel.fetch = func(context.Context) ([]tideui.Task, error) {
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
	if got := panel.Load(); len(got) != 1 || got[0].Title != "Finish TideDeck" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

func TestTasksThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Tasks())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("tasks")
	if !ok {
		t.Fatal("tasks was not attached to the workspace")
	}
	keys := map[string]bool{}
	for _, action := range registered.ActionList() {
		keys[action.Key] = true
	}
	for _, want := range []string{"space", "a", "e"} {
		if !keys[want] {
			t.Fatalf("missing %q action; got %v", want, keys)
		}
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "tasks", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "Finish TideDeck") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
	if badge, ok := deck.Badges()["tasks"]; !ok || badge.Text != "3" {
		t.Fatalf("tasks badge = %#v, want 3", badge)
	}
}
