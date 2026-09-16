package provider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
)

// newTestRepo builds a real repository with an origin it can track, which is
// the only honest way to exercise ahead/behind.
func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	run(t, "", "git", "init", "--quiet", "--bare", origin)
	run(t, "", "git", "init", "--quiet", "-b", "main", work)
	run(t, work, "git", "config", "user.email", "test@example.com")
	run(t, work, "git", "config", "user.name", "Test")
	run(t, work, "git", "remote", "add", "origin", origin)
	writeFile(t, work, "README.md", "hello\n")
	run(t, work, "git", "add", ".")
	// Dated yesterday so the fixture starts with no commits "today", letting
	// the clean and commits-today cases be told apart.
	runAt(t, work, yesterday(), "git", "commit", "--quiet", "-m", "first")
	run(t, work, "git", "push", "--quiet", "-u", "origin", "main")
	return work
}

// yesterday is a git-parsable timestamp before midnight, so a fixture's
// initial commit does not count as "today".
func yesterday() string {
	return time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	runAt(t, dir, "", name, args...)
}

// runAt runs a git command with an optional commit date, so a fixture can have
// history that is not all from today.
func runAt(t *testing.T, dir, when, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if when != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func firstRepo(t *testing.T, path string) tideui.RepoActivity {
	t.Helper()
	items, err := Git(path)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one repo, got %#v", items)
	}
	return items[0]
}

func TestGitCleanRepo(t *testing.T) {
	repo := newTestRepo(t)
	activity := firstRepo(t, repo)
	if activity.Summary != "clean" {
		t.Fatalf("summary = %q, want clean", activity.Summary)
	}
	if !activity.Clean() || activity.NoUpstream {
		t.Fatalf("expected a clean tracked repo, got %+v", activity)
	}
	if activity.Branch != "main" {
		t.Fatalf("branch = %q", activity.Branch)
	}
}

// The defect this replaces: a repository whose commits were never pushed was
// reported as "clean", because only the working tree was inspected.
func TestGitReportsUnpushedCommits(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "--quiet", "-m", "second")

	activity := firstRepo(t, repo)
	if activity.Ahead != 1 {
		t.Fatalf("ahead = %d, want 1", activity.Ahead)
	}
	if activity.Clean() {
		t.Fatal("a repo with unpushed commits is not clean")
	}
	if activity.Summary == "clean" {
		t.Fatalf("summary = %q, must not claim clean with unpushed work", activity.Summary)
	}
	if activity.Tone != tideui.ToneWarning {
		t.Fatalf("tone = %v, want a warning", activity.Tone)
	}
}

// The other defect: committing today used to mask an uncommitted tree, because
// the summary was a switch rather than a list.
func TestGitReportsCommitsAndChangesTogether(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "--quiet", "-m", "today")
	run(t, repo, "git", "push", "--quiet")
	writeFile(t, repo, "b.txt", "uncommitted\n")

	activity := firstRepo(t, repo)
	if activity.Commits == 0 {
		t.Fatal("expected a commit today")
	}
	if activity.Changes != 1 {
		t.Fatalf("changes = %d, want 1", activity.Changes)
	}
	if activity.Summary != "1 changed" {
		t.Fatalf("summary = %q, want the uncommitted file reported", activity.Summary)
	}
}

func TestGitCountsStashes(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, repo, "README.md", "changed\n")
	run(t, repo, "git", "stash", "--quiet")

	activity := firstRepo(t, repo)
	if activity.Stashes != 1 {
		t.Fatalf("stashes = %d, want 1", activity.Stashes)
	}
	if activity.Clean() {
		t.Fatal("a stash is outstanding work")
	}
}

// No tracking branch is a distinct state: the work exists nowhere else, so
// calling it "clean" would be the same lie as ignoring unpushed commits.
func TestGitReportsMissingUpstream(t *testing.T) {
	repo := newTestRepo(t)
	run(t, repo, "git", "checkout", "--quiet", "-b", "detached-work")

	activity := firstRepo(t, repo)
	if !activity.NoUpstream {
		t.Fatal("a branch with no upstream should be marked")
	}
	if activity.Summary != "no upstream" {
		t.Fatalf("summary = %q, want no upstream", activity.Summary)
	}
}

func TestGitReportsInterruptedMerge(t *testing.T) {
	repo := newTestRepo(t)
	// Two branches that touch the same line, merged without committing.
	run(t, repo, "git", "checkout", "--quiet", "-b", "side")
	writeFile(t, repo, "README.md", "side\n")
	run(t, repo, "git", "commit", "--quiet", "-am", "side")
	run(t, repo, "git", "checkout", "--quiet", "main")
	writeFile(t, repo, "README.md", "main\n")
	run(t, repo, "git", "commit", "--quiet", "-am", "main")
	_ = exec.Command("git", "-C", repo, "merge", "side").Run() // expected to conflict

	activity := firstRepo(t, repo)
	if activity.State != "merge" {
		t.Fatalf("state = %q, want merge", activity.State)
	}
	if activity.Tone != tideui.ToneDanger {
		t.Fatalf("tone = %v, want danger for an interrupted merge", activity.Tone)
	}
}

func TestGitNonRepoIsReportedNotFatal(t *testing.T) {
	items, err := Git(t.TempDir())(context.Background())
	if err != nil {
		t.Fatalf("a non-repo must not fail the fetch: %v", err)
	}
	if len(items) != 1 || items[0].Summary != "not a repo" {
		t.Fatalf("items = %#v", items)
	}
}

// A clone URL is a common thing to paste into a repository list, and "not a
// repo" does not explain why it failed or what to do about it.
func TestGitReportsRemoteURLsDistinctly(t *testing.T) {
	urls := []string{
		"https://github.com/allisonhere/tidemail",
		"http://example.com/x.git",
		"git@github.com:allisonhere/tideui.git",
		"ssh://git@example.com/x",
	}
	items, err := Git(urls...)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i, activity := range items {
		if !strings.Contains(activity.Summary, "clone") {
			t.Fatalf("%s summarized as %q, want it to say it needs cloning", urls[i], activity.Summary)
		}
		if activity.Clean() {
			t.Fatalf("%s reported clean; an unreadable entry must not look good", urls[i])
		}
		if activity.Name == "" {
			t.Fatalf("%s produced no name", urls[i])
		}
	}
	// The name comes off the end of the URL, without the .git suffix.
	if items[2].Name != "tideui" {
		t.Fatalf("scp-style URL named %q, want tideui", items[2].Name)
	}
	// A missing local path is still reported as a missing path, not a URL.
	missing, err := Git(t.TempDir() + "/nope")(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if missing[0].Summary != "not a repo" {
		t.Fatalf("missing path summarized as %q", missing[0].Summary)
	}
}

func TestIsRemoteURL(t *testing.T) {
	for _, remote := range []string{
		"https://github.com/a/b", "http://x/y", "ssh://git@h/p", "git://h/p",
		"git@github.com:a/b.git", "user@host:path",
	} {
		if !IsRemoteURL(remote) {
			t.Fatalf("%q should be recognised as remote", remote)
		}
	}
	for _, local := range []string{
		"/home/allie/Projects/tidedeck", "~/Projects/x", "relative/path", ".",
		"/mnt/c/repo", "", "/path/with@sign/but/no/colon",
	} {
		if IsRemoteURL(local) {
			t.Fatalf("%q should be treated as a local path", local)
		}
	}
}
