package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// CatalogSchemaVersion is the version of the machine-readable plugin catalogue
// a site reads. It describes this file only, like DocSchemaVersion describes a
// document.
const CatalogSchemaVersion = 1

// CatalogRepository is where the bundled plugins live. A site composes an
// install source from it and an entry's Source, the same repository#subdir form
// the Plugins page accepts.
const CatalogRepository = "https://github.com/allisonhere/tidedeck"

// CatalogGeneratedBy is the command that writes the catalogue, recorded in the
// file so the site and the test that watches it agree on where it came from.
const CatalogGeneratedBy = "go run ./cmd/plugincatalog"

// Catalog is the discoverable list of the plugins bundled with the app: one
// entry per directory beside the bundled plugins, built from each plugin's own
// manifest so the catalogue cannot claim anything the plugin does not declare.
type Catalog struct {
	SchemaVersion int            `json:"schemaVersion"`
	Repository    string         `json:"repository"`
	GeneratedBy   string         `json:"generatedBy"`
	Plugins       []CatalogEntry `json:"plugins"`
}

// CatalogEntry is one plugin as a site needs it: identity, the pane it draws,
// and where to get it.
type CatalogEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	License     string   `json:"license"`
	Description string   `json:"description"`
	Glyph       string   `json:"glyph"`
	Category    string   `json:"category"`
	Kinds       []string `json:"kinds"`

	// Source is the plugin's directory in this repository. Install is the
	// source string a reader pastes into the Plugins page: repository#Source.
	Source  string `json:"source"`
	Install string `json:"install"`

	RefreshSeconds int              `json:"refreshSeconds"`
	MinWidth       int              `json:"minWidth"`
	MinHeight      int              `json:"minHeight"`
	Priority       int              `json:"priority"`
	Gauge          bool             `json:"gauge"`
	Spark          bool             `json:"spark"`
	Settings       []CatalogSetting `json:"settings"`
}

// CatalogSetting is one declared setting, enough for a site to describe what a
// plugin lets you configure without shipping its whole schema.
type CatalogSetting struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// BuildCatalog reads every plugin directory under dir and returns the
// catalogue, sorted by id so regenerating it always produces the same bytes.
func BuildCatalog(repository, dir string) (Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{
		SchemaVersion: CatalogSchemaVersion,
		Repository:    repository,
		GeneratedBy:   CatalogGeneratedBy,
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := LoadManifest(filepath.Join(dir, entry.Name()))
		if err != nil {
			return Catalog{}, err
		}
		source := filepath.ToSlash(filepath.Join(filepath.Base(dir), entry.Name()))
		catalog.Plugins = append(catalog.Plugins, catalogEntry(manifest, source, repository))
	}
	sort.Slice(catalog.Plugins, func(i, j int) bool { return catalog.Plugins[i].ID < catalog.Plugins[j].ID })
	return catalog, nil
}

func catalogEntry(manifest Manifest, source, repository string) CatalogEntry {
	panel := manifest.Panel
	displayName := panel.DisplayName
	if displayName == "" {
		displayName = manifest.Name
	}
	settings := make([]CatalogSetting, 0, len(panel.Schema))
	for _, field := range panel.Schema {
		settings = append(settings, CatalogSetting{
			Key: field.Key, Type: field.Type, Label: field.Label, Description: field.Description,
		})
	}
	return CatalogEntry{
		ID: manifest.ID, Name: manifest.Name, DisplayName: displayName,
		Version: manifest.Version, Author: manifest.Author, License: manifest.License,
		Description: manifest.Description, Glyph: panel.Glyph, Category: panel.Category,
		Kinds:  append([]string{}, manifest.Kinds...),
		Source: source, Install: repository + "#" + source,
		RefreshSeconds: panel.RefreshSeconds, MinWidth: panel.MinWidth,
		MinHeight: panel.MinHeight, Priority: panel.Priority,
		Gauge: panel.Gauge, Spark: panel.Spark, Settings: settings,
	}
}

// Encode renders the catalogue the one way it is written: two-space JSON with a
// trailing newline, so the generator and the check that watches it agree byte
// for byte.
func (c Catalog) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
