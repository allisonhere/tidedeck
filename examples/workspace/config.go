package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// config is the application's editable configuration. Every field is edited
// through the in-app settings panel (s) and persisted as JSON, so no
// environment variables or hand-editing are required.
type config struct {
	Live        bool              `json:"live"`
	Weather     weatherConfig     `json:"weather"`
	Zones       string            `json:"zones"`
	Clock24     bool              `json:"clock_24"`
	GaugeStyle  string            `json:"gauge_style"`
	SparkStyle  string            `json:"spark_style"`
	PanelGauges map[string]string `json:"panel_gauges,omitempty"`
	// PanelSparks overrides the sparkline style per panel id. "default" or a
	// missing entry follows the workspace SparkStyle.
	PanelSparks map[string]string `json:"panel_sparks,omitempty"`
	Feeds       string            `json:"feeds"`
	Calendars   string            `json:"calendars"`
	Todo        string            `json:"todo"`
	Notes       string            `json:"notes"`
	Repos       string            `json:"repos"`
	Symbols     string            `json:"symbols"`
	Systemd     string            `json:"systemd"`
	Docker      string            `json:"docker"`
	Interface   string            `json:"interface"`
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
		Clock24:    true,
		GaugeStyle: "solid",
		SparkStyle: "blocks",
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
	return cfg
}

// save writes the config to disk, creating the directory if needed.
func (c config) save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
