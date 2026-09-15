package provider

import (
	"bufio"
	"context"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// TodoTxt builds a task source from a todo.txt file.
func TodoTxt(path string) func(context.Context) ([]tideui.Task, error) {
	return func(context.Context) ([]tideui.Task, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		now := time.Now()
		var parsed []parsedTask
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if task, ok := parseTodo(scanner.Text(), now); ok {
				parsed = append(parsed, task)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		sort.SliceStable(parsed, func(i, j int) bool {
			if parsed[i].task.Done != parsed[j].task.Done {
				return !parsed[i].task.Done
			}
			return parsed[i].dueRank < parsed[j].dueRank
		})
		tasks := make([]tideui.Task, len(parsed))
		for i, item := range parsed {
			tasks[i] = item.task
		}
		return tasks, nil
	}
}

type parsedTask struct {
	task    tideui.Task
	dueRank int
}

func parseTodo(line string, now time.Time) (parsedTask, bool) {
	text := strings.TrimSpace(line)
	if text == "" || strings.HasPrefix(text, "//") {
		return parsedTask{}, false
	}
	out := parsedTask{task: tideui.Task{Tone: tideui.ToneMuted}, dueRank: 1 << 30}
	if strings.HasPrefix(text, "x ") {
		out.task.Done = true
		text = strings.TrimSpace(text[2:])
		if fields := strings.Fields(text); len(fields) > 0 {
			if _, ok := parseDate(fields[0]); ok {
				text = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
			}
		}
	}
	var kept []string
	for _, field := range strings.Fields(text) {
		switch {
		case len(field) == 3 && strings.HasPrefix(field, "(") && strings.HasSuffix(field, ")"):
			out.task.Tone = priorityTone(field[1])
		case strings.HasPrefix(field, "+"), strings.HasPrefix(field, "@"):
			out.task.Tags = append(out.task.Tags, field)
		case strings.HasPrefix(field, "due:"):
			if due, ok := parseDate(strings.TrimPrefix(field, "due:")); ok {
				out.task.Due = dueLabel(due, now)
				out.dueRank = daysUntil(now, due)
				if !out.task.Done && out.dueRank <= 0 {
					out.task.Tone = tideui.ToneDanger
				}
			}
		default:
			kept = append(kept, field)
		}
	}
	out.task.Title = strings.Join(kept, " ")
	if out.task.Title == "" {
		return parsedTask{}, false
	}
	return out, true
}

func priorityTone(letter byte) tideui.Tone {
	switch letter {
	case 'A', 'a':
		return tideui.ToneWarning
	case 'B', 'b':
		return tideui.ToneAccent
	default:
		return tideui.ToneNeutral
	}
}

func parseDate(value string) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func dueLabel(due, now time.Time) string {
	days := daysUntil(now, due)
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	case -1:
		return "yesterday"
	}
	if days < 0 {
		return due.Format("Jan 2")
	}
	if days < 7 {
		return due.Format("Mon")
	}
	return due.Format("Jan 2")
}

func daysUntil(now, due time.Time) int {
	ny, nm, nd := now.Date()
	dy, dm, dd := due.Date()
	a := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
	b := time.Date(dy, dm, dd, 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}
