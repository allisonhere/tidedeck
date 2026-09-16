// Package panels holds the built-in dashboard panels. Each file is one
// panel: what it fetches, how it draws itself, and what it lets you
// configure. Adding a panel means adding a file here and registering it.
package panels

import (
	"context"
	"math"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// gpu shows utilisation, memory, temperature, power and clock for whichever
// card the kernel is accounting for.
type gpu struct {
	dash.State[tideui.GPUMetrics]
	fetch func(context.Context) (tideui.GPUMetrics, error)
}

// GPU builds the GPU panel. The rolling utilisation history lives in the
// provider's closure, so each panel instance keeps its own.
func GPU() dash.Panel { return &gpu{fetch: provider.GPU()} }

func (g *gpu) Meta() dash.Meta {
	return dash.Meta{
		ID: "gpu", Title: "GPU",
		Role: tideui.RoleSecondary, Priority: 72,
		MinWidth: 18, MinHeight: 6, HideBelow: 104,
		Interval: time.Second,
	}
}

func (g *gpu) Refresh(ctx context.Context) error {
	metrics, err := g.fetch(ctx)
	if err != nil {
		return err
	}
	g.Store(metrics)
	return nil
}

func (g *gpu) View(ctx tideui.PanelContext) string {
	metrics := g.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderGPUDetail(metrics, ctx.Width)
	}
	return ctx.Renderer.RenderGPU(metrics, ctx.Width)
}

// Demo synthesises a plausible discrete card, so the dashboard has something
// to show before live data is turned on.
func (g *gpu) Demo(now time.Time) {
	t := float64(now.UnixNano()) / float64(time.Second)
	busy := clamp(24+18*wave(t, 11, 1), 2, 99)
	spark := make([]float64, 18)
	for i := range spark {
		spark[i] = clamp(wave(t-float64(len(spark)-i), 6, 1)*0.45+0.3, 0, 1)
	}
	g.Store(tideui.GPUMetrics{
		Name: "amdgpu", BusyPercent: busy, BusySpark: spark,
		MemoryUsed: "5.4 GB", MemoryTotal: "8.0 GB", MemoryLabel: "VRAM", MemoryFrac: 0.67,
		TemperatureC: int(clamp(58+7*wave(t, 30, 2), 40, 92)),
		PowerWatts:   clamp(42+16*wave(t, 13, 3), 8, 140),
		ClockMHz:     int(clamp(1800+300*wave(t, 8, 4), 300, 2600)),
	})
}

func (g *gpu) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
		Run: func() string { return "sampling gpu…" },
	}}
}

// wave is a smooth repeating signal, so demo data drifts instead of jumping.
func wave(t, period, phase float64) float64 {
	return math.Sin(2*math.Pi*(t/period) + phase)
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
