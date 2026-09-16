package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tideui/provider"
)

func TestConfigDefaultsWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := loadConfig()
	if cfg.Live {
		t.Fatal("a fresh config should default to demo data")
	}
	if !cfg.Weather.Enabled || !cfg.Weather.Fahrenheit {
		t.Fatalf("unexpected weather defaults: %+v", cfg.Weather)
	}
	if !cfg.Clock24 {
		t.Fatal("clock should default to 24-hour")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config{
		Live: true,
		Weather: weatherConfig{
			Enabled: true, Latitude: 52.52, Longitude: 13.405,
			Location: "Berlin", Fahrenheit: false, WindMPH: true,
		},
		Zones:       "Europe/London",
		Clock24:     false,
		GaugeStyle:  "circles",
		SparkStyle:  "braille",
		ClockFont:   "block",
		PanelGauges: map[string]string{"system": "blocks"},
		PanelSparks: map[string]string{"network": "dots"},
		Feeds:       "https://a,https://b",
		Symbols:     "AMD,NVDA",
	}
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	got := loadConfig()
	if !got.Live || got.Weather.Latitude != 52.52 || got.Weather.Location != "Berlin" {
		t.Fatalf("round trip = %+v", got)
	}
	if got.Weather.Fahrenheit {
		t.Fatal("fahrenheit did not round-trip as false")
	}
	if got.Clock24 {
		t.Fatal("clock_24 did not round-trip as false")
	}
	if got.GaugeStyle != "circles" {
		t.Fatalf("gauge_style = %q, want circles", got.GaugeStyle)
	}
	if got.PanelGauges["system"] != "blocks" {
		t.Fatalf("panel_gauges = %+v, want system:blocks", got.PanelGauges)
	}
	if got.SparkStyle != "braille" {
		t.Fatalf("spark_style = %q, want braille", got.SparkStyle)
	}
	if got.PanelSparks["network"] != "dots" {
		t.Fatalf("panel_sparks = %+v, want network:dots", got.PanelSparks)
	}
	if got.ClockFont != "block" {
		t.Fatalf("clock_font = %q, want block", got.ClockFont)
	}
	if got.Feeds != "https://a,https://b" || got.Symbols != "AMD,NVDA" {
		t.Fatalf("lists did not round trip: %+v", got)
	}
}

func TestListParsing(t *testing.T) {
	got := list("a, b ,, c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("list = %v", got)
	}
	if list("  ") != nil {
		t.Fatal("blank list should be empty")
	}
}

func TestDefaultFeedsAreSeeded(t *testing.T) {
	cfg := defaultConfig()
	urls := list(cfg.Feeds)
	if len(urls) == 0 {
		t.Fatal("default config has no feeds: the News panel would start empty")
	}
	for _, url := range urls {
		source, ok := provider.NewsSourceByURL(url)
		if !ok {
			t.Fatalf("default feed %q is not in the catalogue", url)
		}
		if !source.Default {
			t.Fatalf("%s is seeded but not marked Default", source.Name)
		}
	}
}

func TestFeedsSplitAndJoinRoundTrip(t *testing.T) {
	catalogue := provider.NewsSources()
	first, second := catalogue[0].URL, catalogue[1].URL
	const custom = "https://example.com/custom.xml"

	presets, rest := splitFeeds(first + "," + custom)
	if rest != custom {
		t.Fatalf("custom URLs = %q, want %q", rest, custom)
	}
	if !presets[0] {
		t.Fatalf("%s should be ticked", catalogue[0].Name)
	}
	for i := 1; i < len(presets); i++ {
		if presets[i] {
			t.Fatalf("%s should not be ticked", catalogue[i].Name)
		}
	}

	// Rejoining keeps both, and a second pass changes nothing.
	joined := joinFeeds(presets, rest)
	if !strings.Contains(joined, first) || !strings.Contains(joined, custom) {
		t.Fatalf("joinFeeds lost a URL: %q", joined)
	}
	again, againRest := splitFeeds(joined)
	if joinFeeds(again, againRest) != joined {
		t.Fatalf("round trip is not stable: %q then %q", joined, joinFeeds(again, againRest))
	}

	// A catalogue URL written differently still ticks its row, and is not
	// also left behind in the custom text.
	loose := strings.Replace(first, "https://", "http://", 1) + "/"
	presets, rest = splitFeeds(loose)
	if !presets[0] {
		t.Fatalf("%q should match %s", loose, catalogue[0].Name)
	}
	if rest != "" {
		t.Fatalf("custom text should be empty, got %q", rest)
	}

	// Duplicates collapse, and two ticks both survive.
	presets, rest = splitFeeds(first + "," + first + "," + second)
	if !presets[0] || !presets[1] {
		t.Fatal("both catalogue URLs should be ticked")
	}
	if rest != "" {
		t.Fatalf("custom text should be empty, got %q", rest)
	}
	if got := strings.Count(joinFeeds(presets, rest), first); got != 1 {
		t.Fatalf("%s appears %d times, want 1", catalogue[0].Name, got)
	}

	// Nothing ticked and nothing typed saves as empty, not as a stray comma.
	if got := joinFeeds(make([]bool, len(catalogue)), ""); got != "" {
		t.Fatalf("empty selection = %q, want empty", got)
	}
	// A short or nil presets slice must not panic.
	if got := joinFeeds(nil, custom); got != custom {
		t.Fatalf("nil presets = %q, want %q", got, custom)
	}
}

func TestRepoListParsingAndNormalization(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir() // a directory, but not a repository

	repos := parseRepoList(fmt.Sprintf(" %s , %s ,, %s , %s/does-not-exist", real, plain, real, plain))
	// The duplicate is dropped and blanks ignored; order is preserved.
	if len(repos) != 3 {
		t.Fatalf("parsed %d repos, want 3 (duplicate and blank dropped): %#v", len(repos), repos)
	}
	if !repos[0].IsRepo {
		t.Fatalf("%s has a .git and should be a repo", repos[0].Full)
	}
	if repos[1].IsRepo || !repos[1].Exists {
		t.Fatalf("a plain directory exists but is not a repo: %#v", repos[1])
	}
	if repos[2].Exists {
		t.Fatalf("a missing path should not report as existing: %#v", repos[2])
	}

	// The home directory is written back as ~, so the stored value stays short.
	collapsed := normalizeRepoList(filepath.Join(home, "Projects", "tidedeck"))
	if collapsed != "~/Projects/tidedeck" {
		t.Fatalf("normalizeRepoList = %q, want ~/Projects/tidedeck", collapsed)
	}
	// ~ and the absolute spelling are the same repository, not two.
	both := normalizeRepoList(filepath.Join(home, "x") + ",~/x")
	if both != "~/x" {
		t.Fatalf("normalizeRepoList = %q, want the duplicate collapsed", both)
	}
	if got := normalizeRepoList("  ,  "); got != "" {
		t.Fatalf("normalizeRepoList of blanks = %q, want empty", got)
	}
}

// Normalization used to run filepath.Clean over every entry, which collapsed
// the "//" in an https URL and silently rewrote what the user typed.
func TestRepoListKeepsRemoteURLsIntact(t *testing.T) {
	const url = "https://github.com/allisonhere/tidemail"
	if got := normalizeRepoList(url); got != url {
		t.Fatalf("normalizeRepoList(%q) = %q, want it unchanged", url, got)
	}
	repos := parseRepoList(url + ",git@github.com:allisonhere/tideui.git")
	if len(repos) != 2 {
		t.Fatalf("parsed %#v, want both URLs kept", repos)
	}
	for _, repo := range repos {
		if !repo.Remote {
			t.Fatalf("%q should be marked remote", repo.Display)
		}
		if repo.IsRepo {
			t.Fatalf("%q is not a working copy", repo.Display)
		}
	}
	// A URL and a local path are both kept, and the local one still resolves.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	mixed := normalizeRepoList(url + "," + filepath.Join(home, "x"))
	if !strings.Contains(mixed, url) || !strings.Contains(mixed, "~/x") {
		t.Fatalf("mixed list = %q, want the URL intact and the path collapsed", mixed)
	}
}

// writeConfig puts a document at the path loadConfig reads, and returns it.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readConfigDoc(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	return out
}

// Saving used to marshal the typed struct alone, so any key the struct had no
// field for was dropped. That is how a panel-owned setting, or one written by
// a newer build, would silently disappear the first time someone opened
// settings and pressed save.
func TestSavePreservesUnknownKeys(t *testing.T) {
	writeConfig(t, `{
  "live": true,
  "aur_helper": "paru",
  "some_future_panel": {"endpoint": "https://example.com", "every": 30},
  "hand_added": "keep me"
}`)
	cfg := loadConfig()
	if !cfg.Live {
		t.Fatalf("known keys did not load: %+v", cfg)
	}

	cfg.Interface = "wlan0"
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}

	saved := readConfigDoc(t)
	if saved["hand_added"] != "keep me" {
		t.Fatalf("a key the struct does not know was dropped: %v", saved["hand_added"])
	}
	nested, ok := saved["some_future_panel"].(map[string]any)
	if !ok || nested["endpoint"] != "https://example.com" || nested["every"] != float64(30) {
		t.Fatalf("a nested unknown key was not preserved: %v", saved["some_future_panel"])
	}
	if saved["interface"] != "wlan0" {
		t.Fatalf("the edited key was not written: %v", saved["interface"])
	}
	// aur_helper belongs to the updates panel now, so the struct has no field
	// for it - and it must survive exactly like any other unowned key.
	if saved["aur_helper"] != "paru" {
		t.Fatalf("a panel-owned key was dropped: %v", saved["aur_helper"])
	}
}

// The other half of the contract: a key the struct owns must be able to go
// away. Preserving the document naively would let a cleared value fall back
// to whatever was on disk.
func TestSaveClearsOwnedKeysThatAreNowEmpty(t *testing.T) {
	writeConfig(t, `{
  "panel_gauges": {"system": "marker"},
  "interface": "eth0",
  "keep": "this"
}`)
	cfg := loadConfig()
	if len(cfg.PanelGauges) != 1 {
		t.Fatalf("panel_gauges did not load: %+v", cfg.PanelGauges)
	}

	// Clearing an omitempty map means the key is absent from the marshalled
	// struct entirely, which must clear it rather than keep the old value.
	cfg.PanelGauges = nil
	cfg.Interface = ""
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}

	saved := readConfigDoc(t)
	if _, present := saved["panel_gauges"]; present {
		t.Fatalf("cleared panel_gauges came back as %v", saved["panel_gauges"])
	}
	if saved["interface"] != "" {
		t.Fatalf("cleared interface = %v, want empty", saved["interface"])
	}
	if saved["keep"] != "this" {
		t.Fatal("clearing an owned key dropped an unowned one")
	}
}

// The settings form rebuilds the config from scratch, so the document has to
// survive that too - this is the path a user actually takes.
func TestSettingsSavePreservesUnknownKeys(t *testing.T) {
	writeConfig(t, `{"aur_helper": "paru", "panel_owned": {"thing": 1}}`)
	cfg := loadConfig()

	form := newSettingsForm()
	form.Open(cfg)
	rebuilt, err := form.state.toConfig(form.deck)
	if err != nil {
		t.Fatal(err)
	}
	if err := rebuilt.save(); err != nil {
		t.Fatal(err)
	}
	saved := readConfigDoc(t)
	nested, ok := saved["panel_owned"].(map[string]any)
	if !ok || nested["thing"] != float64(1) {
		t.Fatalf("settings save dropped an unowned key: %v", saved["panel_owned"])
	}
	if saved["aur_helper"] != "paru" {
		t.Fatalf("settings save changed an untouched key: %v", saved["aur_helper"])
	}
}

// A config written by this build must load and save unchanged, so opening
// settings and saving without editing anything is a no-op on disk.
func TestSaveIsIdempotent(t *testing.T) {
	writeConfig(t, `{"live": true, "aur_helper": "yay", "extra": [1, 2, 3]}`)
	cfg := loadConfig()
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	again := loadConfig()
	if err := again.save(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("saving twice changed the file:\n%s\n---\n%s", first, second)
	}
}

// A missing config file must still save, rather than failing on a nil
// document.
func TestSaveWithoutAnExistingFile(t *testing.T) {
	_ = os.Remove(configPath())
	cfg := defaultConfig()
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	saved := readConfigDoc(t)
	if saved["zones"] != defaultConfig().Zones {
		t.Fatalf("defaults were not written: %v", saved["zones"])
	}
}
