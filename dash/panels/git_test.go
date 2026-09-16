package panels

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/charmbracelet/x/ansi"
)

func TestGitPanelMeta(t *testing.T) {
	meta := Git().Meta()
	if meta.ID != "git" || meta.Title != "Git Activity" {
		t.Fatalf("meta = %#v", meta)
	}
	// The panel keeps the sizing the hand-written registration used, so the
	// layout is unchanged by the migration.
	if meta.MinWidth != 18 || meta.MinHeight != 5 || meta.HideBelow != 80 || meta.Priority != 40 {
		t.Fatalf("sizing changed: %#v", meta)
	}
	if meta.Interval != time.Minute {
		t.Fatalf("interval = %v", meta.Interval)
	}
}

// A configured list builds a fetcher; with none there is nothing to read.
func TestGitConfigureBuildsSourceOrNot(t *testing.T) {
	panel := Git().(*git)
	if err := panel.Configure(dash.NewValues()); err != nil {
		t.Fatal(err)
	}
	if panel.fetch != nil {
		t.Fatal("an empty repository list should build no fetcher")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatalf("an unconfigured refresh should be a no-op, got %v", err)
	}
	values := dash.NewValues()
	values.Set(reposKey, t.TempDir())
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	if panel.fetch == nil {
		t.Fatal("a configured list should build a fetcher")
	}
}

func TestGitPanelRendersItsOwnData(t *testing.T) {
	panel := Git().(*git)
	panel.Demo(time.Unix(1700000000, 0))
	ctx := tideui.PanelContext{ID: "git", Width: 40, Renderer: renderer()}

	body := ansi.Strip(panel.View(ctx))
	for _, want := range []string{"tideui", "tidemail", "z13control"} {
		if !strings.Contains(body, want) {
			t.Fatalf("git body missing %q:\n%s", want, body)
		}
	}
	detail := ansi.Strip(panel.View(tideui.PanelContext{ID: "git", Width: 50, Zoomed: true, Renderer: renderer()}))
	if !strings.Contains(detail, "commit(s) today") || !strings.Contains(detail, "behind") {
		t.Fatalf("zoomed body missing the per-repo facts:\n%s", detail)
	}
	// Every line stays within the panel, at any width.
	for _, width := range []int{4, 12, 20, 40} {
		for _, line := range strings.Split(ansi.Strip(panel.View(tideui.PanelContext{
			ID: "git", Width: width, Renderer: renderer(),
		})), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d produced a %d-cell line: %q", width, got, line)
			}
		}
	}
}

// A failed refresh keeps the last reading on screen.
func TestGitKeepsLastGoodReading(t *testing.T) {
	good := []tideui.RepoActivity{{Name: "tideui", Branch: "main", Summary: "clean"}}
	calls := 0
	panel := &git{}
	panel.fetch = func(context.Context) ([]tideui.RepoActivity, error) {
		calls++
		if calls == 1 {
			return good, nil
		}
		return nil, errors.New("git: not found")
	}
	if err := panel.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Refresh(context.Background()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if got := panel.Load(); len(got) != 1 || got[0].Name != "tideui" {
		t.Fatalf("a failed refresh discarded good data: %#v", got)
	}
}

// The summary is what a one-line row shows, so it must count and describe the
// list rather than print it raw.
func TestGitRepoSummary(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	panel := Git().(*git)
	if got := panel.repoSummary(""); got != "none set" {
		t.Fatalf("empty summary = %q, want none set", got)
	}
	got := panel.repoSummary(real + "," + filepath.Join(real, "missing"))
	if !strings.Contains(got, "2 repos") || !strings.Contains(got, "1 not found") {
		t.Fatalf("summary = %q, want a count and the missing one", got)
	}
	if strings.Contains(got, real) {
		t.Fatalf("the row should summarize, not print the raw paths: %q", got)
	}
	if got := panel.repoSummary("https://github.com/allisonhere/tideui"); !strings.Contains(got, "1 not cloned") {
		t.Fatalf("remote summary = %q, want it reported as not cloned", got)
	}
}

func TestRepoListParsingAndNormalization(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir() // a directory, but not a repository

	repos := parseRepoList(fmt.Sprintf(" %s , %s ,, %s , %s/does-not-exist", real, plain, real, plain))
	// The duplicate is dropped and blanks ignored; order is preserved.
	if len(repos) != 3 {
		t.Fatalf("parsed %d repos, want 3 (duplicate and blank dropped): %#v", len(repos), repos)
	}
	if !repos[0].IsRepo {
		t.Fatalf("%s has a .git and should be a repo", repos[0].Full)
	}
	if repos[1].IsRepo || !repos[1].Exists {
		t.Fatalf("a plain directory exists but is not a repo: %#v", repos[1])
	}
	if repos[2].Exists {
		t.Fatalf("a missing path should not report as existing: %#v", repos[2])
	}

	// The home directory is written back as ~, so the stored value stays short.
	collapsed := normalizeRepoList(filepath.Join(home, "Projects", "tidedeck"))
	if collapsed != "~/Projects/tidedeck" {
		t.Fatalf("normalizeRepoList = %q, want ~/Projects/tidedeck", collapsed)
	}
	// ~ and the absolute spelling are the same repository, not two.
	both := normalizeRepoList(filepath.Join(home, "x") + ",~/x")
	if both != "~/x" {
		t.Fatalf("normalizeRepoList = %q, want the duplicate collapsed", both)
	}
	if got := normalizeRepoList("  ,  "); got != "" {
		t.Fatalf("normalizeRepoList of blanks = %q, want empty", got)
	}
}

// Normalization used to run filepath.Clean over every entry, which collapsed
// the "//" in an https URL and silently rewrote what the user typed.
func TestRepoListKeepsRemoteURLsIntact(t *testing.T) {
	const url = "https://github.com/allisonhere/tidemail"
	if got := normalizeRepoList(url); got != url {
		t.Fatalf("normalizeRepoList(%q) = %q, want it unchanged", url, got)
	}
	repos := parseRepoList(url + ",git@github.com:allisonhere/tideui.git")
	if len(repos) != 2 {
		t.Fatalf("parsed %#v, want both URLs kept", repos)
	}
	for _, repo := range repos {
		if !repo.Remote {
			t.Fatalf("%q should be marked remote", repo.Display)
		}
		if repo.IsRepo {
			t.Fatalf("%q is not a working copy", repo.Display)
		}
	}
	// A URL and a local path are both kept, and the local one still resolves.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	mixed := normalizeRepoList(url + "," + filepath.Join(home, "x"))
	if !strings.Contains(mixed, url) || !strings.Contains(mixed, "~/x") {
		t.Fatalf("mixed list = %q, want the URL intact and the path collapsed", mixed)
	}
}

// The deck must be able to drive the panel, and the refresh key must actually
// refetch rather than only claiming it did.
func TestGitThroughTheDeck(t *testing.T) {
	deck := dash.New()
	panel := Git().(*git)
	deck.Register(panel)
	ws := tideui.NewWorkspace()
	deck.Attach(ws)

	registered, ok := ws.Lookup("git")
	if !ok {
		t.Fatal("git was not attached to the workspace")
	}
	if registered.TitleText() != "Git Activity" {
		t.Fatalf("title = %q", registered.TitleText())
	}
	deck.Tick(time.Unix(1700000000, 0))
	body := ansi.Strip(registered.Render(tideui.PanelContext{ID: "git", Width: 40, Renderer: renderer()}))
	if !strings.Contains(body, "tideui") {
		t.Fatalf("deck tick did not populate the panel:\n%s", body)
	}

	good := []tideui.RepoActivity{{Name: "tideui", Branch: "main", Summary: "clean"}}
	panel.fetch = func(context.Context) ([]tideui.RepoActivity, error) { return good, nil }
	values := dash.NewValues()
	values.Set(reposKey, t.TempDir())
	if err := panel.Configure(values); err != nil {
		t.Fatal(err)
	}
	deck.SetMode(dash.ModeLive)
	now := time.Unix(1700000000, 0)
	deck.Refresh(context.Background(), now)
	if got := panel.Load(); len(got) != 1 {
		t.Fatalf("first refresh = %#v", got)
	}
	panel.Store(nil)
	deck.Refresh(context.Background(), now) // inside the interval: skipped
	if len(panel.Load()) != 0 {
		t.Fatal("a panel inside its interval should not have refetched")
	}
	action, ok := actionByKey(registered, "r")
	if !ok {
		t.Fatal("no fetch action")
	}
	action.Handler(ws)
	deck.Refresh(context.Background(), now)
	if got := panel.Load(); len(got) != 1 {
		t.Fatalf("the fetch key did not make the panel due again: %#v", got)
	}
}
