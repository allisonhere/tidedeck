package provider

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Docker builds a service-status source backed by the Docker Engine API over
// its Unix socket. No third-party client is needed.
func Docker(socket string) func(context.Context) ([]tideui.ServiceStatus, error) {
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	client := &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
	}
	return func(ctx context.Context) ([]tideui.ServiceStatus, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=true", nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		var containers []dockerContainer
		if err := json.NewDecoder(response.Body).Decode(&containers); err != nil {
			return nil, err
		}
		services := make([]tideui.ServiceStatus, 0, len(containers))
		for _, container := range containers {
			name := container.name()
			service := tideui.ServiceStatus{Name: name, Detail: container.Status}
			service.Status, service.State = dockerStatus(container.State, container.Status)
			if uptime := dockerUptime(container.Status); uptime != "" {
				service.Uptime = uptime
				service.Age = uptime
			}
			services = append(services, service)
		}
		return services, nil
	}
}

type dockerContainer struct {
	Names  []string `json:"Names"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

func (c dockerContainer) name() string {
	if len(c.Names) == 0 {
		return "container"
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

func dockerStatus(state, status string) (tideui.StatusKind, string) {
	switch state {
	case "running":
		switch {
		case strings.Contains(status, "(unhealthy)"):
			return tideui.StatusWarning, "unhealthy"
		case strings.Contains(status, "(health: starting)"):
			return tideui.StatusUpdating, "starting"
		case strings.Contains(status, "(healthy)"):
			return tideui.StatusHealthy, "healthy"
		default:
			return tideui.StatusActive, "running"
		}
	case "restarting":
		return tideui.StatusUpdating, "restarting"
	case "paused":
		return tideui.StatusStale, "paused"
	case "exited", "dead":
		return tideui.StatusStopped, "stopped"
	default:
		return tideui.StatusStale, state
	}
}

// dockerUptime extracts "Up 3 days" / "Up 5 minutes" from the status string.
func dockerUptime(status string) string {
	if !strings.HasPrefix(status, "Up ") {
		return ""
	}
	fields := strings.Fields(status)
	if len(fields) < 3 {
		return ""
	}
	value := fields[1]
	unit := strings.TrimSuffix(fields[2], "s")
	switch unit {
	case "second":
		return value + "s"
	case "minute":
		return value + "m"
	case "hour":
		return value + "h"
	case "day":
		return value + "d"
	case "week":
		return value + "w"
	case "month":
		return value + "mo"
	default:
		return value
	}
}
