//go:build linux

package provider

import "testing"

func TestReadMemInfo(t *testing.T) {
	_, total, percent := readMemInfo()
	if total == "" {
		t.Skip("no /proc/meminfo available")
	}
	if percent < 0 || percent > 100 {
		t.Fatalf("memory percent = %v, want 0..100", percent)
	}
}

func TestReadCPUStat(t *testing.T) {
	total, idle := readCPUStat()
	if total == 0 {
		t.Skip("no /proc/stat available")
	}
	if idle > total {
		t.Fatalf("idle %d greater than total %d", idle, total)
	}
}

func TestReadLoadAvg(t *testing.T) {
	load := readLoadAvg()
	if load[0] < 0 {
		t.Fatalf("load = %v", load)
	}
}
