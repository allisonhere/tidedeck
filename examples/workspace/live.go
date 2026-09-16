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
	Agenda(time.Time, int) []tideui.AgendaItem
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
	dashboard := &provider.Dashboard{
		Network: provider.NewFetcher(time.Second, provider.Network(strings.TrimSpace(cfg.Interface))),
		Storage: provider.NewFetcher(2*time.Minute, provider.Storage()),
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

func (s *liveSource) Agenda(now time.Time, dayOffset int) []tideui.AgendaItem {
	items := s.snapshotCopy().Agenda
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, dayOffset)
	var upcoming []tideui.AgendaItem
	for _, item := range items {
		if agendaEndTime(item).After(from) {
			upcoming = append(upcoming, item)
		}
	}
	return upcoming
}

// agendaEndTime is when an event stops occupying the calendar: its End, its
// start for a timed event without one, or the next midnight for an all-day
// event, whose exclusive End may be absent.
func agendaEndTime(item tideui.AgendaItem) time.Time {
	if !item.End.IsZero() {
		return item.End
	}
	if item.AllDay {
		return item.Start.AddDate(0, 0, 1)
	}
	return item.Start
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
