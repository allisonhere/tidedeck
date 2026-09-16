package dash

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/x/ansi"
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
