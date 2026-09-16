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
	Live       bool          `json:"live"`
	Weather    weatherConfig `json:"weather"`
	Zones      string        `json:"zones"`
	Clock24    bool          `json:"clock_24"`
	GaugeStyle string        `json:"gauge_style"`
	SparkStyle string        `json:"spark_style"`
	ClockFont  string        `json:"clock_font"`
	// Icons picks the widget icon family: "plain", "emoji" or "nerd". Nerd
	// needs a patched font and emoji a colour emoji font.
	Icons       string            `json:"icons"`
	PanelGauges map[string]string `json:"panel_gauges,omitempty"`
	// PanelSparks overrides the sparkline style per panel id. "default" or a
	// missing entry follows the workspace SparkStyle.
	PanelSparks map[string]string `json:"panel_sparks,omitempty"`
	Feeds       string            `json:"feeds"`
	// Calendars lists comma-separated iCalendar sources: local .ics paths or
	// https/webcal URLs, such as a Google Calendar "secret address in iCal
	// format".
	Calendars string `json:"calendars"`
	Todo      string `json:"todo"`
	Notes     string `json:"notes"`
	Repos     string `json:"repos"`
	Symbols   string `json:"symbols"`
	Systemd   string `json:"systemd"`
	Docker    string `json:"docker"`
	Interface string `json:"interface"`
	// AURHelper names the AUR wrapper used to count AUR updates ("yay",
	// "paru"). Empty falls back to yay.
	AURHelper string `json:"aur_helper"`

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

// weatherConfig configures the Open-Meteo weather source.
type weatherConfig struct {
	Enabled    bool    `json:"enabled"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Location   string  `json:"location"`
	Fahrenheit bool    `json:"fahrenheit"`
	WindMPH    bool    `json:"wind_mph"`
}

// usable reports whether the weather source has somewhere to look.
func (w weatherConfig) usable() bool {
	return w.Enabled && (w.Latitude != 0 || w.Longitude != 0)
}

func defaultConfig() config {
	return config{
		Weather: weatherConfig{
			Enabled:    true,
			Fahrenheit: true,
			WindMPH:    true,
			Location:   "Local",
		},
		Zones:      "Europe/London,Asia/Tokyo,Sydney",
		Feeds:      defaultFeeds(),
		Clock24:    true,
		GaugeStyle: "solid",
		SparkStyle: "blocks",
		ClockFont:  "dash",
		Icons:      "emoji",
		AURHelper:  "yay",
	}
}

func configPath() string {
	return filepath.Join(userConfigDir(), "tidedeck", "config.json")
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
func list(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = expandPath(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func expandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

// repoPath is one configured repository and what is actually at that path.
type repoPath struct {
	Display string // "~/Projects/tideui", as it should be stored and shown
	Full    string // the expanded absolute path
	Exists  bool
	IsRepo  bool
	Remote  bool // a clone URL rather than a working copy
}

// parseRepoList reads the configured repository paths, expanding ~, dropping
// blanks, and dropping duplicates that differ only in spelling. Order is
// preserved so the list stays the one the user typed.
func parseRepoList(value string) []repoPath {
	var repos []repoPath
	seen := make(map[string]bool)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// A remote URL is not a filesystem path: filepath.Clean would collapse
		// the "//" in "https://" and silently corrupt what was typed.
		if provider.IsRemoteURL(part) {
			if seen[part] {
				continue
			}
			seen[part] = true
			repos = append(repos, repoPath{Display: part, Full: part, Remote: true})
			continue
		}
		full := filepath.Clean(expandPath(part))
		if seen[full] {
			continue
		}
		seen[full] = true
		entry := repoPath{Display: collapseHome(full), Full: full}
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			entry.Exists = true
			if _, err := os.Stat(filepath.Join(full, ".git")); err == nil {
				entry.IsRepo = true
			}
		}
		repos = append(repos, entry)
	}
	return repos
}

// normalizeRepoList rewrites the stored value: deduplicated, trimmed, and with
// the home directory written as ~ so the field stays readable.
func normalizeRepoList(value string) string {
	repos := parseRepoList(value)
	paths := make([]string, 0, len(repos))
	for _, repo := range repos {
		paths = append(paths, repo.Display)
	}
	return strings.Join(paths, ", ")
}

// collapseHome is the inverse of expandPath, so a saved config shows ~ rather
// than a long absolute path.
func collapseHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}
