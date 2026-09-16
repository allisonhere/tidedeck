package main

import (
	"context"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/provider"
)

// dataSource is what the panel views read. The deterministic demo feed and the
// live provider dashboard both satisfy it, so the UI is identical whether the
// data is simulated or real.
type dataSource interface {
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
	dashboard := &provider.Dashboard{}
	if notes := list(cfg.Notes); len(notes) > 0 {
		dashboard.Notes = provider.NewFetcher(30*time.Second, provider.Notes(notes...))
	}
	if symbols := list(cfg.Symbols); len(symbols) > 0 {
		dashboard.Markets = provider.NewFetcher(time.Minute, provider.Markets(symbols...))
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

func (s *liveSource) Markets(time.Time) []tideui.MarketQuote {
	return s.snapshotCopy().Markets
}
