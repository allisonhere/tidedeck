package panels

import (
	"context"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// system shows local CPU, memory, temperature, load and uptime. It is the
// smallest panel on the registry: no settings, so it declares no schema and
// the settings screen gives its page only the visibility and metric styles.
type system struct {
	dash.State[tideui.SystemMetrics]
	fetch func(context.Context) (tideui.SystemMetrics, error)
	// started anchors the demo uptime, which would otherwise count from the
	// epoch once the demo switched to a wall-clock time base.
	started time.Time
}

// System builds the system panel.
func System() dash.Panel {
	return &system{fetch: provider.System(), started: time.Now()}
}

func (s *system) Meta() dash.Meta {
	return dash.Meta{
		ID: "system", Title: "System",
		Role: tideui.RolePrimary, Priority: 95,
		MinWidth: 20, MinHeight: 7,
		Interval: time.Second,
		Gauge:    true,
		Spark:    true,
	}
}

func (s *system) Refresh(ctx context.Context) error {
	metrics, err := s.fetch(ctx)
	if err != nil {
		return err
	}
	s.Store(metrics)
	return nil
}

func (s *system) View(ctx tideui.PanelContext) string {
	metrics := s.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderSystemDetail(metrics, ctx.Width)
	}
	return ctx.Renderer.RenderSystem(metrics, ctx.Width)
}

// Badge advertises the panel as healthy; the widget itself carries the detail.
func (s *system) Badge() (string, tideui.Tone) {
	return "healthy", tideui.ToneGood
}

// Demo synthesises a plausible machine, so the dashboard has system health
// before the /proc and /sys readers are worth sampling.
func (s *system) Demo(now time.Time) {
	t := now.Sub(s.started).Seconds()
	cpu := clamp(18+10*wave(t, 9, 0)+4*wave(t, 3, 1), 3, 98)
	mem := clamp(41+6*wave(t, 60, 2), 18, 94)
	cores := make([]float64, 8)
	for i := range cores {
		cores[i] = clamp(cpu+9*wave(t, 5, float64(i)), 0, 100)
	}
	s.Store(tideui.SystemMetrics{
		CPUPercent:    cpu,
		CPUSpark:      series(t, 18, 7, 0, 0.35, 0.4),
		Cores:         cores,
		MemoryPercent: mem,
		MemoryUsed:    "13.1 GB",
		MemoryTotal:   "32 GB",
		TemperatureC:  54 + int(3*wave(t, 180, 0)),
		Load: [3]float64{
			1.4 + 0.5*wave(t, 30, 0),
			1.1 + 0.4*wave(t, 30, 1),
			0.9 + 0.3*wave(t, 30, 2),
		},
		Uptime:    3*24*time.Hour + 14*time.Hour + time.Duration(t)*time.Second,
		Processes: 312 + int(4*wave(t, 45, 1)),
	})
}

func (s *system) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
		Run: func() string { return "sampling system…" },
	}}
}
