package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeManifest(t *testing.T, doc map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func validManifest() map[string]any {
	return map[string]any{
		"schemaVersion": ManifestSchemaVersion,
		"id":            "author.thing",
		"name":          "Thing",
		"version":       "1.0.0",
		"author":        "author",
		"description":   "does a thing",
		"kinds":         []string{KindPanel},
		"entryPoints":   map[string]any{KindPanel: []string{"./run.sh"}},
	}
}

func TestManifestValidates(t *testing.T) {
	empty := Manifest{}
	if problems := empty.Validate(); len(problems) == 0 {
		t.Fatal("an empty manifest should not validate")
	}

	cases := []struct {
		name   string
		mutate func(map[string]any)
		wants  string
	}{
		{"missing id", func(m map[string]any) { delete(m, "id") }, "id is required"},
		{"unnamespaced id", func(m map[string]any) { m["id"] = "thing" }, "namespaced"},
		{"missing author", func(m map[string]any) { delete(m, "author") }, "author is required"},
		{"wrong schema version", func(m map[string]any) { m["schemaVersion"] = 99 }, "schemaVersion"},
		{"no kinds", func(m map[string]any) { delete(m, "kinds") }, "kinds is required"},
		{"unsupported kind", func(m map[string]any) { m["kinds"] = []string{"bar-widget"} }, "not supported"},
		{"missing entry point", func(m map[string]any) { delete(m, "entryPoints") }, "entryPoints.panel is required"},
		{"qml entry point", func(m map[string]any) {
			m["entryPoints"] = map[string]any{KindPanel: []string{"Panel.qml"}}
		}, "QML"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := validManifest()
			c.mutate(doc)
			var manifest Manifest
			data, _ := json.Marshal(doc)
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			problems := strings.Join(manifest.Validate(), "; ")
			if !strings.Contains(problems, c.wants) {
				t.Fatalf("problems %q do not mention %q", problems, c.wants)
			}
		})
	}

	// Every problem is reported at once, so one run of the checker is enough.
	broken := validManifest()
	delete(broken, "id")
	delete(broken, "author")
	broken["kinds"] = []string{"overlay"}
	data, _ := json.Marshal(broken)
	var manifest Manifest
	_ = json.Unmarshal(data, &manifest)
	if problems := manifest.Validate(); len(problems) < 3 {
		t.Fatalf("expected several problems at once, got %v", problems)
	}
}

func TestManifestSchemaBecomesFields(t *testing.T) {
	doc := validManifest()
	doc["panel"] = map[string]any{
		"schema": []map[string]any{
			{"key": "binary", "type": "string", "label": "Binary", "defaultValue": "ai-usagebar"},
			{"key": "verbose", "type": "boolean", "label": "Verbose", "defaultValue": true},
			{"key": "mode", "type": "choice", "label": "Mode", "options": []string{"a", "b"}},
			{"key": "unlabelled", "type": "string"},
		},
	}
	manifest, err := LoadManifest(writeManifest(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	fields := manifest.Fields()
	if len(fields) != 4 {
		t.Fatalf("fields = %#v", fields)
	}
	// Keys are namespaced, so two plugins cannot collide and neither can
	// collide with a built-in panel's key.
	if fields[0].Key != "plugins.author.thing.binary" {
		t.Fatalf("key = %q", fields[0].Key)
	}
	if fields[0].Kind != FieldText || fields[0].Default != "ai-usagebar" {
		t.Fatalf("string field = %#v", fields[0])
	}
	if fields[1].Kind != FieldBool || fields[1].Default != "true" {
		t.Fatalf("boolean field = %#v", fields[1])
	}
	if fields[2].Kind != FieldChoice || len(fields[2].Options) != 2 {
		t.Fatalf("choice field = %#v", fields[2])
	}
	// A field with no label falls back to its key rather than rendering blank.
	if fields[3].Label != "unlabelled" {
		t.Fatalf("unlabelled field = %#v", fields[3])
	}
}

// A plugin cannot ask to be run many times a second: a subprocess is the
// wrong shape for that, and the floor says so rather than obliging.
func TestManifestIntervalHasAFloor(t *testing.T) {
	for _, c := range []struct {
		seconds int
		want    time.Duration
	}{
		{0, time.Minute}, {-5, time.Minute}, {1, minRefresh}, {2, minRefresh}, {300, 5 * time.Minute},
	} {
		doc := validManifest()
		doc["panel"] = map[string]any{"refreshSeconds": c.seconds}
		data, _ := json.Marshal(doc)
		var manifest Manifest
		_ = json.Unmarshal(data, &manifest)
		if got := manifest.Interval(); got != c.want {
			t.Fatalf("refreshSeconds %d gave %s, want %s", c.seconds, got, c.want)
		}
	}
}

// The format deliberately resembles the Omarchy shell plugin manifest, which
// makes it important that a shell plugin is rejected clearly rather than
// half-accepted. These are the real manifests installed on this machine.
func TestShellPluginManifestsAreRejectedClearly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	dirs, err := filepath.Glob(filepath.Join(home, ".config", "omarchy", "plugins", "*", ManifestFile))
	if err != nil || len(dirs) == 0 {
		t.Skip("no shell plugins installed to check against")
	}
	for _, path := range dirs {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue // not our concern: it is not even JSON we can read
		}
		problems := manifest.Validate()
		if len(problems) == 0 {
			t.Fatalf("%s validated as a tidedeck plugin, which it is not", path)
		}
		joined := strings.Join(problems, "; ")
		if !strings.Contains(joined, "not supported") && !strings.Contains(joined, "entryPoints") {
			t.Fatalf("%s was rejected for an unhelpful reason: %s", path, joined)
		}
		t.Logf("%s: %s", filepath.Base(filepath.Dir(path)), joined)
	}
}

func TestLoadPluginsSkipsBrokenOnes(t *testing.T) {
	root := t.TempDir()
	// One good plugin.
	good := filepath.Join(root, "good")
	os.MkdirAll(good, 0o755)
	data, _ := json.Marshal(validManifest())
	os.WriteFile(filepath.Join(good, ManifestFile), data, 0o644)
	os.WriteFile(filepath.Join(good, "run.sh"), []byte("#!/bin/sh\necho '{}'\n"), 0o755)
	// One broken, one that is not a plugin at all.
	broken := filepath.Join(root, "broken")
	os.MkdirAll(broken, 0o755)
	os.WriteFile(filepath.Join(broken, ManifestFile), []byte("{"), 0o644)
	os.MkdirAll(filepath.Join(root, "empty"), 0o755)

	panels, problems := LoadPlugins(root)
	if len(panels) != 1 {
		t.Fatalf("loaded %d panels, want the one good plugin", len(panels))
	}
	if len(problems) != 2 {
		t.Fatalf("problems = %v, want one per unusable directory", problems)
	}
	// A missing plugin directory is the normal case, not an error.
	if panels, problems := LoadPlugins(filepath.Join(root, "nope")); panels != nil || problems != nil {
		t.Fatalf("a missing directory reported %v / %v", panels, problems)
	}
}

// An input must name a declared setting and say which runes it takes, so an
// application shortcut is never swallowed by accident.
func TestManifestValidatesInput(t *testing.T) {
	build := func(mutate func(panel map[string]any)) []string {
		panel := map[string]any{
			"input":      "expr",
			"inputChars": "0-9",
			"schema":     []map[string]any{{"key": "expr", "type": "string"}},
		}
		mutate(panel)
		raw := validManifest()
		raw["panel"] = panel
		data, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		return manifest.Validate()
	}

	if problems := build(func(map[string]any) {}); len(problems) != 0 {
		t.Fatalf("a valid input manifest reported %v", problems)
	}
	if problems := build(func(p map[string]any) { delete(p, "inputChars") }); !strings.Contains(strings.Join(problems, " "), "inputChars") {
		t.Fatalf("missing inputChars = %v", problems)
	}
	if problems := build(func(p map[string]any) { p["input"] = "other" }); !strings.Contains(strings.Join(problems, " "), "not a declared") {
		t.Fatalf("input naming an undeclared key = %v", problems)
	}
}
