//go:build linux

package provider

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/allisonhere/tideui"
)

// gpuSysfsRoot is the DRM class directory. It is a variable so tests can point
// at a fixture tree instead of real hardware.
var gpuSysfsRoot = "/sys/class/drm"

// integratedVRAMCeiling is the carve-out below which a GPU is treated as
// integrated. An APU reports a few hundred MB of "VRAM" and keeps the real
// pool in GTT; a discrete card reports gigabytes.
const integratedVRAMCeiling = 2 << 30

// GPU builds a Linux GPU source backed by DRM sysfs. It reads whichever card
// reports a utilisation figure, so a machine with both an integrated and a
// discrete GPU follows the one the kernel is accounting for.
func GPU() func(context.Context) (tideui.GPUMetrics, error) {
	var (
		mu       sync.Mutex
		spark    [18]float64
		sparkLen int
	)
	return func(context.Context) (tideui.GPUMetrics, error) {
		mu.Lock()
		defer mu.Unlock()

		metrics := tideui.GPUMetrics{}
		device, ok := findGPUDevice()
		if !ok {
			return metrics, nil
		}
		metrics.Name = gpuDriverName(device)
		metrics.BusyPercent = float64(readSysfsInt(filepath.Join(device, "gpu_busy_percent")))

		// Rolling history for the sparkline.
		if sparkLen < len(spark) {
			spark[sparkLen] = metrics.BusyPercent / 100
			sparkLen++
		} else {
			copy(spark[:], spark[1:])
			spark[len(spark)-1] = metrics.BusyPercent / 100
		}
		metrics.BusySpark = append([]float64(nil), spark[:sparkLen]...)

		readGPUMemory(device, &metrics)
		readGPUHwmon(device, &metrics)
		return metrics, nil
	}
}

// findGPUDevice returns the device directory of the first card that reports a
// utilisation figure. Cards are visited in name order so the choice is stable.
func findGPUDevice() (string, bool) {
	matches, err := filepath.Glob(filepath.Join(gpuSysfsRoot, "card[0-9]*"))
	if err != nil {
		return "", false
	}
	sortStrings(matches)
	for _, card := range matches {
		// Connectors are named card1-DP-1 and carry no metrics of their own.
		if strings.Contains(filepath.Base(card), "-") {
			continue
		}
		device := filepath.Join(card, "device")
		if _, err := os.Stat(filepath.Join(device, "gpu_busy_percent")); err == nil {
			return device, true
		}
	}
	return "", false
}

// readGPUMemory fills in the memory figures, preferring GTT on an integrated
// GPU because its VRAM is a small fixed carve-out of system RAM.
func readGPUMemory(device string, metrics *tideui.GPUMetrics) {
	vramTotal := readSysfsInt(filepath.Join(device, "mem_info_vram_total"))
	vramUsed := readSysfsInt(filepath.Join(device, "mem_info_vram_used"))
	gttTotal := readSysfsInt(filepath.Join(device, "mem_info_gtt_total"))
	gttUsed := readSysfsInt(filepath.Join(device, "mem_info_gtt_used"))

	metrics.Integrated = vramTotal > 0 && vramTotal < integratedVRAMCeiling
	// Labels stay within the widget's five-cell label column. That the
	// integrated pool is shared is said in the detail view rather than by a
	// longer label that would push the value out of line.
	used, total, label := vramUsed, vramTotal, "VRAM"
	if metrics.Integrated && gttTotal > 0 {
		used, total, label = gttUsed, gttTotal, "MEM"
	}
	if total <= 0 {
		return
	}
	metrics.MemoryUsed = humanBytes(float64(used))
	metrics.MemoryTotal = humanBytes(float64(total))
	metrics.MemoryLabel = label
	metrics.MemoryFrac = float64(used) / float64(total)
}

// readGPUHwmon fills in temperature, power and clock from the card's hwmon
// node. Missing files stay zero and the widget leaves their rows out.
func readGPUHwmon(device string, metrics *tideui.GPUMetrics) {
	nodes, err := filepath.Glob(filepath.Join(device, "hwmon", "hwmon[0-9]*"))
	if err != nil || len(nodes) == 0 {
		return
	}
	sortStrings(nodes)
	node := nodes[0]
	if value := readSysfsInt(filepath.Join(node, "temp1_input")); value > 0 {
		metrics.TemperatureC = value / 1000
	}
	if value := readSysfsInt(filepath.Join(node, "power1_average")); value > 0 {
		metrics.PowerWatts = float64(value) / 1e6 // microwatts
	}
	if value := readSysfsInt(filepath.Join(node, "freq1_input")); value > 0 {
		metrics.ClockMHz = value / 1e6 // hertz
	}
}

// gpuDriverName reads the kernel driver backing a card ("amdgpu", "i915"),
// which is the most honest short label sysfs offers: there is no product name.
func gpuDriverName(device string) string {
	data, err := os.ReadFile(filepath.Join(device, "uevent"))
	if err != nil {
		return "gpu"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if name, ok := strings.CutPrefix(line, "DRIVER="); ok {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				return trimmed
			}
		}
	}
	return "gpu"
}

// readSysfsInt reads a single integer from a sysfs file, returning 0 when the
// file is missing or unparsable, as the other Linux readers do.
func readSysfsInt(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return value
}

// sortStrings orders paths so card selection does not depend on glob order.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
