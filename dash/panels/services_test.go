package panels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func TestServicesPanelMeta(t *testing.T) {
	meta := Services().Meta()
	if meta.ID != "services" || meta.Title != "Services" {
		t.Fatalf("meta = %#v", meta)
	}
	if meta.MinWidth != 20 || meta.MinHeight != 6 || meta.HideBelow != 80 || meta.Priority != 65 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != 15*time.Second {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

// Systemd units win over a Docker socket, and neither means nothing to watch.
func TestServicesConfigurePicksSource(t *testing.T) {
	panel := Services().(*services)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("no source configured should build no fetcher")
	}
	docker := dash.NewValues()
	docker.Set(dockerKey, "1")
	if err := panel.Configure(docker); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a docker socket should build a fetcher")
	}
	units := dash.NewValues()
	units.Set(systemdKey, "sshd.service,nginx.service")
	if err := panel.Configure(units); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("systemd units should build a fetcher")
	}
}

func TestServicesPanelRendersItsOwnData(t *testing.T) {
	panel := Services().(*services)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "services", Width: 34, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"jellyfin", "forgejo", "backup"} {
		if !strings.Contains(body, want) {
			t.Fatalf("services body missing %q:\n%s", want, body)
		}
	}
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "services", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

func TestServicesBadgeAndRestart(t *testing.T) {
	panel := Services().(*services)
	panel.Demo(time.Unix(1700000000, 0))
	before, tone := panel.Badge()
	if before == "" || tone != tideui.ToneWarning {
		t.Fatalf("badge = %q %v, want a warning count", before, tone)
	}
	// Restarting the first unhealthy service clears it from the count.
	if message := panel.restart(); !strings.Contains(message, "restarted") {
		t.Fatalf("restart = %q", message)
	}
	after, _ := panel.Badge()
	if after == before {
		t.Fatalf("restart did not change the badge: %q", after)
	}
}

func TestServicesKeepsLastGoodReading(t *testing.T) {
	good := []tideui.ServiceStatus{{Name: "caddy", Status: tideui.StatusHealthy}}
	calls := 0
	panel := &services{}
	panel.fetch = func(context.Context) ([]tideui.ServiceStatus, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return nil, context.DeadlineExceeded
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if got := panel.Load(); len(got) != 1 || got[0].Name != "caddy" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

func TestServicesThroughTheDeck(t *testing.T) {
	deck := dash.New()
	deck.Register(Services())
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("services")
	if !ok {
		t.Fatal("services was not attached to the workspace")
	}
	keys := map[string]bool{}
	for _, action := range registered.ActionList() {
		keys[action.Key] = true
	}
	for _, want := range []string{"r", "l"} {
		if !keys[want] {
			t.Fatalf("missing %q action; got %v", want, keys)
		}
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "services", Width: 30, Renderer: renderer()}))
	if !strings.Contains(body, "jellyfin") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}
	if badge, ok := deck.Badges()["services"]; !ok || badge.Text == "" {
		t.Fatalf("services badge = %#v, want a count", badge)
	}
}
