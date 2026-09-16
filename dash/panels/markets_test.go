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

func TestMarketsPanelMeta(t *testing.T) {
	meta := Markets().Meta()
	if meta.ID != "markets" || meta.Title != "Markets" {
		t.Fatalf("meta = %#v", meta)
	}
	if meta.MinWidth != 18 || meta.MinHeight != 6 || meta.HideBelow != 80 || meta.Priority != 50 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Minute {
		t.Fatalf("interval = %v", meta.Interval)
	}
	// The panel ships its own palette, the way the hand-written registration did.
	if meta.Theme.Name != tideui.GruvboxLight.Name {
		t.Fatalf("theme = %q, want %q", meta.Theme.Name, tideui.GruvboxLight.Name)
	}
}

// With no symbols configured the panel falls back to the default watchlist, so
// it is not blank on a fresh install; configured symbols replace it.
func TestMarketsConfigureFallsBackToDefaults(t *testing.T) {
	panel := Markets().(*markets)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("no configured symbols should still build the default watchlist")
	}
	values := dash.NewValues()
	values.Set(symbolsKey, "AMD,NVDA")
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("configured symbols should build a fetcher")
	}
	// The declared default is what the settings row shows when the key is absent.
	field := Markets().(*markets).Schema()[0]
	if field.Default != defaultSymbols {
		t.Fatalf("schema default = %q, want %q", field.Default, defaultSymbols)
	}
}

func TestMarketsPanelRendersItsOwnData(t *testing.T) {
	panel := Markets().(*markets)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "markets", Width: 30, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"AMD", "NVDA", "SPY"} {
		if !strings.Contains(body, want) {
			t.Fatalf("markets body missing %q:\n%s", want, body)
		}
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "markets", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestMarketsKeepsLastGoodReading(t *testing.T) {
	good := []tideui.MarketQuote{{Symbol: "AMD", Price: 162.4}}
	calls := 0
	panel := &markets{}
	panel.fetch = func(context.Context) ([]tideui.MarketQuote, error) {
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
	if got := panel.Load(); len(got) != 1 || got[0].Symbol != "AMD" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

// The deck attaches the panel and applies its declared theme.
func TestMarketsThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Markets())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("markets")
	if !ok {
		t.Fatal("markets was not attached to the workspace")
	}
	if actions := registered.ActionList(); len(actions) != 1 || actions[0].Key != "r" {
		t.Fatalf("actions = %#v", actions)
	}
	theme, has := registered.PanelTheme()
	if !has || theme.Name != tideui.GruvboxLight.Name {
		t.Fatalf("panel theme = %q,%v want gruvbox-light", theme.Name, has)
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "markets", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "AMD") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
}
