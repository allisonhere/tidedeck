package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/provider"
)

// dataSource is what the panel views read. The deterministic demo feed and the
// live provider dashboard both satisfy it, so the UI is identical whether the
// data is simulated or real.
type dataSource interface {
	Weather(time.Time) tideui.WeatherData
	Clock(time.Time) tideui.ClockData
	Agenda(time.Time, int) []tideui.AgendaItem
	System(time.Time) tideui.SystemMetrics
	Network(time.Time) tideui.NetworkMetrics
	Markets(time.Time) []tideui.MarketQuote
}

// liveSource adapts a provider.Dashboard to the dataSource interface. It reads
// only from the latest cached snapshot, so panel rendering never blocks on
// network or disk; the tick refreshes providers in the background.
type liveSource struct {
	dashboard *provider.Dashboard
	mu        sync.RWMutex
	snapshot  provider.Snapshot
}

// newLiveSource builds the provider dashboard from the application config.
// Sources with no configuration are simply left unset and their panels stay
// empty.
func newLiveSource(cfg config) *liveSource {
	location := strings.TrimSpace(cfg.Weather.Location)
	if location == "" {
		location = "Local"
	}
	dashboard := &provider.Dashboard{
		Clock:   provider.NewFetcher(time.Minute, provider.Clock(location, list(cfg.Zones)...)),
		System:  provider.NewFetcher(time.Second, provider.System()),
		Network: provider.NewFetcher(time.Second, provider.Network(strings.TrimSpace(cfg.Interface))),
		Storage: provider.NewFetcher(2*time.Minute, provider.Storage()),
	}
	if cfg.Weather.usable() {
		dashboard.Weather = provider.NewFetcher(10*time.Minute, provider.Weather(provider.WeatherOptions{
			Latitude:   cfg.Weather.Latitude,
			Longitude:  cfg.Weather.Longitude,
			Location:   location,
			Fahrenheit: cfg.Weather.Fahrenheit,
			WindMPH:    cfg.Weather.WindMPH,
		}))
	}
	if feeds := list(cfg.Feeds); len(feeds) > 0 {
		dashboard.Headlines = provider.NewFetcher(5*time.Minute, provider.Feed(feeds...))
	}
	if repos := list(cfg.Repos); len(repos) > 0 {
		dashboard.Repos = provider.NewFetcher(time.Minute, provider.Git(repos...))
	}
	if todo := expandPath(strings.TrimSpace(cfg.Todo)); todo != "" {
		dashboard.Tasks = provider.NewFetcher(30*time.Second, provider.TodoTxt(todo))
	}
	if notes := list(cfg.Notes); len(notes) > 0 {
		dashboard.Notes = provider.NewFetcher(30*time.Second, provider.Notes(notes...))
	}
	if calendars := list(cfg.Calendars); len(calendars) > 0 {
		dashboard.Agenda = provider.NewFetcher(time.Minute, provider.Calendar(calendars...))
	}
	if symbols := list(cfg.Symbols); len(symbols) > 0 {
		dashboard.Markets = provider.NewFetcher(time.Minute, provider.Markets(symbols...))
	}
	if units := list(cfg.Systemd); len(units) > 0 {
		dashboard.Services = provider.NewFetcher(15*time.Second, provider.Systemd(units...))
	} else if socket := strings.TrimSpace(cfg.Docker); socket != "" {
		if socket == "1" {
			socket = ""
		}
		dashboard.Services = provider.NewFetcher(15*time.Second, provider.Docker(socket))
	}
	return &liveSource{dashboard: dashboard}
}

func (s *liveSource) refresh(ctx context.Context) {
	s.dashboard.Refresh(ctx)
	snapshot := s.dashboard.Snapshot()
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
}

func (s *liveSource) snapshotCopy() provider.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *liveSource) Weather(now time.Time) tideui.WeatherData {
	snapshot := s.snapshotCopy()
	if snapshot.Weather != nil {
		data := *snapshot.Weather
		data.Updated = now
		return data
	}
	return tideui.WeatherData{Location: "loading", Condition: "…", Unit: "F"}
}

func (s *liveSource) Clock(now time.Time) tideui.ClockData {
	snapshot := s.snapshotCopy()
	if snapshot.Clock != nil {
		data := *snapshot.Clock
		data.Local = now
		return data
	}
	return tideui.ClockData{Local: now}
}

func (s *liveSource) Agenda(time.Time, int) []tideui.AgendaItem {
	return s.snapshotCopy().Agenda
}

func (s *liveSource) System(time.Time) tideui.SystemMetrics {
	if snapshot := s.snapshotCopy(); snapshot.System != nil {
		return *snapshot.System
	}
	return tideui.SystemMetrics{}
}

func (s *liveSource) Network(time.Time) tideui.NetworkMetrics {
	if snapshot := s.snapshotCopy(); snapshot.Network != nil {
		return *snapshot.Network
	}
	return tideui.NetworkMetrics{Unit: "Mbps"}
}

func (s *liveSource) Markets(time.Time) []tideui.MarketQuote {
	return s.snapshotCopy().Markets
}
