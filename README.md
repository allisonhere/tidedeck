# tideui

**A themeable, multi-pane terminal UI toolkit for [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).**

`tideui` renders application-provided content inside themed pane shells, status
bars, and overlays. It is deliberately *view-oriented*: your application keeps
its own Bubble Tea model, key routing, persistence, and viewport state — you
hand `tideui` strings and dimensions, and it returns a framed, themed view.


## Lineage

The three-pane layout, theme-preview workflow, and themed modal language began
in [Tide](https://github.com/allisonhere/tide), a terminal RSS reader, and were
refined in [TideMail](https://github.com/allisonhere/tidemail), a keyboard-first
email client. `tideui` packages those reusable primitives for any Bubble Tea
application.

## Install

```bash
go get github.com/allisonhere/tideui
```

## Features

- **Five layout modes** — `StackedRight`, `ThreeColumn`, `SidebarOnly`, `Tabbed`, and `Floating`, each with tunable ratios.
- **Nineteen built-in palettes** (Catppuccin, Nord, Dracula, Gruvbox, and more) with per-field background/foreground/accent overrides.
- **Themed chrome** — pane headers, status bars, centered modal overlays, and a ready-made theme picker.
- **Soft modal panels** — Tide-family modal chrome with embedded border titles, quiet hint footers, and rail-focused rows.
- **Workspace system** — a tiny tiling window manager for TUIs: declarative panels, a layout tree, semantic adaptive reflow, live arrange mode, shift+arrow pane resizing, tab stacks, zoom, peek, contextual actions, a panel picker, command palette, presets, undo/redo, mouse support, and versioned persistence.
- **Visual language** — semantic workspace tokens, reusable chrome primitives (headers, footers, tabs, badges, key capsules, metrics, sparklines, list rows), `Comfortable`/`Compact`/`Dense` density modes, and a configurable focus presentation.
- **Dashboard widgets** — weather, agenda, clock, system, GPU, network, storage, services, updates, news, tasks, notes, git activity, and markets, each driven by a plain data model and backed by a `Renderer` method, with a shared Enter-to-drill-down pattern.
- **Real data sources** — a standard-library `provider` package: background collectors with per-source intervals and graceful degradation, plus providers for Open-Meteo weather, RSS/Atom/RDF with a curated source
  catalogue (`provider.NewsSources()`), Linux system/GPU/network/storage, pending package updates, systemd and Docker, git activity, todo.txt, notes, iCalendar (local files or remote `https`/`webcal` feeds), and markets.
- **Panel registry** — a `dash` package where a panel's data, rendering, and settings are one object: opt-in `Fetcher`/`Configurable`/`Ticker`/`Badger`/`Actor` interfaces, pull-based refresh with per-panel intervals and last-good-value degradation, a settings screen built from the fields panels declare, a configuration document that preserves keys it does not recognise, and external plugins that are ordinary programs printing a JSON document.
- **Per-panel themes** — any panel can take its own full theme or color overrides while density, corners, gutters, and global chrome stay workspace-wide; panel content inherits it through `PanelContext.Renderer`.
- **Full-border pane focus** — every pane renders a 4-sided border colored by focus state, contrast-boosted to a 7:1 floor (square or round corners) so the focused pane is never hard to spot.
- **List primitives** — single-line `Row` and multi-line `Block` with selected/muted states.
- **Per-pane scrolling** via `Pane.ScrollOffset` and the `PaneScroller` helper.
- **Resizable panes** via the `PaneRatio` helper, for shift+arrow-style ratio adjustment.
- **Density + accessibility** — compact/comfortable spacing and a VT52 ASCII mode.
- **Bounded output** — never exceeds the requested terminal dimensions, down to tiny windows.
- **Terminal background control** — exposes the escape sequences so the app, not the library, writes to the terminal.

## Quick start

```go
import "github.com/allisonhere/tideui"

theme, _ := tideui.ThemeByName("catppuccin-mocha")
renderer := tideui.NewRenderer(theme, tideui.StyleOptions{Density: tideui.Compact})

view := renderer.Render(tideui.Layout{
    Width: 80, Height: 24, Mode: tideui.StackedRight,
    Panes: [3]tideui.Pane{
        {Title: "Sidebar", Content: "Item one\nItem two", Focused: true},
        {Title: "List",    Content: "Welcome to tideui"},
        {Title: "Preview", Content: "Application-owned content."},
    },
    Status: &tideui.StatusBar{Left: "ready", Right: "? help"},
})
```

## Bubble Tea integration

`tideui` owns no model state. Track dimensions and theme in your own model and
build a renderer in `View`:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if size, ok := msg.(tea.WindowSizeMsg); ok {
        m.width, m.height = size.Width, size.Height
    }
    return m, nil
}

func (m model) View() string {
    r := tideui.NewRenderer(m.theme, tideui.StyleOptions{Density: m.density})
    return r.Render(tideui.Layout{
        Width: m.width, Height: m.height, Mode: tideui.ThreeColumn,
        Panes: m.panes(),
    })
}
```

## Layout modes

| Mode | Description | Ratio fields |
|---|---|---|
| `StackedRight` | Pane 0 sidebar; panes 1 & 2 stacked on the right | `SidebarRatio`, `UpperRightRatio` |
| `ThreeColumn` | All three panes side by side | `ColumnRatios` |
| `SidebarOnly` | Pane 0 sidebar; pane 1 full-height main (pane 2 unused) | `SidebarRatio` |
| `Tabbed` | Tab bar on top; the focused pane fills the area below | — |
| `Floating` | Pane 0 as background; panes 1 & 2 as floating panels | `FloatWidthRatio`, `FloatHeightRatio` |

```go
layout.Mode = tideui.StackedRight
layout.SidebarRatio, layout.UpperRightRatio = 0.30, 0.45

layout.Mode, layout.ColumnRatios = tideui.ThreeColumn, [3]float64{2, 3, 5}
```

In `Tabbed` mode the first `Focused` pane selects the active tab (falling back to
pane 0). All ratio fields default to sensible values when left zero.

## Theming

```go
theme, ok := tideui.ThemeByName("nord")   // false if unknown
for _, t := range tideui.BuiltinThemes { /* ... */ }
```

Override individual colors without forking a palette:

```go
theme = tideui.ThemeOverrides{
    Accent: "#f5c2e7",
}.Apply(tideui.CatppuccinMocha)
```

`Theme.UsesASCII()` reports VT52 mode (ASCII-only glyphs) so callers can adapt.
`StyleOptions{Density: tideui.Comfortable}` adds spacing, `Compact` is the
default, and `Dense` strips secondary metadata for dashboards (see
[Visual language](#visual-language)).

## Pane borders

Every pane renders a full 4-sided border, colored by theme `Border` when
idle and `BorderFocus` (or `Pane.Accent`, if set) when focused. The focused
color is nudged to clear a 7:1 contrast floor against the theme background,
and the selected-row background (`Styles.ItemSelected`) is nudged to a 3:1
floor — both walk up in lightness steps rather than relying on a fixed
delta, so neither goes unnoticeable on unusually light or dark themes.

Corners default to square; opt into rounded corners per-renderer:

```go
tideui.NewRenderer(theme, tideui.StyleOptions{PaneCorners: tideui.RoundCorners})
```

## Rows and blocks

`RenderRow` draws a single-line list item; `RenderBlock` adds an optional
multi-line body (a `Block` with no `Body` is byte-identical to the matching
`Row`). Both support `Selected` and `Muted`:

```go
renderer.RenderRow(tideui.Row{Prefix: "* ", Text: "Item", Suffix: "12", Selected: true}, width)

renderer.RenderBlock(tideui.Block{
    Prefix: "● ", Header: "alice", Meta: "10:02",
    Body:   "Multi-line body, indented to the header.",
}, width)
```

For fully custom pane content, use the exported `renderer.Styles` (e.g.
`DetailTitle`, `DetailMeta`, `DetailBody`).

## Scrollable panes

Set `Pane.ScrollOffset`, or let `PaneScroller` manage it:

```go
m.scroll.ScrollDown(1)            // ScrollUp / ScrollToTop also available
m.scroll.ClampTo(total, visible)  // optional; renderer clamps out-of-range anyway

pane.ScrollOffset = m.scroll.Offset()
```

`CanScrollDown(total, visible)` reports whether more content lies below.

## Resizable panes

`tideui` still leaves key routing to the application, but `PaneRatio` owns
the bounds and step math for an adjustable split — wire it to your own
shift+arrow (or any other) key handling:

```go
m.sidebarRatio = tideui.NewPaneRatio(tideui.PaneRatioOptions{
    Initial: 0.30, Min: 0.15, Max: 0.5, Step: 0.02,
})

// in Update, on your app's own resize keys:
m.sidebarRatio.Shrink() // e.g. shift+left
m.sidebarRatio.Grow()   // e.g. shift+right

// in View:
layout.SidebarRatio = m.sidebarRatio.Value()
```

Hold one `PaneRatio` per adjustable split — `SidebarRatio`, `UpperRightRatio`,
a `ColumnRatios` entry, or a `Floating` ratio all work the same way.

## Theme picker

A drop-in modal for previewing and confirming themes:

```go
picker := tideui.NewThemePicker(tideui.ThemePickerOptions{InitialTheme: "nord"})
picker.Open("nord")

switch picker.Update(keyMsg) {
case tideui.ThemePickerConfirm:
    m.theme = picker.ConfirmedTheme()
case tideui.ThemePickerCancel:
    // preview reverted to the confirmed theme
}

overlay := picker.Modal(renderer, m.width, m.height) // assign to Layout.Modal
```

The picker previews live as you navigate and restores the confirmed theme on
cancel.

Use the soft-panel variant for the newer Tide-family modal style:

```go
overlay := picker.SoftModal(renderer, 42, m.height, "tidedock")
```

## Soft panels

Soft panels render the newer Tide-family modal chrome with the app prefix and
title embedded in the top border:

```go
content := renderer.Styles.OverlayBody.Width(36).Render("Ready")
overlay := renderer.SoftPanelOverlay(tideui.SoftPanel{
    Prefix: "tidedock",
    Title: "status",
    Content: content,
    Width: 40,
})
layout.Modal = &overlay
```

Use `RenderSoftRow` for command palettes and picker rows, and
`RenderSoftHints` for quiet lowercase footer hints.

## Workspace

`Workspace` is a first-class layout framework for applications that would
otherwise hand-calculate rectangles. Applications declare panels with a purpose
and let the workspace handle focus, responsive reflow, movement, resizing,
persistence, zooming, hiding, and visual state. See
[`examples/workspace`](./examples/workspace) for a runnable demo.

```bash
go run ./examples/workspace
```

### Creating a workspace and registering panels

```go
ws := tideui.NewWorkspace(
    tideui.WithPersistence("tidegit"), // application-scoped layout key
    tideui.WithAdaptiveLayout(),       // semantic responsive reflow
    tideui.WithGaps(1, 0),             // 1-cell column gutter, rows flush
)

ws.Panel("repos", reposView).
    Title("Repositories").
    Role(tideui.RoleNavigation).
    MinWidth(24).MinHeight(6).Priority(80).
    Badge("12").Hint("j/k")

ws.Panel("changes", changesView).
    Title("Changes").Role(tideui.RoleSecondary).MinWidth(28)

ws.Panel("diff", diffView).
    Title("Diff").Role(tideui.RolePrimary).MinWidth(32).Grow(2).Priority(100).
    Actions(
        tideui.Action("stage", "s", stageHunk).Labeled("stage"),
        tideui.Action("open", "o", openHunk).Labeled("open"),
    )

ws.Panel("log", logView).
    Title("Log").Role(tideui.RoleTelemetry).MinWidth(20).HideBelow(90)
```

Gaps are per axis: `WithGap(n)` sets both, while `WithGaps(horizontal, vertical)`
sets them separately. A vertical gap of zero stacks panels flush (their borders
touch) instead of leaving a blank row between them.

A `PanelView` receives a `PanelContext` with the allocated width/height and
interaction state, so a panel can adapt its own content:

```go
func reposView(ctx tideui.PanelContext) string { /* ... */ }
```

Panels can be configured with `Title`, `Description`, `Role`, `MinWidth`,
`MinHeight`, `PreferredWidth`, `PreferredHeight`, `Grow`, `Shrink`, `Priority`,
`Focusable`, `Hideable`, `Zoomable`, `Badge`, `Hint`, `Accent`, `Content`,
`Body`, `Actions`, and `Responsive`. Call `Hide` to start a panel hidden.

### Semantic roles

Roles express intent so the workspace can make responsive decisions without
per-panel tuning:

| Role | Default priority | Typical responsive behaviour |
|---|---|---|
| `RolePrimary` | 100 | survives the longest |
| `RoleNavigation` | 80 | collapses below 60 columns |
| `RoleSecondary` | 70 | collapses below 45 columns |
| `RoleInspector` | 60 | moves below the primary panel below 100 columns |
| `RoleTelemetry` | 40 | hides below 70 columns |
| `RoleOptional` | 20 | hides below 80 columns |

### Layout trees

Layouts are trees, never screen coordinates:

```go
ws.Layout(
    tideui.HStack(
        tideui.Leaf("repos"),
        tideui.VStack(tideui.Leaf("changes"), tideui.Leaf("log")),
        tideui.Weighted(tideui.Leaf("diff"), 2),
        tideui.Tabs("problems", "terminal"),
    ),
)
```

Node constructors: `Leaf`, `Tabs`, `HStack`, `VStack`, and `Weighted` (for an
explicit share). Panels in a `Tabs` node share one region as a tab stack.
Without an explicit layout, the workspace derives a default from panel roles.

### Actions and the command palette

Actions declared on a panel automatically appear in that panel's footer (when
it is focused) and in the command palette as `Category · Label`. They are never
registered twice.

```go
ws.OpenPanelPicker()    // default key: w
ws.OpenCommandPalette() // default key: ctrl+p
```

Commands are searchable by label, category, key, and id. `ws.Commands()` returns
the full list if you want to build your own palette.

### Responsive policies

Responsive behaviour is a generalized set of fallback rules, evaluated from
the narrowest applicable threshold outward. Explicit rules win over role
defaults, and hysteresis prevents flicker around a breakpoint.

```go
panel.HideBelow(70)             // remove entirely
panel.CollapseBelow(90)         // reduce to a header strip
panel.StackBelow(120, "diff")   // merge into `diff`'s tab stack
panel.MoveBelow(100, "editor")  // relocate beneath `editor`
panel.MoveRightOf(100, "editor")
```

The same application is expected to look intentional from 80 columns to 200+,
and as a narrow tmux/SSH pane.

### Focus

`Workspace` owns focus traversal:

```go
ws.FocusNext()                 // tab
ws.FocusPrev()                 // shift+tab
ws.FocusDirection(tideui.DirRight)
ws.Focus("diff")
```

Focus presentation is configurable and theme-driven:

```go
ws.SetFocusPresentation(tideui.FocusPresentation{
    ActiveBorder: true, ActiveTitle: true, DimInactive: true,
    MutedSecondary: true, AccentMarker: true, StatusStrip: true, KeyHints: true,
})
```

### Arrange mode

Arrange mode moves the real panel: each direction key immediately repositions
the focused panel against its neighbour, so the layout itself is the preview.
The gap the panel leaves behind closes, every move is recorded in history, and
`esc` simply leaves the mode.

Left/right dock the panel beside the target; up/down move it **into the
target's row** as another column, so moving a pane onto a row with one pane
gives that row two panes instead of stacking a new row. (The responsive
`MoveBelow` fallback still stacks vertically — that is a different operation.)

### Resizing panes

`shift+arrows` resizes the focused pane directly — no mode to enter, no divider
to pick. The arrow names the **edge** you push:

- If a neighbour sits on that side, the shared edge moves toward it and the
  focused pane **grows**.
- If there is no neighbour on that side, the opposite edge moves inward and the
  focused pane **shrinks**.
- A pane in the middle therefore grows from whichever side you press.

Each press moves the edge by 5% of the split's extent (at least one cell), and
weights are rescaled rather than rewritten, so `MinWidth`/`MinHeight` are always
honored and the move no-ops when there is no headroom. The status strip briefly
shows the resulting size (`width 37%` / `height 42%`), each step is one undo
entry, and `ctrl+arrows` works as an alias.

```go
ws.ToggleArrange()                    // m
ws.ArrangeMove(tideui.DirRight)       // h/j/k/l or arrows: move the panel live
ws.ArrangeMerge()                     // t: fold into the neighbour's tab stack
ws.ExitArrange()                      // esc: leave the mode
ws.Undo()                             // each live move is one history entry

ws.ResizeEdge(tideui.DirRight)        // shift+right (ctrl+right)
ws.ResizeEdgePixels(tideui.DirLeft, 3, true) // drag-sized step in cells
ws.ResizeGrow(0) / ws.ResizeShrink(0) // nearest split axis
ws.ResizeWidth(true) / ws.ResizeHeight(true)
ws.ResizeStatus()                     // transient "width 37%" feedback
```

While arranging, the moving panel keeps the spotlight and a `MOVING` badge,
other panels recede, and the status strip shows an `ARRANGE` capsule with
`h/j/k/l move · t stack · esc done`. Because moves are live, `undo` steps back
through them; `esc` only leaves the mode.

### Zoom, peek, and tab stacks

```go
ws.Zoom("diff")   // fill the workspace; the saved layout is untouched
ws.Unzoom()       // instant restore, focus/scroll preserved
ws.Peek("log")    // temporarily reveal a hidden panel as a floating overlay
ws.Unpeek()
```

Tab stacks can be declared directly (`tideui.Tabs("a", "b")`) or arise from a
responsive `StackBelow` rule. `SetActiveTab` and mouse clicks switch the active
tab.

### History, presets, and persistence

```go
ws.AddPreset("Default", defaultTree)
ws.AddPreset("Review", reviewTree)
ws.ApplyPreset("Review")

ws.Undo() // layout moves, resizes, hide/show, tab merge/split
ws.Redo()

ws.Persist() // to the configured LayoutStore
```

Persistence serializes visible panels, tree structure, tab stacks, weights,
hidden state, and the active preset, and is versioned. Transient states (zoom,
peek, overlays) are never persisted. Supply a durable store with `WithStore`;
the default is in-memory:

```go
type LayoutStore interface {
    Load(key string) ([]byte, error)
    Save(key string, data []byte) error
}
```

Restore is lazy: it runs on first layout, after panels are registered, so saved
layouts survive app upgrades. Unknown panels are dropped; if nothing usable
remains, the declared default is kept. Invalid or future-versioned layouts are
rejected cleanly.

### Theming and rendering

`WorkspaceRenderer` uses `Renderer.Styles.Workspace`, so every frame, tab,
badge, and key hint is drawn from the active theme. Motion is opt-in and
degrades gracefully:

```go
renderer := tideui.NewRenderer(theme, tideui.StyleOptions{
    Density: tideui.Compact, PaneCorners: tideui.RoundCorners,
})
wr := tideui.NewWorkspaceRenderer(renderer)
view := wr.Render(ws, width, height) // solves and draws every frame
```

### Per-panel themes

A panel can opt out of the workspace palette without disturbing the rest of
the dashboard. Density, corners, gutters, and status bar stay workspace-wide;
only the panel's own frame, title, tabs, and content change.

```go
ws.Panel("markets", marketsView).
    Theme(tideui.GruvboxLight)                       // a full scheme
ws.Panel("alerts", alertsView).
    Overrides(tideui.ThemeOverrides{Background: "#1a0f0f"}) // or a tint
ws.Panel("weather", weatherView).
    Theme(tideui.Nord).
    Overrides(tideui.ThemeOverrides{Accent: "#ff8800"})     // layering

panel.ClearTheme() // back to the workspace theme
```

The `heat`, `weighted` and `stroke` sparklines differ from the others: they
size and colour each sample by its own absolute value rather than by where it
falls within the run, so a quiet stretch stays small and green instead of being
stretched to fill the ramp. `heat` grades dot area (`·∘○◉●`), `weighted` grades
vertical stroke weight (`╵╷│┃█`) and `stroke` grades horizontal weight
(`╴─━█`). Because the ramp itself climbs, each still reads where colour is
unavailable. `StyleOptions.SparkRamps` replaces a style's glyphs and
`StyleOptions.SparkBands` its thresholds, both keyed by style; each falls back
to the built-in ramp, or its ASCII form for an ASCII theme.

Gauge and sparkline styles are panel-scoped too, so one panel can use a
different glyph set than the workspace:

```go
panel.Gauge(tideui.GaugeCircles)     // this panel's bars use circles
panel.Sparkline(tideui.SparkBraille) // and its sparklines use braille
panel.ClearGauge()                   // back to the workspace gauge
panel.ClearSparkline()               // back to the workspace sparkline
```

Panels without a theme keep following the global theme picker. Panel content
inherits the panel theme automatically because `PanelContext` exposes the
resolved renderer:

```go
func marketsView(ctx tideui.PanelContext) string {
    return ctx.Renderer.RenderMarkets(quotes, ctx.Width)
}
```

Precedence is `panel.Theme` → `panel.Overrides` → global density/corners. In
the TideDeck demo, `T` opens the theme picker targeted at the focused panel
(live preview on that panel; `Enter` applies, `Esc` restores its previous
theme), `Ctrl+T` clears it, and the Markets panel ships with its own
contrasting theme.

### Keyboard and mouse

| Key | Action |
|---|---|
| `tab` / `shift+tab` | focus next / previous panel |
| `m` | toggle arrange mode (`h/j/k/l` move the focused panel live, `t` stack, `esc` done) |
| `shift+arrows` | resize the focused pane (grows toward a neighbour, shrinks at an edge; `ctrl+arrows` alias) |
| `shift+space` | zoom / restore the focused panel |
| `w` | panel picker |
| `s` | settings panel (edit all provider config) |
| `ctrl+p` | command palette |
| `esc` | dismiss peek / zoom / picker / mode |

Mouse support is additive: clicking focuses a panel, clicking a tab selects it,
and dragging a panel border resizes the adjacent split. `Workspace.HandleMouse`
consumes events; keyboard remains complete without a mouse.

## Visual language

TideUI ships a reusable visual language for panels and dashboards so every Tide
app — TideGit, TideMail, Tide RSS, Docker tools, file managers — shares one
coherent look without hand-styling each widget.

### Semantic tokens

Every component draws from `Styles.Workspace`, a set of semantic tokens
resolved from the active `Theme` with safe fallbacks. Any theme, including a
hand-written or low-colour one, yields a readable, coherent result:

```go
ws := renderer.Styles.Workspace
// surfaces
ws.Bg, ws.SurfaceBg, ws.FocusSurfaceBg, ws.RaisedBg
// frames + titles
ws.FrameActive, ws.FrameIdle, ws.FrameDimmed
ws.TitleActiveBg, ws.TitleActiveFg, ws.TitleIdleFg, ws.TitleDimmedFg, ws.SubtitleFg
// bodies + selection
ws.BodyFg, ws.BodyDimmedFg, ws.SelectionBg, ws.SelectionInactiveBg, ws.SelectionBar
// tabs, badges, chrome, metrics, focus
ws.TabActiveBg, ws.TabIdleBg, ws.BadgeGoodBg, ws.Separator, ws.KeyBg, ws.MetricGood, ws.DockFill
```

Because the tokens are derived, extending the palette never requires touching a
widget, and meaning never depends on a single raw colour.

### Chrome primitives

Reusable, theme-aware components for building panels and dashboards:

| Primitive | Purpose |
|---|---|
| `PanelHeader` / `Renderer.RenderPanelHeader` | title, subtitle, badge, right status, or a tab strip |
| `PanelFooter` / `Renderer.RenderPanelFooter` | focused-panel key hints or a transient mode capsule |
| `TabStrip` / `TabItem` | active/inactive tabs with badges and overflow |
| `Badge` + `Tone` | neutral / accent / good / warning / danger status labels |
| `KeyHint` / `Renderer.RenderKeyHints` | compact key capsules with label-drop fallback |
| `ListItem` / `Renderer.RenderListItem` | polished selectable rows with rail, icon, meta, counter |
| `SectionDivider` | labelled rules for grouping content |
| `MetricRow` / `ProgressBar` / `Sparkline` | aligned metrics, gauges, and trends; gauges pick from six glyph sets, sparklines from six ramps, and sparkline cells grade green→yellow→orange→red across the run's min–max |
| `FocusChrome` | shared "what does focused mean" decisions |
| `StatusBar` regions / `Renderer.RenderStatusRegions` | three-region status strip with priority degradation |

```go
view := renderer.RenderMetricRow(tideui.MetricRow{
    Label: "CPU", Value: "18%", Fraction: 0.18, Bar: true,
    Tone: tideui.ToneGood, LabelWidth: 5, ValueWidth: 5, TotalWidth: width,
}, renderer.Styles.Workspace.Bg)
```

### Density modes

Density is a first-class look-and-feel setting:

| Mode | Row stride | Behaviour |
|---|---|---|
| `Comfortable` | 2 | breathing room; full metadata |
| `Compact` | 1 | default; full metadata |
| `Dense` | 1 | dashboard mode; hides subtitles, right-aligned meta, and counters; tighter footer labels |

```go
renderer := tideui.NewRenderer(theme, tideui.StyleOptions{Density: tideui.Dense})
```

### Focus presentation

`FocusPresentation` decides how the active panel is signalled without disabling
the others:

```go
ws.SetFocusPresentation(tideui.FocusPresentation{
    ActiveBorder: true, ActiveTitle: true, TitleCapsule: true, FocusRail: true,
    DimInactive: true, MutedSecondary: true, AccentMarker: true,
    InactiveSelection: true, StatusStrip: true, KeyHints: true,
})
```

The focused panel gains an accent frame, a title capsule, and an inner accent
rail; low-priority panels recede; an unfocused panel's selection stays visible
but muted so context survives.

### Capability degradation

The language degrades without losing meaning:

- **ASCII / VT52 themes** switch to ASCII borders, `|` separators, `#`/`-`
  gauges, and `.`/`@` sparklines; the demo swaps Unicode icons for ASCII.
- **No truecolor** — tones resolve through the active colour profile.
- **Narrow terminals** drop key labels before keys, drop commands before mode
  and identity, truncate titles, and hide secondary metadata at `Dense`.
- **No mouse / no animation** — every action has a keyboard path and all motion
  is opt-in.

## Dashboard widgets

TideDeck is built from first-party information widgets. They consume plain
data models and return themed, bounded blocks — acquisition stays in the
application, rendering stays in TideUI, so a real provider can be added later
without touching a renderer.

```go
type WeatherData struct{ /* temperature, feels-like, condition/kind, H/L, rain, wind, hourly, daily */ }
type AgendaItem   struct{ /* title, start/end, all-day, location, category, tone */ }
type SystemMetrics struct{ /* cpu, memory, temp, load, uptime, cores */ }
type NetworkMetrics struct{ /* down/up, unit, sparklines, LAN/WAN */ }
type StorageMount  struct{ /* path, used %, used/total */ }
type ServiceStatus struct{ /* name, state, detail, uptime, tone */ }
type Headline      struct{ /* title, source, age, unread */ }
type Task          struct{ /* title, done, due, tags, tone */ }
type Note          struct{ /* title, body, pinned */ }
type RepoActivity  struct{ /* name, branch, summary, commits */ }
type MarketQuote   struct{ /* symbol, price, change % */ }
```

Renderers on `Renderer`:

| Widget | Renderer | Detail renderer |
|---|---|---|
| Weather | `RenderWeather` | `RenderWeatherDetail` |
| Agenda | `RenderAgenda` (all-day events labelled, not `00:00`) | `RenderAgendaDetail` |
| Calendar | `RenderCalendar` (month grid beside the day's agenda) | `RenderCalendarDetail` |
| Clock | `RenderClock` (sun/moon glyph, day-period, world clocks) | `RenderClockDetail` (big digital time, day-progress gauge, analog face, `RenderMiniCalendar`) |
| System | `RenderSystem` | `RenderSystemDetail` |
| Network | `RenderNetwork` | `RenderNetworkDetail` |
| Storage | `RenderStorage` | — |
| Services | `RenderServices` | `RenderServicesDetail` |
| GPU | `RenderGPU` | `RenderGPUDetail` |
| Updates | `RenderUpdates` | `RenderUpdatesDetail` |
| News / RSS | `RenderHeadlines` | `RenderHeadlinesDetail` |
| Tasks | `RenderTasks` | — |
| Notes | `RenderNotes` | — |
| Git activity | `RenderRepoActivity` | `RenderRepoActivityDetail` |
| Markets | `RenderMarkets` | — |

```go
view := renderer.RenderAgenda(items, time.Now(), width)
```

Supporting primitives: `StatusDot`, `StatValue`, `RenderTrend`, `BarGauge`,
`MetricRow`, `ProgressBar`, `Sparkline`, `MiniCalendar`, and `SectionDivider`.

### Status vocabulary

Health and liveness use one shared vocabulary so a state reads the same in
Services, System, Network, and future widgets, and never depends on colour:

```go
tideui.StatusBadge(tideui.StatusWarning, "degraded") // badge + tone
renderer.RenderStatus(tideui.StatusHealthy, "healthy", bg) // ● healthy
```

`StatusHealthy`, `StatusWarning`, `StatusError`, `StatusStopped`,
`StatusActive`, `StatusStale`, and `StatusUpdating` each carry a glyph (with an
ASCII fallback), a semantic tone, and a word.

### Drill-down pattern

Every panel supports the same interaction: `Enter` zooms the focused panel and
the widget switches to its detail rendering; `Esc` returns. Detail is a plain
zoom, so the saved layout is never altered.

### Presets and the demo

`examples/workspace` ships five presets (`Overview`, `System`, `Productivity`,
`Developer`, `Minimal`) and a deterministic fake feed that updates over time.
The feed is a pure function of seed and time, so the demo is live but
reproducible and testable, and no network is involved.

## Real data sources

`github.com/allisonhere/tideui/provider` contains the live data sources for the
dashboard widgets. It uses only the standard library. Rendering and acquisition
stay separate: providers fill the same `tideui` data models the widgets already
consume.

### Collector

Each panel on the [panel registry](#panel-registry) owns its source and calls
it on its own interval, so a slow network call never blocks a render and a cheap
local sample can refresh every second while weather refreshes every ten minutes.
A failing fetch keeps the panel's previous value and is recorded against it;
the rest of the dashboard keeps working.

`provider.Fetcher` is the reusable caching wrapper for a source, and
`dash.State` is how a panel holds the result. A panel's `Refresh` runs off the
UI goroutine under a bounded context, and a failed fetch returns without storing
so the last good value stays on screen.

### Providers

| Source | Constructor | Backed by |
|---|---|---|
| Weather | `provider.Weather(WeatherOptions)` | Open-Meteo (no key) |
| Clock | `provider.Clock(location, zones...)` | local time + IANA zones |
| Calendar | `provider.Calendar(sources...)` | local `.ics` files, `https`/`webcal` iCal URLs, and Google Calendar embed/share links (TZID, all-day, 90-day window) |
| System | `provider.System()` | `/proc`, `/sys` (Linux) |
| Network | `provider.Network(iface)` | `/proc/net/dev` (Linux) |
| Storage | `provider.Storage()` | `/proc/mounts` + `statfs` (Linux) |
| Services | `provider.Systemd(units...)` / `provider.Docker(socket)` | systemd / Docker Engine API |
| GPU | `provider.GPU()` | DRM sysfs (Linux) |
| Updates | `provider.Updates(aurHelper)` | `omarchy`, `checkupdates`, AUR helper |
| News / RSS | `provider.Feed(urls...)` | RSS 2.0, RSS 1.0/RDF, and Atom |
| Tasks | `provider.TodoTxt(path)` | todo.txt |
| Notes | `provider.Notes(paths...)` | Markdown/text files |
| Git activity | `provider.Git(repos...)` | the `git` CLI (branch, ahead/behind, changes, stashes, interrupted rebase or merge) |
| Markets | `provider.Markets(symbols...)` | Yahoo Finance chart endpoint |

Linux-only sources return an error elsewhere, which simply leaves that panel
empty.

### Configuring TideDeck

Press `s` to open the **settings panel** — every provider setting is edited
there, nothing requires environment variables or hand-editing a file. The
panel is organized as a **category list** (`General`, `Weather`, `Calendar`,
`Clock`, `System`, `Network`, `Storage`, `Services`, `News`, `Tasks`, `Notes`,
`Git`, `Markets`) that opens into a page of fields, so a growing configuration
stays readable instead of becoming one long scroll:

- Each panel's category opens with an **enabled** tick, and then a **gauge
  style** and/or **spark style** choice *only for the metric styles that panel
  actually draws* (`Meta.Gauge`/`Meta.Spark`), so a list panel is not asked to
  pick a gauge it never renders. `default` follows the workspace style. A
  hidden panel's row shows `off` in the category list; the choices are saved
  with the layout.
- **Live data** toggles between the deterministic demo feed and real providers;
  **gauge style** cycles the glyph set used by every progress bar and metric
  gauge (`solid`, `blocks`, `circles`, `fisheye`, `marker`, `bars`) and
  **spark style** the ramp used by sparklines (`blocks`, `dots`, `braille`,
  `bullets`, `ticks`, `shades`, `heat`, `weighted`, `stroke`), **icons** the
  widget icon family (`emoji`, `plain`, `nerd`), and **clock font** the large-clock glyphs
  (`dash`, `block`) — set them via `StyleOptions.Gauge`/`StyleOptions.Sparkline`/
  `StyleOptions.ClockFont`, `tideui.GaugeStyles()`/`tideui.SparklineStyles()`/
  `tideui.ClockFonts()`. The gauge and sparkline pickers show the actual glyphs
  as their value.
- **Weather**: enable, city or ZIP, look up coordinates, latitude, longitude,
  location label, Fahrenheit, wind mph.
  Type a **city name or US ZIP** into "city or ZIP" and activate the
  **[ Look up coordinates ]** button just below it — it geocodes via Open-Meteo
  (with a Zippopotam ZIP fallback), fills in latitude, longitude, and the
  location label, and turns on live data.
  The panel then shows `unsaved changes — ctrl+s to apply`.
- **Clock** a 12/24-hour toggle and zones, **News** a tick list of curated
  sources plus a row for any other feed URL, **Calendar** `.ics` paths,
  `https`/`webcal` URLs, or a Google Calendar embed link (comma-separated;
  a private Google calendar needs its secret iCal address),
  **Tasks** `todo.txt`, **Notes** paths, **Git** repository paths,
  **Markets** symbols, **Updates** the AUR helper to count with,
  **Services** systemd units or a Docker socket, and the
  **Network** interface.

A panel on the registry brings its own category instead of being listed here:
its fields come from the `Schema()` it declares, under the same keys they
already had in the file. Keys the running build does not recognise — a plugin's
settings, a panel you have removed — are preserved on save rather than dropped.

On the category list, `↑/↓` choose and `enter` opens a category. Inside a
category, `↑/↓` move between fields, `enter` toggles a boolean or edits text
(or runs an action such as the lookup), and `esc` returns to the category list.
Choice fields such as **gauge style** step with `←/→` (or `enter`) and show the
actual glyph set as their value rather than a style name; the dashboard
previews live as they change. `ctrl+s` applies from anywhere, and `esc` on the
category list closes without saving (undoing any preview). On save the dashboard
is rebuilt from the new configuration immediately.

The config is stored at `~/.config/tidedeck/config.json` (application-scoped,
versionless) and the status strip shows `live` instead of `demo data`. Panels
whose provider is not configured simply start empty.

## Panel registry

`github.com/allisonhere/tideui/dash` binds a panel's three concerns — its data,
its rendering, and its settings — into one object, so adding a panel means
writing one file rather than editing ten.

It sits downstream of both other packages, and has to: `provider` imports
`tideui`, so `tideui` can never import `provider`, and a registry that both
fetches and draws must live below them. That is also what keeps `tideui`'s
promise to be purely presentational.

```go
deck := dash.New()
deck.Register(panels.GPU(), panels.Updates(), panels.Clock())
deck.Attach(workspace)           // registers each panel with the workspace

deck.Configure(values)           // hand every panel its settings
deck.Refresh(ctx, time.Now())    // fetch whatever is due
deck.Tick(time.Now())            // advance clock-driven panels
```

`Refresh` is **pull-based**: it checks each panel's interval on the tick the
application is already doing, rather than running a goroutine per panel. It
runs each fetch synchronously under a bounded context (`dash.DefaultTimeout`,
12 s), so call it from a Bubble Tea command rather than from `Update` — a slow
source would otherwise stall the frame. A failed fetch is recorded against the
panel (`deck.Err(id)`) and leaves the panel's last good value in place;
`deck.RefreshNow(id)` makes a panel due again, which is what a panel's own
refresh action should do.

`Tick` does no I/O and is safe on the UI goroutine: it advances `Ticker`
panels, and in `ModeDemo` refills every `Demoable` panel with sample data, so
the dashboard looks alive before anything is configured.

### Writing a panel

`Panel` is two methods. Everything else is opt-in, so the smallest panel is a
`Meta` and a `View`:

| Interface | Methods | Implement it when the panel |
|---|---|---|
| `Panel` | `Meta() Meta`, `View(tideui.PanelContext) string` | *(required)* |
| `Fetcher` | `Refresh(context.Context) error` | acquires data |
| `Configurable` | `Schema() []Field`, `Configure(Values) error` | has settings |
| `Demoable` | `Demo(now time.Time)` | can synthesise sample data |
| `Ticker` | `Tick(now time.Time)` | changes with the clock between refreshes |
| `Badger` | `Badge() (string, tideui.Tone)` | advertises a header badge |
| `Actor` | `Actions() []Action` | offers contextual keys |
| `Input` | `Type(rune) bool`, `Backspace() bool` | takes typing while focused (see `panels.Calculator`) |
| `Copier` | `Copy() (string, bool)` | offers content the `c` key copies (see `panels.Calculator`) |

An `Action` with `Refresh: true` makes the deck treat the panel as due again
before `Run` is called, so a refresh key refetches rather than only printing a
message claiming it did; a panel has no reference to the deck and cannot ask
for that itself.

Nothing in `Panel` mentions where data comes from, and no panel's model type
appears in it. That is deliberate: a panel backed by a local provider and a
panel backed by an out-of-process program are the same kind of thing to a
`Deck`.

`dash.State[T]` is how a panel holds its data — a mutex-guarded value that
`Refresh` stores into and `View` loads from, so the fetching and rendering
goroutines never have to coordinate:

```go
type gpu struct {
    dash.State[tideui.GPUMetrics]
    fetch func(context.Context) (tideui.GPUMetrics, error)
}

func (g *gpu) Meta() dash.Meta {
    return dash.Meta{ID: "gpu", Title: "GPU", Role: tideui.RoleSecondary,
        Priority: 72, MinWidth: 18, MinHeight: 6, HideBelow: 104,
        Interval: time.Second}
}

func (g *gpu) Refresh(ctx context.Context) error {
    metrics, err := g.fetch(ctx)
    if err != nil {
        return err // the last good reading stays on screen
    }
    g.Store(metrics)
    return nil
}

func (g *gpu) View(ctx tideui.PanelContext) string {
    return ctx.Renderer.RenderGPU(g.Load(), ctx.Width)
}
```

A panel with `Interval: 0` that implements `Fetcher` is fetched once. A panel
that implements `Ticker` and not `Fetcher` — the clock is the example — does no
I/O at all and simply recomputes on the tick.

### Settings a panel declares

A `Configurable` panel declares its own fields, and the settings screen builds
that panel's category from them. There is no hand-written list to keep in sync,
and a panel's settings arrive back at the panel through `Configure`:

```go
func (u *updates) Schema() []dash.Field {
    return []dash.Field{{
        Key: "aur_helper", Label: "aur helper", Kind: dash.FieldText, Default: "yay",
    }}
}
```

| `FieldKind` | Edited as |
|---|---|
| `FieldText` | free text |
| `FieldBool` | a tick |
| `FieldChoice` | one of `Options`, stepped with `←/→` |
| `FieldFloat` | a number, validated on save |
| `FieldAction` | a button that runs `Field.Run` |

`Field.Key` is a dotted path into the configuration document, and it names the
key that is *already* in `config.json` — moving a setting onto its panel does
not rename it or rewrite anyone's file. `Normalize` tidies a value on save and
`Summary` renders a long value to fit one row.

`dash.Values` is that document: decoded JSON addressed by path, which **keeps
keys it does not recognise**. A key belonging to a panel this build does not
have survives a load and a save unchanged, so installing, removing, and
reinstalling a panel does not cost you its configuration — which a typed struct
cannot do, because it silently drops every field it has no name for.

```go
values.String("weather.location")
values.Float("weather.latitude")
values.List("zones")            // comma-separated, trimmed
values.Has("clock_24")          // present, as opposed to false
```

Use `Has` for any boolean whose default is `true`: `Bool` returns `false` for
an absent key, so without it a fresh install reads as a deliberate *off*.

### External plugins: a panel that is a program

A plugin is a directory with a `manifest.json` and an executable that prints a
JSON document. The entry point is run on the panel's interval, with no
arguments, and exits; there is nothing to supervise, restart, or leak.

> **These are not Quickshell/Omarchy plugins, and the two are not
> interchangeable.** An Omarchy plugin is QML loaded into a running shell
> process and drawn on a Wayland surface. tidedeck writes text to a terminal
> and runs a program. The *shape* of the manifest is deliberately the same, so
> that someone who has written one already knows this format — but a QML entry
> point is rejected, with that as the reason.

`~/.config/tidedeck/plugins/<id>/manifest.json`:

```json
{
  "schemaVersion": 1,
  "id": "drbayless.ai-usage",
  "name": "AI Usage",
  "version": "1.0.0",
  "author": "drbayless",
  "license": "MIT",
  "description": "Plan usage and balances, read from the ai-usagebar binary.",
  "kinds": ["panel"],
  "entryPoints": { "panel": ["./render.sh"] },
  "panel": {
    "displayName": "AI Usage",
    "category": "AI",
    "refreshSeconds": 300,
    "minWidth": 22, "minHeight": 5, "priority": 45,
    "gauge": true, "spark": true,
    "schema": [
      { "key": "binary", "type": "string", "label": "ai-usagebar binary",
        "defaultValue": "ai-usagebar" }
    ]
  }
}
```

`id` must be namespaced (`author.name`), `kinds` is `["panel"]` in this build,
and a relative entry point resolves against the plugin's own directory, not the
working directory the dashboard was started in. `refreshSeconds` has a floor of
two seconds: a program that wants to be sampled faster than that is the wrong
shape for a subprocess. `Manifest.Validate` reports everything wrong at once,
rather than one problem per run.

`gauge` and `spark` claim whether the document prints gauge or spark rows, so
the plugin's settings page offers only the metric styles it uses and not the
other. A plugin that only prints text leaves both off.

Declared settings reach the program as environment variables —
`TIDEDECK_PLUGIN_<KEY>`, uppercased, non-alphanumerics replaced with `_` — and
only the declared ones: a plugin gets what it asked for, and the dashboard's
other settings are not its business. In the configuration document they live
under `plugins.<id>.<key>`, so a plugin cannot collide with a built-in panel's
key or with another plugin's.

#### The document

```json
{
  "schemaVersion": 1,
  "badge": { "text": "3", "tone": "warning" },
  "rows": [
    { "type": "metric",  "label": "Session", "value": "43%", "percent": 43, "severity": "low" },
    { "type": "gauge",   "label": "MEM", "value": "41%", "percent": 41 },
    { "type": "spark",   "label": "CPU", "value": "18%", "history": [0.1, 0.4, 0.9] },
    { "type": "text",    "label": "Balance", "value": "$7.02" },
    { "type": "block",   "label": "Credits", "body": ["balance: 0"] },
    { "type": "divider", "label": "DETAIL" },
    { "type": "spacer" }
  ],
  "detail": [ { "type": "text", "label": "Plan", "value": "Claude Pro" } ]
}
```

| Row type | Renders as |
|---|---|
| `metric` | label, value, and a bar when `percent` is set |
| `gauge` | label, value, and a bar |
| `spark` | label, value, and a sparkline from `history` (0..1 samples) |
| `text` | a label/value pair |
| `block` | a label plus indented `body` lines |
| `divider` | a section divider |
| `spacer` | a blank line |

A plugin's settings are edited in the settings screen and reach the program as
environment variables on the next run. A plugin can also **take typing**:
declaring `panel.input` names a setting that receives keystrokes while the panel
is focused, and `panel.inputChars` lists the runes it accepts, so anything else
(and the workspace's own keys, like `alt+1`–`alt+5` for presets) still reaches
the application. What was typed is passed to the program as that setting's
environment variable, and the program is re-run on each keystroke:

```json
"panel": {
  "input": "expression",
  "inputChars": "0123456789.+-*/() ",
  "schema": [{ "key": "expression", "type": "string", "label": "expression" }]
}
```

A panel with no `input` is a read-only rendering of its document.

A plugin can also offer a value to copy: `panel.copy` names a row whose value
the `c` key puts on the clipboard (via OSC 52, so it works over SSH), with no
code of its own:

```json
"panel": { "copy": "Balance" }
```

`detail` is what a zoomed panel shows; absent means reuse `rows`. Colour comes
from `tone` (`good`, `warning`, `danger`, `muted`, `accent`) or from `severity`
(`low`, `mid`, `high`), which is accepted because other tools in this space
already speak it; `tone` wins when both are given. An unknown row type is
skipped rather than failing the document, so a plugin written against a later
schema loses a line instead of disappearing. Every row is drawn with the same
primitives the built-in panels use and bounded by `Renderer.RenderLines`, so a
plugin cannot overflow its pane whatever it prints.

`contrib/ai-usage/` is a working example: a `jq` projection of
`ai-usagebar usage --json`, which is a projection rather than a translation
because the two formats already agree on `type`/`label`/`value`/`percent`/
`severity`. `contrib/calculator/` is a smaller one, and is also published at
[`allisonhere/tidedeck-plugins`](https://github.com/allisonhere/tidedeck-plugins)
so you can install it from a live URL with
`https://github.com/allisonhere/tidedeck-plugins#calculator`.

#### Running them

```go
plugins, problems := dash.LoadPlugins(filepath.Join(configDir, "plugins"))
deck.Register(plugins...)   // a plugin panel is just a Panel
```

Each subdirectory is one plugin; one that does not validate is skipped with its
reason rather than stopping the others from loading, and a missing plugin
directory is the normal case, not an error. `dash.Exec(manifest)` builds a
single one.

To install one at runtime, `dash.Install(dir, source)` fetches a source into
staging, validates it, and moves it into `dir`; a `source#subdir` names the
directory inside the source that holds the plugin, so one repository can host
several. `dash.Installed(dir)` lists what is there, and `dash.Remove`/`dash.Update`
delete or re-fetch one. A panel added while the app is running is attached with
`deck.AttachPanel(ws, panel)` and removed with `deck.Unregister(id)` plus
`ws.RemovePanel(id)`.

Failure of any kind — a non-zero exit, unparseable output, a timeout, more than
1 MB printed, or a `schemaVersion` this build does not know — keeps the last
good document and records the error, exactly as a failing provider does.
`stderr` is captured for that error message and never rendered as content.
Killing an entry point does not necessarily end what it started, so a script
that leaves a child holding the output pipe is bounded too.

#### Installing and removing one

Open settings (`s`) → **Plugins**. Paste a plugin's repository URL — or a local
directory, useful while writing one — and press **Install**. The app clones
into `~/.config/tidedeck/plugins/` off the UI thread, validates the manifest,
and registers the panel. Each installed plugin is listed with an **update**
(re-fetches from its source) and a **remove** row. The same can be done by
hand: drop a directory into `~/.config/tidedeck/plugins/` and restart.

A source may name a **subdirectory** so one repository can host several
plugins: `https://github.com/you/tidedeck-plugins#calculator` installs the
plugin whose manifest lives in `calculator/`. A local path takes the same form
(`~/Projects/tidedeck-plugins#calculator`).

A manifest that does not validate is skipped and its reason is shown in the
status strip (or on its row in the Plugins page); the others still load.

A plugin **starts hidden**, so installing one does not rearrange every preset.
Enable it from the panel picker (`space`) or from its page in settings, which
is also where its declared settings are edited. It runs whatever the
demo/live setting says: a plugin is a real program, not the sample data demo
mode stands in for. Its settings live under `plugins.<id>.<key>` in the
configuration document, and those keys are preserved when the plugin is
removed, so reinstalling it restores its settings.

**Installing runs no plugin code**: a clone does not execute anything the
repository ships. The program first runs when the panel is enabled and
refreshed, which is the moment to trust it.

**Plugins run unsandboxed, with your user permissions.** This is an extension
mechanism for a single-user dashboard, not a security boundary, and there is no
honest way to describe it otherwise: a plugin can do anything you can do.

## Terminal background

`tideui` never writes to the terminal itself. To paint the terminal background
to match the theme, fetch the sequences and emit them from your program:

```go
set, reset := tideui.TerminalBackgroundSequences(theme)
```

## Design

Applications retain ownership of their model, commands, key routing,
persistence, and viewport state. `tideui` is purely presentational — it turns
content + dimensions + a theme into a bounded, framed string. The one exception
is the optional `ThemePicker`, which manages its own navigation once opened.

## License

MIT.
