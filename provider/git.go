package provider

import (
	"context"
	"fmt"
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
			name := filepath.Base(repo)
			activity := tideui.RepoActivity{Name: name, Tone: tideui.ToneGood}
			if inside, err := gitOutput(ctx, repo, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(inside) != "true" {
				activity.Summary = "not a repo"
				activity.Tone = tideui.ToneMuted
				activities = append(activities, activity)
				continue
			}
			if branch, err := gitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
				activity.Branch = strings.TrimSpace(branch)
			}
			if count, err := gitOutput(ctx, repo, "rev-list", "--count", "--since=midnight", "HEAD"); err == nil {
				activity.Commits, _ = strconv.Atoi(strings.TrimSpace(count))
			}
			changes := 0
			if status, err := gitOutput(ctx, repo, "status", "--porcelain"); err == nil {
				for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
					if strings.TrimSpace(line) != "" {
						changes++
					}
				}
			}
			switch {
			case activity.Commits > 0:
				activity.Summary = fmt.Sprintf("%d commits today", activity.Commits)
				activity.Tone = tideui.ToneAccent
			case changes > 0:
				activity.Summary = fmt.Sprintf("%d changes", changes)
				activity.Tone = tideui.ToneWarning
			default:
				activity.Summary = "clean"
			}
			activities = append(activities, activity)
		}
		return activities, nil
	}
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	full := append([]string{"-C", repo}, args...)
	command := exec.CommandContext(ctx, "git", full...)
	output, err := command.Output()
	return string(output), err
}
