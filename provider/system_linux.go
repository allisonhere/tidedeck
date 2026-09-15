//go:build linux

package provider

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
)

// System builds a Linux system-health source backed by /proc and /sys. CPU
// usage is computed from the delta between samples, so the first fetch reports
// 0% until a second sample arrives.
func System() func(context.Context) (tideui.SystemMetrics, error) {
	var (
		mu        sync.Mutex
		prevTotal uint64
		prevIdle  uint64
		spark     [18]float64
		sparkLen  int
	)
	return func(context.Context) (tideui.SystemMetrics, error) {
		mu.Lock()
		defer mu.Unlock()

		metrics := tideui.SystemMetrics{}
		total, idle := readCPUStat()
		if prevTotal != 0 && total > prevTotal {
			deltaTotal := total - prevTotal
			deltaIdle := idle - prevIdle
			metrics.CPUPercent = 100 * float64(deltaTotal-deltaIdle) / float64(deltaTotal)
			if metrics.CPUPercent < 0 {
				metrics.CPUPercent = 0
			}
		}
		prevTotal, prevIdle = total, idle

		// Rolling CPU history for the sparkline.
		if sparkLen < len(spark) {
			spark[sparkLen] = metrics.CPUPercent / 100
			sparkLen++
		} else {
			copy(spark[:], spark[1:])
			spark[len(spark)-1] = metrics.CPUPercent / 100
		}
		metrics.CPUSpark = append([]float64(nil), spark[:sparkLen]...)

		metrics.MemoryUsed, metrics.MemoryTotal, metrics.MemoryPercent = readMemInfo()
		metrics.Load = readLoadAvg()
		metrics.Uptime = readUptime()
		metrics.TemperatureC = readTemperature()
		metrics.Processes = readProcessCount()
		return metrics, nil
	}
}

func readCPUStat() (total, idle uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	line := firstLine(string(data))
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}
	var values []uint64
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	for i, value := range values {
		total += value
		if i == 3 || i == 4 { // idle + iowait
			idle += value
		}
	}
	return total, idle
}

func readMemInfo() (used, total string, percent float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "", "", 0
	}
	var totalKB, availableKB float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseFloat(fields[1], 64)
		switch fields[0] {
		case "MemTotal:":
			totalKB = value
		case "MemAvailable:":
			availableKB = value
		}
	}
	if totalKB <= 0 {
		return "", "", 0
	}
	usedKB := totalKB - availableKB
	if usedKB < 0 {
		usedKB = 0
	}
	return humanBytes(usedKB * 1024), humanBytes(totalKB * 1024), 100 * usedKB / totalKB
}

func readLoadAvg() [3]float64 {
	var load [3]float64
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return load
	}
	fields := strings.Fields(string(data))
	for i := 0; i < 3 && i < len(fields); i++ {
		load[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return load
}

func readUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	return time.Duration(seconds * float64(time.Second))
}

func readTemperature() int {
	paths := []string{
		"/sys/class/thermal/thermal_zone0/temp",
		"/sys/class/hwmon/hwmon0/temp1_input",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || value == 0 {
			continue
		}
		return value / 1000
	}
	return 0
}

func readProcessCount() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && isNumeric(entry.Name()) {
			count++
		}
	}
	return count
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
