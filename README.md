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
- **Workspace system** — a tiny tiling window manager for TUIs: declarative panels, a layout tree, semantic adaptive reflow, live arrange/resize modes, tab stacks, zoom, peek, contextual actions, a panel picker, command palette, presets, undo/redo, mouse support, and versioned persistence.
- **Visual language** — semantic workspace tokens, reusable chrome primitives (headers, footers, tabs, badges, key capsules, metrics, sparklines, list rows), `Comfortable`/`Compact`/`Dense` density modes, and a configurable focus presentation.
- **Dashboard widgets** — weather, agenda, clock, system, network, storage, services, news, tasks, notes, git activity, and markets, each driven by a plain data model and backed by a `Renderer` method, with a shared Enter-to-drill-down pattern.
- **Real data sources** — a standard-library `provider` package: background collectors with per-source intervals and graceful degradation, plus providers for Open-Meteo weather, RSS/Atom, Linux system/network/storage, systemd and Docker, git activity, todo.txt, notes, iCalendar, and markets.
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

### Arrange and resize modes

Arrange mode moves the real panel: each direction key immediately repositions
the focused panel against its neighbour, so the layout itself is the preview.
The gap the panel leaves behind closes, every move is recorded in history, and
`esc` simply leaves the mode. Panels that are surrounded on all sides are moved
one step at a time without any docking cursor or overlay.

Resize mode works on **dividers** — the boundary between two adjacent regions —
rather than on a panel. Select a divider, then move it in either direction
(`h`/`l` for a column boundary, `k`/`j` for a row boundary). This means a panel
surrounded on every side is resized by choosing the boundary you care about,
not by guessing which edge a direction maps to. The selected divider is
highlighted while the mode is active, and weights are shifted rather than
absolute coordinates, so minimum sizes are always honored.

```go
ws.ToggleArrange()                    // m
ws.ArrangeMove(tideui.DirRight)       // h/j/k/l or arrows: move the panel live
ws.ArrangeMerge()                     // t: fold into the neighbour's tab stack
ws.ExitArrange()                      // esc: leave the mode
ws.Undo()                             // each live move is one history entry

ws.SetResizeMode(true)   // R; direction key selects a divider, then moves it
ws.Dividers()            // boundaries of the current layout
ws.CycleResizeDivider(1) // tab: cycle dividers (so you can pick any boundary)
ws.ResizeDivider(tideui.DirRight) // move the selected divider
ws.SetResizeMode(false)  // esc

// Panel-relative resize remains available programmatically:
ws.Resize(tideui.DirRight, 0)         // ctrl+right
ws.ResizeGrow(0) / ws.ResizeShrink(0) // nearest split axis
ws.ResizeWidth(true) / ws.ResizeHeight(true)
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
| `R` | toggle resize mode (direction key selects a divider, then moves it; `tab` cycles dividers; `esc` done) |
| `ctrl+arrows` | one-step resize of the focused split |
| `shift+space` | zoom / restore the focused panel |
| `w` | panel picker |
| `ctrl+p` | command palette |
| `esc` | dismiss peek / zoom / picker / mode |

Mouse support is additive: clicking focuses a panel, clicking a tab selects it,
and dragging a separator resizes the adjacent split. `Workspace.HandleMouse`
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
| `MetricRow` / `ProgressBar` / `Sparkline` | aligned metrics, gauges, and trends |
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
type WeatherData struct{ /* temperature, condition, H/L, rain, wind, hourly, daily */ }
type AgendaItem   struct{ /* title, start/end, location, category, tone */ }
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
| Agenda | `RenderAgenda` | `RenderAgendaDetail` |
| Clock | `RenderClock` | `RenderClockDetail` (with `RenderMiniCalendar`) |
| System | `RenderSystem` | `RenderSystemDetail` |
| Network | `RenderNetwork` | `RenderNetworkDetail` |
| Storage | `RenderStorage` | — |
| Services | `RenderServices` | `RenderServicesDetail` |
| News / RSS | `RenderHeadlines` | `RenderHeadlinesDetail` |
| Tasks | `RenderTasks` | — |
| Notes | `RenderNotes` | — |
| Git activity | `RenderRepoActivity` | — |
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

A `Dashboard` holds one background `Fetcher` per source. Each fetch runs off
the UI goroutine with its own interval and timeout, so a slow network call never
blocks a render and a cheap local sample can refresh every second while weather
refreshes every ten minutes. A failing source keeps its previous value and
records an error; the rest of the dashboard keeps working.

```go
dashboard := &provider.Dashboard{
    Weather: provider.NewFetcher(10*time.Minute, provider.Weather(provider.WeatherOptions{
        Latitude: 52.52, Longitude: 13.405, Fahrenheit: true, WindMPH: true,
    })),
    System:  provider.NewFetcher(time.Second, provider.System()),
    Network: provider.NewFetcher(time.Second, provider.Network("wlan0")),
    Storage: provider.NewFetcher(2*time.Minute, provider.Storage()),
    Clock:   provider.NewFetcher(time.Minute, provider.Clock("Local", "Europe/London", "Asia/Tokyo")),
    // ...
}

// on the application tick:
dashboard.Refresh(ctx)
snapshot := dashboard.Snapshot() // reads cache only, never blocks
```

### Providers

| Source | Constructor | Backed by |
|---|---|---|
| Weather | `provider.Weather(WeatherOptions)` | Open-Meteo (no key) |
| Clock | `provider.Clock(location, zones...)` | local time + IANA zones |
| Calendar | `provider.Calendar(paths...)` | local `.ics` files |
| System | `provider.System()` | `/proc`, `/sys` (Linux) |
| Network | `provider.Network(iface)` | `/proc/net/dev` (Linux) |
| Storage | `provider.Storage()` | `/proc/mounts` + `statfs` (Linux) |
| Services | `provider.Systemd(units...)` / `provider.Docker(socket)` | systemd / Docker Engine API |
| News / RSS | `provider.Feed(urls...)` | RSS 2.0 and Atom |
| Tasks | `provider.TodoTxt(path)` | todo.txt |
| Notes | `provider.Notes(paths...)` | Markdown/text files |
| Git activity | `provider.Git(repos...)` | the `git` CLI |
| Markets | `provider.Markets(symbols...)` | Yahoo Finance chart endpoint |

Linux-only sources return an error elsewhere, which simply leaves that panel
empty.

### Running TideDeck live

Set `TIDEDECK_LIVE=1` to switch the demo from the fake feed to real providers.
Everything else is optional:

```bash
TIDEDECK_LIVE=1 \
TIDEDECK_LAT=52.52 TIDEDECK_LON=13.405 TIDEDECK_LOCATION=Berlin \
TIDEDECK_ZONES=Europe/London,Asia/Tokyo \
TIDEDECK_FEEDS=https://lwn.net/headlines/rss \
TIDEDECK_ICS=~/.local/share/cal.ics \
TIDEDECK_TODO=~/todo.txt \
TIDEDECK_NOTES=~/notes \
TIDEDECK_REPOS=~/Projects/tidedeck,~/Projects/tideui \
TIDEDECK_SYMBOLS=AMD,NVDA,SPY \
TIDEDECK_SYSTEMD=sshd,docker \
go run ./examples/workspace
```

The status strip shows `live` instead of `demo data`. Unset any variable and the
corresponding panel simply starts empty.

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
