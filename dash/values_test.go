package dash

import (
	"os"
	"path/filepath"
	"testing"
)

// The document is the schema. A key this build does not recognise must still
// be there after a load and a save, or upgrading tidedeck would quietly throw
// away settings a newer or older build wrote.
func TestValuesPreserveUnknownKeys(t *testing.T) {
	const source = `{
  "live": true,
  "zones": "Europe/London,Asia/Tokyo",
  "weather": {
    "latitude": 41.88,
    "enabled": true
  },
  "some_future_panel": {
    "endpoint": "https://example.com"
  }
}`
	values, err := LoadValues([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	data, err := values.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	round, err := LoadValues(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := round.String("some_future_panel.endpoint"); got != "https://example.com" {
		t.Fatalf("unknown key lost: %q", got)
	}
	if !round.Bool("live") || round.Float("weather.latitude") != 41.88 {
		t.Fatalf("known keys changed: live=%v lat=%v", round.Bool("live"), round.Float("weather.latitude"))
	}
}

func TestValuesReadTypesAndPaths(t *testing.T) {
	values, err := LoadValues([]byte(`{
  "clock_24": true,
  "aur_helper": "yay",
  "zones": " Europe/London , Asia/Tokyo ,, Sydney ",
  "weather": {"latitude": 41.88, "location": "Chicago"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := values.String("aur_helper"); got != "yay" {
		t.Fatalf("String = %q", got)
	}
	if got := values.String("weather.location"); got != "Chicago" {
		t.Fatalf("nested String = %q", got)
	}
	if !values.Bool("clock_24") {
		t.Fatal("Bool failed")
	}
	if got := values.Float("weather.latitude"); got != 41.88 {
		t.Fatalf("Float = %v", got)
	}
	// A number read as text comes back without JSON's float formatting, so a
	// settings field shows "41.88" rather than "4.188e+01".
	if got := values.String("weather.latitude"); got != "41.88" {
		t.Fatalf("Float as String = %q", got)
	}
	list := values.List("zones")
	if len(list) != 3 || list[0] != "Europe/London" || list[2] != "Sydney" {
		t.Fatalf("List = %#v", list)
	}
	// Absent keys and wrong-typed keys read as zero rather than panicking.
	if values.String("nope") != "" || values.Bool("nope") || values.Float("nope") != 0 {
		t.Fatal("absent key did not read as zero")
	}
	if values.String("weather.nope.deeper") != "" {
		t.Fatal("walking past a leaf did not read as zero")
	}
	if values.Float("aur_helper") != 0 {
		t.Fatal("a non-numeric string should read as 0")
	}
}

func TestValuesSetCreatesNesting(t *testing.T) {
	values := NewValues()
	values.Set("aur_helper", "paru")
	values.Set("weather.latitude", 41.88)
	values.Set("weather.location", "Chicago")
	if values.String("aur_helper") != "paru" || values.Float("weather.latitude") != 41.88 {
		t.Fatalf("set values did not read back: %#v", values)
	}
	if values.String("weather.location") != "Chicago" {
		t.Fatal("second key under the same parent overwrote the first")
	}
	// A clone is independent, so a settings form can be edited and discarded.
	clone := values.Clone()
	clone.Set("aur_helper", "yay")
	if values.String("aur_helper") != "paru" {
		t.Fatal("editing a clone changed the original")
	}
}

func TestValuesKeysAreDotted(t *testing.T) {
	values, err := LoadValues([]byte(`{"a": 1, "b": {"c": 2, "d": {"e": 3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, key := range values.Keys() {
		found[key] = true
	}
	for _, want := range []string{"a", "b.c", "b.d.e"} {
		if !found[want] {
			t.Fatalf("Keys() = %v, missing %q", values.Keys(), want)
		}
	}
	if len(found) != 3 {
		t.Fatalf("Keys() = %v, want three leaves", values.Keys())
	}
}

// The real config on this machine must survive a round trip unchanged. This
// is the guard that makes the migration safe: if it fails, someone's settings
// are about to be rewritten.
func TestValuesRoundTripRealConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".config", "tidedeck", "config.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no config.json to check against")
	}
	values, err := LoadValues(original)
	if err != nil {
		t.Fatalf("the real config does not load: %v", err)
	}
	encoded, err := values.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Compare decoded forms rather than bytes: key order in a Go map is not
	// stable, and the file's own order is not meaningful.
	before, after := map[string]any{}, map[string]any{}
	mustDecode(t, original, &before)
	mustDecode(t, encoded, &after)
	if len(before) != len(after) {
		t.Fatalf("round trip changed the key count: %d -> %d", len(before), len(after))
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			t.Fatalf("round trip dropped %q", key)
		}
	}
}

func mustDecode(t *testing.T, data []byte, into *map[string]any) {
	t.Helper()
	values, err := LoadValues(data)
	if err != nil {
		t.Fatal(err)
	}
	*into = values.raw
}
