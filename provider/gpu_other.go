//go:build !linux

package provider

import (
	"context"
	"errors"

	"github.com/allisonhere/tideui"
)

// GPU is only implemented on Linux. Elsewhere it returns an error so the
// dashboard simply leaves the panel empty.
func GPU() func(context.Context) (tideui.GPUMetrics, error) {
	return func(context.Context) (tideui.GPUMetrics, error) {
		return tideui.GPUMetrics{}, errors.New("provider: gpu metrics are implemented on linux only")
	}
}
