//go:build linux

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeGPU builds a sysfs-shaped fixture tree and points gpuSysfsRoot at it.
func fakeGPU(t *testing.T, card string, files map[string]string) {
	t.Helper()
	root := t.TempDir()
	device := filepath.Join(root, card, "device")
	if err := os.MkdirAll(filepath.Join(device, "hwmon", "hwmon3"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range files {
		path := filepath.Join(device, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	original := gpuSysfsRoot
	gpuSysfsRoot = root
	t.Cleanup(func() { gpuSysfsRoot = original })
}

// An APU carves a few hundred MB of "VRAM" out of system RAM and keeps the
// real pool in GTT, so the widget must be given GTT.
func TestGPUIntegratedPrefersGTT(t *testing.T) {
	fakeGPU(t, "card1", map[string]string{
		"uevent":                      "DRIVER=amdgpu\nPCI_ID=1002:1586\n",
		"gpu_busy_percent":            "7\n",
		"mem_info_vram_total":         "536870912\n",
		"mem_info_vram_used":          "466870272\n",
		"mem_info_gtt_total":          "33253330944\n",
		"mem_info_gtt_used":           "31301365760\n",
		"hwmon/hwmon3/temp1_input":    "48000\n",
		"hwmon/hwmon3/power1_average": "16200000\n",
		"hwmon/hwmon3/freq1_input":    "1003000000\n",
	})
	metrics, err := GPU()(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Name != "amdgpu" {
		t.Fatalf("name = %q, want amdgpu", metrics.Name)
	}
	if !metrics.Integrated {
		t.Fatal("a 512MB VRAM carve-out should be detected as integrated")
	}
	if metrics.MemoryTotal != "31.0 GB" || metrics.MemoryUsed != "29.2 GB" {
		t.Fatalf("memory = %s/%s, want the GTT pool", metrics.MemoryUsed, metrics.MemoryTotal)
	}
	if metrics.MemoryLabel != "MEM" {
		t.Fatalf("label = %q, want MEM", metrics.MemoryLabel)
	}
	if metrics.BusyPercent != 7 {
		t.Fatalf("busy = %v, want 7", metrics.BusyPercent)
	}
	if metrics.TemperatureC != 48 {
		t.Fatalf("temp = %d, want 48", metrics.TemperatureC)
	}
	if metrics.PowerWatts < 16.1 || metrics.PowerWatts > 16.3 {
		t.Fatalf("power = %v, want ~16.2W", metrics.PowerWatts)
	}
	if metrics.ClockMHz != 1003 {
		t.Fatalf("clock = %d, want 1003", metrics.ClockMHz)
	}
	if len(metrics.BusySpark) != 1 {
		t.Fatalf("spark = %#v, want one sample after one fetch", metrics.BusySpark)
	}
}

// A discrete card reports gigabytes of real VRAM, where the percentage is a
// genuine pressure signal and the widget gauges it.
func TestGPUDiscreteUsesVRAM(t *testing.T) {
	fakeGPU(t, "card0", map[string]string{
		"uevent":              "DRIVER=nvidia\n",
		"gpu_busy_percent":    "63\n",
		"mem_info_vram_total": "8589934592\n",
		"mem_info_vram_used":  "5476083302\n",
		"mem_info_gtt_total":  "16000000000\n",
		"mem_info_gtt_used":   "1000000000\n",
	})
	metrics, err := GPU()(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Integrated {
		t.Fatal("8GB of VRAM is a discrete card")
	}
	if metrics.MemoryLabel != "VRAM" || metrics.MemoryTotal != "8.0 GB" {
		t.Fatalf("memory = %s %s/%s, want the VRAM pool", metrics.MemoryLabel, metrics.MemoryUsed, metrics.MemoryTotal)
	}
	if metrics.MemoryFrac < 0.63 || metrics.MemoryFrac > 0.65 {
		t.Fatalf("memory fraction = %v, want ~0.64", metrics.MemoryFrac)
	}
	// Absent hwmon files stay zero so the widget can leave those rows out.
	if metrics.TemperatureC != 0 || metrics.PowerWatts != 0 || metrics.ClockMHz != 0 {
		t.Fatalf("missing hwmon should read zero, got %+v", metrics)
	}
}

// Connector directories (card1-DP-1) carry no metrics and must be skipped.
func TestGPUAbsentIsEmptyNotAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "card1-DP-1", "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := gpuSysfsRoot
	gpuSysfsRoot = root
	t.Cleanup(func() { gpuSysfsRoot = original })

	metrics, err := GPU()(context.Background())
	if err != nil {
		t.Fatalf("no GPU is not an error: %v", err)
	}
	if metrics.Name != "" {
		t.Fatalf("expected an empty result, got %+v", metrics)
	}
}

func TestGPUSparkGrowsAndStaysBounded(t *testing.T) {
	fakeGPU(t, "card1", map[string]string{
		"uevent":              "DRIVER=amdgpu\n",
		"gpu_busy_percent":    "50\n",
		"mem_info_vram_total": "536870912\n",
		"mem_info_gtt_total":  "33253330944\n",
	})
	fetch := GPU()
	var metrics = struct{ len int }{}
	for i := 0; i < 25; i++ {
		m, err := fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		metrics.len = len(m.BusySpark)
		for _, v := range m.BusySpark {
			if v < 0 || v > 1 {
				t.Fatalf("spark value %v outside 0..1", v)
			}
		}
	}
	if metrics.len != 18 {
		t.Fatalf("spark length = %d, want it capped at 18", metrics.len)
	}
}
