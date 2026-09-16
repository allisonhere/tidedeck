package panels

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	"github.com/allisonhere/tideui/provider"
)

// reposKey is the configuration key this panel owns: the repository paths.
const reposKey = "repos"

// git shows the activity of each configured working copy: today's commits,
// unpushed work, a dirty tree, and a half-finished rebase or merge.
type git struct {
	dash.State[[]tideui.RepoActivity]

	mu    sync.Mutex
	fetch func(context.Context) ([]tideui.RepoActivity, error)
	// summarySource and summaryText cache the repositories row's summary:
	// checking the paths touches the filesystem, and the row renders on every
	// keystroke.
	summarySource string
	summaryText   string
}

// Git builds the git activity panel.
func Git() dash.Panel { return &git{} }

func (g *git) Meta() dash.Meta {
	return dash.Meta{
		ID: "git", Title: "Git Activity",
		Role: tideui.RoleOptional, Priority: 40,
		MinWidth: 18, MinHeight: 5, HideBelow: 80,
		Interval: time.Minute,
	}
}

func (g *git) Schema() []dash.Field {
	return []dash.Field{{
		Key: reposKey, Label: "repositories", Kind: dash.FieldText,
		Normalize: normalizeRepoList,
		Summary:   g.repoSummary,
	}}
}

// Configure rebuilds the source from the configured working copies. With none
// there is nothing to read, so the panel fetches nothing.
func (g *git) Configure(values dash.Values) error {
	repos := parseRepoList(values.String(reposKey))
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(repos) == 0 {
		g.fetch = nil
		return nil
	}
	paths := make([]string, 0, len(repos))
	for _, repo := range repos {
		paths = append(paths, repo.Full)
	}
	g.fetch = provider.Git(paths...)
	return nil
}

func (g *git) Refresh(ctx context.Context) error {
	g.mu.Lock()
	fetch := g.fetch
	g.mu.Unlock()
	if fetch == nil {
		return nil
	}
	activities, err := fetch(ctx)
	if err != nil {
		return err
	}
	g.Store(activities)
	return nil
}

func (g *git) View(ctx tideui.PanelContext) string {
	activities := g.Load()
	if ctx.Zoomed {
		return ctx.Renderer.RenderRepoActivityDetail(activities, ctx.Width)
	}
	return ctx.Renderer.RenderRepoActivity(activities, ctx.Width)
}

// Demo synthesises a few repositories, so the dashboard shows the shape of the
// panel before any paths are configured.
func (g *git) Demo(now time.Time) {
	g.Store([]tideui.RepoActivity{
		{Name: "tideui", Branch: "main", Summary: "3 today", Commits: 3, Tone: tideui.ToneAccent},
		{Name: "tidegit", Branch: "main", Summary: "clean", Tone: tideui.ToneGood},
		{Name: "tidemail", Branch: "feat/rules", Summary: "2 unpushed · 2 changed",
			Commits: 2, Ahead: 2, Changes: 2, Tone: tideui.ToneWarning},
		{Name: "z13control", Branch: "main", Summary: "6 unpushed", Ahead: 6, Tone: tideui.ToneWarning},
	})
}

// Actions offers a refresh that actually refetches, which the hand-written
// registration could not do: it only printed a message claiming it had.
func (g *git) Actions() []dash.Action {
	return []dash.Action{{
		ID: "fetch", Key: "r", Label: "fetch", Refresh: true,
		Run: func() string { return "fetched all remotes" },
	}}
}

// repoSummary describes the configured repositories in the space a row has:
// how many there are, and how many of them are not actually repositories. The
// full value is still what gets edited; showing it raw truncated mid-path and
// pushed the field's own label off the row.
func (g *git) repoSummary(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none set"
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.summarySource != value || g.summaryText == "" {
		repos := parseRepoList(value)
		missing, remote := 0, 0
		for _, repo := range repos {
			switch {
			case repo.Remote:
				remote++
			case !repo.IsRepo:
				missing++
			}
		}
		text := fmt.Sprintf("%d %s", len(repos), plural(len(repos), "repo", "repos"))
		if missing > 0 {
			text += fmt.Sprintf(" · %d not found", missing)
		}
		// A clone URL is a different mistake from a wrong path, and saying
		// which one it is saves a round of guessing.
		if remote > 0 {
			text += fmt.Sprintf(" · %d not cloned", remote)
		}
		g.summarySource, g.summaryText = value, text
	}
	return g.summaryText
}

// repoPath is one configured repository and what is actually at that path.
type repoPath struct {
	Display string // "~/Projects/tideui", as it should be stored and shown
	Full    string // the expanded absolute path
	Exists  bool
	IsRepo  bool
	Remote  bool // a clone URL rather than a working copy
}

// parseRepoList reads the configured repository paths, expanding ~, dropping
// blanks, and dropping duplicates that differ only in spelling. Order is
// preserved so the list stays the one the user typed.
func parseRepoList(value string) []repoPath {
	var repos []repoPath
	seen := make(map[string]bool)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// A remote URL is not a filesystem path: filepath.Clean would collapse
		// the "//" in "https://" and silently corrupt what was typed.
		if provider.IsRemoteURL(part) {
			if seen[part] {
				continue
			}
			seen[part] = true
			repos = append(repos, repoPath{Display: part, Full: part, Remote: true})
			continue
		}
		full := filepath.Clean(expandHome(part))
		if seen[full] {
			continue
		}
		seen[full] = true
		entry := repoPath{Display: collapseHome(full), Full: full}
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			entry.Exists = true
			if _, err := os.Stat(filepath.Join(full, ".git")); err == nil {
				entry.IsRepo = true
			}
		}
		repos = append(repos, entry)
	}
	return repos
}

// normalizeRepoList rewrites the stored value: deduplicated, trimmed, and with
// the home directory written as ~ so the field stays readable.
func normalizeRepoList(value string) string {
	repos := parseRepoList(value)
	paths := make([]string, 0, len(repos))
	for _, repo := range repos {
		paths = append(paths, repo.Display)
	}
	return strings.Join(paths, ", ")
}

// expandHome resolves a leading ~ so a config can name "~/Projects/x" the way
// a shell would.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// collapseHome is the inverse of expandHome, so a saved config shows ~ rather
// than a long absolute path.
func collapseHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// plural picks a word form for a count.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}
