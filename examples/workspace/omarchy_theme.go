package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
)

// omarchyCurrentTheme reads the generated Omarchy palette rather than
// guessing which TideUI palette is closest. Omarchy refreshes this directory
// whenever its theme changes, so opening either theme picker gets a fresh
// snapshot without coupling TideDeck to Omarchy's shell process.
func omarchyCurrentTheme() (tideui.Theme, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return tideui.Theme{}, false
	}
	root := filepath.Join(home, ".local", "state", "omarchy", "current")
	nameBytes, err := os.ReadFile(filepath.Join(root, "theme.name"))
	if err != nil {
		return tideui.Theme{}, false
	}
	colors, err := os.ReadFile(filepath.Join(root, "theme", "colors.toml"))
	if err != nil {
		return tideui.Theme{}, false
	}
	name := strings.TrimSpace(string(nameBytes))
	if name == "" {
		name = "current"
	}
	theme, ok := themeFromOmarchyColors(name, string(colors))
	return theme, ok
}

func themeFromOmarchyColors(name, source string) (tideui.Theme, bool) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(source))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if comment := strings.Index(line, " #"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		if strings.HasPrefix(value, "#") && len(value) >= 7 {
			values[key] = value
		}
	}
	if scanner.Err() != nil || values["background"] == "" || values["foreground"] == "" || values["accent"] == "" {
		return tideui.Theme{}, false
	}
	muted := firstColor(values, "muted", "dark_foreground", "border")
	selection := firstColor(values, "selection", "lighter_background", "accent")
	surface := firstColor(values, "lighter_background", "dark_background", "background")
	return tideui.Theme{
		Name:          "omarchy · " + name,
		Bg:            lipgloss.Color(values["background"]),
		Fg:            lipgloss.Color(values["foreground"]),
		Border:        lipgloss.Color(muted),
		BorderFocus:   lipgloss.Color(values["accent"]),
		Selected:      lipgloss.Color(selection),
		Unread:        lipgloss.Color(firstColor(values, "green", "bright_green", "accent")),
		Dimmed:        lipgloss.Color(muted),
		StatusBar:     lipgloss.Color(surface),
		StatusFg:      lipgloss.Color(values["foreground"]),
		Error:         lipgloss.Color(firstColor(values, "red", "accent")),
		Overlay:       lipgloss.Color(surface),
		OverlayBorder: lipgloss.Color(values["accent"]),
	}, true
}

func firstColor(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := values[key]; value != "" {
			return value
		}
	}
	return "#000000"
}

func themePickerThemes() []tideui.Theme {
	themes := append([]tideui.Theme(nil), tideui.BuiltinThemes...)
	if theme, ok := omarchyCurrentTheme(); ok {
		themes = append(themes, theme)
	}
	return themes
}
