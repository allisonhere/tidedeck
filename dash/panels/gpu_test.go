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

func renderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{Density: tideui.Compact})
}

func TestGPUPanelMeta(t *testing.T) {
	meta := GPU().Meta()
	if meta.ID != "gpu" || meta.Title != "GPU" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 18 || meta.MinHeight != 6 || meta.HideBelow != 80 || meta.Priority != 72 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

func TestGPUPanelRendersItsOwnData(t *testing.T) {
	panel := GPU()
	ctx := tideui.PanelContext{ID: "gpu", Width: 34, Renderer: renderer()}

	// With nothing fetched the panel says so rather than drawing zeroes.
	if empty := ansi.Strip(panel.View(ctx)); !strings.Contains(empty, "No GPU") {
		t.Fatalf("empty panel = %q", empty)
	}

	panel.(dash.Demoable).Demo(time.Unix(1700000000, 0))
	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"GPU", "VRAM", "TEMP", "PWR", "CLK"} {
		if !strings.Contains(body, want) {
			t.Fatalf("demo body missing %q:\n%s", want, body)
		}
	}
	detail := ansi.Strip(panel.View(tideui.PanelContext{ID: "gpu", Width: 40, Zoomed: true, Renderer: renderer()}))
	if !strings.Contains(detail, "DEVICE") {
		t.Fatalf("zoomed body missing the device section:\n%s", detail)
	}
	// Every line stays within the panel, at any width.
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "gpu", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestGPUPanelRefreshStoresAndKeepsLastGood(t *testing.T) {
	good := tideui.GPUMetrics{Name: "amdgpu", BusyPercent: 42, MemoryLabel: "MEM"}
	calls := 0
	panel := &gpu{fetch: func(context.Context) (tideui.GPUMetrics, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return tideui.GPUMetrics{}, context.DeadlineExceeded
	}}

	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if panel.Load().BusyPercent != 42 {
		t.Fatalf("refresh did not store: %#v", panel.Load())
	}
	// A failed refresh reports the error and leaves the last good reading in
	// place, so the panel never blanks out because one sample was missed.
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if panel.Load().BusyPercent != 42 {
		t.Fatalf("a failed refresh discarded good data: %#v", panel.Load())
	}
}

// The deck must be able to drive the panel with no special-casing.
func TestGPUPanelThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(GPU())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("gpu")
	if !ok {
		t.Fatal("gpu was not attached to the workspace")
	}
	if registered.TitleText() != "GPU" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "r" {
		t.Fatalf("actions = %#v", actions)
	}
	// Demo mode fills the panel through the deck's tick alone.
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "gpu", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "GPU") || strings.Contains(body, "No GPU") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
