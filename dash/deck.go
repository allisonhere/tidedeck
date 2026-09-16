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
// and a panel that does fetch does so in the background. That means no
// goroutine lifecycle to manage, nothing to join on shutdown, and no way to
// leak a ticker when settings are reapplied.
type Deck struct {
	order   []Panel
	index   map[string]Panel
	last    map[string]time.Time
	errs    map[string]error
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
		index:   map[string]Panel{},
		last:    map[string]time.Time{},
		errs:    map[string]error{},
		timeout: DefaultTimeout,
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
		if actor, ok := panel.(Actor); ok {
			builder.Actions(d.actions(meta.ID, actor)...)
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
// The fetches themselves are synchronous, so a caller with a UI should drive
// this from a command rather than from its update loop: a panel is bounded by
// the deck's timeout, but a slow source would still hold up the frame.
func (d *Deck) Refresh(ctx context.Context, now time.Time) {
	if d.mode != ModeLive {
		return
	}
	for _, panel := range d.order {
		fetcher, ok := panel.(Fetcher)
		if !ok {
			continue
		}
		meta := panel.Meta()
		if interval := meta.Interval; interval > 0 {
			if last, seen := d.last[meta.ID]; seen && now.Sub(last) < interval {
				continue
			}
		} else if _, seen := d.last[meta.ID]; seen {
			continue // no interval: fetch once
		}
		d.last[meta.ID] = now
		d.errs[meta.ID] = d.refreshOne(ctx, fetcher)
	}
}

// refreshOne bounds a single refresh so one unreachable source cannot stall
// the others.
func (d *Deck) refreshOne(ctx context.Context, fetcher Fetcher) error {
	runCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	return fetcher.Refresh(runCtx)
}

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

// Schema returns one settings category per panel, in registration order.
// Panels with no settings still appear, because the settings screen adds a
// visibility toggle and metric styles to every panel's page.
func (d *Deck) Schema() []Category {
	out := make([]Category, 0, len(d.order))
	for _, panel := range d.order {
		meta := panel.Meta()
		category := Category{PanelID: meta.ID, Name: meta.Title}
		if configurable, ok := panel.(Configurable); ok {
			category.Fields = configurable.Schema()
		}
		out = append(out, category)
	}
	return out
}
