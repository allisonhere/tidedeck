package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// config is the application's editable configuration. Every field is edited
// through the in-app settings panel (s) and persisted as JSON, so no
// environment variables or hand-editing are required.
type config struct {
	Live       bool   `json:"live"`
	GaugeStyle string `json:"gauge_style"`
	SparkStyle string `json:"spark_style"`
	ClockFont  string `json:"clock_font"`
	// Icons picks the widget icon family: "plain", "emoji" or "nerd". Nerd
	// needs a patched font and emoji a colour emoji font.
	Icons string `json:"icons"`
	// GlyphMode controls panel header glyphs: on, off, or per_panel.
	GlyphMode   string          `json:"glyph_mode"`
	PanelGlyphs map[string]bool `json:"panel_glyphs,omitempty"`
	// LayoutThemes maps preset/slot identifiers to workspace theme names.
	LayoutThemes map[string]string `json:"layout_themes,omitempty"`
	PanelGauges  map[string]string `json:"panel_gauges,omitempty"`
	// PanelSparks overrides the sparkline style per panel id. "default" or a
	// missing entry follows the workspace SparkStyle.
	PanelSparks map[string]string `json:"panel_sparks,omitempty"`
	Feeds       string            `json:"feeds"`
	Todo        string            `json:"todo"`
	// doc is the document this config was decoded from. Saving writes the
	// document back rather than only the fields below, so a key this build
	// does not recognise - one a panel owns, or one a newer build wrote -
	// survives being edited in settings instead of being silently dropped.
	// It is unexported, so encoding/json ignores it in both directions.
	doc dash.Values
}

// configKeys are the top-level keys the struct above owns. Saving replaces
// exactly these in the document and leaves everything else untouched, which
// is what makes an omitted key (an empty panel_gauges, say) actually clear
// rather than falling back to the stale value on disk.
func configKeys() []string {
	fields := reflect.VisibleFields(reflect.TypeOf(config{}))
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if tag != "" && tag != "-" {
			keys = append(keys, tag)
		}
	}
	return keys
}

// document renders the config as the document to persist: what was loaded,
// with the keys this struct owns replaced by its current values.
func (c config) document() (dash.Values, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return dash.Values{}, err
	}
	typed, err := dash.LoadValues(data)
	if err != nil {
		return dash.Values{}, err
	}
	out := c.doc.Clone()
	for _, key := range configKeys() {
		out.Delete(key)
	}
	out.Merge(typed)
	return out, nil
}

// withDoc returns the config carrying a source document, so a config rebuilt
// from the settings form keeps the keys the form does not know about.
func (c config) withDoc(doc dash.Values) config {
	c.doc = doc
	return c
}

func defaultConfig() config {
	return config{
		Feeds:      defaultFeeds(),
		GaugeStyle: "solid",
		SparkStyle: "blocks",
		ClockFont:  "dash",
		Icons:      "emoji",
		GlyphMode:  glyphModeOn,
	}
}

func configPath() string {
	return filepath.Join(userConfigDir(), "tidedeck", "config.json")
}

// pluginsDir is where installed plugins live: one subdirectory each.
func pluginsDir() string {
	return filepath.Join(userConfigDir(), "tidedeck", "plugins")
}

// loadConfig reads the config file, falling back to defaults when it is
// missing or unreadable so the app always starts.
func loadConfig() config {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return defaultConfig()
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	// Keep what was on disk, so saving does not drop keys this build has no
	// field for.
	if doc, err := dash.LoadValues(data); err == nil {
		cfg.doc = doc
	}
	return cfg
}

// save writes the config to disk, creating the directory if needed.
func (c config) save() error {
	document, err := c.document()
	if err != nil {
		return err
	}
	data, err := document.Bytes()
	if err != nil {
		return err
	}
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// defaultFeeds is the starting set of news sources, so the News panel has
// something in it before anyone has pasted a URL.
func defaultFeeds() string {
	var urls []string
	for _, source := range provider.DefaultNewsSources() {
		urls = append(urls, source.URL)
	}
	return strings.Join(urls, ",")
}

// splitFeeds separates a saved feeds value into catalogue ticks and the URLs
// that are not in the catalogue, which stay editable as free text. The ticks
// line up with provider.NewsSources() by index.
func splitFeeds(value string) (presets []bool, custom string) {
	catalogue := provider.NewsSources()
	presets = make([]bool, len(catalogue))
	index := make(map[string]int, len(catalogue))
	for i, source := range catalogue {
		index[provider.CanonicalFeedURL(source.URL)] = i
	}
	seen := make(map[string]bool)
	var rest []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := provider.CanonicalFeedURL(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		if i, ok := index[key]; ok {
			presets[i] = true
			continue
		}
		rest = append(rest, part)
	}
	return presets, strings.Join(rest, ", ")
}

// joinFeeds rebuilds the saved value: ticked catalogue entries in catalogue
// order, then custom URLs as typed, with duplicates dropped. Saving twice
// without editing anything is a no-op.
func joinFeeds(presets []bool, custom string) string {
	var out []string
	seen := make(map[string]bool)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		key := provider.CanonicalFeedURL(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, raw)
	}
	for i, source := range provider.NewsSources() {
		if i < len(presets) && presets[i] {
			add(source.URL)
		}
	}
	for _, part := range strings.Split(custom, ",") {
		add(part)
	}
	return strings.Join(out, ",")
}

// list splits a comma-separated config value, trims blanks, and expands a
// leading ~.
func formatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}
