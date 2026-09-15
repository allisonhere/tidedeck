package tideui

import "time"

// Data models for the first-party dashboard widgets. They are deliberately
// plain values: renderers consume them, so a real provider can populate them
// later without changing any rendering code.

// ForecastPoint is one entry in an hourly or daily forecast.
type ForecastPoint struct {
	Label       string // "3PM", "Mon"
	Temperature int
	Condition   string
	RainChance  int
}

// WeatherData backs the weather widget.
type WeatherData struct {
	Location    string
	Temperature int
	Unit        string // "F" or "C"
	Condition   string
	High        int
	Low         int
	RainChance  int
	WindSpeed   int
	WindUnit    string
	Hourly      []ForecastPoint
	Daily       []ForecastPoint
	Updated     time.Time
}

// AgendaItem is one calendar event.
type AgendaItem struct {
	Title    string
	Start    time.Time
	End      time.Time
	Location string
	Category string
	Tone     Tone
	Done     bool
}

// WorldClock is one city in the clock widget.
type WorldClock struct {
	City   string
	Time   time.Time
	Offset string // "+9", "-5"
}

// ClockData backs the clock widget.
type ClockData struct {
	Local    time.Time
	Location string
	Zones    []WorldClock
}

// SystemMetrics backs the system-health widget.
type SystemMetrics struct {
	CPUPercent    float64
	CPUSpark      []float64
	Cores         []float64
	MemoryPercent float64
	MemoryUsed    string
	MemoryTotal   string
	TemperatureC  int
	Load          [3]float64
	Uptime        time.Duration
	Processes     int
}

// NetworkMetrics backs the network widget.
type NetworkMetrics struct {
	Interface string
	Download  float64
	Upload    float64
	Unit      string
	DownSpark []float64
	UpSpark   []float64
	LAN       string
	WAN       string
}

// StorageMount is one mounted filesystem.
type StorageMount struct {
	Path        string
	UsedPercent float64
	Used        string
	Total       string
	Tone        Tone
}

// ServiceStatus is one container or generic service. Status carries the
// shared dashboard vocabulary; State/Tone are optional display overrides for
// providers that want a custom word or colour.
type ServiceStatus struct {
	Name   string
	Status StatusKind
	Age    string // short age, e.g. "3d", "2m", "--"
	Detail string
	Uptime string
	State  string
	Tone   Tone
}

// resolved returns the effective status kind, word, and tone.
func (s ServiceStatus) resolved() (StatusKind, string, Tone) {
	kind := s.Status
	label := s.State
	if label == "" {
		label = kind.Label()
	}
	tone := s.Tone
	if tone == ToneNeutral {
		tone = kind.Tone()
	}
	return kind, label, tone
}

// Headline is one news or RSS item.
type Headline struct {
	Title  string
	Source string
	Age    string
	Unread bool
	Tone   Tone
}

// Task is one task-list item.
type Task struct {
	Title string
	Done  bool
	Due   string
	Tags  []string
	Tone  Tone
}

// Note is one note.
type Note struct {
	Title  string
	Body   string
	Pinned bool
}

// RepoActivity is one repository's activity summary.
type RepoActivity struct {
	Name    string
	Branch  string
	Summary string
	Commits int
	Tone    Tone
}

// MarketQuote is one watchlist entry.
type MarketQuote struct {
	Symbol    string
	Price     float64
	ChangePct float64
	Currency  string
}
