package dash

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cloneTimeout bounds a plugin clone or pull. A stuck network call must not
// hang the caller forever.
const cloneTimeout = 90 * time.Second

// PluginInfo is one installed plugin, and the reason it could not be read if
// its manifest is broken.
type PluginInfo struct {
	Manifest Manifest
	Dir      string
	Problem  error
}

// installRecordFile remembers where an installed plugin came from, so Update
// can re-fetch it. It lives inside the plugin directory, which is otherwise
// exactly the plugin.
const installRecordFile = ".tidedeck-install.json"

// installRecord is the source an installed plugin came from.
type installRecord struct {
	Source string `json:"source"`
	Subdir string `json:"subdir,omitempty"`
}

// Install puts a plugin into dir from a source and returns its manifest. A
// source is an http(s)/git/ssh URL or a local directory, optionally with a
// "#subdir" naming the directory inside it that holds the plugin - so one
// repository can host several plugins.
//
// The fetch goes into a staging directory first and is only moved into place
// once the manifest validates, so a bad source leaves no half-installed
// directory behind. Cloning runs git, which does not execute anything the
// repository ships; the plugin's program only runs once the panel is enabled
// and refreshed.
func Install(dir, source string) (Manifest, error) {
	base, subdir, err := parseSource(source)
	if err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}
	stage, err := os.MkdirTemp(dir, ".staging-")
	if err != nil {
		return Manifest{}, err
	}
	// Removed unless the rename below consumes it; a no-op afterwards.
	defer os.RemoveAll(stage)

	if err := fetchSource(base, dir, stage); err != nil {
		return Manifest{}, err
	}
	pluginPath := stage
	if subdir != "" {
		pluginPath = filepath.Join(stage, subdir)
	}
	manifest, err := LoadManifest(pluginPath)
	if err != nil {
		return Manifest{}, err
	}
	final := filepath.Join(dir, manifest.ID)
	if _, err := os.Stat(final); err == nil {
		return Manifest{}, fmt.Errorf("plugin %s is already installed", manifest.ID)
	}
	if err := os.Rename(pluginPath, final); err != nil {
		return Manifest{}, err
	}
	if err := writeInstallRecord(final, installRecord{Source: base, Subdir: subdir}); err != nil {
		return Manifest{}, err
	}
	// Re-read from the final path: the manifest's resolved directory must be
	// where it now lives, or its entry point would resolve against the staging
	// directory that no longer exists.
	return LoadManifest(final)
}

// Installed lists the plugins under dir, in name order. An entry whose manifest
// does not validate is reported with its problem rather than dropped, so the
// settings screen can say what is wrong.
func Installed(dir string) []PluginInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		manifest, err := LoadManifest(full)
		out = append(out, PluginInfo{Manifest: manifest, Dir: full, Problem: err})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DisplayName() < out[j].DisplayName()
	})
	return out
}

// DisplayName is the plugin's name for a list, falling back to its id or
// directory when the manifest could not be read.
func (p PluginInfo) DisplayName() string {
	if p.Manifest.Name != "" {
		return p.Manifest.Name
	}
	if p.Manifest.ID != "" {
		return p.Manifest.ID
	}
	return filepath.Base(p.Dir)
}

// Remove deletes an installed plugin.
func Remove(dir, id string) error {
	if !validPluginID(id) {
		return fmt.Errorf("invalid plugin id %q", id)
	}
	return os.RemoveAll(filepath.Join(dir, id))
}

// Update re-fetches an installed plugin from the source it came from and
// returns its (possibly changed) manifest. It re-clones or re-copies rather than
// pulling a checkout, so a plugin installed from a local directory - or from a
// subdirectory of a repository - updates the same way as one from a repo root.
func Update(dir, id string) (Manifest, error) {
	if !validPluginID(id) {
		return Manifest{}, fmt.Errorf("invalid plugin id %q", id)
	}
	final := filepath.Join(dir, id)
	record, err := readInstallRecord(final)
	if err != nil {
		return Manifest{}, err
	}
	stage, err := os.MkdirTemp(dir, ".staging-")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(stage)

	if err := fetchSource(record.Source, dir, stage); err != nil {
		return Manifest{}, err
	}
	pluginPath := stage
	if record.Subdir != "" {
		pluginPath = filepath.Join(stage, record.Subdir)
	}
	manifest, err := LoadManifest(pluginPath)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.ID != id {
		return Manifest{}, fmt.Errorf("plugin id changed from %s to %s; remove and reinstall it", id, manifest.ID)
	}
	if err := os.RemoveAll(final); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(pluginPath, final); err != nil {
		return Manifest{}, err
	}
	if err := writeInstallRecord(final, record); err != nil {
		return Manifest{}, err
	}
	return LoadManifest(final)
}

// parseSource splits "source#subdir" into the fetch source and the directory
// inside it that holds the plugin. A subdir may have several segments
// ("contrib/ai-usage") but may not escape the fetched source.
func parseSource(source string) (base, subdir string, err error) {
	base, fragment, hasFragment := strings.Cut(source, "#")
	base = strings.TrimSpace(base)
	if base == "" {
		return "", "", fmt.Errorf("no plugin source given")
	}
	if !hasFragment {
		return base, "", nil
	}
	subdir = strings.Trim(strings.TrimSpace(fragment), "/")
	if subdir == "" {
		return base, "", nil
	}
	for _, part := range strings.Split(subdir, "/") {
		if part == ".." || part == "." {
			return "", "", fmt.Errorf("invalid plugin subdirectory %q", fragment)
		}
	}
	return base, subdir, nil
}

// fetchSource puts a source's contents into dest: a git clone for a URL, a copy
// for a local directory.
func fetchSource(source, workdir, dest string) error {
	if isRemoteSource(source) {
		return gitClone(workdir, source, dest)
	}
	path := expandHome(source)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("not a git URL or an existing directory")
	}
	return copyTree(path, dest)
}

func writeInstallRecord(pluginDir string, record installRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pluginDir, installRecordFile), append(data, '\n'), 0o644)
}

func readInstallRecord(pluginDir string) (installRecord, error) {
	data, err := os.ReadFile(filepath.Join(pluginDir, installRecordFile))
	if err != nil {
		return installRecord{}, fmt.Errorf("no install record for %s; reinstall it", filepath.Base(pluginDir))
	}
	var record installRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return installRecord{}, err
	}
	if strings.TrimSpace(record.Source) == "" {
		return installRecord{}, fmt.Errorf("no install record for %s; reinstall it", filepath.Base(pluginDir))
	}
	return record, nil
}

// isRemoteSource reports whether a source is something git clones rather than a
// directory to copy.
func isRemoteSource(source string) bool {
	for _, prefix := range []string{"http://", "https://", "git://", "ssh://", "file://", "git@"} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	return false
}

// validPluginID guards Remove and Update against a path that would escape the
// plugins directory.
func validPluginID(id string) bool {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return false
	}
	return true
}

// gitClone clones source into dest. The working directory is the plugins
// directory, so a relative source resolves the way the user typed it.
func gitClone(dir, source, dest string) error {
	return runGit(dir, "clone", "--depth", "1", source, dest)
}

// runGit runs a git command with a bounded context. The command's stderr is
// folded into the error, so "repository not found" reaches the user - the last
// line, which is where git puts the reason ("fatal: …"), not the "Cloning
// into …" progress line it prints first. Git is told not to prompt: a TUI has
// no console to answer a username or password prompt, so a private repository
// should fail with a message rather than hang.
func runGit(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git %s timed out after %s", args[0], cloneTimeout)
		}
		if message := lastLine(string(output)); message != "" {
			return fmt.Errorf("git %s: %s", args[0], gitReason(message))
		}
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}

// gitReason turns git's failure into a short reason a one-line settings notice
// can show. The raw message repeats the URL the user just typed and runs long
// enough to wrap the settings page.
func gitReason(message string) string {
	switch {
	case strings.Contains(message, "terminal prompts disabled"),
		strings.Contains(message, "could not read Username"),
		strings.Contains(message, "Authentication failed"):
		return "repository private or missing — git has no credentials"
	case strings.Contains(message, "not found"),
		strings.Contains(message, "Repository not found"):
		return "repository not found — check the URL"
	case strings.Contains(message, "Could not resolve host"),
		strings.Contains(message, "unable to access"):
		return "host unreachable — check the URL"
	default:
		return message
	}
}

// lastLine returns the last non-empty line of some output, which is where git
// reports a failure.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// copyTree copies a directory, so a plugin can be installed from a local
// working copy without a checkout under the config directory.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, relative)
		switch {
		case info.IsDir():
			// A .git directory is of no use in an installed copy and can be
			// large, so it is skipped.
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// expandHome resolves a leading ~, the way a shell would.
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
