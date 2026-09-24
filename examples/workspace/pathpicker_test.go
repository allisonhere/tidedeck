package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/dash/panels"
	tea "github.com/charmbracelet/bubbletea"
)

// pickerHome makes a home folder with notes and code in it.
func pickerHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	for _, d := range []string{"notes", "code/tide", "code/tidesms", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(h, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"notes/todo.txt", "notes/ideas.md"} {
		if err := os.WriteFile(filepath.Join(h, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func pathForm(t *testing.T) *settingsForm {
	t.Helper()
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panels.Tasks(), panels.Git())
	deck.Attach(ws)
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)
	return form
}

func press(form *settingsForm, keys ...string) {
	for _, k := range keys {
		switch k {
		case "enter":
			form.Update(tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			form.Update(tea.KeyMsg{Type: tea.KeyEsc})
		case "tab":
			form.Update(tea.KeyMsg{Type: tea.KeyTab})
		case "down":
			form.Update(tea.KeyMsg{Type: tea.KeyDown})
		case "backspace":
			form.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		default:
			form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

// o on a file setting browses for it: folders open with Enter, typing filters,
// and choosing a file fills the setting in.
func TestPickAFile(t *testing.T) {
	h := pickerHome(t)
	form := pathForm(t)
	openCategory(t, form, "Tasks")
	moveTo(t, form, "todo.txt")
	if !strings.Contains(form.hintBar(), "o browse") {
		t.Fatalf("a path row does not offer browsing: %q", form.hintBar())
	}
	press(form, "o")
	if form.picker == nil {
		t.Fatal("o did not open the picker")
	}
	page := rendered(t, form)
	if !strings.Contains(page, "notes/") || strings.Contains(page, ".hidden") {
		t.Fatalf("home is not listed, or hidden folders are:\n%s", page)
	}
	press(form, "no", "enter") // filter to notes/, open it
	press(form, "t", "enter")  // filter to todo.txt, choose it
	if form.picker != nil {
		t.Fatal("choosing a file did not close the picker")
	}
	if got := *form.currentField().text; got != "~/notes/todo.txt" {
		t.Fatalf("todo.txt = %q", got)
	}
	if !form.dirty {
		t.Error("a pick did not mark settings unsaved")
	}
	_ = h
}

// A folder setting lists folders only and offers the one being browsed; on a
// list it is added to what is there, not put in its place.
func TestPickAFolderIntoAList(t *testing.T) {
	pickerHome(t)
	form := pathForm(t)
	openCategory(t, form, "Git Activity")
	moveTo(t, form, "repositories")
	*form.currentField().text = "/srv/old"
	press(form, "o", "~/code/") // a leading ~ starts afresh from where it opened
	if page := rendered(t, form); !strings.Contains(page, "use this folder") || !strings.Contains(page, "tidesms/") {
		t.Fatalf("a folder setting does not offer folders:\n%s", page)
	}
	press(form, "tides", "tab") // complete into code/tidesms/
	press(form, "enter")        // use this folder
	if got := *form.currentField().text; got != "/srv/old, ~/code/tidesms" {
		t.Fatalf("repositories = %q", got)
	}
}

// Esc leaves the setting as it was; backspace climbs back out of a folder.
func TestPickerCancelAndBackspace(t *testing.T) {
	pickerHome(t)
	form := pathForm(t)
	openCategory(t, form, "Tasks")
	moveTo(t, form, "todo.txt")
	before := *form.currentField().text
	press(form, "o", "notes/")
	if !strings.Contains(rendered(t, form), "ideas.md") {
		t.Fatal("typing a folder did not list it")
	}
	press(form, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	if form.picker.query != "~/" {
		t.Fatalf("backspace left %q", form.picker.query)
	}
	press(form, "esc")
	if form.picker != nil || *form.currentField().text != before || form.dirty {
		t.Fatal("esc changed the setting")
	}
}
