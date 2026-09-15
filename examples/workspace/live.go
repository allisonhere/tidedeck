package main

import (
	"context"
	"os"
	"strconv"
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

func newLiveSource() *liveSource {
	dashboard := &provider.Dashboard{
		Clock:   provider.NewFetcher(time.Minute, provider.Clock(envOr("TIDEDECK_LOCATION", "Local"), envList("TIDEDECK_ZONES")...)),
		System:  provider.NewFetcher(time.Second, provider.System()),
		Network: provider.NewFetcher(time.Second, provider.Network(envOr("TIDEDECK_IFACE", ""))),
		Storage: provider.NewFetcher(2*time.Minute, provider.Storage()),
	}
	if latitude, longitude, ok := envCoordinates(); ok {
		dashboard.Weather = provider.NewFetcher(10*time.Minute, provider.Weather(provider.WeatherOptions{
			Latitude:   latitude,
			Longitude:  longitude,
			Location:   envOr("TIDEDECK_LOCATION", "Local"),
			Fahrenheit: !strings.EqualFold(envOr("TIDEDECK_UNITS", "f"), "c"),
			WindMPH:    !strings.EqualFold(envOr("TIDEDECK_WIND", "mph"), "kmh"),
		}))
	}
	if feeds := envList("TIDEDECK_FEEDS"); len(feeds) > 0 {
		dashboard.Headlines = provider.NewFetcher(5*time.Minute, provider.Feed(feeds...))
	}
	if repos := envList("TIDEDECK_REPOS"); len(repos) > 0 {
		dashboard.Repos = provider.NewFetcher(time.Minute, provider.Git(repos...))
	}
	if todo := envOr("TIDEDECK_TODO", ""); todo != "" {
		dashboard.Tasks = provider.NewFetcher(30*time.Second, provider.TodoTxt(todo))
	}
	if notes := envList("TIDEDECK_NOTES"); len(notes) > 0 {
		dashboard.Notes = provider.NewFetcher(30*time.Second, provider.Notes(notes...))
	}
	if calendars := envList("TIDEDECK_ICS"); len(calendars) > 0 {
		dashboard.Agenda = provider.NewFetcher(time.Minute, provider.Calendar(calendars...))
	}
	if symbols := envList("TIDEDECK_SYMBOLS"); len(symbols) > 0 {
		dashboard.Markets = provider.NewFetcher(time.Minute, provider.Markets(symbols...))
	}
	if units := envList("TIDEDECK_SYSTEMD"); len(units) > 0 {
		dashboard.Services = provider.NewFetcher(15*time.Second, provider.Systemd(units...))
	} else if socket := envOr("TIDEDECK_DOCKER", ""); socket != "" {
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

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	var values []string
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func envCoordinates() (float64, float64, bool) {
	latitudeRaw := strings.TrimSpace(os.Getenv("TIDEDECK_LAT"))
	longitudeRaw := strings.TrimSpace(os.Getenv("TIDEDECK_LON"))
	if latitudeRaw == "" || longitudeRaw == "" {
		return 0, 0, false
	}
	latitude, err1 := strconv.ParseFloat(latitudeRaw, 64)
	longitude, err2 := strconv.ParseFloat(longitudeRaw, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return latitude, longitude, true
}
