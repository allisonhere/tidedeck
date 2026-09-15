package provider

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Systemd builds a service-status source by querying systemd units. It returns
// the previous state as unavailable rather than failing the whole dashboard
// when systemctl is missing.
func Systemd(units ...string) func(context.Context) ([]tideui.ServiceStatus, error) {
	cloned := append([]string(nil), units...)
	return func(ctx context.Context) ([]tideui.ServiceStatus, error) {
		services := make([]tideui.ServiceStatus, 0, len(cloned))
		for _, unit := range cloned {
			props, err := systemctlShow(ctx, unit)
			if err != nil {
				services = append(services, tideui.ServiceStatus{Name: unit, Status: tideui.StatusStale, Detail: "systemctl unavailable"})
				continue
			}
			active := props["ActiveState"]
			sub := props["SubState"]
			service := tideui.ServiceStatus{
				Name:   unit,
				Status: statusForSystemd(active, sub),
				Detail: strings.TrimSpace(active + "/" + sub),
			}
			if start, ok := parseSystemdTime(props["ExecMainStartTimestamp"]); ok {
				since := time.Since(start)
				service.Age = humanAge(since)
				service.Uptime = humanAge(since)
			}
			if restarts := props["NRestarts"]; restarts != "" && restarts != "0" {
				service.Detail += "  ·  " + restarts + " restarts"
			}
			services = append(services, service)
		}
		return services, nil
	}
}

func systemctlShow(ctx context.Context, unit string) (map[string]string, error) {
	command := exec.CommandContext(ctx, "systemctl", "show",
		"-p", "ActiveState", "-p", "SubState",
		"-p", "ExecMainStartTimestamp", "-p", "NRestarts", "--", unit)
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	properties := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[key] = value
		}
	}
	return properties, nil
}

func statusForSystemd(active, sub string) tideui.StatusKind {
	switch active {
	case "active":
		if sub == "running" {
			return tideui.StatusHealthy
		}
		return tideui.StatusActive
	case "failed":
		return tideui.StatusError
	case "inactive":
		return tideui.StatusStopped
	case "activating", "reloading", "deactivating":
		return tideui.StatusUpdating
	default:
		return tideui.StatusStale
	}
}

// parseSystemdTime handles the timestamp format systemd emits, e.g.
// "Mon 2026-09-14 09:00:00 UTC".
func parseSystemdTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 MST"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
