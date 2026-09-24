package dash

import (
	"context"
	"time"

	"github.com/allisonhere/tideui"
)

// Mode selects where panels get their data.
type Mode int

const (
	// ModeDemo asks panels to synthesise sample data and never fetches.
	ModeDemo Mode = iota
	// ModeLive refreshes panels from their real sources.
	ModeLive
)

// Deck is the registry: it holds the panels, attaches them to a workspace,
// and drives their refreshes.
//
// Refreshing is pull-based rather than one goroutine per panel. The caller
// ticks the deck, the deck skips any panel whose interval has not elapsed,
// and hands back one job for each panel that is due, which the caller runs in
// the background and reports back through Finish. That means no goroutine
// lifecycle to manage, nothing to join on shutdown, and no way to leak a
// ticker when settings are reapplied.
type Deck struct {
	order []Panel
	index map[string]Panel
	last  map[string]time.Time
	errs  map[string]error
	// inflight holds the panels whose background refresh has been started
	// and not yet finished, so a slow source is never asked twice at once.
	inflight map[string]bool
	// again holds panels asked to refresh while already refreshing: the
	// running fetch was started for an older state (a pane that has since been
	// resized, say), so the panel is due again as soon as it finishes.
	again   map[string]bool
	mode    Mode
	status  func(string)
	timeout time.Duration
}

// DefaultTimeout bounds one panel's refresh. It matches the provider
// package's own default.
const DefaultTimeout = 12 * time.Second

// New returns an empty deck in demo mode.
func New() *Deck {
	return &Deck{
		index:    map[string]Panel{},
		last:     map[string]time.Time{},
		errs:     map[string]error{},
		inflight: map[string]bool{},
		again:    map[string]bool{},
		timeout:  DefaultTimeout,
	}
}

// Register adds a panel. Registration order is the order panels are attached
// to the workspace and listed in settings. Registering the same id twice
// replaces the first, so an application can substitute a panel.
func (d *Deck) Register(panels ...Panel) {
	for _, panel := range panels {
		id := panel.Meta().ID
		if id == "" {
			continue
		}
		if _, exists := d.index[id]; exists {
			for i, existing := range d.order {
				if existing.Meta().ID == id {
					d.order[i] = panel
					break
				}
			}
		} else {
			d.order = append(d.order, panel)
		}
		d.index[id] = panel
	}
}

// Panels returns the registered panels in registration order.
func (d *Deck) Panels() []Panel { return append([]Panel(nil), d.order...) }

// Lookup finds a panel by id.
func (d *Deck) Lookup(id string) (Panel, bool) {
	panel, ok := d.index[id]
	return panel, ok
}

// SetMode switches between demo and live data. Switching clears the refresh
// clock so live panels fetch immediately rather than waiting out an interval
// that elapsed while the deck was in demo mode.
func (d *Deck) SetMode(mode Mode) {
	if mode == d.mode {
		return
	}
	d.mode = mode
	d.last = map[string]time.Time{}
}

// Mode reports the current mode.
func (d *Deck) Mode() Mode { return d.mode }

// OnStatus sets the sink for panel action messages.
func (d *Deck) OnStatus(fn func(string)) { d.status = fn }

// Attach registers every panel with a workspace, wiring its metadata and any
// actions it advertises. The panel's View is the workspace's PanelView, so no
// adapter closure is needed per panel.
func (d *Deck) Attach(ws *tideui.Workspace) {
	if ws == nil {
		return
	}
	for _, panel := range d.order {
		d.attachOne(ws, panel)
	}
}

// AttachPanel attaches a single panel, for one that was added or replaced at
// runtime - a plugin installed while the app is running. A panel that already
// exists has its view and metadata refreshed in place. Do not attach a panel
// that advertises actions twice: Actions appends, so a second attach would
// duplicate them. Plugins declare no actions, so this is safe for them.
func (d *Deck) AttachPanel(ws *tideui.Workspace, panel Panel) {
	if ws == nil || panel == nil {
		return
	}
	d.attachOne(ws, panel)
}

func (d *Deck) attachOne(ws *tideui.Workspace, panel Panel) {
	meta := panel.Meta()
	view := panel.View // captured per panel, not per loop iteration
	builder := ws.Panel(meta.ID, func(ctx tideui.PanelContext) string { return view(ctx) }).
		Title(meta.Title).Role(meta.Role).Priority(meta.Priority).
		MinWidth(meta.MinWidth).MinHeight(meta.MinHeight)
	if meta.Subtitle != "" {
		builder = builder.Subtitle(meta.Subtitle)
	}
	if meta.HideBelow > 0 {
		builder = builder.HideBelow(meta.HideBelow)
	}
	if meta.Theme.Name != "" {
		builder = builder.Theme(meta.Theme)
	}
	if meta.Hidden {
		builder = builder.Hide()
	}
	if actor, ok := panel.(Actor); ok {
		builder.Actions(d.actions(meta.ID, actor)...)
	}
}

// Unregister removes a panel from the deck, so it is no longer refreshed,
// configured, listed or badged. The workspace panel is removed separately.
func (d *Deck) Unregister(id string) {
	if _, ok := d.index[id]; !ok {
		return
	}
	delete(d.index, id)
	delete(d.last, id)
	delete(d.errs, id)
	for i, panel := range d.order {
		if panel.Meta().ID == id {
			d.order = append(d.order[:i], d.order[i+1:]...)
			break
		}
	}
}

// actions adapts a panel's actions to the workspace's handler shape, so panel
// code never takes a workspace reference.
func (d *Deck) actions(id string, actor Actor) []tideui.PanelAction {
	declared := actor.Actions()
	out := make([]tideui.PanelAction, 0, len(declared))
	for _, action := range declared {
		run, refresh := action.Run, action.Refresh
		out = append(out, tideui.Action(action.ID, action.Key, func(*tideui.Workspace) {
			if refresh {
				d.RefreshNow(id)
			}
			if run == nil {
				return
			}
			if message := run(); message != "" && d.status != nil {
				d.status(message)
			}
		}).Labeled(action.Label))
	}
	return out
}

// Refresh brings stale panels up to date. It is cheap to call often: a panel
// whose interval has not elapsed is skipped, and in demo mode nothing fetches
// at all.
//
// It runs every fetch in turn and returns when they are done, which suits a
// test or a one-shot program. An interactive caller uses Start and Finish
// instead, so a slow source never holds up the frame.
func (d *Deck) Refresh(ctx context.Context, now time.Time) {
	for _, job := range d.Start(ctx, now) {
		d.Finish(job())
	}
}

// Refreshed reports one panel's refresh finishing.
type Refreshed struct {
	ID  string
	Err error
}

// Start picks the panels due for a refresh and returns one job each, for the
// caller to run off its UI goroutine; each job's result goes back through
// Finish, on the UI goroutine. Panels keep their data behind their own locks,
// so a job may run while the panel is drawn.
//
// A panel whose previous job has not finished is not started again: a source
// that is slow to answer is waited for, not asked a second and third time.
func (d *Deck) Start(ctx context.Context, now time.Time) []func() Refreshed {
	var jobs []func() Refreshed
	for _, panel := range d.order {
		fetcher, ok := panel.(Fetcher)
		if !ok {
			continue
		}
		// Demo mode stands in sample data for real sources, so it skips them
		// all - except a panel that says it is real regardless, like a plugin.
		if d.mode != ModeLive {
			live, ok := panel.(AlwaysLive)
			if !ok || !live.AlwaysLive() {
				continue
			}
		}
		meta := panel.Meta()
		if d.inflight[meta.ID] {
			continue
		}
		if interval := meta.Interval; interval > 0 {
			if last, seen := d.last[meta.ID]; seen && now.Sub(last) < interval {
				continue
			}
		} else if _, seen := d.last[meta.ID]; seen {
			continue // no interval: fetch once
		}
		d.last[meta.ID] = now
		jobs = append(jobs, d.job(ctx, meta.ID, fetcher))
	}
	return jobs
}

// StartPanel returns a job refreshing one panel now, whatever its interval -
// after a program it opened has changed its data, or its pane changed shape.
// It returns nil for a panel that does not fetch, and for one already
// refreshing, which is then due again the moment that refresh finishes.
func (d *Deck) StartPanel(ctx context.Context, id string) func() Refreshed {
	panel, ok := d.index[id]
	if !ok {
		return nil
	}
	if d.inflight[id] {
		d.again[id] = true
		return nil
	}
	fetcher, ok := panel.(Fetcher)
	if !ok {
		return nil
	}
	return d.job(ctx, id, fetcher)
}

// job marks a panel in flight and returns the work, capturing everything it
// needs so it touches nothing of the deck's while it runs.
func (d *Deck) job(ctx context.Context, id string, fetcher Fetcher) func() Refreshed {
	d.inflight[id] = true
	timeout := d.timeout
	return func() Refreshed {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return Refreshed{ID: id, Err: fetcher.Refresh(runCtx)}
	}
}

// Finish records a job's result. It belongs on the UI goroutine, like every
// other use of the deck.
func (d *Deck) Finish(r Refreshed) {
	delete(d.inflight, r.ID)
	if d.again[r.ID] {
		delete(d.again, r.ID)
		d.RefreshNow(r.ID)
	}
	if _, ok := d.index[r.ID]; ok {
		d.errs[r.ID] = r.Err
	}
}

// Refreshing reports whether a panel's refresh is under way.
func (d *Deck) Refreshing(id string) bool { return d.inflight[id] }

// RefreshNow forces a panel to refresh on the next Refresh, which is what a
// panel's own refresh action should do.
func (d *Deck) RefreshNow(id string) { delete(d.last, id) }

// Err reports the last refresh error for a panel, if any.
func (d *Deck) Err(id string) error { return d.errs[id] }

// Tick advances panels that change with the clock, and in demo mode refills
// them with sample data. It does no I/O and is safe on the UI goroutine.
func (d *Deck) Tick(now time.Time) {
	for _, panel := range d.order {
		if d.mode == ModeDemo {
			if demo, ok := panel.(Demoable); ok {
				demo.Demo(now)
			}
		}
		if ticker, ok := panel.(Ticker); ok {
			ticker.Tick(now)
		}
	}
}

// Badges collects the header badges panels advertise, keyed by panel id.
func (d *Deck) Badges() map[string]Badge {
	out := map[string]Badge{}
	for _, panel := range d.order {
		badger, ok := panel.(Badger)
		if !ok {
			continue
		}
		if text, tone := badger.Badge(); text != "" {
			out[panel.Meta().ID] = Badge{Text: text, Tone: tone}
		}
	}
	return out
}

// Badge is one panel's header badge.
type Badge struct {
	Text string
	Tone tideui.Tone
}

// Configure applies settings to every panel that has them, collecting errors
// per panel rather than stopping at the first, so one bad setting does not
// leave the rest unconfigured.
func (d *Deck) Configure(values Values) map[string]error {
	out := map[string]error{}
	for _, panel := range d.order {
		configurable, ok := panel.(Configurable)
		if !ok {
			continue
		}
		if err := configurable.Configure(values); err != nil {
			out[panel.Meta().ID] = err
		}
	}
	// Settings may have changed where data comes from, so nothing cached is
	// known to be current.
	d.last = map[string]time.Time{}
	return out
}

// ConfigurePanel applies settings to one panel, and forgets that panel's last
// run so it re-reads its source.
//
// A save configures everything, because a save may have changed anything. A
// keystroke did not: the live path reconfigures one page, and a full Configure
// there forgets *every* panel's interval, so the next tick re-fetches the whole
// dashboard - news, weather, markets, updates - none of which was edited. With
// one write per typed character and one per arrow key, that is a stall for the
// reader on every keystroke, and it is what made a plugin's dropdown feel
// broken: the frame waited for the slowest unrelated source before the page
// could follow what the panel had just reported.
func (d *Deck) ConfigurePanel(id string, values Values) error {
	for _, panel := range d.order {
		if panel.Meta().ID != id {
			continue
		}
		configurable, ok := panel.(Configurable)
		if !ok {
			return nil
		}
		if err := configurable.Configure(values); err != nil {
			return err
		}
		d.RefreshNow(id)
		return nil
	}
	return nil
}

// Schema returns one settings category per panel, in registration order.
// Panels with no settings still appear, because the settings screen adds a
// visibility toggle and metric styles to every panel's page.
func (d *Deck) Schema() []Category {
	out := make([]Category, 0, len(d.order))
	for _, panel := range d.order {
		meta := panel.Meta()
		category := Category{PanelID: meta.ID, Name: meta.Title, Gauge: meta.Gauge, Spark: meta.Spark}
		if configurable, ok := panel.(Configurable); ok {
			category.Fields = configurable.Schema()
		}
		out = append(out, category)
	}
	return out
}
