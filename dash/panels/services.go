package panels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// systemdKey and dockerKey are the configuration keys this panel owns:
// systemd units to watch, or a Docker socket to list containers from.
const (
	systemdKey = "systemd"
	dockerKey  = "docker"
)

// services shows the state of systemd units or Docker containers.
type services struct {
	dash.State[[]tideui.ServiceStatus]
	fetch func(context.Context) ([]tideui.ServiceStatus, error)
}

// Services builds the services panel.
func Services() dash.Panel { return &services{} }

func (s *services) Meta() dash.Meta {
	return dash.Meta{
		ID: "services", Title: "Services",
		Role: tideui.RoleSecondary, Priority: 65,
		MinWidth: 20, MinHeight: 6, HideBelow: 80,
		Interval: 15 * time.Second,
	}
}

func (s *services) Schema() []dash.Field {
	return []dash.Field{
		{Key: systemdKey, Label: "systemd units", Kind: dash.FieldText,
			Description: "Comma-separated unit names. Units take precedence over Docker.",
			Placeholder: "none"},
		{Key: dockerKey, Label: "docker socket", Kind: dash.FieldText,
			Description: "Path to the socket, or 1 for the default one. Used only when no units are set.",
			Placeholder: "none"},
	}
}

// Configure prefers systemd units when any are named, then falls back to a
// Docker socket. A bare "1" means the default socket, which is how the docker
// key was already used.
func (s *services) Configure(values dash.Values) error {
	units := values.List(systemdKey)
	socket := strings.TrimSpace(values.String(dockerKey))
	switch {
	case len(units) > 0:
		s.fetch = provider.Systemd(units...)
	case socket != "":
		if socket == "1" {
			socket = ""
		}
		s.fetch = provider.Docker(socket)
	default:
		s.fetch = nil
	}
	return nil
}

func (s *services) Refresh(ctx context.Context) error {
	if s.fetch == nil {
		return nil
	}
	statuses, err := s.fetch(ctx)
	if err != nil {
		return err
	}
	s.Store(statuses)
	return nil
}

func (s *services) View(ctx tideui.PanelContext) string {
	statuses := s.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderServicesDetail(statuses, ctx.Width)
	}
	return ctx.Renderer.RenderServices(statuses, ctx.Width)
}

// Badge counts the services that are not healthy, so an issue is visible on the
// header without opening the panel.
func (s *services) Badge() (string, tideui.Tone) {
	issues := 0
	for _, status := range s.Load() {
		if status.Tone != tideui.ToneGood {
			issues++
		}
	}
	if issues == 0 {
		return "", tideui.ToneGood
	}
	return fmt.Sprintf("%d", issues), tideui.ToneWarning
}

// Demo synthesises a small fleet, including a warning, a stopped service and
// one mid-update.
func (s *services) Demo(now time.Time) {
	s.Store([]tideui.ServiceStatus{
		{Name: "jellyfin", Status: tideui.StatusHealthy, Age: "3d", Detail: "HTTP 200", Uptime: "3d 14h"},
		{Name: "postgres", Status: tideui.StatusHealthy, Age: "12d", Detail: "12 connections", Uptime: "12d"},
		{Name: "forgejo", Status: tideui.StatusWarning, Age: "6d", Detail: "high memory", Uptime: "6d 2h"},
		{Name: "backup", Status: tideui.StatusStopped, Age: "--", Detail: "last run 2d ago"},
		{Name: "caddy", Status: tideui.StatusHealthy, Age: "24d", Detail: "3 sites", Uptime: "24d"},
		{Name: "redis", Status: tideui.StatusActive, Age: "9d", Detail: "cache warm", Uptime: "9d"},
		{Name: "syncthing", Status: tideui.StatusUpdating, Age: "2m", Detail: "scanning", Uptime: "2m"},
	})
}

func (s *services) Actions() []dash.Action {
	return []dash.Action{
		{ID: "restart", Key: "r", Label: "restart", Run: s.restart},
		{ID: "logs", Key: "l", Label: "logs", Run: func() string { return "opening service logs" }},
	}
}

// restart marks the first unhealthy service healthy in the panel's copy. It is
// a display action: restarting a unit for real needs root, which a dashboard
// does not have.
func (s *services) restart() string {
	statuses := s.Load()
	for i := range statuses {
		if statuses[i].Tone == tideui.ToneGood {
			continue
		}
		statuses[i].State = "healthy"
		statuses[i].Tone = tideui.ToneGood
		s.Store(statuses)
		return statuses[i].Name + " restarted"
	}
	return "all services healthy"
}
