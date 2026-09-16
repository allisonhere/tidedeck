package main

import "testing"

func TestThemeFromOmarchyColors(t *testing.T) {
	theme, ok := themeFromOmarchyColors("azure-glow", `
background = "#0a0f1a"
foreground = "#a8dfff"
accent = "#00aaff"
selection = "#a8dfff"
muted = "#123247"
green = "#00e0b8"
red = "#0099cc"
lighter_background = "#123247"
`)
	if !ok {
		t.Fatal("valid Omarchy colors were rejected")
	}
	if theme.Name != "omarchy · azure-glow" || string(theme.Bg) != "#0a0f1a" || string(theme.BorderFocus) != "#00aaff" {
		t.Fatalf("parsed Omarchy theme = %+v", theme)
	}
}

func TestThemeFromOmarchyColorsRequiresCoreColors(t *testing.T) {
	if _, ok := themeFromOmarchyColors("broken", `background = "#000000"`); ok {
		t.Fatal("incomplete Omarchy palette was accepted")
	}
}
