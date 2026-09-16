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

func TestSystemPanelMeta(t *testing.T) {
	meta := System().Meta()
	if meta.ID != "system" || meta.Title != "System" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 20 || meta.MinHeight != 7 || meta.Priority != 95 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

func TestSystemPanelRendersItsOwnData(t *testing.T) {
	panel := System()
	ctx := tideui.PanelContext{ID: "system", Width: 34, Renderer: renderer()}

	panel.(dash.Demoable).Demo(time.Unix(1700000000, 0))
	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"CPU", "MEM", "TEMP", "LOAD", "UP"} {
		if !strings.Contains(body, want) {
			t.Fatalf("demo body missing %q:\n%s", want, body)
		}
	}
	detail := ansi.Strip(panel.View(tideui.PanelContext{ID: "system", Width: 40, Zoomed: true, Renderer: renderer()}))
	if !strings.Contains(detail, "CORES") {
		t.Fatalf("zoomed body missing the cores section:\n%s", detail)
	}
	// Every line stays within the panel, at any width.
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "system", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestSystemPanelKeepsLastGoodReading(t *testing.T) {
	good := tideui.SystemMetrics{CPUPercent: 42, MemoryUsed: "1 GB", MemoryTotal: "2 GB"}
	calls := 0
	panel := &system{fetch: func(context.Context) (tideui.SystemMetrics, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return tideui.SystemMetrics{}, context.DeadlineExceeded
	}}

	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if panel.Load().CPUPercent != 42 {
		t.Fatalf("refresh did not store: %#v", panel.Load())
	}
	// A failed sample reports the error and leaves the last good reading on
	// screen, so the panel never blanks out.
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if panel.Load().CPUPercent != 42 {
		t.Fatalf("a failed refresh discarded good data: %#v", panel.Load())
	}
}

// The deck must be able to drive the panel with no special-casing.
func TestSystemPanelThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(System())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("system")
	if !ok {
		t.Fatal("system was not attached to the workspace")
	}
	if registered.TitleText() != "System" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "r" {
		t.Fatalf("actions = %#v", actions)
	}
	if badge, ok := deck.Badges()["system"]; !ok || badge.Text != "healthy" || badge.Tone != tideui.ToneGood {
		t.Fatalf("badge = %#v", badge)
	}
	// Demo mode fills the panel through the deck's tick alone.
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "system", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "CPU") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}

// On Linux the real reader is exercised end to end: the first sample reports
// 0% CPU (a delta needs two), so two refreshes are taken and the second is
// checked to be a sane percentage.
func TestSystemPanelReadsProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system metrics are linux-only")
	}
	panel := &system{fetch: provider.System(), started: time.Now()}
	for i := 0; i < 2; i++ {
		if err := panel.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if cpu := panel.Load().CPUPercent; cpu < 0 || cpu > 100 {
		t.Fatalf("cpu percent = %v", cpu)
	}
}
