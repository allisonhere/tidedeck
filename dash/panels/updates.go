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

// aurHelperKey is the configuration key this panel owns. It is the key that
// is already in config.json, so migrating the panel does not rewrite anyone's
// file.
const (
	aurHelperKey     = "aur_helper"
	defaultAURHelper = "yay"
)

// updates shows the Omarchy version and how many packages are out of date. It
// only ever asks what is available: applying updates needs root, takes a
// filesystem snapshot and can reboot, so it is never run from a dashboard.
type updates struct {
	dash.State[tideui.UpdateStatus]
	helper string
	fetch  func(context.Context) (tideui.UpdateStatus, error)
}

// Updates builds the updates panel.
func Updates() dash.Panel {
	panel := &updates{helper: defaultAURHelper}
	panel.fetch = provider.Updates(panel.helper)
	return panel
}

func (u *updates) Meta() dash.Meta {
	return dash.Meta{
		ID: "updates", Title: "Updates", Subtitle: "system",
		Role: tideui.RoleOptional, Priority: 50,
		MinWidth: 20, MinHeight: 5, HideBelow: 120,
		// checkupdates syncs a temporary package database over the network,
		// so this is deliberately slow-polled.
		Interval: 30 * time.Minute,
	}
}

func (u *updates) Schema() []dash.Field {
	return []dash.Field{{
		Key:     aurHelperKey,
		Label:   "aur helper",
		Kind:    dash.FieldText,
		Default: defaultAURHelper,
	}}
}

// Configure rebuilds the fetcher only when the helper actually changed, so
// reapplying settings does not discard a good reading for no reason.
func (u *updates) Configure(values dash.Values) error {
	helper := strings.TrimSpace(values.String(aurHelperKey))
	if helper == "" {
		helper = defaultAURHelper
	}
	if helper == u.helper && u.fetch != nil {
		return nil
	}
	u.helper, u.fetch = helper, provider.Updates(helper)
	return nil
}

func (u *updates) Refresh(ctx context.Context) error {
	status, err := u.fetch(ctx)
	if err != nil {
		return err
	}
	u.Store(status)
	return nil
}

func (u *updates) View(ctx tideui.PanelContext) string {
	status := u.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderUpdatesDetail(status, ctx.Width)
	}
	return ctx.Renderer.RenderUpdates(status, ctx.Width)
}

// Badge shows the pending count, distinguishing "nothing to do" from "the
// check could not run" - the two must not look alike.
func (u *updates) Badge() (string, tideui.Tone) {
	status := u.Load()
	switch pending := status.Pending(); {
	case status.Unavailable != "":
		return "?", tideui.ToneMuted
	case pending > 0:
		return fmt.Sprintf("%d", pending), tideui.ToneWarning
	default:
		return "ok", tideui.ToneGood
	}
}

func (u *updates) Demo(now time.Time) {
	u.Store(tideui.UpdateStatus{
		Omarchy:        "4.0.3-1",
		OmarchyPending: "4.0.4-1",
		Repo: []tideui.UpdatePackage{
			{Name: "omarchy", From: "4.0.3-1", To: "4.0.4-1"},
			{Name: "omarchy-settings", From: "4.0.3-1", To: "4.0.4-1"},
			{Name: "linux", From: "6.17.2.arch1-1", To: "6.17.4.arch1-1"},
		},
		AUR:     []tideui.UpdatePackage{{Name: "yay", From: "12.4.2-1", To: "12.5.0-1"}},
		Checked: now,
	})
}

func (u *updates) Actions() []dash.Action {
	return []dash.Action{{
		ID: "refresh", Key: "r", Label: "refresh",
		Run: func() string { return "checked for updates" },
	}}
}
