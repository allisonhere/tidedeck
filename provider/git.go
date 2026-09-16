package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/allisonhere/tideui"
)

// Git builds a repository-activity source by shelling out to git for each
// configured working copy. Missing repos or git are reported per-entry rather
// than failing the whole dashboard.
func Git(repos ...string) func(context.Context) ([]tideui.RepoActivity, error) {
	cloned := append([]string(nil), repos...)
	return func(ctx context.Context) ([]tideui.RepoActivity, error) {
		activities := make([]tideui.RepoActivity, 0, len(cloned))
		for _, repo := range cloned {
			activities = append(activities, readRepo(ctx, repo))
		}
		return activities, nil
	}
}

// IsRemoteURL reports whether a configured repository is a remote address
// rather than a working copy on disk: an https/git/ssh URL, or scp-style
// "git@host:path". Git reads local working copies, so a remote address needs
// cloning first; saying so is more use than reporting it as "not a repo".
func IsRemoteURL(repo string) bool {
	repo = strings.TrimSpace(repo)
	if strings.Contains(repo, "://") {
		return true
	}
	at := strings.Index(repo, "@")
	colon := strings.Index(repo, ":")
	return at > 0 && colon > at+1
}

// readRepo collects one repository's state.
func readRepo(ctx context.Context, repo string) tideui.RepoActivity {
	activity := tideui.RepoActivity{Name: repoName(repo), Tone: tideui.ToneMuted}
	if IsRemoteURL(repo) {
		activity.Summary = "remote URL — clone it first"
		return activity
	}
	if inside, err := gitOutput(ctx, repo, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(inside) != "true" {
		activity.Summary = "not a repo"
		return activity
	}
	activity.Tone = tideui.ToneGood
	if branch, err := gitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		activity.Branch = strings.TrimSpace(branch)
	}
	if count, err := gitOutput(ctx, repo, "rev-list", "--count", "--since=midnight", "HEAD"); err == nil {
		activity.Commits, _ = strconv.Atoi(strings.TrimSpace(count))
	}
	if status, err := gitOutput(ctx, repo, "status", "--porcelain"); err == nil {
		activity.Changes = countLines(status)
	}
	if stash, err := gitOutput(ctx, repo, "stash", "list"); err == nil {
		activity.Stashes = countLines(stash)
	}
	activity.Ahead, activity.Behind, activity.NoUpstream = readTracking(ctx, repo)
	activity.State = readRepoState(ctx, repo)
	activity.Summary, activity.Tone = summarizeRepo(activity)
	return activity
}

// readTracking counts commits ahead of and behind the tracking branch. A
// repository with no upstream is reported as such: it is a distinct state from
// being up to date, and reporting it as "clean" would hide work that exists
// nowhere else.
func readTracking(ctx context.Context, repo string) (ahead, behind int, noUpstream bool) {
	counts, err := gitOutput(ctx, repo, "rev-list", "--count", "--left-right", "@{upstream}...HEAD")
	if err != nil {
		return 0, 0, true
	}
	fields := strings.Fields(counts)
	if len(fields) != 2 {
		return 0, 0, true
	}
	behind, _ = strconv.Atoi(fields[0])
	ahead, _ = strconv.Atoi(fields[1])
	return ahead, behind, false
}

// readRepoState reports a half-finished rebase or merge, which is worth saying
// loudly: it is easy to walk away from one and forget.
func readRepoState(ctx context.Context, repo string) string {
	dir, err := gitOutput(ctx, repo, "rev-parse", "--git-dir")
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(dir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	for _, marker := range []struct{ path, state string }{
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
		{"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
	} {
		if _, err := os.Stat(filepath.Join(gitDir, marker.path)); err == nil {
			return marker.state
		}
	}
	return ""
}

// summarizeRepo composes the one-line summary. Every outstanding thing is
// listed rather than only the first: a repository can have commits today and
// an unpushed branch and a dirty tree at the same time, and reporting only one
// of them hides the rest.
func summarizeRepo(a tideui.RepoActivity) (string, tideui.Tone) {
	var parts []string
	if a.State != "" {
		parts = append(parts, strings.ToUpper(a.State))
	}
	// A branch that tracks nothing is outstanding in its own right: git cannot
	// tell you whether that work exists anywhere else, so it must not fall
	// through to "clean" or be masked by today's commit count.
	if a.NoUpstream && a.Branch != "" {
		parts = append(parts, "no upstream")
	}
	if a.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed", a.Ahead))
	}
	if a.Behind > 0 {
		parts = append(parts, fmt.Sprintf("%d behind", a.Behind))
	}
	if a.Changes > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", a.Changes))
	}
	if a.Stashes > 0 {
		parts = append(parts, fmt.Sprintf("%d stashed", a.Stashes))
	}
	if len(parts) == 0 {
		if a.Commits > 0 {
			return fmt.Sprintf("%d today", a.Commits), tideui.ToneAccent
		}
		return "clean", tideui.ToneGood
	}
	return strings.Join(parts, " · "), repoTone(a)
}

// repoTone ranks what is outstanding: an interrupted operation first, then
// work that exists only locally, then everything else.
func repoTone(a tideui.RepoActivity) tideui.Tone {
	switch {
	case a.State != "":
		return tideui.ToneDanger
	case a.Ahead > 0:
		return tideui.ToneWarning
	case a.Changes > 0:
		return tideui.ToneWarning
	default:
		return tideui.ToneMuted
	}
}

// repoName labels an entry: the directory name for a working copy, and the
// last path segment for a remote URL, with any .git suffix dropped.
func repoName(repo string) string {
	repo = strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(repo), "/"), ".git")
	if index := strings.LastIndexAny(repo, "/:"); index >= 0 && index+1 < len(repo) {
		return repo[index+1:]
	}
	return filepath.Base(repo)
}

func countLines(output string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	full := append([]string{"-C", repo}, args...)
	command := exec.CommandContext(ctx, "git", full...)
	output, err := command.Output()
	return string(output), err
}
