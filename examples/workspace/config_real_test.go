package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The real config on this machine, run through load and save inside the
// test's own config dir. The fixtures above cover the semantics on any
// machine; this one covers a document with every key a real install has.
// test's own config dir, must come back with every key intact.
func TestRealConfigSurvivesASave(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	real, err := os.ReadFile(filepath.Join(home, ".config", "tidedeck", "config.json"))
	if err != nil {
		t.Skip("no real config")
	}
	// Add a key this build has no field for.
	var doc map[string]any
	if err := json.Unmarshal(real, &doc); err != nil {
		t.Fatal(err)
	}
	doc["future_panel"] = map[string]any{"endpoint": "https://example.com"}
	seeded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, string(seeded))

	cfg := loadConfig()
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	after := readConfigDoc(t)

	var before, got []string
	for key := range doc {
		before = append(before, key)
	}
	for key := range after {
		got = append(got, key)
	}
	sort.Strings(before)
	sort.Strings(got)
	t.Logf("before %d keys, after %d keys", len(before), len(got))
	if len(before) != len(got) {
		t.Fatalf("key count changed:\nbefore %v\nafter  %v", before, got)
	}
	for i := range before {
		if before[i] != got[i] {
			t.Fatalf("keys differ: %v vs %v", before, got)
		}
	}
	nested, ok := after["future_panel"].(map[string]any)
	if !ok || nested["endpoint"] != "https://example.com" {
		t.Fatalf("unknown key lost: %v", after["future_panel"])
	}
	// Every value the struct owns must be unchanged by a no-op save.
	for _, key := range configKeys() {
		if key == "panel_gauges" || key == "panel_sparks" {
			continue // omitempty: absent when empty, checked separately
		}
		b, _ := json.Marshal(doc[key])
		a, _ := json.Marshal(after[key])
		if string(b) != string(a) {
			t.Fatalf("key %q changed on a no-op save: %s -> %s", key, b, a)
		}
	}
}
