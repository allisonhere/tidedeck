package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var enter = tea.KeyMsg{Type: tea.KeyEnter}

// A lookup that is running cannot be started again: a second enter used to
// queue a second request, and nothing on screen said the first was under way.
func TestLookupCannotFireTwice(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Weather")
	form.state.place = "Berlin"
	moveTo(t, form, "Look up coordinates")
	form.Update(enter)
	if q := form.TakeLookup(); q != "Berlin" {
		t.Fatalf("first press queued %q", q)
	}
	form.Update(enter)
	if q := form.TakeLookup(); q != "" {
		t.Fatalf("a second press queued another lookup: %q", q)
	}
	page := rendered(t, form)
	if strings.Contains(page, "[ Look up coordinates ]") || !strings.Contains(page, "Look up coordinates…") {
		t.Fatalf("the running lookup is not drawn as running:\n%s", page)
	}
	form.LookupDone()
	form.Update(enter)
	if q := form.TakeLookup(); q != "Berlin" {
		t.Fatalf("after it finished, a press queued %q", q)
	}
}

// One plugin operation at a time: they all write the plugins directory. The
// row that started it draws the spinner, and the guard outlives settings
// being closed, because the model ends it whether settings is open or not.
func TestPluginOperationsRunOneAtATime(t *testing.T) {
	form := weatherForm(t)
	openCategory(t, form, "Plugins")
	form.state.pluginSource = "/tmp/some-plugin"
	moveTo(t, form, "Install")
	form.Update(enter)
	if op, ok := form.TakePluginOp(); !ok || op.kind != "install" {
		t.Fatalf("first press queued %+v %v", op, ok)
	}
	form.Update(enter)
	if op, ok := form.TakePluginOp(); ok {
		t.Fatalf("a second install was queued while the first ran: %+v", op)
	}
	if page := rendered(t, form); !strings.Contains(page, "Install…") {
		t.Fatalf("the running install is not drawn as running:\n%s", page)
	}
	if !form.Working() {
		t.Fatal("the model is not told work is running, so the spinner would not animate")
	}
	form.PluginOpDone()
	if form.Working() || strings.Contains(rendered(t, form), "Install…") {
		t.Fatal("the install still shows as running after it finished")
	}
}
