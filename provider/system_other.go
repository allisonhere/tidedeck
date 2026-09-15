//go:build !linux

package provider

import (
	"context"
	"errors"

	"github.com/allisonhere/tideui"
)

// System is only implemented on Linux. Elsewhere it returns an error so the
// dashboard simply leaves the panel empty.
func System() func(context.Context) (tideui.SystemMetrics, error) {
	return func(context.Context) (tideui.SystemMetrics, error) {
		return tideui.SystemMetrics{}, errors.New("provider: system metrics are implemented on linux only")
	}
}
