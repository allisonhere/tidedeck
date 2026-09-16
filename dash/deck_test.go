package dash

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

// fake is a panel that records what the deck asked of it.
type fake struct {
	State[string]
	meta      Meta
	refresh   int
	demo      int
	tick      int
	fail      error
	schema    []Field
	applied   Values
	badge     string
	actionRan bool
}

func (f *fake) Meta() Meta { return f.meta }

func (f *fake) View(ctx tideui.PanelContext) string {
	if ctx.Zoomed {
		return "detail:" + f.Load()
	}
	return f.Load()
}

func (f *fake) Refresh(context.Context) error {
	f.refresh++
	if f.fail != nil {
		return f.fail
	}
	f.Store("live")
	return nil
}

func (f *fake) Demo(time.Time)  { f.demo++; f.Store("demo") }
func (f *fake) Tick(time.Time)  { f.tick++ }
func (f *fake) Schema() []Field { return f.schema }
func (f *fake) Configure(v Values) error {
	f.applied = v
	return nil
}
func (f *fake) Badge() (string, tideui.Tone) { return f.badge, tideui.ToneWarning }
func (f *fake) Actions() []Action {
	return []Action{{ID: "go", Key: "g", Label: "go", Run: func() string {
		f.actionRan = true
		return "ran"
	}}}
}

func newFake(id string, interval time.Duration) *fake {
	return &fake{meta: Meta{ID: id, Title: id, Interval: interval, MinWidth: 10, MinHeight: 4}}
}

func TestDeckRegistrationOrderAndReplacement(t *testing.T) {
	deck := New()
	first, second := newFake("a", time.Second), newFake("b", time.Second)
	deck.Register(first, second)
	if got := deck.Panels(); len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("registration order not preserved: %#v", got)
	}
	// Re-registering an id replaces in place rather than appending, so an
	// application can substitute a panel without changing its position.
	replacement := newFake("a", time.Second)
	deck.Register(replacement)
	got := deck.Panels()
	if len(got) != 2 {
		t.Fatalf("replacement appended instead of substituting: %d panels", len(got))
	}
	if got[0] != replacement {
		t.Fatal("replacement did not take the original's position")
	}
	if found, ok := deck.Lookup("a"); !ok || found != replacement {
		t.Fatal("Lookup returns the stale panel")
	}
	// A panel with no id is ignored rather than registered under "".
	deck.Register(&fake{})
	if len(deck.Panels()) != 2 {
		t.Fatal("a panel without an id should not register")
	}
}

func TestDeckRefreshHonoursIntervalAndMode(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Minute)
	deck.Register(panel)
	ctx := context.Background()
	start := time.Now()

	// Demo mode never fetches, however often it is ticked.
	deck.Refresh(ctx, start)
	if panel.refresh != 0 {
		t.Fatalf("demo mode fetched %d times", panel.refresh)
	}

	deck.SetMode(ModeLive)
	deck.Refresh(ctx, start)
	if panel.refresh != 1 {
		t.Fatalf("first live refresh = %d, want 1", panel.refresh)
	}
	// Inside the interval, refreshing is a no-op, so the caller can tick as
	// often as it likes.
	deck.Refresh(ctx, start.Add(30*time.Second))
	if panel.refresh != 1 {
		t.Fatalf("refreshed early: %d", panel.refresh)
	}
	deck.Refresh(ctx, start.Add(2*time.Minute))
	if panel.refresh != 2 {
		t.Fatalf("refresh after the interval = %d, want 2", panel.refresh)
	}
	// A manual refresh takes effect on the next tick without waiting.
	deck.RefreshNow("a")
	deck.Refresh(ctx, start.Add(2*time.Minute))
	if panel.refresh != 3 {
		t.Fatalf("RefreshNow did not force a fetch: %d", panel.refresh)
	}
}

func TestDeckRefreshOnceWithoutInterval(t *testing.T) {
	deck := New()
	panel := newFake("a", 0) // no interval: fetch once and keep it
	deck.Register(panel)
	deck.SetMode(ModeLive)
	now := time.Now()
	for i := 0; i < 5; i++ {
		deck.Refresh(context.Background(), now.Add(time.Duration(i)*time.Hour))
	}
	if panel.refresh != 1 {
		t.Fatalf("a panel with no interval fetched %d times, want 1", panel.refresh)
	}
}

func TestDeckRecordsErrorsWithoutStopping(t *testing.T) {
	deck := New()
	failing, working := newFake("bad", time.Minute), newFake("good", time.Minute)
	failing.fail = errors.New("boom")
	deck.Register(failing, working)
	deck.SetMode(ModeLive)
	deck.Refresh(context.Background(), time.Now())

	if deck.Err("bad") == nil {
		t.Fatal("a failing panel's error was not recorded")
	}
	if deck.Err("good") != nil {
		t.Fatalf("a working panel reported %v", deck.Err("good"))
	}
	if working.refresh != 1 {
		t.Fatal("one panel's failure stopped the others refreshing")
	}
	// The failing panel keeps whatever it had rather than being blanked.
	if failing.Load() != "" {
		t.Fatalf("failing panel stored %q", failing.Load())
	}
}

func TestDeckSwitchingModeRefetchesImmediately(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Hour)
	deck.Register(panel)
	deck.SetMode(ModeLive)
	now := time.Now()
	deck.Refresh(context.Background(), now)
	if panel.refresh != 1 {
		t.Fatal("expected an initial fetch")
	}
	// Going to demo and back must not leave the panel waiting out an hour
	// that elapsed while it was not fetching.
	deck.SetMode(ModeDemo)
	deck.SetMode(ModeLive)
	deck.Refresh(context.Background(), now.Add(time.Second))
	if panel.refresh != 2 {
		t.Fatalf("refresh after a mode round trip = %d, want 2", panel.refresh)
	}
}

func TestDeckTickDrivesDemoAndTickers(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Second)
	deck.Register(panel)
	now := time.Now()

	deck.Tick(now)
	if panel.demo != 1 || panel.tick != 1 {
		t.Fatalf("demo=%d tick=%d, want 1 and 1", panel.demo, panel.tick)
	}
	if panel.Load() != "demo" {
		t.Fatalf("demo data not stored: %q", panel.Load())
	}
	// In live mode the clock still advances, but sample data must not
	// overwrite what was fetched.
	deck.SetMode(ModeLive)
	panel.Store("live")
	deck.Tick(now)
	if panel.demo != 1 {
		t.Fatalf("demo ran in live mode: %d", panel.demo)
	}
	if panel.tick != 2 {
		t.Fatalf("tick = %d, want 2", panel.tick)
	}
	if panel.Load() != "live" {
		t.Fatalf("live data was overwritten with %q", panel.Load())
	}
}

func TestDeckConfigureAndSchema(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Minute)
	panel.schema = []Field{{Key: "a.thing", Label: "thing", Kind: FieldText}}
	plain := newFake("b", time.Minute)
	plain.schema = nil
	deck.Register(panel, plain)

	values := NewValues()
	values.Set("a.thing", "set")
	if errs := deck.Configure(values); len(errs) != 0 {
		t.Fatalf("unexpected configure errors: %v", errs)
	}
	if panel.applied.String("a.thing") != "set" {
		t.Fatal("values were not handed to the panel")
	}

	schema := deck.Schema()
	if len(schema) != 2 {
		t.Fatalf("schema has %d categories, want one per panel", len(schema))
	}
	if schema[0].PanelID != "a" || len(schema[0].Fields) != 1 {
		t.Fatalf("first category = %#v", schema[0])
	}
	// A panel with no settings still gets a page, because the settings screen
	// adds a visibility toggle to every panel.
	if schema[1].PanelID != "b" || len(schema[1].Fields) != 0 {
		t.Fatalf("second category = %#v", schema[1])
	}
}

func TestDeckConfigureInvalidatesCachedData(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Hour)
	deck.Register(panel)
	deck.SetMode(ModeLive)
	now := time.Now()
	deck.Refresh(context.Background(), now)

	// Settings may have changed where the data comes from, so what was
	// fetched a moment ago is not known to be current.
	deck.Configure(NewValues())
	deck.Refresh(context.Background(), now.Add(time.Second))
	if panel.refresh != 2 {
		t.Fatalf("refresh after reconfiguring = %d, want 2", panel.refresh)
	}
}

func TestDeckAttachRegistersPanelsAndActions(t *testing.T) {
	deck := New()
	panel := newFake("a", time.Second)
	panel.meta.Title = "Panel A"
	panel.meta.HideBelow = 80
	panel.Store("body")
	deck.Register(panel)

	var status string
	deck.OnStatus(func(message string) { status = message })

	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("a")
	if !ok {
		t.Fatal("panel was not registered with the workspace")
	}
	if registered.TitleText() != "Panel A" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	// The panel's own View is the workspace's view: no adapter per panel.
	body := registered.Render(tideui.PanelContext{ID: "a", Width: 20})
	if body != "body" {
		t.Fatalf("view = %q, want the panel's own output", body)
	}
	if detail := registered.Render(tideui.PanelContext{ID: "a", Width: 20, Zoomed: true}); detail != "detail:body" {
		t.Fatalf("zoomed view = %q", detail)
	}
	// An action runs the panel's function and reports through the sink,
	// without the panel ever touching the workspace.
	actions := registered.ActionList()
	if len(actions) != 1 || actions[0].Key != "g" {
		t.Fatalf("actions = %#v", actions)
	}
	actions[0].Handler(ws)
	if !panel.actionRan {
		t.Fatal("action did not reach the panel")
	}
	if status != "ran" {
		t.Fatalf("status = %q, want the action's message", status)
	}
}

func TestDeckBadges(t *testing.T) {
	deck := New()
	loud, quiet := newFake("loud", time.Second), newFake("quiet", time.Second)
	loud.badge = "3"
	deck.Register(loud, quiet)

	badges := deck.Badges()
	if len(badges) != 1 {
		t.Fatalf("badges = %#v, want only the panel advertising one", badges)
	}
	if badges["loud"].Text != "3" || badges["loud"].Tone != tideui.ToneWarning {
		t.Fatalf("badge = %#v", badges["loud"])
	}
}

// A panel that implements nothing optional must still work, so the smallest
// plugin really is two methods.
type minimal struct{}

func (minimal) Meta() Meta                      { return Meta{ID: "min", Title: "Min"} }
func (minimal) View(tideui.PanelContext) string { return "min" }

func TestDeckToleratesAMinimalPanel(t *testing.T) {
	deck := New()
	deck.Register(minimal{})
	deck.SetMode(ModeLive)
	deck.Refresh(context.Background(), time.Now())
	deck.Tick(time.Now())
	if errs := deck.Configure(NewValues()); len(errs) != 0 {
		t.Fatalf("configure errors: %v", errs)
	}
	if len(deck.Badges()) != 0 {
		t.Fatal("a minimal panel should advertise no badge")
	}
	if schema := deck.Schema(); len(schema) != 1 || len(schema[0].Fields) != 0 {
		t.Fatalf("schema = %#v", schema)
	}
	ws := tideui.NewWorkspace()
	deck.Attach(ws)
	if _, ok := ws.Lookup("min"); !ok {
		t.Fatal("minimal panel was not attached")
	}
}
