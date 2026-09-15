package main

import "testing"

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
