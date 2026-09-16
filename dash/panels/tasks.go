package panels

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// todoKey is the configuration key this panel owns: the todo.txt path.
const todoKey = "todo"

// tasks shows the open items in a todo.txt file.
type tasks struct {
	dash.State[[]tideui.Task]
	fetch func(context.Context) ([]tideui.Task, error)
}

// Tasks builds the task panel.
func Tasks() dash.Panel { return &tasks{} }

func (t *tasks) Meta() dash.Meta {
	return dash.Meta{
		ID: "tasks", Title: "Tasks",
		Role: tideui.RoleSecondary, Priority: 72,
		MinWidth: 20, MinHeight: 6, HideBelow: 80,
		Interval: 30 * time.Second,
	}
}

func (t *tasks) Schema() []dash.Field {
	return []dash.Field{{
		Key: todoKey, Label: "todo.txt", Kind: dash.FieldText,
	}}
}

// Configure builds the source from the configured path. A leading ~ is expanded
// the way a shell would.
func (t *tasks) Configure(values dash.Values) error {
	path := expandHome(strings.TrimSpace(values.String(todoKey)))
	if path == "" {
		t.fetch = nil
		return nil
	}
	t.fetch = provider.TodoTxt(path)
	return nil
}

func (t *tasks) Refresh(ctx context.Context) error {
	if t.fetch == nil {
		return nil
	}
	tasks, err := t.fetch(ctx)
	if err != nil {
		return err
	}
	t.Store(tasks)
	return nil
}

func (t *tasks) View(ctx tideui.PanelContext) string {
	return ctx.Renderer.RenderTasks(t.Load(), ctx.Width)
}

// Badge counts the open items, so outstanding work is visible on the header.
func (t *tasks) Badge() (string, tideui.Tone) {
	open := 0
	for _, task := range t.Load() {
		if !task.Done {
			open++
		}
	}
	if open == 0 {
		return "", tideui.ToneMuted
	}
	return strconv.Itoa(open), tideui.ToneMuted
}

// Demo synthesises a short list. The last item is already done, so the panel
// shows both states.
func (t *tasks) Demo(now time.Time) {
	t.Store([]tideui.Task{
		{Title: "Finish TideDeck", Due: "today", Tone: tideui.ToneWarning, Tags: []string{"#tide"}},
		{Title: "Update docs", Due: "tomorrow", Tone: tideui.ToneAccent, Tags: []string{"#docs"}},
		{Title: "Fix panel spacing", Done: true},
		{Title: "Record demo GIF", Due: "Thu", Tone: tideui.ToneAccent},
	})
}

func (t *tasks) Actions() []dash.Action {
	return []dash.Action{
		{ID: "toggle", Key: "space", Label: "toggle", Run: t.toggle},
		{ID: "add", Key: "a", Label: "add", Run: t.add},
		{ID: "edit", Key: "e", Label: "edit", Run: func() string { return "editing task" }},
	}
}

// toggle completes the first open task in the panel's copy, so the badge drops
// without waiting for the file to be rewritten.
func (t *tasks) toggle() string {
	tasks := t.Load()
	for i := range tasks {
		if tasks[i].Done {
			continue
		}
		tasks[i].Done = true
		t.Store(tasks)
		return "completed: " + tasks[i].Title
	}
	return "no open tasks"
}

func (t *tasks) add() string {
	tasks := append(t.Load(), tideui.Task{Title: "New task", Due: "today", Tone: tideui.ToneAccent})
	t.Store(tasks)
	return "task added"
}
