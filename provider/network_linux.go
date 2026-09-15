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

// Network builds a Linux throughput source from /proc/net/dev. If iface is
// empty, all non-loopback interfaces are summed. Rates are computed from the
// delta between samples, so the first fetch reports zero.
func Network(iface string) func(context.Context) (tideui.NetworkMetrics, error) {
	var (
		mu             sync.Mutex
		prevRx, prevTx uint64
		prevTime       time.Time
		history        [24]float64
		historyLen     int
	)
	return func(context.Context) (tideui.NetworkMetrics, error) {
		mu.Lock()
		defer mu.Unlock()

		rx, tx, err := readNetDev(iface)
		if err != nil {
			return tideui.NetworkMetrics{}, err
		}
		now := time.Now()
		metrics := tideui.NetworkMetrics{Interface: iface, Unit: "Mbps"}
		if !prevTime.IsZero() {
			if elapsed := now.Sub(prevTime).Seconds(); elapsed > 0 {
				down := float64(rx-prevRx) / elapsed
				up := float64(tx-prevTx) / elapsed
				metrics.Download = down * 8 / 1e6
				metrics.Upload = up * 8 / 1e6
				if historyLen < len(history) {
					history[historyLen] = metrics.Download
					historyLen++
				} else {
					copy(history[:], history[1:])
					history[len(history)-1] = metrics.Download
				}
			}
		}
		prevRx, prevTx, prevTime = rx, tx, now

		max := 0.0
		for _, value := range history[:historyLen] {
			if value > max {
				max = value
			}
		}
		if max > 0 {
			spark := make([]float64, historyLen)
			for i, value := range history[:historyLen] {
				spark[i] = value / max
			}
			metrics.DownSpark = spark
		}
		return metrics, nil
	}
}

// readNetDev returns received and transmitted bytes for the chosen interface,
// or the sum across non-loopback interfaces.
func readNetDev(iface string) (rx, tx uint64, err error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || name == "" || strings.HasPrefix(line, "Inter-") {
			continue
		}
		if iface != "" && name != iface {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		received, _ := strconv.ParseUint(fields[0], 10, 64)
		transmitted, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += received
		tx += transmitted
	}
	return rx, tx, nil
}
