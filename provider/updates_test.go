package provider

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// stubBin writes an executable that prints body and exits with code, then
// points name at it for the duration of the test.
func stubBin(t *testing.T, target *string, name, body string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub scripts are POSIX shell")
	}
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\n"
	if body != "" {
		script += "cat <<'EOF'\n" + body + "\nEOF\n"
	}
	script += "exit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	original := *target
	*target = path
	t.Cleanup(func() { *target = original })
}

func TestUpdatesParsesPendingPackages(t *testing.T) {
	stubBin(t, &omarchyBin, "omarchy", "omarchy 4.0.3-1 -> 4.0.4-1", 0)
	stubBin(t, &checkupdatesBin, "checkupdates", "omarchy 4.0.3-1 -> 4.0.4-1\nlinux 6.17.2-1 -> 6.17.4-1", 0)
	stubBin(t, &aurHelperBin, "yay", "yay 12.4.2-1 -> 12.5.0-1", 0)

	status, err := Updates("")(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Repo) != 2 {
		t.Fatalf("repo updates = %#v, want 2", status.Repo)
	}
	if status.Repo[1].Name != "linux" || status.Repo[1].To != "6.17.4-1" {
		t.Fatalf("second repo update = %#v", status.Repo[1])
	}
	if len(status.AUR) != 1 || status.AUR[0].Name != "yay" {
		t.Fatalf("aur updates = %#v", status.AUR)
	}
	if status.OmarchyPending != "4.0.4-1" {
		t.Fatalf("omarchy pending = %q, want 4.0.4-1", status.OmarchyPending)
	}
	if status.Pending() != 3 {
		t.Fatalf("pending = %d, want 3", status.Pending())
	}
	if status.Unavailable != "" {
		t.Fatalf("unexpected unavailable: %q", status.Unavailable)
	}
}

// "Nothing to update" is reported through a nonzero exit by every one of these
// tools. Treating it as an error would be silent and durable: Fetcher keeps the
// previous value whenever a fetch fails, so the panel would sit on a stale
// count and never report a clean system.
func TestUpdatesTreatsNoUpdatesAsSuccess(t *testing.T) {
	stubBin(t, &omarchyBin, "omarchy", "Omarchy is up to date", omarchyNone)
	stubBin(t, &checkupdatesBin, "checkupdates", "", checkupdatesNone)
	stubBin(t, &aurHelperBin, "yay", "", aurNone)

	status, err := Updates("")(context.Background())
	if err != nil {
		t.Fatalf("no-updates must not be an error: %v", err)
	}
	if status.Pending() != 0 {
		t.Fatalf("pending = %d, want 0", status.Pending())
	}
	if status.OmarchyPending != "" {
		t.Fatalf("omarchy pending = %q, want empty", status.OmarchyPending)
	}
	if status.Unavailable != "" {
		t.Fatalf("a clean system is not unavailable: %q", status.Unavailable)
	}
}

// A tool that is missing or genuinely broken must say so, rather than being
// reported as zero updates, which would read as "nothing to do".
func TestUpdatesReportsUnavailableTools(t *testing.T) {
	stubBin(t, &omarchyBin, "omarchy", "Omarchy is up to date", omarchyNone)
	stubBin(t, &checkupdatesBin, "checkupdates", "database sync failed", 1)
	missing := filepath.Join(t.TempDir(), "definitely-not-installed")

	status, err := Updates(missing)(context.Background())
	if err != nil {
		t.Fatalf("a missing tool must not fail the fetch: %v", err)
	}
	if status.Unavailable == "" {
		t.Fatal("a failed check should be reported, not silently zero")
	}
	for _, want := range []string{"checkupdates", "definitely-not-installed"} {
		if !strings.Contains(status.Unavailable, want) {
			t.Fatalf("unavailable = %q, want it to mention %q", status.Unavailable, want)
		}
	}
	if status.Pending() != 0 {
		t.Fatalf("pending = %d, want 0", status.Pending())
	}
}

func TestParseUpdateLines(t *testing.T) {
	packages := parseUpdateLines("omarchy 4.0.3-1 -> 4.0.4-1\nomarchy-dev-checkout 3 new commits on origin/master\n\nlinux 6.17.2-1 -> 6.17.4-1")
	if len(packages) != 2 {
		t.Fatalf("packages = %#v, want 2 (non-version lines dropped)", packages)
	}
	if packages[0].From != "4.0.3-1" || packages[0].To != "4.0.4-1" {
		t.Fatalf("first package = %#v", packages[0])
	}
}
