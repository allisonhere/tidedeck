package main

import (
	"math"
	"time"

	"github.com/allisonhere/tideui"
)

// demoFeed is a deterministic fake data source. Everything it produces is a
// pure function of the seed and the observation time, so the dashboard is
// lively but reproducible and tests can pin a moment. No network is involved.
type demoFeed struct {
	seed    int64
	started time.Time
}

func newDemoFeed(seed int64, started time.Time) *demoFeed {
	return &demoFeed{seed: seed, started: started}
}

func (f *demoFeed) elapsed(now time.Time) float64 {
	return now.Sub(f.started).Seconds() + float64(f.seed)
}

func wave(t, period, phase float64) float64 { return math.Sin(t/period + phase) }

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp01(v float64) float64 { return clampRange(v, 0, 1) }

// series samples a smooth signal into n buckets ending at t.
func series(t float64, n int, period, phase, base, amp float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := t - float64(n-i)
		out[i] = clamp01(base + amp*wave(x, period, phase+float64(i)*0.35))
	}
	return out
}

func (f *demoFeed) Markets(now time.Time) []tideui.MarketQuote {
	t := f.elapsed(now)
	quote := func(symbol string, price, drift, phase float64) tideui.MarketQuote {
		p := price + drift*wave(t, 30, phase)
		return tideui.MarketQuote{
			Symbol:    symbol,
			Price:     math.Round(p*100) / 100,
			ChangePct: math.Round((drift/price*100*wave(t, 30, phase))*10) / 10,
		}
	}
	return []tideui.MarketQuote{
		quote("AMD", 162.40, 2.2, 0),
		quote("NVDA", 214.10, -1.4, 1),
		quote("SPY", 612.32, 1.1, 2),
		quote("GOOG", 187.65, 0.9, 3),
	}
}

// Static collections live in the demo state so actions can mutate them while
// still originating from the same deterministic feed.

func (f *demoFeed) Notes() []tideui.Note {
	return []tideui.Note{
		{Title: "Remember", Pinned: true, Body: "- test narrow layouts\n- record demo GIF\n- add real data providers"},
		{Title: "Ideas", Body: "- weather provider\n- calendar sync\n- market watchlist"},
	}
}
