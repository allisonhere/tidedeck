package panels

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// interfaceKey is the configuration key this panel owns: the interface to
// sample, or empty for every non-loopback interface summed.
const interfaceKey = "interface"

// network shows throughput and a short-term graph for one interface or all of
// them.
type network struct {
	dash.State[tideui.NetworkMetrics]

	mu    sync.Mutex
	iface string
	fetch func(context.Context) (tideui.NetworkMetrics, error)
	// newFetcher is the constructor, so a test can count rebuilds without
	// reaching the network.
	newFetcher func(string) func(context.Context) (tideui.NetworkMetrics, error)
}

// Network builds the network panel. The rolling history lives in the provider's
// closure, so the panel starts it once here and only rebuilds it when the
// interface actually changes.
func Network() dash.Panel {
	n := &network{newFetcher: provider.Network}
	n.fetch = n.newFetcher("")
	return n
}

func (n *network) Meta() dash.Meta {
	return dash.Meta{
		ID: "network", Title: "Network",
		Role: tideui.RoleSecondary, Priority: 70,
		MinWidth: 18, MinHeight: 7, HideBelow: 104,
		Interval: time.Second,
	}
}

func (n *network) Schema() []dash.Field {
	return []dash.Field{{
		Key: interfaceKey, Label: "interface", Kind: dash.FieldText,
	}}
}

// Configure rebuilds the source only when the interface changed, so reapplying
// settings does not discard the rolling throughput history.
func (n *network) Configure(values dash.Values) error {
	iface := strings.TrimSpace(values.String(interfaceKey))
	n.mu.Lock()
	defer n.mu.Unlock()
	if iface == n.iface && n.fetch != nil {
		return nil
	}
	build := n.newFetcher
	if build == nil {
		build = provider.Network
	}
	n.iface, n.fetch = iface, build(iface)
	return nil
}

func (n *network) Refresh(ctx context.Context) error {
	n.mu.Lock()
	fetch := n.fetch
	n.mu.Unlock()
	if fetch == nil {
		return nil
	}
	metrics, err := fetch(ctx)
	if err != nil {
		return err
	}
	n.Store(metrics)
	return nil
}

func (n *network) View(ctx tideui.PanelContext) string {
	metrics := n.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderNetworkDetail(metrics, ctx.Width)
	}
	return ctx.Renderer.RenderNetwork(metrics, ctx.Width)
}

// Badge advertises the link as up; the widget itself carries the throughput.
func (n *network) Badge() (string, tideui.Tone) { return "up", tideui.ToneGood }

// Demo synthesises plausible throughput, so the dashboard has a graph before
// live data is turned on.
func (n *network) Demo(now time.Time) {
	t := float64(now.UnixNano()) / float64(time.Second)
	n.Store(tideui.NetworkMetrics{
		Interface: "wlan0",
		Download:  clamp(87+35*wave(t, 8, 0), 0, 950),
		Upload:    clamp(14+9*wave(t, 6, 1), 0, 400),
		Unit:      "Mbps",
		DownSpark: series(t, 24, 5, 0, 0.4, 0.45),
		UpSpark:   series(t, 24, 7, 1, 0.25, 0.3),
		LAN:       "940 Mbps",
		WAN:       "87↓ / 14↑ Mbps",
	})
}

func (n *network) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh", Refresh: true,
		Run: func() string { return "sampling network…" },
	}}
}
