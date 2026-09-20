// Package dash binds a dashboard panel's three concerns - its data, its
// rendering and its settings - into one object, so adding a panel means
// writing one file rather than editing ten.
//
// The package sits downstream of both tideui and provider. It has to: the
// provider package imports tideui, so tideui can never import provider, and a
// registry that both fetches and draws must therefore live below them. That
// also keeps tideui's promise to be purely presentational.
//
// Nothing in the Panel interface mentions where data comes from, and no
// panel's model type appears in it. That is deliberate: a panel backed by a
// local provider and a panel backed by an out-of-process plugin are the same
// kind of thing to a Deck, so the second can be added later without changing
// the interface. Keep it that way.
package dash

import (
	"context"
	"time"

	"github.com/allisonhere/tideui"
)

// Meta is a panel's static identity: what the workspace needs to place it,
// what the settings screen needs to title its page, and how often it wants
// fresh data.
type Meta struct {
	ID       string // stable identifier, e.g. "gpu"
	Title    string // shown in the panel header
	Glyph    string // optional panel glyph; the host supplies a fallback when empty
	Subtitle string

	Role      tideui.PanelRole
	Priority  int
	MinWidth  int
	MinHeight int
	HideBelow int // auto-hide under this many columns; 0 never hides

	// Theme, when set, gives the panel its own palette instead of the
	// workspace's, the way the hand-written registration could.
	Theme tideui.Theme

	// Hidden starts the panel hidden, so it joins the workspace off until it
	// is enabled rather than dropping into every preset. A plugin uses this:
	// installing one should not rearrange the dashboard.
	Hidden bool

	// Gauge and Spark say whether the panel draws progress bars or sparklines.
	// The settings screen offers that metric style only when it is used, so a
	// list panel is not asked to pick a gauge it never renders.
	Gauge bool
	Spark bool

	// Interval is how often Refresh is worth calling. Zero means the panel
	// has no data of its own, or fetches once and keeps it.
	Interval time.Duration
}

// Panel is one dashboard panel. The two required methods are everything the
// workspace needs; data, settings, demo content and actions are opt-in
// through the interfaces below, so the smallest panel is two methods.
type Panel interface {
	Meta() Meta
	// View renders the panel body. ctx.Zoomed selects the detail rendering,
	// and ctx.Renderer carries the resolved theme.
	View(tideui.PanelContext) string
}

// Fetcher is implemented by a panel that acquires data. Refresh is called
// under a bounded context, and is expected to be the only part of a panel that
// blocks; a panel stores the result itself and keeps its last good value on
// error, so View never has to handle failure.
type Fetcher interface {
	Refresh(context.Context) error
}

// AlwaysLive is implemented by a panel whose source is real even in demo mode,
// so the deck refreshes it whatever its mode. A plugin program is the example:
// it is a program the user installed, not the sample data the demo mode exists
// to stand in for.
type AlwaysLive interface {
	AlwaysLive() bool
}

// PaneSized is implemented by a panel whose fetches depend on how big its pane
// is - a picture panel orders as many pixels as the pane has room for. A panel
// learns its pane only by being drawn, so a pane-sized panel is refreshed when
// the pane it was drawn in changes shape: without that, a picture fetched for a
// small pane is stretched across a zoomed one until its next interval, which for
// a radar is five minutes of the wrong picture.
type PaneSized interface {
	PaneSized() bool
}

// Input is implemented by a panel that takes typing while it has focus, so a
// panel can offer a small live input - a calculator. Only the keys the panel
// wants are consumed (Type reports whether it took the rune); every other key,
// including application shortcuts and focus movement, is left to the caller.
type Input interface {
	Type(r rune) bool
	Backspace() bool
}

// InputClearer is implemented by a panel whose typed input can be reset. A host
// clears it once the row a search picked has been acted on, so the pane
// collapses back to its input row instead of showing the same matches again.
type InputClearer interface {
	Clear() bool
}

// Copier is implemented by a panel whose content can be copied. Copy returns
// the text and whether there is anything to copy; the caller puts it on the
// clipboard, so the panel stays independent of the terminal.
type Copier interface {
	Copy() (string, bool)
}

// Cursor is implemented by a panel with a moveable selection, so the arrow
// keys can drive it without the application knowing what a "row" is. Move
// changes the selection by delta (-1 up, +1 down) and reports whether the
// panel took the key; a panel with nothing to select returns false so the key
// still moves focus.
type Cursor interface {
	Move(delta int) bool
}

// Activator is implemented by a panel whose current selection has a primary
// action - copy a link and mark it read. Activate returns the text to copy
// (empty when there is none) and a status message for the strip.
type Activator interface {
	Activate() (copy, status string)
}

// Launcher is implemented by a panel whose current selection opens a program -
// the mail preview opening the message in TideMail. Launch returns the argv to
// run, the line for the status strip, and whether the panel has such an action
// at all: a panel that declares no open command returns ok false, so the key it
// would have used stays the workspace's; one with nothing selected returns a nil
// argv and a status saying what to do instead.
//
// The caller runs the command, the way it puts a Copier's text on the clipboard,
// so a panel never owns the terminal.
type Launcher interface {
	Launch() (argv []string, status string, ok bool)
}

// Editor is implemented by a panel whose selection can also be *changed* in the
// program that owns it: the pane previews, the program edits. Edit returns the
// same three things Launcher does, so a panel that declares no edit command
// leaves that key to the rest of the application.
type Editor interface {
	Edit() (argv []string, status string, ok bool)
}

// Clicker is implemented by a panel that maps a click inside its content area
// to an item, with x and y relative to the body. It returns the text to copy
// and a status message, and whether the click hit an item.
type Clicker interface {
	Click(x, y int) (copy, status string, hit bool)
}

// Configurable is implemented by a panel with settings of its own. Schema
// declares the fields; Configure applies them.
type Configurable interface {
	Schema() []Field
	Configure(Values) error
}

// Demoable is implemented by a panel that can synthesise sample data, so the
// dashboard looks alive before anything is configured.
type Demoable interface {
	Demo(now time.Time)
}

// Ticker is implemented by a panel whose rendering changes with the clock
// between refreshes, such as the clock itself or a day rollover.
type Ticker interface {
	Tick(now time.Time)
}

// Badger is implemented by a panel that advertises a header badge.
type Badger interface {
	Badge() (text string, tone tideui.Tone)
}

// Actor is implemented by a panel offering contextual key actions.
type Actor interface {
	Actions() []Action
}

// Action is one panel action. Run returns the status message to show, so a
// panel never needs a reference to the workspace.
type Action struct {
	ID    string
	Key   string
	Label string
	Run   func() string

	// Refresh makes the deck treat the panel as due again before Run is
	// called, so a refresh key actually refetches. Without it a panel cannot
	// refresh itself - it has no reference to the deck - and a "refresh"
	// action can only print a message claiming it did.
	Refresh bool
}
