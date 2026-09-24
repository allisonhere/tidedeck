package panels

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// symbolsKey is the configuration key this panel owns: the ticker symbols.
const symbolsKey = "symbols"

// defaultSymbols is the watchlist used when none is configured, so the panel
// is useful out of the box the way the clock has default zones.
const defaultSymbols = "AMD,NVDA,SPY,GOOG"

// markets shows a quote and day change for each configured symbol. It ships
// with its own contrasting theme, to show that a panel can opt out of the
// workspace palette entirely.
type markets struct {
	dash.State[[]tideui.MarketQuote]
	// mu guards fetch, which Configure replaces on the UI goroutine while a
	// refresh may be reading it in the background.
	mu    sync.Mutex
	fetch func(context.Context) ([]tideui.MarketQuote, error)
}

// Markets builds the markets panel.
func Markets() dash.Panel { return &markets{} }

func (m *markets) Meta() dash.Meta {
	return dash.Meta{
		ID: "markets", Title: "Markets",
		Role: tideui.RoleOptional, Priority: 50,
		// Keep the watchlist available on normal terminals; the adaptive
		// workspace hides optional panels only once the terminal is genuinely
		// narrow.
		MinWidth: 18, MinHeight: 6, HideBelow: 80,
		Interval: time.Minute,
		Theme:    tideui.GruvboxLight,
	}
}

func (m *markets) Schema() []dash.Field {
	return []dash.Field{{
		Key: symbolsKey, Label: "symbols", Kind: dash.FieldText, Default: defaultSymbols,
	}}
}

// Configure builds the source from the configured symbols, falling back to the
// default watchlist when none is set, so the panel is not blank on a fresh
// install. An explicit empty value is treated the same as unset: there is no
// way to ask for no quotes and have anything to show.
func (m *markets) Configure(values dash.Values) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	symbols := values.List(symbolsKey)
	if len(symbols) == 0 {
		symbols = strings.Split(defaultSymbols, ",")
	}
	m.fetch = provider.Markets(symbols...)
	return nil
}

func (m *markets) Refresh(ctx context.Context) error {
	m.mu.Lock()
	fetch := m.fetch
	m.mu.Unlock()
	if fetch == nil {
		return nil
	}
	quotes, err := fetch(ctx)
	if err != nil {
		return err
	}
	m.Store(quotes)
	return nil
}

func (m *markets) View(ctx tideui.PanelContext) string {
	if ctx.Zoomed {
		return ctx.Renderer.RenderMarketsDetail(m.Load(), ctx.Width)
	}
	return ctx.Renderer.RenderMarkets(m.Load(), ctx.Width)
}

// Demo synthesises a few drifting quotes, so the panel looks alive before any
// symbols are configured.
func (m *markets) Demo(now time.Time) {
	t := float64(now.UnixNano()) / float64(time.Second)
	quote := func(symbol string, price, drift, phase float64) tideui.MarketQuote {
		p := price + drift*wave(t, 30, phase)
		return tideui.MarketQuote{
			Symbol:    symbol,
			Price:     math.Round(p*100) / 100,
			High:      math.Round((p+math.Abs(drift)*0.8)*100) / 100,
			Low:       math.Round((p-math.Abs(drift)*0.8)*100) / 100,
			ChangePct: math.Round((drift/price*100*wave(t, 30, phase))*10) / 10,
		}
	}
	m.Store([]tideui.MarketQuote{
		quote("AMD", 162.40, 2.2, 0),
		quote("NVDA", 214.10, -1.4, 1),
		quote("SPY", 612.32, 1.1, 2),
		quote("GOOG", 187.65, 0.9, 3),
	})
}

func (m *markets) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
		Run: func() string { return "quotes refreshed" },
	}}
}
