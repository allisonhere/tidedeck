package provider

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Update tooling, held in variables so tests can point at stub scripts.
var (
	checkupdatesBin = "checkupdates"
	omarchyBin      = "omarchy"
	aurHelperBin    = "yay"
)

// Exit codes that mean "nothing to update" rather than "something went wrong".
// checkupdates(8) documents 2; omarchy-update-available and yay -Qua both use
// 1. Treating these as errors would be worse than cosmetic: Fetcher keeps the
// previous value whenever a fetch errors, so the panel would freeze on the
// last non-zero count and never report a clean system.
const (
	checkupdatesNone = 2
	omarchyNone      = 1
	aurNone          = 1
)

// Updates builds a pending-update source: the Omarchy version and whether a
// newer one is waiting, plus repository and AUR package counts.
//
// It only ever asks what is available. Applying updates needs root, takes a
// filesystem snapshot and may reboot, so it is never run from here.
func Updates(aurHelper string) func(context.Context) (tideui.UpdateStatus, error) {
	helper := strings.TrimSpace(aurHelper)
	if helper == "" {
		helper = aurHelperBin
	}
	return func(ctx context.Context) (tideui.UpdateStatus, error) {
		status := tideui.UpdateStatus{Checked: time.Now()}
		var unavailable []string

		status.Omarchy = firstOutputLine(commandOutput(ctx, omarchyBin, "version"))
		if out, state := runUpdateCheck(ctx, omarchyNone, omarchyBin, "update", "available"); state == updateFailed {
			unavailable = append(unavailable, "omarchy")
		} else if state == updateFound {
			// "omarchy 4.0.3-1 -> 4.0.4-1"; a dev checkout reports differently
			// and is left to the package line below.
			for _, pkg := range parseUpdateLines(out) {
				if strings.HasPrefix(pkg.Name, "omarchy") {
					status.OmarchyPending = pkg.To
					break
				}
			}
		}

		switch out, state := runUpdateCheck(ctx, checkupdatesNone, checkupdatesBin); state {
		case updateFound:
			status.Repo = parseUpdateLines(out)
		case updateFailed:
			unavailable = append(unavailable, "checkupdates")
		}

		switch out, state := runUpdateCheck(ctx, aurNone, helper, "-Qua"); state {
		case updateFound:
			status.AUR = parseUpdateLines(out)
		case updateFailed:
			unavailable = append(unavailable, helper)
		}

		if len(unavailable) > 0 {
			status.Unavailable = strings.Join(unavailable, ", ") + " unavailable"
		}
		return status, nil
	}
}

// updateState distinguishes the three outcomes of an update check, because
// "nothing to update" and "the check did not run" must not look alike.
type updateState int

const (
	updateFound updateState = iota
	updateNone
	updateFailed
)

// runUpdateCheck runs a check tool, mapping its "nothing to update" exit code
// to updateNone. Anything else that fails is reported so the panel can say the
// count is unknown rather than quietly showing zero.
func runUpdateCheck(ctx context.Context, noneCode int, name string, args ...string) (string, updateState) {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	text := strings.TrimSpace(string(output))
	var exit *exec.ExitError
	switch {
	case err == nil:
		if text == "" {
			return "", updateNone
		}
		return text, updateFound
	case errors.As(err, &exit) && exit.ExitCode() == noneCode:
		return "", updateNone
	default:
		return "", updateFailed
	}
}

// commandOutput runs a command and returns its trimmed output, or "" if it
// could not run. Used where a missing value is not worth reporting.
func commandOutput(ctx context.Context, name string, args ...string) string {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// firstOutputLine returns the first line of command output. It is separate
// from the Linux-only firstLine because this file builds everywhere.
func firstOutputLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return strings.TrimSpace(text[:index])
	}
	return strings.TrimSpace(text)
}

// parseUpdateLines reads "name old -> new" lines, the shared format of
// checkupdates, yay -Qua and omarchy update available.
func parseUpdateLines(output string) []tideui.UpdatePackage {
	var packages []tideui.UpdatePackage
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := tideui.UpdatePackage{Name: fields[0]}
		for i, field := range fields {
			if field == "->" && i > 0 && i+1 < len(fields) {
				pkg.From, pkg.To = fields[i-1], fields[i+1]
				break
			}
		}
		if pkg.To == "" {
			// Not a version line (e.g. a dev checkout's "3 new commits").
			continue
		}
		packages = append(packages, pkg)
	}
	return packages
}
