//go:build !linux

package provider

import (
	"context"
	"errors"

	"github.com/allisonhere/tideui"
)

// Network is only implemented on Linux.
func Network(iface string) func(context.Context) (tideui.NetworkMetrics, error) {
	return func(context.Context) (tideui.NetworkMetrics, error) {
		return tideui.NetworkMetrics{}, errors.New("provider: network is implemented on linux only")
	}
}
