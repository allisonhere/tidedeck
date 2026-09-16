package panels

import (
	"context"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// notesKey is the configuration key this panel owns: the note file paths.
const notesKey = "notes"

// notes shows a few Markdown or text files side by side.
type notes struct {
	dash.State[[]tideui.Note]
	fetch func(context.Context) ([]tideui.Note, error)
}

// Notes builds the notes panel.
func Notes() dash.Panel { return &notes{} }

func (n *notes) Meta() dash.Meta {
	return dash.Meta{
		ID: "notes", Title: "Notes",
		Role: tideui.RoleOptional, Priority: 45,
		MinWidth: 18, MinHeight: 5, HideBelow: 130,
		Interval: 30 * time.Second,
	}
}

func (n *notes) Schema() []dash.Field {
	return []dash.Field{{
		Key: notesKey, Label: "paths", Kind: dash.FieldText,
	}}
}

// Configure builds the source from the configured paths, expanding a leading ~
// the way a shell would.
func (n *notes) Configure(values dash.Values) error {
	paths := values.List(notesKey)
	for i := range paths {
		paths[i] = expandHome(paths[i])
	}
	if len(paths) == 0 {
		n.fetch = nil
		return nil
	}
	n.fetch = provider.Notes(paths...)
	return nil
}

func (n *notes) Refresh(ctx context.Context) error {
	if n.fetch == nil {
		return nil
	}
	notes, err := n.fetch(ctx)
	if err != nil {
		return err
	}
	n.Store(notes)
	return nil
}

func (n *notes) View(ctx tideui.PanelContext) string {
	return ctx.Renderer.RenderNotes(n.Load(), ctx.Width)
}

// Demo synthesises a couple of notes, one pinned, so the panel looks inhabited
// before any path is configured.
func (n *notes) Demo(now time.Time) {
	n.Store([]tideui.Note{
		{Title: "Remember", Pinned: true, Body: "- test narrow layouts\n- record demo GIF\n- add real data providers"},
		{Title: "Ideas", Body: "- weather provider\n- calendar sync\n- market watchlist"},
	})
}

func (n *notes) Actions() []dash.Action {
	return []dash.Action{{
		ID: "edit", Key: "e", Label: "edit",
		Run: func() string { return "editing note" },
	}}
}
