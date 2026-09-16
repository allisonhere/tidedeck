// Package provider contains real data sources for the TideUI dashboard
// widgets. Rendering consumes the tideui data models; these types acquire them
// from the system, local files, or HTTP endpoints.
//
// Every source is optional and independent, and a Dashboard degrades
// gracefully when one fails: a failing fetch keeps the previous value and
// records the error, while the rest of the dashboard keeps working. Fetches run
// in the background with per-source intervals, so a slow network call never
// blocks the UI and a cheap local sample can refresh every second while a
// remote feed refreshes every five minutes.
//
// A panel on the dash registry holds its own source instead, so a Dashboard is
// only needed for sources whose panel has not moved across yet.
//
// The package uses only the standard library.
package provider

import (
	"context"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
)

// defaultTimeout bounds a single fetch.
const defaultTimeout = 12 * time.Second

// Snapshot is the latest value of every configured source. Pointer fields are
// nil until the first successful fetch; Errors carries the most recent failure
// keyed by source name.
type Snapshot struct {
	Network   *tideui.NetworkMetrics
	Storage   []tideui.StorageMount
	Services  []tideui.ServiceStatus
	Headlines []tideui.Headline
	Tasks     []tideui.Task
	Notes     []tideui.Note
	Repos     []tideui.RepoActivity
	Markets   []tideui.MarketQuote

	Updated time.Time
	Errors  map[string]error
}

// Fetcher caches one source, refreshing it in the background when stale. A nil
// *Fetcher is a no-op, so optional sources need no special casing.
type Fetcher[T any] struct {
	every   time.Duration
	timeout time.Duration
	fetch   func(context.Context) (T, error)

	mu       sync.RWMutex
	value    T
	err      error
	last     time.Time
	have     bool
	inflight bool
}

// NewFetcher wraps a fetch function with a refresh interval. An interval of
// zero refreshes on every call.
func NewFetcher[T any](every time.Duration, fetch func(context.Context) (T, error)) *Fetcher[T] {
	return &Fetcher[T]{every: every, timeout: defaultTimeout, fetch: fetch}
}

// Refresh starts a background fetch when the cached value is stale and no
// fetch is already running. It never blocks.
func (f *Fetcher[T]) Refresh(ctx context.Context) {
	if f == nil || f.fetch == nil {
		return
	}
	f.mu.Lock()
	if f.inflight || (f.have && f.every > 0 && time.Since(f.last) < f.every) {
		f.mu.Unlock()
		return
	}
	f.inflight = true
	f.mu.Unlock()

	timeout := f.timeout
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		value, err := f.fetch(runCtx)
		f.mu.Lock()
		if err == nil {
			f.value = value
			f.have = true
		}
		f.err = err
		f.last = time.Now()
		f.inflight = false
		f.mu.Unlock()
		_ = ctx // the caller's context does not outlive the UI tick
	}()
}

// Value returns the cached value, the last error, and whether a value has been
// fetched at least once.
func (f *Fetcher[T]) Value() (T, error, bool) {
	if f == nil {
		var zero T
		return zero, nil, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.value, f.err, f.have
}

// Dashboard bundles the available sources. Any field may be nil.
type Dashboard struct {
	Network   *Fetcher[tideui.NetworkMetrics]
	Storage   *Fetcher[[]tideui.StorageMount]
	Services  *Fetcher[[]tideui.ServiceStatus]
	Headlines *Fetcher[[]tideui.Headline]
	Tasks     *Fetcher[[]tideui.Task]
	Notes     *Fetcher[[]tideui.Note]
	Repos     *Fetcher[[]tideui.RepoActivity]
	Markets   *Fetcher[[]tideui.MarketQuote]
}

// Refresh kicks off any stale fetches. Call it from the application tick.
func (d *Dashboard) Refresh(ctx context.Context) {
	if d == nil {
		return
	}
	d.Network.Refresh(ctx)
	d.Storage.Refresh(ctx)
	d.Services.Refresh(ctx)
	d.Headlines.Refresh(ctx)
	d.Tasks.Refresh(ctx)
	d.Notes.Refresh(ctx)
	d.Repos.Refresh(ctx)
	d.Markets.Refresh(ctx)
}

// Snapshot reads the cached values without blocking on network or disk.
func (d *Dashboard) Snapshot() Snapshot {
	snap := Snapshot{Updated: time.Now(), Errors: map[string]error{}}
	if d == nil {
		return snap
	}
	if value, ok := read(d.Network, snap.Errors, "network"); ok {
		snap.Network = &value
	}
	if value, ok := read(d.Storage, snap.Errors, "storage"); ok {
		snap.Storage = value
	}
	if value, ok := read(d.Services, snap.Errors, "services"); ok {
		snap.Services = value
	}
	if value, ok := read(d.Headlines, snap.Errors, "headlines"); ok {
		snap.Headlines = value
	}
	if value, ok := read(d.Tasks, snap.Errors, "tasks"); ok {
		snap.Tasks = value
	}
	if value, ok := read(d.Notes, snap.Errors, "notes"); ok {
		snap.Notes = value
	}
	if value, ok := read(d.Repos, snap.Errors, "repos"); ok {
		snap.Repos = value
	}
	if value, ok := read(d.Markets, snap.Errors, "markets"); ok {
		snap.Markets = value
	}
	return snap
}

func read[T any](fetcher *Fetcher[T], errors map[string]error, name string) (T, bool) {
	value, err, ok := fetcher.Value()
	if err != nil && name != "" {
		errors[name] = err
	}
	return value, ok
}
