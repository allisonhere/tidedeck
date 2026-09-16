// Package provider contains real data sources for the TideUI dashboard
// widgets. Rendering consumes the tideui data models; these types acquire them
// from the system, local files, or HTTP endpoints.
//
// Every source is optional and independent, and a failing fetch keeps the
// previous value while the rest of the dashboard keeps working. A panel on the
// dash registry owns its source and calls it on its own interval, so a slow
// network call never blocks the UI and a cheap local sample can refresh every
// second while a remote feed refreshes every five minutes.
//
// The package uses only the standard library.
package provider

import (
	"context"
	"sync"
	"time"
)

// defaultTimeout bounds a single fetch.
const defaultTimeout = 12 * time.Second

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
