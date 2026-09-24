package main

import (
	"strings"
	"testing"
)

// / finds a setting on any page and Enter goes to it: the page opens with the
// row under the cursor.
func TestSearchGoesToTheSetting(t *testing.T) {
	pickerHome(t)
	form := pathForm(t)
	if !strings.Contains(form.hintBar(), "/ search") {
		t.Fatalf("the hint bar does not offer search: %q", form.hintBar())
	}
	press(form, "/", "todo")
	if form.search == nil || len(form.search.matches) == 0 {
		t.Fatal("nothing found for todo")
	}
	if first := form.search.matches[0].title; first != "Tasks › todo.txt" {
		t.Fatalf("first match %q", first)
	}
	if page := rendered(t, form); !strings.Contains(page, "Tasks › todo.txt") {
		t.Fatalf("the match is not drawn:\n%s", page)
	}
	press(form, "enter")
	if form.search != nil || form.view != viewFields {
		t.Fatal("enter did not close the search and open the page")
	}
	if f := form.currentField(); f == nil || f.label != "todo.txt" || form.categories[form.category].name != "Tasks" {
		t.Fatalf("landed on %+v", f)
	}
}

// A label that starts with the query ranks above one that only contains it,
// and both above a match found only in a description or a page name.
func TestSearchRanksNamesFirst(t *testing.T) {
	pickerHome(t)
	form := pathForm(t)
	press(form, "/", "repo")
	if len(form.search.matches) == 0 || !strings.HasSuffix(form.search.matches[0].title, "› repositories") {
		t.Fatalf("matches %+v", form.search.matches)
	}
}

// Esc leaves everything where it was.
func TestSearchCancels(t *testing.T) {
	pickerHome(t)
	form := pathForm(t)
	category, view := form.category, form.view
	press(form, "/", "zzz-nothing")
	if len(form.search.matches) != 0 || !strings.Contains(rendered(t, form), "nothing matches") {
		t.Fatal("an unmatched query does not say so")
	}
	press(form, "esc")
	if form.search != nil || form.category != category || form.view != view {
		t.Fatal("esc moved the form")
	}
}
