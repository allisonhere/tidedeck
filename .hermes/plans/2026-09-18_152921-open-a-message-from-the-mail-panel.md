# Open the selected message in TideMail from the mail panel

## Goal

Pressing `Enter` on the focused mail panel lets the arrow keys walk a cursor
through the messages it previews, and pressing `Enter` again on one launches
TideMail on that message.

## What is already there (verified in this checkout, 2026-09-18)

Read these before writing anything; every task below assumes them.

- The dashboard is `examples/workspace/main.go` (a bubbletea `model`); the
  library is the module root (`package tideui`) plus `dash/`, `form/`,
  `provider/`.
- Panels are objects. The application routes keys to a focused panel through
  small interfaces declared in `dash/dash.go`:
  - `dash.Cursor` (`Move(delta int) bool`) - a panel with a moveable selection.
  - `dash.Activator` (`Activate() (copy, status string)`) - a panel whose
    selection has a primary action. The news list uses it to copy a link.
  - `dash.Copier`, `dash.Clicker`, `dash.Input` - same idea, other verbs.
- `examples/workspace/main.go:887` `moveSelection(delta)` hands a move to the
  focused panel's `dash.Cursor` only while the panel owns the screen
  (`m.ws.Zoomed() != ""`), because in the tiled dashboard the arrow keys belong
  to pane navigation. `main_test.go:384` locks that in. Task 7 widens the gate to
  "the pane has the keyboard", which space will provide.
- `examples/workspace/main.go:1032` the `enter` case: `activateFocused()` first,
  then zoom/unzoom. `main.go:923` `activateFocused()` does the `Activator` call.
  Task 7 removes the panel-action half of that, so Enter is the zoom and nothing
  else.
- `Workspace.HandleKey` (`workspace.go:1435`) handles `tab`/`m`/`w`/`ctrl+p`/
  `shift+space`(zoom)/`esc`, then falls through to `RunFocusedAction(key)`
  (`workspace.go:1517`), which dispatches a key to the focused panel's bound
  actions and already answers to `" "` as `"space"`. That is the door space
  enters a pane through (Task 6).
- `dash/panels/news.go` is the reference implementation of all of this: it keeps
  `cursor int`, marks a copy of its data (`marked[cursor].Selected = true`)
  **only when `ctx.Focused`**, renders the selection as one highlighted block
  (`styles.go` `SelectionBg`/`SelectionFg`, see `dashboard_widgets.go:1212`),
  and clamps the cursor in `Move`. `ctx.Focused` becomes `ctx.Entered` for the
  cursor (Task 6).
- Plugins are programs. `dash/exec.go` `execPanel` runs an entry point on an
  interval and stores the `Doc` it prints; `dash/doc.go` renders it. The mail
  panel is the plugin `contrib/mail/` (installed at
  `~/.config/tidedeck/plugins/tidedeck.mail/`), whose `render.sh` reads
  TideMail's SQLite cache read-only.
- `dash/manifest.go` declares what a plugin may ask for: `entryPoints.panel`,
  `panel.schema`, `panel.input`/`inputChars`, `panel.copy`. `Manifest.Command`
  resolves a `./relative` entry point against the plugin directory.
- TideMail (`~/Projects/tidemail`, installed as `/usr/bin/tidemail` v1.0.23,
  AUR-packaged) is a foreground TUI. `main.go:27 parseStartupOptions` reads a
  handful of dev flags and **silently ignores unknown arguments**, which is what
  makes the two-phase delivery in Task 10 safe.

## Architecture

A document row may carry an `id`; the manifest declares the command that opens
such a row, with `{id}` standing for the picked row's id. `space` gives a pane
the keyboard (any pane, `esc` to leave), so the plugin panel (`execPanel`) can
keep a cursor over the rows that carry an id and draw it as the same selection
block the news list uses. The panel exposes that selection through two more of
the existing panel interfaces: `dash.Cursor` for the arrows and a new
`dash.Launcher` that returns the argv to run. Running it is the application's
job, not the panel's: `examples/workspace` suspends itself with
`tea.ExecProcess`, hands the terminal to TideMail, and refreshes the panel when
TideMail exits.

## Decisions recorded (do not re-litigate these while implementing)

1. **Selection is drawn as a block, not a rail or a colour change.** It is the
   same `SelectionBg`/`SelectionFg` block the news cursor is drawn with, so a
   plugin's list reads like a built-in one.
2. **`space` enters a pane; `enter` stays zoom.** Space hands the keyboard to the
   focused pane - any pane, not only one with a list in it - so a list can be
   walked in the tiled layout without leaving it. `esc` hands the keys back, and
   moving focus leaves the pane behind. Enter keeps one meaning at pane level
   everywhere: zoom / restore (`shift+space` keeps working as the long-standing
   shortcut for the same thing).
3. **Inside an entered pane, Enter is that pane's primary action** - open the
   picked message, copy the news link - which is the gesture the news list
   already ships with, and the one the original ask describes ("hitting enter on
   one launches tidemail"). Zoom stays one `esc` away. If Enter should keep
   zooming inside a pane as well, the action moves to another key, and the only
   place to change is the `case "enter"` branch in `examples/workspace/main.go`.
4. **The arrows follow the keyboard.** Not entered, the workspace keys behave
   exactly as today: `tab`/`h`/`j`/`k`/`l`/arrows move focus, `shift+space`
   zooms. Entered, `↑`/`↓` (and `j`/`k`) drive the pane's cursor and the pane's
   own keys work.
5. **The id is the plugin's own opaque string.** For mail it is TideMail's
   `messages.id` rowid. Nothing in `dash` parses or interprets it.
6. **The panel returns argv; the application runs it.** A panel that ran a
   program itself would have to own the terminal, which is exactly the coupling
   `dash.Copier` avoids by returning text for the caller to put on a clipboard.
7. **No new terminal window.** The dashboard suspends itself (alt-screen off,
   terminal handed over), the same as quitting and running `tidemail` by hand.
8. **The mail panel launches `tidemail --open {id}`.** TideMail 1.0.23 ignores
   the unknown flag, so this ships now and starts landing on the message when
   Task 10 ships. Do not gate Tasks 1-9 on TideMail changing.

## Not doing (YAGNI, asked for nothing)

- Clicking a message row (`dash.Clicker`) - only the keyboard path is in scope.
- Passing the plugin's configured settings to the open command (see Risks).
- A `{label}`/`{value}` placeholder: only `{id}` until something needs more.
- Selecting metric/gauge/spark/divider rows: an id is honoured on `text` and
  `block` rows only, the two shapes a list is made of.

---

## Task 1 - Give a document row an identity

Files: `dash/doc.go`, `dash/doc_test.go`.

**1a. Write the failing test.** Append to `dash/doc_test.go`:

```go
// A row may carry an id, which is what makes it openable: it is an opaque value
// the plugin chose - a mail plugin puts the message's row id here - and the
// panel hands it back to the command the manifest declares.
func TestRowCarriesAnID(t *testing.T) {
	var doc Doc
	if err := json.Unmarshal([]byte(`{"rows":[
		{"type":"block","id":"41","label":"ana@example.com","body":["standup notes"]},
		{"type":"spacer"}]}`), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(doc.Rows))
	}
	if doc.Rows[0].ID != "41" {
		t.Errorf("id = %q, want %q", doc.Rows[0].ID, "41")
	}
	// A row without an id is text, not a destination.
	if doc.Rows[1].ID != "" {
		t.Errorf("a spacer has id %q", doc.Rows[1].ID)
	}
}
```

`dash/doc_test.go` does not import `encoding/json` yet - add it to the import
block.

Run it:

```
cd /home/allie/Projects/tidedeck && go test -run TestRowCarriesAnID ./dash/
```

Expected: it fails to **compile** (`doc.Rows[0].ID undefined`) - that is the red
state.

**1b. Make it pass.** In `dash/doc.go`, add to `Row` (after `BodyTone`):

```go
	// ID makes a row openable. It is an opaque identifier the plugin chose -
	// the mail plugin puts the message's row id here - and the panel hands it
	// back to the command the manifest declares under "open". A row without
	// one is text: the cursor skips it, so a setup hint is not a destination.
	ID string `json:"id"`
```

Run:

```
go test -run TestRowCarriesAnID ./dash/
```

Expected: `ok  github.com/allisonhere/tideui/dash`.

**1c. Commit.**

```
git add dash/doc.go dash/doc_test.go
git commit -m "Make a document row openable by giving it an id"
```

---

## Task 2 - Draw the selected row as the cursor block

Files: `dash/doc.go`, `dash/exec.go`, `dash/doc_test.go`, `dash/readme_test.go`.

**2a. Write the failing test.** Append to `dash/doc_test.go`:

```go
// The row the reader picked is drawn as one selection block - every line of it
// in the cursor colours, padded to the pane - the same affair the news list
// draws its cursor with, so a plugin's list reads like a built-in one.
func TestRenderDocMarksTheSelectedRow(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	renderer := docRenderer()
	ws := renderer.Styles.Workspace
	doc := Doc{Rows: []Row{
		{Type: "block", ID: "1", Label: "ana@example.com · 5m", Body: []string{"standup notes"}},
		{Type: "spacer"},
		{Type: "block", ID: "2", Label: "sam@example.com · 1h", Body: []string{"dinner?"}},
	}}

	out := RenderDoc(renderer, doc, 30, false, "2")
	block := lipgloss.NewStyle().Background(ws.SelectionBg).Foreground(ws.SelectionFg).Width(30)
	if want := block.Render("sam@example.com · 1h"); !strings.Contains(out, want) {
		t.Errorf("the selected block's label is not the cursor block:\n%q", out)
	}
	if want := block.Render("  dinner?"); !strings.Contains(out, want) {
		t.Errorf("the selected block's body is not the cursor block:\n%q", out)
	}
	// An id nothing carries marks nothing, so a document that changed under the
	// cursor still draws.
	plain := RenderDoc(renderer, doc, 30, false, "gone")
	for _, line := range strings.Split(plain, "\n") {
		for _, want := range []string{"dinner?", "standup notes"} {
			if strings.Contains(line, want) && strings.Contains(line, "\x1b[48;2;") &&
				strings.Contains(line, string(ws.SelectionBg)) {
				t.Errorf("an id nothing carries still marked a row: %q", line)
			}
		}
	}
}
```

Run:

```
go test -run TestRenderDocMarksTheSelectedRow ./dash/
```

Expected: compile error, `too many arguments in call to RenderDoc` - red.

**2b. Make it pass.** In `dash/doc.go`:

1. Add the import `"github.com/charmbracelet/x/ansi"`.
2. Extract the rows a document draws (used by the panel too):

```go
// shownRows is the row list a document draws: its detail rows when it has them
// and the panel is zoomed, otherwise its rows. The panel keeps the same answer
// for the cursor it moves, so what the cursor counts is what the reader sees.
func shownRows(doc Doc, zoomed bool) []Row {
	if zoomed && len(doc.Detail) > 0 {
		return doc.Detail
	}
	return doc.Rows
}
```

3. Change `RenderDoc` to take the selected row's id and use `shownRows`:

```go
// RenderDoc draws a document at a width. selected is the id of the row the
// reader put the cursor on (empty for none); that row is drawn as the panel
// selection block. Rows are rendered with the same primitives the built-in
// panels use, and the result is bounded by Renderer.RenderLines, so a plugin
// cannot overflow its pane whatever it prints.
func RenderDoc(renderer tideui.Renderer, doc Doc, width int, zoomed bool, selected string) string {
	rows := shownRows(doc, zoomed)
	...
	for _, row := range rows {
		lines = append(lines, renderRow(renderer, row, width, labelWidth, bg, row.ID != "" && row.ID == selected)...)
	}
```

4. Give `renderRow` the flag and add the selected rendering:

```go
func renderRow(renderer tideui.Renderer, row Row, width, labelWidth int, bg lipgloss.Color, selected bool) []string {
	if selected {
		return renderRowSelected(renderer, row, width, labelWidth, bg)
	}
	// ...the existing switch, unchanged...
}

// renderRowSelected draws a row the reader picked as one selection block: every
// line of it takes the cursor colours and is padded to the pane, the way the
// news list's cursor is drawn. A type that is not a list row (a gauge, a
// divider) is drawn as usual, so a plugin cannot mark one by attaching an id.
func renderRowSelected(renderer tideui.Renderer, row Row, width, labelWidth int, bg lipgloss.Color) []string {
	ws := renderer.Styles.Workspace
	style := lipgloss.NewStyle().Background(ws.SelectionBg).Foreground(ws.SelectionFg).Width(width)
	switch strings.ToLower(strings.TrimSpace(row.Type)) {
	case "text":
		return []string{style.Render(ansi.Truncate(docRowText(row.Label, row.Value, labelWidth), width, "…"))}
	case "block":
		lines := make([]string, 0, len(row.Body)+1)
		if row.Label != "" {
			lines = append(lines, style.Render(ansi.Truncate(docRowText(row.Label, row.Value, labelWidth), width, "…")))
		}
		for _, line := range row.Body {
			lines = append(lines, style.Render(ansi.Truncate("  "+line, width, "…")))
		}
		return lines
	}
	return renderRow(renderer, row, width, labelWidth, bg, false)
}
```

5. Extract the pair's text so a row draws the same bytes either way, and keep
   `docPair` as the unstyled path:

```go
// docRowText is what a label/value row says, without styling, so the same bytes
// can be drawn as a muted pair or as one selection block.
func docRowText(label, value string, labelWidth int) string {
	if label == "" {
		return value
	}
	return padLabel(label, labelWidth) + "  " + value
}
```

6. Update the call in `dash/exec.go` `View` for now:
   `RenderDoc(ctx.Renderer, e.Load(), ctx.Width, ctx.Zoomed, "")`.
7. Add `, ""` to every remaining `RenderDoc(` call: `dash/readme_test.go:56,62`
   and the ten in `dash/doc_test.go`. `go build ./... && go vet ./...` names each
   one that is still missing it.

Run:

```
go build ./... && go vet ./... && go test ./dash/
```

Expected: build clean, `ok  github.com/allisonhere/tideui/dash`.

**2c. Commit.**

```
git add dash/doc.go dash/exec.go dash/doc_test.go dash/readme_test.go
git commit -m "Draw the row the reader picked as the panel's cursor block"
```

---

## Task 3 - A cursor over the openable rows

Files: `dash/exec.go`, `dash/exec_test.go`.

**3a. Write the failing tests.** Append to `dash/exec_test.go`:

```go
// openableFixture is a plugin whose document holds two openable blocks and the
// non-rows a real one prints between them.
func openableFixture(t *testing.T) Panel {
	t.Helper()
	manifest := plugin(t, `cat <<'JSON'
{"rows":[
  {"type":"text","label":"mail","value":"3 unread","tone":"muted"},
  {"type":"block","id":"7","label":"ana@example.com · 5m","body":["standup notes"]},
  {"type":"spacer"},
  {"type":"block","id":"9","label":"sam@example.com · 1h","body":["dinner?"]},
  {"type":"spacer"}]}
JSON`, map[string]any{"panel": map[string]any{"open": []string{"tidemail", "--open", "{id}"}}})
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return panel
}

// view draws the panel once, which is what tells it which rows are on screen.
// Entered is what has the keyboard: space enters a pane, and a zoomed pane has
// it by definition.
func view(panel Panel, zoomed bool) string {
	return panel.View(tideui.PanelContext{
		ID: "test.plugin", Width: 30, Zoomed: zoomed, Entered: true, Renderer: docRenderer(),
	})
}

// The cursor counts the rows that carry an id, not every row: a spacer or a
// setup hint is not a destination.
func TestExecPanelCursorSkipsRowsWithNoID(t *testing.T) {
	panel := openableFixture(t)
	view(panel, false)

	cursor, ok := panel.(Cursor)
	if !ok {
		t.Fatal("a plugin with openable rows is not a cursor")
	}
	if !cursor.Move(1) {
		t.Fatal("the cursor refused to move over two openable rows")
	}
	if argv, _, _ := panel.(Launcher).Launch(); len(argv) == 0 || argv[len(argv)-1] != "9" {
		t.Fatalf("after one Move the selection is %v, want the second block", argv)
	}
	// Clamped at both ends rather than wrapping or running off.
	cursor.Move(1)
	argv, _, _ := panel.(Launcher).Launch()
	if argv[len(argv)-1] != "9" {
		t.Fatalf("moving past the last row selected %v", argv)
	}
	cursor.Move(-5)
	argv, _, _ = panel.(Launcher).Launch()
	if argv[len(argv)-1] != "7" {
		t.Fatalf("moving above the first row selected %v", argv)
	}
}

// A document with nothing openable leaves the arrows to the workspace, so they
// still move focus instead of being swallowed by a panel.
func TestExecPanelWithoutOpenableRowsIsNotACursor(t *testing.T) {
	manifest := plugin(t, `printf '{"rows":[{"type":"text","label":"mail","value":"empty"}]}\n'`, nil)
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, false)
	if cursor, ok := panel.(Cursor); ok && cursor.Move(1) {
		t.Fatal("a document with nothing openable took the arrow key")
	}
}
```

Note: the second test asserts the *behaviour* (`Move` returns false). The panel
still satisfies `Cursor`; the application only calls it, and only proceeds when
it returns true.

Run:

```
go test -run TestExecPanel ./dash/
```

Expected: compile error (`Cursor`, `Launcher` undefined) - red.

**3b. Make it pass.** In `dash/exec.go`:

1. Extend the struct (the existing `mu`, `settings` and `input` stay):

```go
	mu       sync.Mutex
	settings map[string]string
	input    string
	// shown is the rows the last frame drew - the detail rows while zoomed -
	// and cursor is the index into their openable rows. The document itself is
	// never marked: a cursor is a rendering concern, so the document a refresh
	// replaced stays exactly as the program printed it.
	shown  []Row
	cursor int
```

2. Replace `View`:

```go
func (e *execPanel) View(ctx tideui.PanelContext) string {
	doc := e.Load()
	e.mu.Lock()
	e.shown = shownRows(doc, ctx.Zoomed)
	e.cursor = clampCursor(e.cursor, len(openable(e.shown)))
	selected := ""
	// A pane draws its cursor when it has the keyboard: entered with space, or
	// owning the screen. Anything else would show a selection the arrows cannot
	// move.
	if ctx.Entered || ctx.Zoomed {
		if row, ok := openableAt(e.shown, e.cursor); ok {
			selected = row.ID
		}
	}
	e.mu.Unlock()
	return RenderDoc(ctx.Renderer, doc, ctx.Width, ctx.Zoomed, selected)
}
```

3. Add the cursor and its helpers:

```go
// Move changes the selection by delta (-1 up, +1 down), clamped to the rows
// that carry an id. It reports whether the panel took the key: a document with
// nothing openable leaves the arrows to the workspace, which is how they still
// move focus.
func (e *execPanel) Move(delta int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := len(openable(e.shown))
	if count == 0 {
		return false
	}
	e.cursor = clampCursor(e.cursor+delta, count)
	return true
}

// openable is the rows of a document that carry an id, in the order they are
// drawn.
func openable(rows []Row) []Row {
	var out []Row
	for _, row := range rows {
		if strings.TrimSpace(row.ID) != "" {
			out = append(out, row)
		}
	}
	return out
}

// openableAt is the row at an index of openable(rows).
func openableAt(rows []Row, index int) (Row, bool) {
	items := openable(rows)
	if index < 0 || index >= len(items) {
		return Row{}, false
	}
	return items[index], true
}

// clampCursor keeps a cursor inside a list of count items. A cursor with no
// list to sit in is 0, so a panel whose document shrank cannot keep pointing at
// a row that is gone.
func clampCursor(cursor, count int) int {
	switch {
	case count <= 0:
		return 0
	case cursor < 0:
		return 0
	case cursor >= count:
		return count - 1
	default:
		return cursor
	}
}
```

`openable` returns nil for a list with nothing openable, so `openableAt` cannot
panic on it.

Run:

```
go build ./... && go vet ./... && go test ./dash/
```

Expected: `ok  github.com/allisonhere/tideui/dash`.

**3c. Commit.**

```
git add dash/exec.go dash/exec_test.go
git commit -m "Let a plugin's document be walked with a cursor"
```

---

## Task 4 - Let a manifest declare the command that opens a row

Files: `dash/manifest.go`, `dash/manifest_test.go`.

**4a. Write the failing tests.** Append to `dash/manifest_test.go`:

```go
// The command that opens a row lives in the manifest, and it must say where the
// row's id belongs: a template with no placeholder would open the same thing
// whatever the reader picked, which is worse than refusing to run it.
func TestManifestRejectsAnOpenCommandWithNoPlaceholder(t *testing.T) {
	doc := validManifest()
	doc["panel"] = map[string]any{"open": []string{"tidemail"}}
	if _, err := LoadManifest(writeManifest(t, doc)); err == nil {
		t.Fatal("a manifest whose open command names no {id} loaded")
	}
}

// An open command resolves against the plugin directory the same way an entry
// point does, so a plugin can ship "./open.sh" and be run from anywhere.
func TestOpenCommandResolvesAgainstThePluginDirectory(t *testing.T) {
	doc := validManifest()
	doc["panel"] = map[string]any{"open": []string{"./open.sh", "--open", "{id}"}}
	manifest, err := LoadManifest(writeManifest(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	argv, ok := manifest.OpenArgv()
	if !ok {
		t.Fatal("the manifest declares an open command but reports none")
	}
	if !filepath.IsAbs(argv[0]) || filepath.Base(argv[0]) != "open.sh" {
		t.Errorf("argv[0] = %q, want an absolute path to open.sh", argv[0])
	}
	if argv[2] != "{id}" {
		t.Errorf("the placeholder was eaten: %v", argv)
	}
	// A manifest that declares none is not an error, it is a panel with no
	// primary action.
	if _, ok := (Manifest{}).OpenArgv(); ok {
		t.Error("a manifest with no open command reports one")
	}
}
```

Run:

```
go test -run 'TestManifestRejectsAnOpenCommand|TestOpenCommandResolves' ./dash/
```

Expected: compile error (`OpenArgv` undefined) - red.

**4b. Make it pass.** In `dash/manifest.go`:

1. Declare the placeholder:

```go
// OpenIDPlaceholder is the token a manifest's open command puts where the id of
// the row the reader picked belongs.
const OpenIDPlaceholder = "{id}"
```

2. Add the field to `PanelManifest` (after `Copy`):

```go
	// Open is the argv that opens a row the reader selected - the program that
	// owns the data this panel previews. OpenIDPlaceholder stands for that
	// row's id, so one template serves every row, and the command runs with
	// the terminal handed to it.
	Open []string `json:"open"`
```

3. Validate it in `Validate`, after the `panel.input` block:

```go
	// An open command has to say where the id goes. Without one it would run
	// the same thing whatever the reader picked.
	if len(m.Panel.Open) > 0 {
		if strings.TrimSpace(m.Panel.Open[0]) == "" {
			problems = append(problems, "panel.open[0] is empty")
		}
		named := false
		for _, arg := range m.Panel.Open {
			if strings.Contains(arg, OpenIDPlaceholder) {
				named = true
			}
		}
		if !named {
			problems = append(problems, fmt.Sprintf("panel.open names no %s placeholder", OpenIDPlaceholder))
		}
	}
```

4. Share the relative-path resolution between both commands (DRY) - replace the
   body of `Command` with a call to a new `resolve`:

```go
// Command resolves a kind's entry point against the plugin directory, so a
// manifest can name "./render.sh".
func (m Manifest) Command(kind string) []string {
	return m.resolve(m.EntryPoints[kind])
}

// OpenArgv is the manifest's open command, resolved against the plugin
// directory. The placeholder stays in place: the panel substitutes the selected
// row's id at launch, so one command is resolved once and used for every row.
func (m Manifest) OpenArgv() ([]string, bool) {
	if len(m.Panel.Open) == 0 {
		return nil, false
	}
	return m.resolve(m.Panel.Open), true
}

// resolve makes a command's first element absolute against the plugin, not
// against wherever the dashboard was started from.
func (m Manifest) resolve(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	out := append([]string(nil), argv...)
	if strings.HasPrefix(out[0], "./") || strings.HasPrefix(out[0], "../") {
		resolved := filepath.Join(m.dir, out[0])
		if absolute, err := filepath.Abs(resolved); err == nil {
			resolved = absolute
		}
		out[0] = resolved
	}
	return out
}
```

`dash/manifest_test.go` needs `path/filepath` - it already imports it.

Run:

```
go build ./... && go vet ./... && go test ./dash/
```

Expected: `ok  github.com/allisonhere/tideui/dash`.

**4c. Commit.**

```
git add dash/manifest.go dash/manifest_test.go
git commit -m "Let a plugin declare the command that opens a row"
```

---

## Task 5 - Launch the selected row

Files: `dash/dash.go`, `dash/exec.go`, `dash/exec_test.go`.

**5a. Write the failing tests.** Append to `dash/exec_test.go`:

```go
// Launch is the selected row's primary action: the argv the manifest declared,
// with the picked row's id in place of the placeholder. The panel does not run
// it - it does not know whether there is a terminal to hand over.
func TestExecPanelLaunchesTheSelectedRow(t *testing.T) {
	panel := openableFixture(t)
	view(panel, false)

	launcher, ok := panel.(Launcher)
	if !ok {
		t.Fatal("a plugin with an open command is not a launcher")
	}
	argv, status, declared := launcher.Launch()
	if !declared {
		t.Fatal("the panel declared no open action")
	}
	if len(argv) != 3 || argv[0] != "tidemail" || argv[1] != "--open" || argv[2] != "7" {
		t.Fatalf("argv = %v, want tidemail --open 7", argv)
	}
	// The status names the row the reader picked, so the strip says what
	// happened rather than only that something did.
	if !strings.Contains(status, "ana@example.com") {
		t.Errorf("status = %q, want it to name the row", status)
	}
}

// A panel that declares no open command is not a primary action, so Enter still
// zooms there.
func TestExecPanelWithoutAnOpenCommandDeclaresNoAction(t *testing.T) {
	manifest := plugin(t, `printf '{"rows":[{"type":"block","id":"7","label":"x","body":["y"]}]}\n'`, nil)
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, false)
	if _, _, declared := panel.(Launcher).Launch(); declared {
		t.Fatal("a panel with no open command declared an action")
	}
}

// The document a zoomed panel draws is the list the cursor walks: a message
// picked while zoomed is the message opened.
func TestExecPanelCursorFollowsTheZoomedRows(t *testing.T) {
	manifest := plugin(t, `cat <<'JSON'
{"rows":[{"type":"block","id":"1","label":"row","body":["compact"]}],
 "detail":[{"type":"block","id":"11","label":"ana","body":["zoomed"]},
           {"type":"block","id":"12","label":"sam","body":["also zoomed"]}]}
JSON`, map[string]any{"panel": map[string]any{"open": []string{"tidemail", "--open", "{id}"}}})
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, true)
	panel.(Cursor).Move(1)
	argv, _, _ := panel.(Launcher).Launch()
	if len(argv) == 0 || argv[len(argv)-1] != "12" {
		t.Fatalf("argv = %v, want the second zoomed row", argv)
	}
}
```

Run:

```
go test -run TestExecPanel ./dash/
```

Expected: compile error (`Launcher` undefined) - red.

**5b. Make it pass.** In `dash/dash.go`, after the `Activator` interface:

```go
// Launcher is implemented by a panel whose current selection opens a program -
// the mail preview opening the message in TideMail. Launch returns the argv to
// run, the line for the status strip, and whether the panel has such an action
// at all. A panel that declares no open command returns ok false, so Enter still
// zooms; one with nothing selected returns a nil argv and a status saying so.
// The caller runs it, the way it puts a Copier's text on the clipboard, so a
// panel never owns the terminal.
type Launcher interface {
	Launch() (argv []string, status string, ok bool)
}
```

In `dash/exec.go`, after `Move`:

```go
// Launch is the selected row's primary action: the command the manifest
// declared under "open", which is the program that owns what the panel is
// previewing.
func (e *execPanel) Launch() ([]string, string, bool) {
	template, declared := e.manifest.OpenArgv()
	if !declared {
		return nil, "", false
	}
	e.mu.Lock()
	row, ok := openableAt(e.shown, e.cursor)
	e.mu.Unlock()
	if !ok {
		return nil, "nothing selected to open", true
	}
	return substituteOpenID(template, row.ID), "opening " + rowTitle(row), true
}

// substituteOpenID puts the picked row's id where the manifest's placeholder
// is. Every occurrence is replaced: an argument that names the id twice (a URL
// and a display name) is a plugin's business, not ours.
func substituteOpenID(argv []string, id string) []string {
	out := make([]string, len(argv))
	for i, arg := range argv {
		out[i] = strings.ReplaceAll(arg, OpenIDPlaceholder, id)
	}
	return out
}

// rowTitle names a row in a status line: the label it was given, else the first
// line of its body (a mail row's subject), else its id.
func rowTitle(row Row) string {
	if label := strings.TrimSpace(row.Label); label != "" {
		return label
	}
	for _, line := range row.Body {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return row.ID
}
```

Run:

```
go build ./... && go vet ./... && gofmt -l . && go test ./dash/
```

Expected: `gofmt -l .` prints nothing; `ok  github.com/allisonhere/tideui/dash`.

**5c. Commit.**

```
git add dash/dash.go dash/exec.go dash/exec_test.go
git commit -m "Give a plugin's selection a launch action"
```

---

## Task 6 - The workspace learns an entered pane, on `space`

Files: `workspace.go` (state, verbs, key handling, focus), `workspace_panel.go`
(`PanelContext`), `workspace_render.go` (building the context), `workspace_test.go`
and `workspace_render_test.go` (tests).

This is the library half of the key model: `space` hands the keyboard to the
focused pane - any pane - and `esc` hands it back. Nothing here knows about mail;
the mail panel is just the first pane with something to walk.

**6a. Write the failing tests.** Append to `workspace_test.go` (add the `tea`
import if it is not there):

```go
// space enters the focused pane - any pane - so its own keys work while the
// tiled layout stays on screen, and esc hands the keyboard back.
func TestSpaceEntersTheFocusedPane(t *testing.T) {
	ws := newTestWorkspace(t)
	if !ws.HandleKey(tea.KeyMsg{Type: tea.KeySpace}) {
		t.Fatal("space was not taken by the workspace")
	}
	if got := ws.EnteredPane(); got != "main" {
		t.Fatalf("entered = %q, want the focused pane", got)
	}
	if !ws.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("esc was not taken while a pane had the keyboard")
	}
	if got := ws.EnteredPane(); got != "" {
		t.Fatalf("entered = %q after esc, want none", got)
	}
}

// Moving focus leaves the pane behind: the keyboard belongs to where you are,
// and the next space enters that one. tab and the directional keys come through
// the same door, so all four spellings are checked.
func TestFocusChangeLeavesTheEnteredPane(t *testing.T) {
	for _, move := range []struct {
		name string
		run  func(*Workspace)
	}{
		{"focus", func(ws *Workspace) { ws.Focus("nav") }},
		{"tab", func(ws *Workspace) { ws.FocusNext() }},
		{"shift+tab", func(ws *Workspace) { ws.FocusPrev() }},
		{"arrows", func(ws *Workspace) { ws.FocusDirection(DirLeft) }},
	} {
		t.Run(move.name, func(t *testing.T) {
			ws := newTestWorkspace(t)
			ws.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
			move.run(ws)
			if got := ws.EnteredPane(); got != "" {
				t.Fatalf("entered = %q after %s, want none", got, move.name)
			}
		})
	}
}

// A pane that owns the screen has the keyboard too, so anything asking "who
// reads the keys" has one answer rather than two.
func TestZoomedPaneHasTheKeyboard(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Zoom("main")
	if !ws.PaneHasKeyboard("main") {
		t.Fatal("a zoomed pane does not report the keyboard")
	}
	ws.Unzoom()
	if ws.PaneHasKeyboard("main") {
		t.Fatal("an unzoomed, unentered pane reports the keyboard")
	}
	ws.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	if !ws.PaneHasKeyboard(ws.Focused()) {
		t.Fatal("an entered pane does not report the keyboard")
	}
}

// A pane that binds space itself keeps it: entering is the fallback, not the
// first claim on the key. The existing RunFocusedAction test covers the
// dispatch; this covers the order it happens in.
func TestASpaceBoundActionStillFires(t *testing.T) {
	ws := newTestWorkspace(t)
	ran := ""
	ws.Panel("main", Text("main")).Actions(Action("toggle", " ", func(*Workspace) { ran = "toggle" }))
	ws.Focus("main")
	if !ws.HandleKey(tea.KeyMsg{Type: tea.KeySpace}) {
		t.Fatal("space was not taken")
	}
	if ran != "toggle" {
		t.Fatalf("the space-bound action did not run (entered = %q)", ws.EnteredPane())
	}
	if ws.EnteredPane() != "" {
		t.Fatal("space both fired the action and entered the pane")
	}
}
```

And in `workspace_render_test.go`, the panel itself is told:

```go
// A pane that has the keyboard is told so: the panel is what draws its own
// cursor, and it cannot guess. Focused keeps its old meaning - this is the pane
// the workspace is pointed at.
func TestPanelContextReportsTheKeyboard(t *testing.T) {
	wr, ws := renderFixture(t)
	var entered, focused []bool
	ws.Panel("main", func(ctx PanelContext) string {
		entered = append(entered, ctx.Entered)
		focused = append(focused, ctx.Focused)
		return "main body"
	})
	ws.Focus("main")

	wr.Render(ws, 100, 30)
	if len(entered) == 0 || entered[len(entered)-1] {
		t.Fatalf("Entered = %v before anything was entered", entered)
	}

	ws.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	wr.Render(ws, 100, 30)
	if !entered[len(entered)-1] {
		t.Fatal("an entered pane was not told it has the keyboard")
	}
	if !focused[len(focused)-1] {
		t.Fatal("the focused pane stopped reporting itself as focused")
	}
}
```

Run:

```
go test -run 'TestSpaceEntersTheFocusedPane|TestFocusChangeLeavesTheEnteredPane|TestZoomedPaneHasTheKeyboard|TestASpaceBoundActionStillFires' .
go test -run TestPanelContextReportsTheKeyboard .
```

Expected: compile errors (`ws.EnteredPane undefined`, `ctx.Entered undefined`) -
red.

**6b. Make it pass.**

1. `workspace.go` - the interaction state, next to `peeked` and `arrange`
   (`workspace.go:32-34`):

```go
	// entered is the pane holding the keyboard, if any. It is not the focus
	// (which panel the workspace is pointed at) and not the zoom (how much room
	// a panel gets): space enters a pane, esc leaves it, and a pane that moved
	// away from is left behind.
	entered string
```

2. `workspace.go` - the verbs, beside `Zoom`/`Unzoom`/`Peek` (`workspace.go:689`):

```go
// EnterPane gives the focused pane the keyboard: its own keys work - a list's
// cursor, a form's fields - while the tiled layout stays on screen, so a panel
// can be walked without zooming it. Any pane can be entered; one with nothing of
// its own to walk simply takes the keys it has. LeavePane hands them back.
//
// Entering is not zooming. Zoom decides how much room a panel gets; entering
// decides who reads the keys.
func (ws *Workspace) EnterPane() bool {
	id := ws.focus.Current()
	if id == "" || ws.entered == id {
		return false
	}
	ws.entered = id
	return true
}

// LeavePane returns the keyboard to the workspace.
func (ws *Workspace) LeavePane() bool {
	if ws.entered == "" {
		return false
	}
	ws.entered = ""
	return true
}

// EnteredPane is the pane holding the keyboard, or "" when the workspace has it.
func (ws *Workspace) EnteredPane() string { return ws.entered }

// PaneHasKeyboard reports whether a pane reads the keys itself, because it was
// entered or because it owns the screen. One question, one answer: a panel that
// draws a cursor asks this rather than guessing from the zoom.
func (ws *Workspace) PaneHasKeyboard(id string) bool {
	return id != "" && (ws.entered == id || ws.zoomCandidate() == id)
}
```

3. `workspace.go` - focusing elsewhere drops the entered pane. All four focus
   calls go through `ws.focus.Set`, so they get one new door (`workspace.go:569-619`):

```go
// setFocus moves the focus and drops the entered pane: the keyboard belongs to
// where you are, so pointing the workspace somewhere else hands the keys back.
func (ws *Workspace) setFocus(id string) {
	if ws.entered != "" && ws.entered != id {
		ws.entered = ""
	}
	ws.focus.Set(id)
}
```

Replace the four `ws.focus.Set(...)` calls in `Focus`, `FocusNext`, `FocusPrev`
and `FocusDirection` with `ws.setFocus(...)`, passing the same argument.

4. `workspace.go` - `HandleKey`: space enters, esc peels one layer at a time.
   Add the `space` case to the `switch key` block (`workspace.go:1484`), and put
   the leave in front of the existing `esc` case:

```go
	case " ":
		// A pane that binds space itself keeps it - the action dispatch at the
		// end of this function is asked first - so a panel cannot be broken by
		// the workspace claiming its key. Otherwise space enters the focused
		// pane, and leaves it.
		if ws.RunFocusedAction(key) {
			return true
		}
		if ws.EnterPane() {
			return true
		}
		return ws.LeavePane()
	case "esc":
		// Esc peels one layer: the pane's keyboard first, then a peek, then a
		// zoom. Leaving the pane before unzooming is what lets a zoomed, entered
		// pane be walked and then restored with two presses.
		if ws.LeavePane() {
			return true
		}
		if ws.peeked != "" {
			ws.Unpeek()
			return true
		}
		if ws.zoomCandidate() != "" {
			ws.Unzoom()
			return true
		}
```

5. `workspace_panel.go` - `PanelContext` gains the field, beside `Zoomed`
   (`workspace_panel.go:79`):

```go
	// Entered says this panel has the keyboard: space entered it, or it owns
	// the screen. A panel that draws its own cursor draws it when Entered is
	// set - Focused alone means the workspace is pointed here, which is not the
	// same thing.
	Entered bool
```

6. `workspace_render.go:172` - build it:

```go
		ctx := PanelContext{
			ID:       asset,
			Title:    panel.TitleText(),
			Width:    ctxWidth,
			Height:   innerHeight,
			Focused:  focused,
			Zoomed:   ws.zoomCandidate() == asset,
			Entered:  ws.PaneHasKeyboard(asset),
			Arrange:  ws.Arranging(),
			Renderer: renderer,
		}
```

Run the whole library gate:

```
go build ./... && go vet ./... && gofmt -l . && go test -count=1 .
```

Expected: no `gofmt` output, `ok  github.com/allisonhere/tideui`. In particular
`TestPanelCursorDoesNotCaptureTiledNavigation` and the existing
`workspace_interaction_test.go` action test must still pass - if the space-bound
action test fails, `RunFocusedAction` is not being asked before `EnterPane`.

**6c. Commit.**

```
git add workspace.go workspace_panel.go workspace_render.go workspace_test.go workspace_render_test.go
git commit -m "Enter a pane with space, and leave it with esc"
```

---

## Task 7 - Space picks a message, Enter opens it, the dashboard hands over the terminal

Files: `examples/workspace/main.go`, `examples/workspace/main_test.go`.

**7a. Write the failing tests.** Append to `examples/workspace/main_test.go`:

```go
// fakeLauncher is a panel whose selection opens a program, so the dashboard's
// key path can be tested without a plugin, a subprocess or a terminal.
type fakeLauncher struct {
	rows   []string
	cursor int
	asked  []string
}

func (f *fakeLauncher) Meta() dash.Meta { return dash.Meta{ID: "fake", Title: "Fake"} }

func (f *fakeLauncher) View(tideui.PanelContext) string { return strings.Join(f.rows, "\n") }

func (f *fakeLauncher) Move(delta int) bool {
	f.cursor = clamp(f.cursor+delta, 0, len(f.rows)-1)
	return true
}

func (f *fakeLauncher) Launch() ([]string, string, bool) {
	picked := f.rows[f.cursor]
	f.asked = append(f.asked, picked)
	return []string{"/bin/echo", picked}, "opening " + picked, true
}

// Space, arrows, Enter: space gives the pane the keyboard in the tiled layout,
// the arrows pick a message, Enter opens the one under the cursor. Enter on its
// own is still the zoom, and opens nothing.
func TestSpacePicksAMessageAndEnterOpensIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	panel := &fakeLauncher{rows: []string{"one", "two", "three"}}
	m.deck.Register(panel)
	m.deck.AttachPanel(m.ws, panel)
	m.ws.Focus("fake")

	// At pane level Enter is the zoom, and it is not a way to open anything.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ws.Zoomed() != "fake" {
		t.Fatalf("zoomed = %q, want the focused panel", m.ws.Zoomed())
	}
	if len(panel.asked) != 0 {
		t.Fatalf("Enter opened %v before anything was picked", panel.asked)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ws.Zoomed() != "" {
		t.Fatalf("zoomed = %q, want the second Enter to restore the layout", m.ws.Zoomed())
	}

	// Space hands the pane the keyboard, without taking the layout away.
	m = update(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.ws.EnteredPane() != "fake" {
		t.Fatalf("entered = %q, want the focused pane", m.ws.EnteredPane())
	}
	if m.ws.Zoomed() != "" {
		t.Fatalf("space zoomed the pane (zoomed = %q)", m.ws.Zoomed())
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if panel.cursor != 1 {
		t.Fatalf("cursor = %d after down, want 1", panel.cursor)
	}

	// And inside the pane, Enter is that pane's primary action.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(panel.asked) != 1 || panel.asked[0] != "two" {
		t.Fatalf("asked = %v, want the row the cursor sat on", panel.asked)
	}
	if !strings.Contains(m.state.status, "two") {
		t.Errorf("status = %q, want it to name the row", m.state.status)
	}

	// esc hands the keys back, and the arrows are focus movement again.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.ws.EnteredPane() != "" {
		t.Fatalf("entered = %q after esc, want none", m.ws.EnteredPane())
	}
}

// plainPanel is a panel with no primary action of its own, so Enter keeps its
// job of toggling the zoom.
type plainPanel struct{}

func (plainPanel) Meta() dash.Meta                 { return dash.Meta{ID: "plain", Title: "Plain"} }
func (plainPanel) View(tideui.PanelContext) string { return "nothing to open" }

// A zoomed panel with nothing to open still leaves the zoom to Enter, so the
// dashboard is not stuck the way it would be if Enter only ever launched.
func TestEnterStillUnzoomsAPanelWithNoAction(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	m.deck.Register(plainPanel{})
	m.deck.AttachPanel(m.ws, plainPanel{})
	m.ws.Focus("plain")

	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ws.Zoomed() != "plain" {
		t.Fatalf("zoomed = %q, want the focused panel", m.ws.Zoomed())
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ws.Zoomed() != "" {
		t.Fatalf("zoomed = %q, want the second Enter to leave the zoom", m.ws.Zoomed())
	}
}
```

// Space enters any pane, not only one with something to open: a pane with
// nothing of its own to walk still takes the keyboard, and gives it back.
func TestSpaceEntersAPaneWithNothingToOpen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel()
	m.width, m.height = 150, 44
	m.deck.Register(plainPanel{})
	m.deck.AttachPanel(m.ws, plainPanel{})
	m.ws.Focus("plain")

	m = update(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.ws.EnteredPane() != "plain" {
		t.Fatalf("entered = %q, want the focused pane", m.ws.EnteredPane())
	}
	if m.ws.Zoomed() != "" {
		t.Fatalf("space zoomed the pane (zoomed = %q)", m.ws.Zoomed())
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.ws.EnteredPane() != "" {
		t.Fatalf("entered = %q after esc, want none", m.ws.EnteredPane())
	}
}

Also add, beside the existing `TestOSC52Encoding`-style unit checks:

```go
// The launched program is handed the terminal and the dashboard takes it back
// when the program exits.
func TestLaunchCmdSuspendsForTheProgram(t *testing.T) {
	if cmd := launchCmd("fake", []string{"/bin/echo", "hi"}); cmd == nil {
		t.Fatal("launchCmd returned no command")
	}
}
```

`examples/workspace/main_test.go` must import `dash` if it does not already.

Run:

```
go test -run 'TestSpacePicksAMessageAndEnterOpensIt|TestSpaceEntersAPaneWithNothingToOpen|TestEnterStillUnzooms|TestLaunchCmd' ./examples/workspace/
```

Expected: compile error (`launchCmd` undefined) - red.

**7b. Make it pass.** In `examples/workspace/main.go`:

1. Add `"os/exec"` to the imports.
2. Add the launch path, beside `activateFocused` (`main.go:923`):

```go
// launchFocused hands the terminal to the program the focused pane's selection
// opens. It reports whether the pane had such an action, so the caller can tell
// "nothing to open" from "opened nothing".
func (m *model) launchFocused() (tea.Cmd, bool) {
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return nil, false
	}
	launcher, ok := panel.(dash.Launcher)
	if !ok {
		return nil, false
	}
	argv, status, declared := launcher.Launch()
	if !declared {
		return nil, false
	}
	m.state.status = status
	if len(argv) == 0 {
		// Nothing selected, and the strip now says so rather than the panel
		// silently doing nothing.
		return nil, true
	}
	return launchCmd(m.ws.Focused(), argv), true
}
```

3. Add the command and its message, beside `pluginOpCmd` (`main.go:686`):

```go
// launchMsg reports that a program a panel's selection opened has exited.
type launchMsg struct {
	panel string
	argv  []string
	err   error
}

// launchCmd hands the terminal to a program and takes it back when the program
// exits, so the dashboard suspends rather than opening a second terminal: the
// tool the panel opens is then the one thing on screen, exactly as running it
// by hand would be, and no window is left behind.
func launchCmd(panel string, argv []string) tea.Cmd {
	command := exec.Command(argv[0], argv[1:]...)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return launchMsg{panel: panel, argv: argv, err: err}
	})
}
```

Check the signature before trusting it:

```
go doc github.com/charmbracelet/bubbletea.ExecProcess
```

Expected: `func ExecProcess(c *exec.Cmd, fn ExecCallback) Cmd` with
`type ExecCallback func(error) Msg`. If this version differs, adapt the
callback to what `go doc` prints.

4. Handle the message in `Update`'s switch, beside `case pluginOpMsg:`:

```go
	case launchMsg:
		if msg.err != nil {
			m.state.status = "could not run " + msg.argv[0] + ": " + msg.err.Error()
		} else {
			m.state.status = "back from " + msg.argv[0]
		}
		// The program just changed the data this panel previews - TideMail
		// marks the message it opened as read - so the panel is due again
		// instead of stale until its next interval.
		m.deck.RefreshNow(msg.panel)
		return m, m.refreshFocusedCmd()
```

5. Add the pane's own key routing, beside `launchFocused`:

```go
// handleEnteredKey routes a key to the pane that has the keyboard. Only what
// walking that pane needs is taken - esc to leave it, Enter for its primary
// action, the arrows and j/k for its cursor - so the application's shortcuts and
// the workspace's own keys still work while a pane is entered, and a pane that
// wants a key itself (a calculator typing an expression) keeps it, because the
// panel input runs first.
func (m *model) handleEnteredKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return nil, m.ws.LeavePane()
	case "enter":
		if cmd, handled := m.launchFocused(); handled {
			return cmd, true
		}
		// A pane whose primary action is not a program - the news list copies
		// the selected story - runs here too.
		return nil, m.activateFocused()
	case "up", "k":
		return nil, m.moveSelection(-1)
	case "down", "j":
		return nil, m.moveSelection(1)
	}
	return nil, false
}
```

Call it before the application's own keys, inside the existing
`if !m.ws.Arranging()` block and directly after the panel-input call:

```go
	if !m.ws.Arranging() {
		if cmd, handled := m.handlePanelInput(msg); handled {
			return m, cmd
		}
		// A pane that has the keyboard reads the keys first: walking a list is
		// not a moment for the workspace's own keys to move focus out from
		// under the reader.
		if m.ws.EnteredPane() != "" {
			if cmd, handled := m.handleEnteredKey(msg); handled {
				return m, cmd
			}
		}
		switch msg.String() {
```

6. Replace the `enter` case (`main.go:1032`) with the plain zoom. With the pane's
   action run inside the pane, Enter no longer has to mean two things:

```go
		case "enter":
			// Enter is the zoom, everywhere, whatever the panel is: one key,
			// one meaning, and no panel makes it mean something else. A pane's
			// own action runs inside the pane, where space has put the keyboard.
			if m.ws.Zoomed() != "" {
				m.ws.Unzoom()
			} else {
				m.ws.Zoom(m.ws.Focused())
			}
			return m, nil
```

7. Let the arrows drive the cursor while the pane has the keyboard - entered
   with space, or owning the screen. `moveSelection` is the one gate
   (`main.go:887`):

```go
// moveSelection hands a move to the focused pane's cursor only while that pane
// has the keyboard - entered with space, or owning the screen. In the tiled
// dashboard the arrows are reserved for moving between panes; otherwise a list
// panel such as News can trap the user's focus in its first few rows.
func (m *model) moveSelection(delta int) bool {
	if m.ws.EnteredPane() == "" && m.ws.Zoomed() == "" {
		return false
	}
	panel, ok := m.deck.Lookup(m.ws.Focused())
	if !ok {
		return false
	}
	cursor, ok := panel.(dash.Cursor)
	if !ok {
		return false
	}
	return cursor.Move(delta)
}
```

and the arrow cases get the same courtesy the zoomed panel had:

```go
		case "up":
			if m.moveSelection(-1) {
				return m, nil
			}
			m.ws.FocusDirection(tideui.DirUp)
			return m, nil
		case "down":
			if m.moveSelection(1) {
				return m, nil
			}
			m.ws.FocusDirection(tideui.DirDown)
			return m, nil
```

Run:

```
go build ./... && go vet ./... && go test ./examples/workspace/
```

Expected: `ok  github.com/allisonhere/tideui/examples/workspace`, including the
pre-existing `TestPanelCursorDoesNotCaptureTiledNavigation` and
`TestArrowsMoveNewsCursor`. If the first fails, `moveSelection` is being called
somewhere with no gate; if a news test fails, the news panel's Enter-to-copy is
being asked for at pane level, where Enter is now only ever the zoom.

**7c. Commit.**

```
git add examples/workspace/main.go examples/workspace/main_test.go
git commit -m "Enter a pane with space, and open what it points at with Enter"
```

---

## Task 8 - The mail plugin marks each message openable

Files: `contrib/mail/render.sh`, `contrib/mail/manifest.json`,
`examples/workspace/cascade_test.go`.

**8a. Write the failing test.** Append to `examples/workspace/cascade_test.go`
(it already has `mailFixtureHome` and the four-account fixture from step 0):

```go
// Every message row carries the id of the message it shows, so the panel can
// open the one the reader picked; the setup rows carry none, so the cursor
// skips them and the manifest's open command is what runs.
func TestMailMessageRowsCarryTheirMessageID(t *testing.T) {
	mailFixtureHome(t)
	manifest, err := dash.LoadManifest("../../contrib/mail")
	if err != nil {
		t.Skip("no mail plugin:", err)
	}
	panel := dash.Exec(manifest)
	if err := panel.(dash.Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := panel.View(tideui.PanelContext{
		ID: manifest.ID, Width: 40, Focused: true,
		Renderer: tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{}),
	})

	// The fixture's messages are ids 1..5; the rows the panel draws name them.
	ids := map[string]bool{}
	for _, row := range panel.(interface{ Load() dash.Doc }).Load().Rows {
		if row.ID != "" {
			ids[row.ID] = true
		}
	}
	for _, want := range []string{"1", "2", "4"} {
		if !ids[want] {
			t.Errorf("no row carries message %s; rows = %v\n%s", want, ids, view)
		}
	}

	// And the manifest opens the pick: the same argv the dashboard would run.
	argv, ok := manifest.OpenArgv()
	if !ok {
		t.Fatal("the mail manifest declares no open command")
	}
	if len(argv) < 2 || argv[len(argv)-1] != "{id}" {
		t.Fatalf("open command = %v, want the id placeholder last", argv)
	}

	// Moving the cursor and launching names a real message.
	cursor, ok := panel.(dash.Cursor)
	if !ok || !cursor.Move(1) {
		t.Fatal("the mail panel does not walk its messages")
	}
	launched, status, declared := panel.(dash.Launcher).Launch()
	if !declared || len(launched) == 0 || strings.HasSuffix(launched[len(launched)-1], "{id}") {
		t.Fatalf("launch = %v (%q), want a message id substituted", launched, status)
	}
}
```

If `panel.(interface{ Load() dash.Doc })` reads badly to you, `dash` exposes
`State[Doc]` through the panel, so this is the one place a test can read a
document back; keep it as written rather than widening the library's API for a
test.

Run:

```
go test -run TestMailMessageRowsCarry ./examples/workspace/
```

Expected: `FAIL` with "no row carries message 1" - the fixture rows have no ids
yet.

**8b. Make it pass.**

In `contrib/mail/render.sh`, the messages subquery gains the id:

```
  SELECT m.id AS mid, m.from_addr AS f, m.subject AS s, m.read AS r, m.starred AS st,
```

and the `json_object` gains it:

```
  'id', mid, 'from', f, 'subject', s, 'read', r, 'starred', st, 'attach', at,
```

and the row builder in the `jq` projection gains `id` (it must be a string,
because a document row's id is a string):

```
      | { type: "block",
          id: (.id | tostring),
          label: ((.from | sender)
                  + (if .attach == 1 then " @" else "" end)
                  + " · " + (.age | age)),
```

Add the line to the block's own comment, in the script's voice:

```
  # id is the message's row in TideMail's database, which is what --open wants:
  # the panel shows the message, the program opens it.
```

In `contrib/mail/manifest.json`, add to the `panel` block (after `"copy"`-less
`priority`, anywhere valid JSON):

```json
    "open": ["tidemail", "--open", "{id}"],
```

Bump the plugin's `"version"` to `1.5.0`.

Run:

```
go test -run TestMailMessageRowsCarry ./examples/workspace/
```

Expected: `ok  github.com/allisonhere/tideui/examples/workspace` (and `SKIP` if
`sqlite3`/`jq` are absent, which is not a pass - install them before believing
this one).

Then run the plugin by hand against the real mailbox to see the ids:

```
XDG_DATA_HOME=$HOME/.local/share ./contrib/mail/render.sh | jq -r '.rows[] | select(.id) | "\(.id) \(.label)"'
```

Expected: one line per previewed message, e.g. `4821 ana@example.com · 5m`.

**8c. Commit.**

```
git add contrib/mail/render.sh contrib/mail/manifest.json examples/workspace/cascade_test.go
git commit -m "Let the mail panel open the message the reader picked"
```

---

## Task 9 - Docs, and the installed copy of the plugin

Files: `README.md`, `contrib/mail/README.md`.

1. In the plugin section of `README.md` (next to the `panel.copy` paragraph,
   around line 1204), document the contract:

````
A plugin whose document is a list can also **open** what the reader picked:
attach an `id` to a row and declare the command that opens it, with `{id}` where
that row's id belongs. `space` enters the panel, the arrows (or `j`/`k`) move a
cursor over the rows that carry an id, and `Enter` inside the panel hands the
terminal to the command - so the tool the panel previews opens on the thing the
cursor was on. `esc` leaves the panel.

```json
"panel": { "open": ["tidemail", "--open", "{id}"] }
```

The cursor skips rows without an id, so a plugin's setup hint is not a
destination, and a manifest whose open command names no `{id}` is refused rather
than run on the wrong row. `Enter` at pane level is always the zoom, whatever a
panel declares: a panel's own action runs inside the panel, never instead of the
drill-down.
````

2. Update the keyboard table (`README.md:548`), so the key model is written down
   where a reader looks for keys:

```
| `space` | enter the focused pane: its own keys work, `esc` returns |
| `enter` | zoom / restore the focused panel |
| `shift+space` | zoom / restore (the long-standing shortcut, unchanged) |
```

3. Update the drill-down paragraph (`README.md:721`). It currently says a panel
   with a primary action takes `Enter` *instead of* zooming, which is no longer
   true: space enters the panel, and Enter inside it is that panel's action.
   Name the mail panel beside the news list.

4. In `contrib/mail/README.md`, say what `space` and `Enter` do there.

5. Refresh the installed plugin, which is a copy:

```
LATEST=$(cat ~/.config/tidedeck/plugins/tidedeck.mail/.tidedeck-install.json)
echo "$LATEST"          # where this copy came from; if it names a URL or a
                        # checkout other than this one, install it that way
                        # instead of copying over the top
```

Then, if it was installed from this checkout:

```
cp contrib/mail/render.sh contrib/mail/manifest.json ~/.config/tidedeck/plugins/tidedeck.mail/
```

Expected: no output. The panel picks the new script up on its next refresh
(60s); the cursor and the launch need the dashboard restarted.

**Commit.**

```
git add README.md contrib/mail/README.md
git commit -m "Document how a plugin opens what the reader picked"
```

---

## Task 10 - TideMail learns `--open` (separate repository, follow-up)

This is what makes "launch TideMail **to that email**" literal. It is not a
dependency: `/usr/bin/tidemail` v1.0.23 ignores unknown arguments, so Tasks 1-9
already launch TideMail when this is not done, and starting landing on the
message is what this adds. It ships in `~/Projects/tidemail`, installed from the
AUR package, so it needs a version bump and an AUR rebuild (sudo) - do it when
you want it, not as part of the dashboard work.

Everything needed is in that repository:

1. `main.go` `startupOptions` gains `openMessageID string`, `parseStartupOptions`
   a `case "--open"` that takes the next argument (and a `--open` with no value
   is an error, not a silent no-op - that is the one way this could be confusing
   once the flag exists).
2. After the initial folder's messages load (`internal/ui/model.go`, the
   `filteredMessages` the list is built from), resolve the id to an index, set
   `m.messageCursor` to it, call `m.setViewportForCurrentRow()`
   (`internal/ui/content.go:374`) and give the content pane focus. An id that is
   not in the loaded folder should focus the folder and select nothing rather
   than failing to start.
3. Size it as one small PR, with a test that a model built with the flag lands on
   the message and one that an unknown id still starts.

---

## Tests and validation

After every task, and again at the end:

```
cd /home/allie/Projects/tidedeck && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...
```

Expected: no `gofmt` output, and `ok` for `tideui`, `dash`, `dash/panels`,
`examples/workspace`, `form`, `provider`.

Then the CI simulation, because the mail plugin's tests are the ones that read
the machine's own mailbox unless the fixture is in place:

```
T=$(mktemp -d) && env HOME=$T XDG_DATA_HOME=$T/data go test -count=1 ./... ; rm -rf $T
```

Expected: the same `ok` lines, no `SKIP` for the mail tests.

By hand, once the dashboard is rebuilt and restarted with the Mail panel enabled:

1. `space` on Mail: the pane takes the keyboard, and the layout stays on screen.
   Nothing launches, and the panel does not fill the screen.
2. `↓`/`↑` (or `j`/`k`): one message carries the selection block at a time, and
   the block moves one message per press and stops at the ends.
3. `Enter` on a message: the dashboard's screen is replaced by TideMail; quitting
   TideMail returns to the dashboard, which says `back from tidemail` and has
   re-run the panel.
4. `Esc`: the pane gives the keys back, and `↑`/`↓` move between panes again.
5. `Enter` on any pane, before entering it: zoom, always, whatever the panel is -
   and the news list no longer copies a link on Enter at pane level, only inside
   the pane.
6. `space` on any pane, including one with nothing in it: the same entry, and
   the same `esc` back. `shift+space` still zooms.

## Risks, tradeoffs, open questions

- **The cursor walks what is drawn.** `execPanel` counts the rows of the row
  list it last rendered - the detail rows while zoomed, the compact ones
  otherwise. A plugin whose `detail` rows carry *different* ids than its `rows`
  will have a cursor that counts one list and highlights the other; the mail
  plugin uses the same ids in both, which is the sane shape. Documented in
  `README.md` rather than enforced.
- **Ids are only honoured on `text` and `block` rows.** A gauge with an id is
  simply drawn as a gauge. Cheaper than a rule per row type, and nothing wants
  it.
- **The launch runs without the plugin's settings environment.** A plugin whose
  open command needs the path it was configured with (say, a second mailbox
  database) cannot express that yet. Not needed for TideMail, whose own
  configuration decides; add it if a plugin asks.
- **`exec.Command` resolves argv[0] on `PATH` of the dashboard's process.** A
  plugin naming a program that is not installed must be told so plainly: the
  launch message already reports the error (`could not run tidemail: ...`) - do
  not turn a missing program into a silent no-op.
- **TideMail may already be running.** It is a single-instance TUI; two
  instances share the SQLite file in WAL mode, and the second one is at best
  useless. Out of scope: the dashboard cannot know, and pretending to know would
  mean probing the process table.
- **The dashboard suspends while TideMail runs**, so a plugin panel's data is
  only as fresh as `launchMsg`'s refresh makes it. That is the same trade a
  shell `exec` makes, and why the panel is re-run on the way back.
- **Open question - the status line names the row's label**, which for a mail
  row is "sender · age" (`opening ana@example.com · 5m`). If you would rather it
  named the subject, that needs a second optional field on the row (the subject
  is the block's first body line), which is exactly the kind of thing to add
  when someone asks for it - not now.
- **The news list changes its convention with this.** Its Enter-to-copy moves
  inside the pane: at pane level Enter is the zoom, everywhere, and the news
  panel's action runs when the pane has the keyboard (space). That is a visible
  change to something already shipped, and it is the price of one key keeping one
  meaning - the alternative is a second convention for the same gesture.
- **Open question - what Enter does *inside* an entered pane.** As planned it is
  that pane's primary action, which is what the original ask describes and what
  the news list already does. If instead Enter should keep zooming inside a pane
  as well, the action needs another key (say `o` for "open"), and the change is
  the `case "enter"` branch of `handleEnteredKey` plus the news panel's hint
  text - nothing structural.
