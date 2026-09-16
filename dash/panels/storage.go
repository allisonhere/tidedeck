package panels

import (
	"context"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// storage shows how full each mount is. It has no settings, so it declares no
// schema.
type storage struct {
	dash.State[[]tideui.StorageMount]
	fetch func(context.Context) ([]tideui.StorageMount, error)
}

// Storage builds the storage panel.
func Storage() dash.Panel { return &storage{fetch: provider.Storage()} }

func (s *storage) Meta() dash.Meta {
	return dash.Meta{
		ID: "storage", Title: "Storage",
		Role: tideui.RoleSecondary, Priority: 55,
		MinWidth: 18, MinHeight: 6, HideBelow: 80,
		Interval: 2 * time.Minute,
		// Progress bars, not sparklines.
		Gauge: true,
	}
}

func (s *storage) Refresh(ctx context.Context) error {
	mounts, err := s.fetch(ctx)
	if err != nil {
		return err
	}
	s.Store(mounts)
	return nil
}

func (s *storage) View(ctx tideui.PanelContext) string {
	return ctx.Renderer.RenderStorage(s.Load(), ctx.Width)
}

// Demo synthesises a few mounts, so the dashboard has bars before /proc is
// sampled.
func (s *storage) Demo(now time.Time) {
	s.Store([]tideui.StorageMount{
		{Path: "/", UsedPercent: 72, Used: "460 GB", Total: "640 GB"},
		{Path: "/home", UsedPercent: 48, Used: "384 GB", Total: "800 GB"},
		{Path: "/media", UsedPercent: 81, Used: "5.8 TB", Total: "7.2 TB"},
	})
}

func (s *storage) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
		Run: func() string { return "sampling storage…" },
	}}
}
