package dash

import (
	"strings"
	"testing"
)

// A manifest has always been able to describe a setting. Until the description
// was carried into Field it was parsed and dropped, so a plugin could explain
// itself and never be heard.
func TestManifestCarriesDescriptions(t *testing.T) {
	manifest := Manifest{
		ID: "author.name",
		Panel: PanelManifest{Schema: []SchemaField{
			{Key: "account", Type: "string", Label: "account",
				Description: "Blank shows every account."},
			{Key: "unread", Type: "boolean", Label: "unread only"},
		}},
	}
	fields := manifest.Fields()
	if len(fields) != 2 {
		t.Fatalf("got %d fields", len(fields))
	}
	if fields[0].Description != "Blank shows every account." {
		t.Fatalf("description = %q, want it carried through", fields[0].Description)
	}
	if fields[1].Description != "" {
		t.Fatalf("description = %q, want empty", fields[1].Description)
	}
}

// A plugin can declare a number, which reaches the screen as a numeric field
// rather than as free text.
func TestManifestNumberKind(t *testing.T) {
	manifest := Manifest{
		ID:    "author.name",
		Panel: PanelManifest{Schema: []SchemaField{{Key: "count", Type: "number"}}},
	}
	if kind := manifest.Fields()[0].Kind; kind != FieldFloat {
		t.Fatalf("kind = %v, want FieldFloat", kind)
	}
	problems := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ID:            "author.name",
		Kinds:         []string{KindPanel},
		EntryPoints:   map[string][]string{KindPanel: {"./render.sh"}},
		Panel:         PanelManifest{Schema: []SchemaField{{Key: "count", Type: "number"}}},
	}.Validate()
	for _, problem := range problems {
		if strings.Contains(problem, "number") {
			t.Fatalf("number rejected by Validate: %s", problem)
		}
	}
}

func TestFieldBounded(t *testing.T) {
	if (Field{}).Bounded() {
		t.Fatal("an unset range reported as bounded")
	}
	if !(Field{Min: -90, Max: 90}).Bounded() {
		t.Fatal("a real range reported as unbounded")
	}
}

// A manifest is static JSON written before the machine it runs on existed, so
// it cannot name the accounts, interfaces or units that are actually present.
// The program can, because it just looked, and what it reports reaches the
// setting as its list of choices.
func TestSchemaFoldsInDiscoveredOptions(t *testing.T) {
	manifest := Manifest{
		ID: "author.name",
		Panel: PanelManifest{Schema: []SchemaField{
			{Key: "account", Type: "string", Label: "account"},
			{Key: "mode", Type: "choice", Label: "mode", Options: []string{"a", "b"}},
			{Key: "path", Type: "string", Label: "path"},
		}},
	}
	panel := Exec(manifest).(*execPanel)
	panel.Store(Doc{Options: map[string][]string{
		"account": {"", "Gmail", "work"},
		"mode":    {"smuggled"},
	}})

	fields := panel.Schema()
	if got := fields[0].Options; len(got) != 3 || got[1] != "Gmail" {
		t.Errorf("account options = %v, want the discovered list", got)
	}
	// A declared choice keeps its declared options: the manifest is the
	// contract, and a program must not be able to widen it.
	if got := fields[1].Options; len(got) != 2 || got[0] != "a" {
		t.Errorf("mode options = %v, want the manifest's own", got)
	}
	if got := fields[2].Options; len(got) != 0 {
		t.Errorf("path options = %v, want none", got)
	}
}

// Options are keyed by the name the plugin declared, not by the namespaced
// configuration key — a plugin should not have to know where its settings are
// filed to describe them.
func TestSettingNameStripsTheNamespace(t *testing.T) {
	m := Manifest{ID: "author.name"}
	if got := m.settingName(m.SettingKey("account")); got != "account" {
		t.Errorf("settingName = %q, want account", got)
	}
}
