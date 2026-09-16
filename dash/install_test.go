package dash

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// pluginDir writes a plugin directory with a valid manifest and returns it.
func pluginDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeManifestInto(t, dir, validManifest())
	return dir
}

func writeManifestInto(t *testing.T, dir string, doc map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitRepo builds a real committed repository holding a plugin.
func gitRepo(t *testing.T, doc map[string]any) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	writeManifestInto(t, dir, doc)
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-qm", "init"},
	} {
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return dir
}

func TestInstallFromLocalDirectory(t *testing.T) {
	plugins := t.TempDir()
	source := pluginDir(t)

	manifest, err := Install(plugins, source)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "author.thing" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if _, err := os.Stat(filepath.Join(plugins, "author.thing", ManifestFile)); err != nil {
		t.Fatalf("plugin not installed where expected: %v", err)
	}
	// No staging directories are left behind.
	entries, _ := os.ReadDir(plugins)
	for _, entry := range entries {
		if entry.Name()[0] == '.' {
			t.Fatalf("staging directory left behind: %s", entry.Name())
		}
	}
}

func TestInstallFromGitURL(t *testing.T) {
	plugins := t.TempDir()
	repo := gitRepo(t, validManifest())

	manifest, err := Install(plugins, "file://"+repo)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "author.thing" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestInstallRejectsABadSource(t *testing.T) {
	plugins := t.TempDir()
	if _, err := Install(plugins, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing directory should not install")
	}
	// A directory without a manifest is not a plugin, and leaves nothing behind.
	if _, err := Install(plugins, t.TempDir()); err == nil {
		t.Fatal("a directory without a manifest should not install")
	}
	entries, _ := os.ReadDir(plugins)
	if len(entries) != 0 {
		t.Fatalf("a failed install left files behind: %v", entries)
	}
}

func TestInstallRefusesADuplicate(t *testing.T) {
	plugins := t.TempDir()
	source := pluginDir(t)
	if _, err := Install(plugins, source); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(plugins, source); err == nil {
		t.Fatal("installing the same plugin twice should fail")
	}
}

func TestInstalledListsAndReportsProblems(t *testing.T) {
	plugins := t.TempDir()
	if _, err := Install(plugins, pluginDir(t)); err != nil {
		t.Fatal(err)
	}
	// A broken manifest is reported rather than dropped.
	broken := filepath.Join(plugins, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, ManifestFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	installed := Installed(plugins)
	if len(installed) != 2 {
		t.Fatalf("installed = %d, want 2: %#v", len(installed), installed)
	}
	if installed[0].Problem != nil || installed[0].Manifest.ID != "author.thing" {
		t.Fatalf("valid plugin = %#v", installed[0])
	}
	if installed[1].Problem == nil {
		t.Fatalf("broken plugin should report a problem: %#v", installed[1])
	}
}

func TestRemoveAndUpdate(t *testing.T) {
	plugins := t.TempDir()
	repo := gitRepo(t, validManifest())
	if _, err := Install(plugins, "file://"+repo); err != nil {
		t.Fatal(err)
	}

	// Update pulls a new commit and returns its manifest.
	changed := validManifest()
	changed["version"] = "2.0.0"
	writeManifestInto(t, repo, changed)
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-qm", "bump"},
	} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	manifest, err := Update(plugins, "author.thing")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "2.0.0" {
		t.Fatalf("updated manifest = %#v", manifest)
	}

	// Removing deletes the directory.
	if err := Remove(plugins, "author.thing"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(plugins, "author.thing")); !os.IsNotExist(err) {
		t.Fatalf("plugin was not removed: %v", err)
	}
	// A path that would escape the plugins directory is refused.
	if err := Remove(plugins, "../escape"); err == nil {
		t.Fatal("Remove should refuse a traversing id")
	}
}

func TestUpdateRejectsALocalCopy(t *testing.T) {
	plugins := t.TempDir()
	if _, err := Install(plugins, pluginDir(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(plugins, "author.thing"); err == nil {
		t.Fatal("a local copy has no checkout to update")
	}
}
