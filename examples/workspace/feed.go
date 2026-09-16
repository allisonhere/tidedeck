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

func (f *demoFeed) Weather(now time.Time) tideui.WeatherData {
	t := f.elapsed(now)
	return tideui.WeatherData{
		Location:     "Springfield",
		Temperature:  int(math.Round(71 + 3*wave(t, 900, 0))),
		Unit:         "F",
		Condition:    "Partly Cloudy",
		FeelsLike:    int(math.Round(70 + 3*wave(t, 900, 0))),
		HasFeelsLike: true,
		High:         76,
		Low:          61,
		RainChance:   int(clampRange(12+8*wave(t, 420, 1), 0, 100)),
		WindSpeed:    int(clampRange(9+4*wave(t, 300, 2), 0, 45)),
		WindUnit:     "mph",
		Hourly: []tideui.ForecastPoint{
			{Label: "3PM", Temperature: 74, Condition: "Partly Cloudy", RainChance: 10},
			{Label: "6PM", Temperature: 70, Condition: "Cloudy", RainChance: 15},
			{Label: "9PM", Temperature: 64, Condition: "Cloudy", RainChance: 20},
			{Label: "12AM", Temperature: 61, Condition: "Fog", RainChance: 12},
		},
		Daily: []tideui.ForecastPoint{
			{Label: "Today", Temperature: 74, Condition: "Partly Cloudy"},
			{Label: "Tue", Temperature: 71, Condition: "Rain"},
			{Label: "Wed", Temperature: 68, Condition: "Cloudy"},
			{Label: "Thu", Temperature: 75, Condition: "Sunny"},
			{Label: "Fri", Temperature: 73, Condition: "Sunny"},
		},
		Updated: now,
	}
}

func (f *demoFeed) Agenda(now time.Time, dayOffset int) []tideui.AgendaItem {
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := func(day, hour, minute int) time.Time {
		return base.AddDate(0, 0, day).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	all := []tideui.AgendaItem{
		{Title: "Project review", Start: at(0, 9, 30), End: at(0, 10, 15), Location: "Meet", Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Dentist", Start: at(0, 11, 0), Location: "Clinic", Category: "Health", Tone: tideui.ToneGood},
		{Title: "Focus block", Start: at(0, 14, 0), End: at(0, 16, 0), Category: "Deep work", Tone: tideui.ToneAccent},
		{Title: "Team offsite", Start: at(1, 0, 0), AllDay: true, Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Standup", Start: at(1, 8, 0), Location: "Meet", Category: "Work", Tone: tideui.ToneAccent},
		{Title: "Gym", Start: at(1, 18, 30), Category: "Health", Tone: tideui.ToneGood},
		{Title: "Ship TideDeck", Start: at(2, 10, 0), Category: "Work", Tone: tideui.ToneWarning},
	}
	// Like a live calendar, the offset moves the starting day rather than
	// shifting the schedule, so "next" drops a day off the front.
	target := base.AddDate(0, 0, dayOffset)
	upcoming := make([]tideui.AgendaItem, 0, len(all))
	for _, item := range all {
		if !item.Start.Before(target) {
			upcoming = append(upcoming, item)
		}
	}
	return upcoming
}

func (f *demoFeed) Clock(now time.Time) tideui.ClockData {
	utc := now.UTC()
	return tideui.ClockData{
		Local:    now,
		Location: now.Format("MST"),
		Zones: []tideui.WorldClock{
			{City: "London", Time: utc, Offset: "UTC"},
			{City: "Tokyo", Time: utc.Add(9 * time.Hour), Offset: "+9"},
			{City: "Sydney", Time: utc.Add(10 * time.Hour), Offset: "+10"},
			{City: "New York", Time: utc.Add(-5 * time.Hour), Offset: "-5"},
		},
	}
}

func (f *demoFeed) GPU(now time.Time) tideui.GPUMetrics {
	t := f.elapsed(now)
	busy := clampRange(24+18*wave(t, 11, 1), 2, 99)
	return tideui.GPUMetrics{
		Name:         "amdgpu",
		BusyPercent:  busy,
		BusySpark:    series(t, 18, 6, 1, 0.3, 0.45),
		MemoryUsed:   "5.4 GB",
		MemoryTotal:  "8.0 GB",
		MemoryLabel:  "VRAM",
		MemoryFrac:   0.67,
		TemperatureC: int(clampRange(58+7*wave(t, 30, 2), 40, 92)),
		PowerWatts:   clampRange(42+16*wave(t, 13, 3), 8, 140),
		ClockMHz:     int(clampRange(1800+300*wave(t, 8, 4), 300, 2600)),
	}
}

func (f *demoFeed) System(now time.Time) tideui.SystemMetrics {
	t := f.elapsed(now)
	cpu := clampRange(18+10*wave(t, 9, 0)+4*wave(t, 3, 1), 3, 98)
	mem := clampRange(41+6*wave(t, 60, 2), 18, 94)
	cores := make([]float64, 8)
	for i := range cores {
		cores[i] = clampRange(cpu+9*wave(t, 5, float64(i)), 0, 100)
	}
	return tideui.SystemMetrics{
		CPUPercent:    cpu,
		CPUSpark:      series(t, 18, 7, 0, 0.35, 0.4),
		Cores:         cores,
		MemoryPercent: mem,
		MemoryUsed:    "13.1 GB",
		MemoryTotal:   "32 GB",
		TemperatureC:  54 + int(3*wave(t, 180, 0)),
		Load: [3]float64{
			1.4 + 0.5*wave(t, 30, 0),
			1.1 + 0.4*wave(t, 30, 1),
			0.9 + 0.3*wave(t, 30, 2),
		},
		Uptime:    3*24*time.Hour + 14*time.Hour + time.Duration(t)*time.Second,
		Processes: 312 + int(4*wave(t, 45, 1)),
	}
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

// Updates is the demo update status: a pending Omarchy bump and a couple of
// packages, so the panel has something to show without touching pacman.
func (f *demoFeed) Updates(now time.Time) tideui.UpdateStatus {
	return tideui.UpdateStatus{
		Omarchy:        "4.0.3-1",
		OmarchyPending: "4.0.4-1",
		Repo: []tideui.UpdatePackage{
			{Name: "omarchy", From: "4.0.3-1", To: "4.0.4-1"},
			{Name: "omarchy-settings", From: "4.0.3-1", To: "4.0.4-1"},
			{Name: "linux", From: "6.17.2.arch1-1", To: "6.17.4.arch1-1"},
		},
		AUR:     []tideui.UpdatePackage{{Name: "yay", From: "12.4.2-1", To: "12.5.0-1"}},
		Checked: now,
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

func (f *demoFeed) RepoActivity() []tideui.RepoActivity {
	return []tideui.RepoActivity{
		{Name: "tideui", Branch: "main", Summary: "3 today", Commits: 3, Tone: tideui.ToneAccent},
		{Name: "tidegit", Branch: "main", Summary: "clean", Tone: tideui.ToneGood},
		{Name: "tidemail", Branch: "feat/rules", Summary: "2 unpushed · 2 changed",
			Commits: 2, Ahead: 2, Changes: 2, Tone: tideui.ToneWarning},
		{Name: "z13control", Branch: "main", Summary: "6 unpushed", Ahead: 6, Tone: tideui.ToneWarning},
	}
}
