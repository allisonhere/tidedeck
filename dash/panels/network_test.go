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

func TestNetworkPanelMeta(t *testing.T) {
	meta := Network().Meta()
	if meta.ID != "network" || meta.Title != "Network" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 18 || meta.MinHeight != 7 || meta.HideBelow != 104 || meta.Priority != 70 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

// An interface setting rebuilds the source; an unchanged or empty one keeps the
// rolling history alive.
func TestNetworkConfigureRebuildsOnChange(t *testing.T) {
	panel := Network().(*network)
	builds := 0
	panel.newFetcher = func(string) func(context.Context) (tideui.NetworkMetrics, error) {
		builds++
		return func(context.Context) (tideui.NetworkMetrics, error) { return tideui.NetworkMetrics{}, nil }
	}
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if builds != 0 {
		t.Fatalf("an unchanged interface rebuilt the source %d times", builds)
	}
	values := dash.NewValues()
	values.Set(interfaceKey, "wlan0")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("a changed interface should rebuild once, got %d", builds)
	}
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("reapplying the same interface should not rebuild, got %d", builds)
	}
}

func TestNetworkPanelRendersItsOwnData(t *testing.T) {
	panel := Network().(*network)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "network", Width: 34, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"wlan0", "Mbps"} {
		if !strings.Contains(body, want) {
			t.Fatalf("network body missing %q:\n%s", want, body)
		}
	}
	detail := ansi.Strip(panel.View(tideui.PanelContext{ID: "network", Width: 40, Zoomed: true, Renderer: renderer()}))
	if !strings.Contains(detail, "LAN") {
		t.Fatalf("zoomed body missing the LAN/WAN summary:\n%s", detail)
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "network", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestNetworkKeepsLastGoodReading(t *testing.T) {
	good := tideui.NetworkMetrics{Interface: "wlan0", Download: 87, Unit: "Mbps"}
	calls := 0
	panel := &network{}
	panel.fetch = func(context.Context) (tideui.NetworkMetrics, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return tideui.NetworkMetrics{}, context.DeadlineExceeded
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if got := panel.Load().Download; got != 87 {
		t.Fatalf("a failed refresh discarded good data: %v", got)
	}
}

func TestNetworkThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Network())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("network")
	if !ok {
		t.Fatal("network was not attached to the workspace")
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "r" {
		t.Fatalf("actions = %#v", actions)
	}
	if badge, ok := deck.Badges()["network"]; !ok || badge.Text != "up" || badge.Tone != tideui.ToneGood {
		t.Fatalf("badge = %#v", badge)
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "network", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "Mbps") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
