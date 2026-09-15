package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Clock builds a clock source for the local machine plus the given IANA time
// zones (for example "Europe/London", "Asia/Tokyo").
func Clock(location string, zones ...string) func(context.Context) (tideui.ClockData, error) {
	return func(context.Context) (tideui.ClockData, error) {
		now := time.Now()
		data := tideui.ClockData{Local: now, Location: location}
		for _, name := range zones {
			loc, err := time.LoadLocation(name)
			if err != nil {
				continue
			}
			_, offset := now.In(loc).Zone()
			data.Zones = append(data.Zones, tideui.WorldClock{
				City:   cityName(name),
				Time:   now.In(loc),
				Offset: formatOffset(offset),
			})
		}
		return data, nil
	}
}

// cityName turns "Europe/London" into "London".
func cityName(zone string) string {
	if index := strings.LastIndexByte(zone, '/'); index >= 0 {
		zone = zone[index+1:]
	}
	return strings.ReplaceAll(zone, "_", " ")
}

// formatOffset renders a UTC offset in seconds as "+9", "-5", or "+5:30".
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if minutes == 0 {
		return fmt.Sprintf("%s%d", sign, hours)
	}
	return fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
}
