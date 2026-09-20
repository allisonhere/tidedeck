package dash

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The catalogue at the repository root is what a site reads, so it is held to
// the manifests it claims to describe: a plugin added, removed or renamed
// without regenerating it makes the file lie, and this is what says so.
func TestCatalogMatchesTheBundledManifests(t *testing.T) {
	built, err := BuildCatalog(CatalogRepository, filepath.Join("..", "contrib"))
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	want, err := built.Encode()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "plugins", "index.json")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v (run: go run ./cmd/plugincatalog)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of date; run: go run ./cmd/plugincatalog", path)
	}
}

// Each entry has to point at a plugin that is really there, and name the source
// a reader would paste into the Plugins page.
func TestCatalogEntriesPointAtTheirPlugins(t *testing.T) {
	catalog, err := BuildCatalog(CatalogRepository, filepath.Join("..", "contrib"))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Plugins) == 0 {
		t.Fatal("the catalogue is empty")
	}
	if catalog.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", catalog.SchemaVersion, CatalogSchemaVersion)
	}

	seen := map[string]bool{}
	for _, plugin := range catalog.Plugins {
		if plugin.ID == "" {
			t.Errorf("an entry has no id: %+v", plugin)
		}
		if seen[plugin.ID] {
			t.Errorf("duplicate id %q", plugin.ID)
		}
		seen[plugin.ID] = true

		if _, err := os.Stat(filepath.Join("..", filepath.FromSlash(plugin.Source))); err != nil {
			t.Errorf("%s: source %q does not exist: %v", plugin.ID, plugin.Source, err)
		}
		if want := CatalogRepository + "#" + plugin.Source; plugin.Install != want {
			t.Errorf("%s: install = %q, want %q", plugin.ID, plugin.Install, want)
		}
		if plugin.Glyph == "" {
			t.Errorf("%s: no glyph, so a site could not show its pane icon", plugin.ID)
		}
	}
}
