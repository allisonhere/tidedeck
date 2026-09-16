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

func (f *demoFeed) Network(now time.Time) tideui.NetworkMetrics {
	t := f.elapsed(now)
	down := clampRange(87+35*wave(t, 8, 0), 0, 950)
	up := clampRange(14+9*wave(t, 6, 1), 0, 400)
	return tideui.NetworkMetrics{
		Interface: "wlan0",
		Download:  down,
		Upload:    up,
		Unit:      "Mbps",
		DownSpark: series(t, 24, 5, 0, 0.4, 0.45),
		UpSpark:   series(t, 24, 7, 1, 0.25, 0.3),
		LAN:       "940 Mbps",
		WAN:       "87↓ / 14↑ Mbps",
	}
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

func (f *demoFeed) Storage() []tideui.StorageMount {
	return []tideui.StorageMount{
		{Path: "/", UsedPercent: 72, Used: "460 GB", Total: "640 GB"},
		{Path: "/home", UsedPercent: 48, Used: "384 GB", Total: "800 GB"},
		{Path: "/media", UsedPercent: 81, Used: "5.8 TB", Total: "7.2 TB"},
	}
}

func (f *demoFeed) Services() []tideui.ServiceStatus {
	return []tideui.ServiceStatus{
		{Name: "jellyfin", Status: tideui.StatusHealthy, Age: "3d", Detail: "HTTP 200", Uptime: "3d 14h"},
		{Name: "postgres", Status: tideui.StatusHealthy, Age: "12d", Detail: "12 connections", Uptime: "12d"},
		{Name: "forgejo", Status: tideui.StatusWarning, Age: "6d", Detail: "high memory", Uptime: "6d 2h"},
		{Name: "backup", Status: tideui.StatusStopped, Age: "--", Detail: "last run 2d ago"},
		{Name: "caddy", Status: tideui.StatusHealthy, Age: "24d", Detail: "3 sites", Uptime: "24d"},
		{Name: "redis", Status: tideui.StatusActive, Age: "9d", Detail: "cache warm", Uptime: "9d"},
		{Name: "syncthing", Status: tideui.StatusUpdating, Age: "2m", Detail: "scanning", Uptime: "2m"},
	}
}

func (f *demoFeed) Headlines() []tideui.Headline {
	return []tideui.Headline{
		{Title: "Linux 6.12 released", Source: "kernel.org", Age: "18m", Unread: true, Tone: tideui.ToneAccent},
		{Title: "New Rust TUI framework", Source: "GitHub", Age: "42m", Unread: true, Tone: tideui.ToneAccent},
		{Title: "Arch update lands", Source: "archlinux.org", Age: "1h", Tone: tideui.ToneMuted},
		{Title: "SQLite 3.47 ships", Source: "sqlite.org", Age: "2h", Tone: tideui.ToneMuted},
		{Title: "Go 1.24 beta available", Source: "go.dev", Age: "3h", Tone: tideui.ToneMuted},
		{Title: "Rust 1.83 released", Source: "blog.rust-lang.org", Age: "4h", Tone: tideui.ToneMuted},
		{Title: "Wayland 1.24 planned", Source: "phoronix", Age: "5h", Tone: tideui.ToneMuted},
		{Title: "Kubernetes 1.32 ships", Source: "k8s.io", Age: "7h", Tone: tideui.ToneMuted},
	}
}

func (f *demoFeed) Tasks() []tideui.Task {
	return []tideui.Task{
		{Title: "Finish TideDeck", Due: "today", Tone: tideui.ToneWarning, Tags: []string{"#tide"}},
		{Title: "Update docs", Due: "tomorrow", Tone: tideui.ToneAccent, Tags: []string{"#docs"}},
		{Title: "Fix panel spacing", Done: true},
		{Title: "Record demo GIF", Due: "Thu", Tone: tideui.ToneAccent},
	}
}

func (f *demoFeed) Notes() []tideui.Note {
	return []tideui.Note{
		{Title: "Remember", Pinned: true, Body: "- test narrow layouts\n- record demo GIF\n- add real data providers"},
		{Title: "Ideas", Body: "- weather provider\n- calendar sync\n- market watchlist"},
	}
}
