package provider

import (
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

func TestParseTodo(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	task, ok := parseTodo("(A) Finish TideDeck +tide @desk due:2026-09-14", now)
	if !ok {
		t.Fatal("task not parsed")
	}
	if task.task.Title != "Finish TideDeck" {
		t.Fatalf("title = %q", task.task.Title)
	}
	if task.task.Due != "today" {
		t.Fatalf("due = %q, want today", task.task.Due)
	}
	if task.task.Tone != tideui.ToneDanger {
		t.Fatalf("overdue priority tone = %v, want danger", task.task.Tone)
	}
	if len(task.task.Tags) != 2 {
		t.Fatalf("tags = %v", task.task.Tags)
	}

	future, _ := parseTodo("Update docs due:2026-09-16", now)
	if future.task.Due != "Wed" {
		t.Fatalf("future due = %q, want Wed", future.task.Due)
	}

	done, ok := parseTodo("x 2026-09-13 Fix panel spacing", now)
	if !ok || !done.task.Done {
		t.Fatalf("done task = %+v ok=%v", done.task, ok)
	}
	if done.task.Title != "Fix panel spacing" {
		t.Fatalf("done title = %q", done.task.Title)
	}

	if _, ok := parseTodo("   ", now); ok {
		t.Fatal("blank line parsed as a task")
	}
}
