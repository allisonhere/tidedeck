package panels

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
	"github.com/charmbracelet/x/ansi"
)

func TestStoragePanelMeta(t *testing.T) {
	meta := Storage().Meta()
	if meta.ID != "storage" || meta.Title != "Storage" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 18 || meta.MinHeight != 6 || meta.HideBelow != 80 || meta.Priority != 55 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != 2*time.Minute {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

func TestStoragePanelRendersItsOwnData(t *testing.T) {
	panel := Storage().(*storage)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "storage", Width: 30, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"/home", "48%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("storage body missing %q:\n%s", want, body)
		}
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "storage", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestStorageKeepsLastGoodReading(t *testing.T) {
	good := []tideui.StorageMount{{Path: "/", UsedPercent: 72}}
	calls := 0
	panel := &storage{}
	panel.fetch = func(context.Context) ([]tideui.StorageMount, error) {
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
	if got := panel.Load(); len(got) != 1 || got[0].UsedPercent != 72 {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

// On Linux the real reader is exercised end to end.
func TestStoragePanelReadsProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("storage metrics are linux-only")
	}
	panel := &storage{fetch: provider.Storage()}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(panel.Load()) == 0 {
		t.Fatal("no mounts read from /proc/mounts")
	}
}

func TestStorageThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Storage())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("storage")
	if !ok {
		t.Fatal("storage was not attached to the workspace")
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "r" {
		t.Fatalf("actions = %#v", actions)
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "storage", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "/home") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
