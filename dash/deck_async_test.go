package dash

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Start hands out the work without doing it, so an interactive caller can run
// it off its UI goroutine; nothing is fetched until a job runs.
func TestStartDefersTheWork(t *testing.T) {
	deck := New()
	deck.SetMode(ModeLive)
	panel := newFake("a", time.Minute)
	deck.Register(panel)
	jobs := deck.Start(context.Background(), time.Now())
	if len(jobs) != 1 || panel.refresh != 0 {
		t.Fatalf("jobs %d, fetched %d before any job ran", len(jobs), panel.refresh)
	}
	r := jobs[0]()
	if r.ID != "a" || panel.refresh != 1 {
		t.Fatalf("job reported %+v, fetched %d", r, panel.refresh)
	}
	deck.Finish(r)
	if deck.Refreshing("a") {
		t.Error("a finished refresh is still in flight")
	}
}

// A panel still refreshing is not started again, however many ticks pass: a
// slow source is waited for, not piled up on.
func TestASlowPanelIsNotStartedTwice(t *testing.T) {
	deck := New()
	deck.SetMode(ModeLive)
	deck.Register(newFake("a", time.Second))
	now := time.Now()
	first := deck.Start(context.Background(), now)
	if len(first) != 1 {
		t.Fatalf("first tick started %d", len(first))
	}
	for i := 1; i <= 5; i++ {
		if again := deck.Start(context.Background(), now.Add(time.Duration(i)*time.Minute)); len(again) != 0 {
			t.Fatalf("tick %d restarted a panel still refreshing", i)
		}
	}
	deck.Finish(first[0]())
	if len(deck.Start(context.Background(), now.Add(10*time.Minute))) != 1 {
		t.Fatal("a finished panel was not started on its next interval")
	}
}

// A refresh asked for while one is running (a pane resized mid-fetch) is not
// lost: the panel is due again as soon as the running one finishes, rather
// than at its next interval.
func TestARefreshAskedMidFlightRunsNext(t *testing.T) {
	deck := New()
	deck.SetMode(ModeLive)
	deck.Register(newFake("a", time.Hour))
	now := time.Now()
	jobs := deck.Start(context.Background(), now)
	if deck.StartPanel(context.Background(), "a") != nil {
		t.Fatal("a second job was handed out for a panel in flight")
	}
	deck.Finish(jobs[0]())
	if len(deck.Start(context.Background(), now.Add(time.Second))) != 1 {
		t.Fatal("the refresh asked for mid-flight was dropped until the hour was up")
	}
}

// A job's error is recorded when it is reported, and one for a panel removed
// meanwhile is ignored.
func TestFinishRecordsErrors(t *testing.T) {
	deck := New()
	deck.SetMode(ModeLive)
	deck.Register(newFake("a", time.Minute), newFake("b", time.Minute))
	deck.Start(context.Background(), time.Now())
	boom := errors.New("unreachable")
	deck.Finish(Refreshed{ID: "a", Err: boom})
	if !errors.Is(deck.Err("a"), boom) {
		t.Fatalf("err = %v", deck.Err("a"))
	}
	deck.Unregister("b")
	deck.Finish(Refreshed{ID: "b", Err: boom})
	if deck.Err("b") != nil {
		t.Error("an error was recorded for a panel no longer registered")
	}
}
