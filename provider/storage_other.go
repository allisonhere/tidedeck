//go:build !linux

package provider

import (
	"context"
	"errors"

	"github.com/allisonhere/tideui"
)

// Storage is only implemented on Linux.
func Storage() func(context.Context) ([]tideui.StorageMount, error) {
	return func(context.Context) ([]tideui.StorageMount, error) {
		return nil, errors.New("provider: storage is implemented on linux only")
	}
}
