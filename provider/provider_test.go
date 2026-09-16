package provider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestFetcherCachesUntilStale(t *testing.T) {
	var calls int32
	fetcher := NewFetcher(40*time.Millisecond, func(context.Context) (int, error) {
		return int(atomic.AddInt32(&calls, 1)), nil
	})
	fetcher.Refresh(context.Background())
	waitFor(t, func() bool { _, _, ok := fetcher.Value(); return ok })
	first, _, _ := fetcher.Value()

	fetcher.Refresh(context.Background())
	time.Sleep(10 * time.Millisecond)
	if got, _, _ := fetcher.Value(); got != first {
		t.Fatalf("refreshed before stale: %d -> %d", first, got)
	}

	time.Sleep(50 * time.Millisecond)
	fetcher.Refresh(context.Background())
	waitFor(t, func() bool { got, _, _ := fetcher.Value(); return got != first })
}

func TestFetcherKeepsValueOnError(t *testing.T) {
	var fail atomic.Bool
	fetcher := NewFetcher(0, func(context.Context) (int, error) {
		if fail.Load() {
			return 0, errors.New("boom")
		}
		return 7, nil
	})
	fetcher.Refresh(context.Background())
	waitFor(t, func() bool { _, _, ok := fetcher.Value(); return ok })

	fail.Store(true)
	fetcher.Refresh(context.Background())
	waitFor(t, func() bool { _, err, _ := fetcher.Value(); return err != nil })
	if value, _, _ := fetcher.Value(); value != 7 {
		t.Fatalf("value = %d, want previous 7", value)
	}
}

func TestNilFetcherIsSafe(t *testing.T) {
	var fetcher *Fetcher[int]
	fetcher.Refresh(context.Background())
	if _, _, ok := fetcher.Value(); ok {
		t.Fatal("nil fetcher reported a value")
	}
}
