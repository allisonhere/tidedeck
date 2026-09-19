package dash

import (
	"context"
	"encoding/json"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// plugin writes a plugin directory whose entry point is the given shell
// script, and returns the loaded manifest.
func plugin(t *testing.T, script string, manifest map[string]any) Manifest {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixtures are POSIX shell")
	}
	dir := t.TempDir()
	entry := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(entry, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"schemaVersion": ManifestSchemaVersion,
		"id":            "test.plugin",
		"name":          "Test",
		"version":       "1.0.0",
		"author":        "test",
		"description":   "a test plugin",
		"kinds":         []string{KindPanel},
		"entryPoints":   map[string]any{KindPanel: []string{"./run.sh"}},
	}
	for key, value := range manifest {
		base[key] = value
	}
	data, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("fixture manifest did not load: %v", err)
	}
	return loaded
}

func TestExecPanelRendersWhatTheProgramPrints(t *testing.T) {
	manifest := plugin(t, `cat <<'JSON'
{"rows":[{"type":"metric","label":"Session","value":"43%","percent":43,"severity":"low"},
         {"type":"text","label":"Balance","value":"$7.02"}],
 "badge":{"text":"3","tone":"warning"}}
JSON`, nil)

	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(panel.View(tideui.PanelContext{ID: "test.plugin", Width: 36, Renderer: docRenderer()}))
	for _, want := range []string{"Session", "43%", "Balance", "$7.02"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plugin output missing %q:\n%s", want, out)
		}
	}
	text, tone := panel.(Badger).Badge()
	if text != "3" || tone != tideui.ToneWarning {
		t.Fatalf("badge = %q/%v", text, tone)
	}
}

// openableFixture is a plugin whose document holds two openable blocks and the
// non-rows a real one prints between them: a tally row above, spacers below.
func openableFixture(t *testing.T) Panel {
	t.Helper()
	manifest := plugin(t, `cat <<'JSON'
{"rows":[
  {"type":"text","label":"mail","value":"3 unread","tone":"muted"},
  {"type":"block","id":"7","label":"ana@example.com · 5m","body":["standup notes"]},
  {"type":"spacer"},
  {"type":"block","id":"9","label":"sam@example.com · 1h","body":["dinner?"]},
  {"type":"spacer"}]}
JSON`, map[string]any{"panel": map[string]any{
		"open": []string{"tidemail", "--open", "{id}"},
		"edit": []string{"favourites", "edit", "{id}"},
	}})
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return panel
}

// view draws the panel once, which is what tells it which rows are on screen.
// Entered is what has the keyboard: a pane entered with space, or one that owns
// the screen. The colour profile is set before the renderer is built, because
// without one there are no escapes for a test to assert on.
func view(panel Panel, zoomed bool) string {
	lipgloss.SetColorProfile(termenv.TrueColor)
	return panel.View(tideui.PanelContext{
		ID: "test.plugin", Width: 30, Zoomed: zoomed, Entered: true, Renderer: docRenderer(),
	})
}

// cursorMark is the escape the panel draws the row the reader is on with, read
// out of lipgloss rather than spelled out here, so the assertion is about the
// colours the panel chose and not about this test's idea of them. TrueColor is
// set for the same reason: without a colour profile there are no escapes to
// assert on and every one of these tests would pass vacuously.
func cursorMark() string {
	lipgloss.SetColorProfile(termenv.TrueColor)
	ws := docRenderer().Styles.Workspace
	style := lipgloss.NewStyle().Background(ws.SelectionBg).Foreground(ws.SelectionFg)
	open := strings.SplitN(style.Render("x"), "x", 2)[0]
	if !strings.HasPrefix(open, "\x1b[") {
		panic("could not read the selection escape out of lipgloss")
	}
	return open
}

// The cursor walks the rows that carry an id, in order, clamped at both ends.
func TestExecPanelCursorWalksTheOpenableRows(t *testing.T) {
	panel := openableFixture(t)
	cursor, ok := panel.(Cursor)
	if !ok {
		t.Fatal("a plugin with openable rows is not a cursor")
	}

	out := view(panel, false)
	// The first openable row is where a fresh panel starts, and the text row
	// above it - which carries no id - is not a destination.
	if !strings.Contains(out, cursorMark()+"ana@example.com · 5m") {
		t.Fatalf("the first openable row is not marked:\n%q", out)
	}
	if strings.Contains(out, cursorMark()+"3 unread") {
		t.Fatalf("the cursor started on a row with no id:\n%q", out)
	}

	if !cursor.Move(1) {
		t.Fatal("the cursor refused to move over two openable rows")
	}
	out = view(panel, false)
	if !strings.Contains(out, cursorMark()+"sam@example.com · 1h") {
		t.Fatalf("the cursor did not move to the second row:\n%q", out)
	}
	if strings.Contains(out, cursorMark()+"ana@example.com · 5m") {
		t.Fatalf("the first row kept the cursor:\n%q", out)
	}

	// Clamped at both ends rather than wrapping or running off the list.
	cursor.Move(1)
	if out := view(panel, false); !strings.Contains(out, cursorMark()+"sam@example.com · 1h") {
		t.Fatalf("moving past the last row lost the selection:\n%q", out)
	}
	cursor.Move(-5)
	if out := view(panel, false); !strings.Contains(out, cursorMark()+"ana@example.com · 5m") {
		t.Fatalf("moving above the first row lost the selection:\n%q", out)
	}
}

// A document that draws its rows a second way when it fills the screen is still
// the same list: the cursor counts the rows on screen, so zooming does not move
// the selection onto a different message.
func TestExecPanelCursorFollowsTheZoomedRows(t *testing.T) {
	panel := openableFixture(t)
	view(panel, true)
	if !panel.(Cursor).Move(1) {
		t.Fatal("the cursor refused to move on the zoomed rows")
	}
	if out := view(panel, true); !strings.Contains(out, cursorMark()+"sam@example.com · 1h") {
		t.Fatalf("the zoomed detail did not mark the second row:\n%q", out)
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

// Launch is the selected row's primary action: the argv the manifest declared,
// with the picked row's id in place of the placeholder. The panel does not run
// it - it has no idea whether there is a terminal to hand over.
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
	// The status names the row, so the strip says what happened rather than only
	// that something did.
	if !strings.Contains(status, "ana@example.com") {
		t.Errorf("status = %q, want it to name the row", status)
	}

	// And the id follows the cursor: the second row's id is what comes out.
	panel.(Cursor).Move(1)
	argv, _, _ = launcher.Launch()
	if len(argv) != 3 || argv[2] != "9" {
		t.Fatalf("argv = %v, want the second row's id", argv)
	}
}

// A panel that declares no open command is not a primary action at all, so the
// key it would have used stays the workspace's.
func TestExecPanelWithoutAnOpenCommandDeclaresNoAction(t *testing.T) {
	manifest := plugin(t, `printf '{"rows":[{"type":"block","id":"7","label":"x","body":["y"]}]}\n'`, nil)
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, false)
	launcher, ok := panel.(Launcher)
	if !ok {
		t.Fatal("a panel is not a launcher, not even to decline")
	}
	if argv, _, declared := launcher.Launch(); declared || len(argv) != 0 {
		t.Fatalf("Launch = %v/%v, want no action declared", argv, declared)
	}
}

// A panel that declares an open command and has nothing openable says so rather
// than running the command with no id in it.
func TestExecPanelWithNothingSelectedSaysSo(t *testing.T) {
	manifest := plugin(t, `printf '{"rows":[{"type":"text","label":"mail","value":"empty"}]}\n'`,
		map[string]any{"panel": map[string]any{"open": []string{"tidemail", "--open", OpenIDPlaceholder}}})
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, false)
	argv, status, declared := panel.(Launcher).Launch()
	if !declared {
		t.Fatal("the panel declared no action despite an open command")
	}
	if len(argv) != 0 {
		t.Fatalf("argv = %v, want nothing to run", argv)
	}
	if !strings.Contains(status, "select") {
		t.Errorf("status = %q, want it to say what to do instead", status)
	}
}

// Everything that can go wrong with someone else's program must leave the
// last good content on screen and report why, never blank the panel.
func TestExecPanelKeepsLastGoodDocument(t *testing.T) {
	cases := []struct {
		name, script, wants string
	}{
		{"non-zero exit", "echo 'it broke' >&2; exit 1", "it broke"},
		{"invalid json", "printf '{'", "unexpected EOF"},
		{"not json at all", "echo hello", "invalid character"},
		{"too much output", "head -c 2000000 /dev/zero | tr '\\0' 'x'", "bytes"},
		{"wrong schema", `echo '{"schemaVersion":99,"rows":[]}'`, "schemaVersion"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			manifest := plugin(t, c.script, nil)
			panel := Exec(manifest).(*execPanel)
			// Seed a good document, as a successful earlier refresh would.
			good := Doc{Rows: []Row{{Type: "text", Label: "Last", Value: "good"}}}
			panel.Store(good)

			err := panel.Refresh(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("error %q does not mention %q", err, c.wants)
			}
			if len(panel.Load().Rows) != 1 || panel.Load().Rows[0].Value != "good" {
				t.Fatalf("the last good document was lost: %#v", panel.Load())
			}
		})
	}
}

// A program that never exits must be killed, not waited on forever.
func TestExecPanelTimesOut(t *testing.T) {
	manifest := plugin(t, "sleep 30", nil)
	panel := Exec(manifest).(*execPanel)
	panel.timeout = 200 * time.Millisecond

	start := time.Now()
	err := panel.Refresh(context.Background())
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want a timeout", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("waited %s for a program that never exits", elapsed)
	}
}

// Settings reach the program as environment variables, and only the ones it
// declared.
func TestExecPanelPassesDeclaredSettings(t *testing.T) {
	manifest := plugin(t,
		`printf '{"rows":[{"type":"text","label":"provider","value":"%s"}]}' "$TIDEDECK_PLUGIN_PROVIDER"`,
		map[string]any{
			"panel": map[string]any{
				"schema": []map[string]any{
					{"key": "provider", "type": "string", "label": "Provider", "defaultValue": "anthropic"},
				},
			},
		})

	panel := Exec(manifest)
	// With nothing configured, the declared default is used.
	if err := panel.(Configurable).Configure(NewValues()); err != nil {
		t.Fatal(err)
	}
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(panel.View(tideui.PanelContext{Width: 40, Renderer: docRenderer()}))
	if !strings.Contains(out, "anthropic") {
		t.Fatalf("declared default did not reach the program:\n%s", out)
	}

	// A configured value overrides it, under the namespaced key.
	values := NewValues()
	values.Set("plugins.test.plugin.provider", "openai")
	if err := panel.(Configurable).Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out := ansi.Strip(panel.View(tideui.PanelContext{Width: 40, Renderer: docRenderer()})); !strings.Contains(out, "openai") {
		t.Fatalf("configured value did not reach the program:\n%s", out)
	}

	// The schema the settings screen sees uses the namespaced key.
	fields := panel.(Configurable).Schema()
	if len(fields) != 1 || fields[0].Key != "plugins.test.plugin.provider" {
		t.Fatalf("schema = %#v", fields)
	}
	if fields[0].Default != "anthropic" {
		t.Fatalf("default = %q", fields[0].Default)
	}
}

// A plugin panel must be indistinguishable from a built-in one to the deck.
func TestExecPanelWorksThroughTheDeck(t *testing.T) {
	manifest := plugin(t, `echo '{"rows":[{"type":"text","label":"Hi","value":"there"}]}'`, nil)
	deck := New()
	deck.Register(Exec(manifest))
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("test.plugin")
	if !ok {
		t.Fatal("plugin was not attached")
	}
	deck.SetMode(ModeLive)
	deck.Refresh(context.Background(), time.Now())
	if err := deck.Err("test.plugin"); err != nil {
		t.Fatalf("refresh error: %v", err)
	}
	out := ansi.Strip(registered.Render(tideui.PanelContext{ID: "test.plugin", Width: 30, Renderer: docRenderer()}))
	if !strings.Contains(out, "there") {
		t.Fatalf("deck did not drive the plugin:\n%s", out)
	}
}

// A plugin with a declared input takes typed runes within inputChars, and what
// was typed reaches the program as that setting on the next run.
func TestExecPanelTakesInput(t *testing.T) {
	manifest := plugin(t,
		`printf '{"rows":[{"type":"text","label":"query","value":"%s"}]}' "$TIDEDECK_PLUGIN_QUERY"`,
		map[string]any{
			"panel": map[string]any{
				"input":      "query",
				"inputChars": "abc",
				"schema":     []map[string]any{{"key": "query", "type": "string", "label": "Query"}},
			},
		})
	panel := Exec(manifest).(*execPanel)
	if err := panel.Configure(NewValues()); err != nil {
		t.Fatal(err)
	}

	for _, r := range "cab" {
		if !panel.Type(r) {
			t.Fatalf("declined %q, which inputChars allows", r)
		}
	}
	if panel.Type('z') {
		t.Fatal("accepted a rune outside inputChars")
	}
	if !panel.Backspace() {
		t.Fatal("backspace was declined")
	}

	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(panel.View(tideui.PanelContext{Width: 40, Renderer: docRenderer()}))
	if !strings.Contains(out, "ca") {
		t.Fatalf("typed input did not reach the program:\n%s", out)
	}
}

// A plugin with no input declines every rune, so shortcuts still work.
func TestExecPanelWithoutInputDeclinesTyping(t *testing.T) {
	manifest := plugin(t, `echo '{"rows":[]}'`, nil)
	panel := Exec(manifest).(*execPanel)
	if panel.Type('a') || panel.Backspace() {
		t.Fatal("a panel without input should decline typing")
	}
}

// A manifest can name a row to copy, so a plugin offers a copyable value with
// no code of its own.
func TestExecPanelCopyNamesARow(t *testing.T) {
	manifest := plugin(t,
		`echo '{"rows":[{"type":"text","label":"Balance","value":"$7.02"},{"type":"text","label":"Plan","value":"Pro"}]}'`,
		map[string]any{"panel": map[string]any{"copy": "Balance"}})
	panel := Exec(manifest).(*execPanel)
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if text, ok := panel.Copy(); !ok || text != "$7.02" {
		t.Fatalf("copy = %q,%v want $7.02", text, ok)
	}

	// A panel with no copy, or a missing row, copies nothing.
	plain := Exec(plugin(t, `echo '{"rows":[]}'`, nil))
	if _, ok := plain.(Copier).Copy(); ok {
		t.Fatal("a panel with no copy field should copy nothing")
	}
}

func TestExecPanelOptionalGlyph(t *testing.T) {
	manifest := plugin(t, `echo '{"rows":[]}'`, map[string]any{
		"panel": map[string]any{"glyph": "🧩"},
	})
	if got := Exec(manifest).Meta().Glyph; got != "🧩" {
		t.Fatalf("plugin glyph = %q, want 🧩", got)
	}
	manifest.Panel.Glyph = "too\nwide"
	if got := Exec(manifest).Meta().Glyph; got != "" {
		t.Fatalf("invalid plugin glyph = %q, want fallback", got)
	}
}

// The second action is the form: the same row id, run by the program that owns
// the data. A pane that previews what it cannot change sends the reader out of
// the app to change it.
func TestExecPanelEditsTheSelectedRow(t *testing.T) {
	panel := openableFixture(t)
	view(panel, false)

	editor, ok := panel.(Editor)
	if !ok {
		t.Fatal("a plugin with an edit command is not an editor")
	}
	argv, status, declared := editor.Edit()
	if !declared {
		t.Fatal("the panel declared no edit action")
	}
	if len(argv) != 3 || argv[0] != "favourites" || argv[1] != "edit" || argv[2] != "7" {
		t.Fatalf("argv = %v, want favourites edit 7", argv)
	}
	if !strings.Contains(status, "ana@example.com") {
		t.Errorf("status = %q, want it to name the row", status)
	}
	// The id follows the cursor, exactly as the open command's does.
	panel.(Cursor).Move(1)
	if argv, _, _ = editor.Edit(); len(argv) != 3 || argv[2] != "9" {
		t.Fatalf("argv = %v, want the second row's id", argv)
	}
}

// A panel that declares only an open command has no second action, so the edit
// key stays the application's - the same rule the open key follows.
func TestExecPanelWithoutAnEditCommandDeclaresNoAction(t *testing.T) {
	manifest := plugin(t, `printf '{"rows":[{"type":"block","id":"7","label":"x","body":["y"]}]}\n'`,
		map[string]any{"panel": map[string]any{"open": []string{"tidemail", "--open", "{id}"}}})
	panel := Exec(manifest)
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	view(panel, false)
	editor, ok := panel.(Editor)
	if !ok {
		t.Fatal("a panel is not an editor, not even to decline")
	}
	if argv, _, declared := editor.Edit(); declared || len(argv) != 0 {
		t.Fatalf("Edit = %v/%v, want no action declared", argv, declared)
	}
}

// A declared edit command is checked the way an open command is: without the id
// placeholder it would open the same form whatever row the reader picked, which
// looks like it worked and edits the wrong thing. Written to a directory by hand,
// because the fixture helper refuses a manifest that does not validate.
func TestManifestChecksADeclaredEditCommand(t *testing.T) {
	dir := t.TempDir()
	manifest := map[string]any{
		"schemaVersion": ManifestSchemaVersion, "id": "test.plugin", "name": "Test",
		"version": "1.0.0", "author": "test", "description": "a test plugin",
		"kinds":       []string{KindPanel},
		"entryPoints": map[string]any{KindPanel: []string{"true"}},
		"panel":       map[string]any{"edit": []string{"favourites", "edit"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var problems []string
	loaded, err := LoadManifest(dir)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = loaded.Validate()
	}
	found := false
	for _, problem := range problems {
		if strings.Contains(problem, "panel.edit") && strings.Contains(problem, "{id}") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an edit command naming no id was accepted: %v", problems)
	}
}

func TestExecPanelDrawsAnImageRowFromAPlugin(t *testing.T) {
	noPlaceholdersForTests(t)
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	frame := filepath.Join(t.TempDir(), "frame.png")
	writePNGAt(t, frame, 2, 2, color.RGBA{R: 255, A: 255})

	manifest := plugin(t,
		`printf '{"rows":[{"type":"image","src":"%s","rows":2}]}\n' "$TIDEDECK_PLUGIN_FRAME"`,
		map[string]any{
			"panel": map[string]any{
				"schema": []map[string]any{
					{"key": "frame", "type": "string", "label": "Frame"},
				},
			},
		})

	panel := Exec(manifest)
	values := NewValues()
	values.Set(manifest.SettingKey("frame"), frame)
	if err := panel.(Configurable).Configure(values); err != nil {
		t.Fatal(err)
	}
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx := tideui.PanelContext{ID: "test.plugin", Width: 8, Renderer: docRenderer()}
	out := panel.View(ctx)
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Background(lipgloss.Color("#ff0000")).Render("▀")
	if !strings.Contains(out, red) {
		t.Fatalf("the plugin's picture was not drawn:\n%q", out)
	}

	// And the next frame arrives without anyone being told: the loader keys on
	// the file's own state.
	writePNGAt(t, frame, 2, 2, color.RGBA{B: 255, A: 255})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(frame, future, future); err != nil {
		t.Fatal(err)
	}
	if err := panel.(Fetcher).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	out = panel.View(ctx)
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#0000ff")).Background(lipgloss.Color("#0000ff")).Render("▀")
	if !strings.Contains(out, blue) {
		t.Fatalf("a rewritten picture was not picked up:\n%q", out)
	}
}

// A pane's own colour glyph arrives from a plugin manifest, and the host has to keep it:
// an emoji like the mail pane's envelope is two cells wide and several runes long, which
// is exactly what a naive "one rune, one cell" rule would throw away. Rubbish still gets
// dropped, because a multi-line or three-cell glyph damages the pane's border.
func TestNormalizeGlyphKeepsAColourGlyph(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"colour emoji", "📧", "📧"},
		{"emoji with a variation selector", "🌤️", "🌤️"},
		{"a plain symbol", "✉", "✉"},
		{"padded", " 📧 ", "📧"},
		{"three cells", "🇺🇸🇺🇸", ""},
		{"a newline inside it", "📧\nM", ""},
		{"trailing whitespace", "📧\n", "📧"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeGlyph(c.value); got != c.want {
				t.Fatalf("normalizeGlyph(%q) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}
