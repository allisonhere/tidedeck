package dash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tideui"
)

// maxPluginOutput caps what a plugin may print. A program that produces more
// than this has gone wrong, and reading it all would take the dashboard with
// it.
const maxPluginOutput = 1 << 20

// execPanel is a panel whose content comes from a program: the entry point is
// run on the panel's interval and prints a document.
//
// It implements the same interfaces a built-in panel does, which is the whole
// point of keeping model types out of them - a Deck cannot tell the two
// apart, and nothing in the registry needed changing to accept this.
//
// Plugins run unsandboxed, with your user permissions. This is an extension
// mechanism for a single-user dashboard, not a security boundary.
type execPanel struct {
	State[Doc]
	manifest Manifest
	argv     []string
	timeout  time.Duration

	// settings holds the last Configure's values by declared key; input is what
	// has been typed into the panel's input setting since. Both are read when a
	// run's environment is built, input overriding its setting.
	mu       sync.Mutex
	settings map[string]string
	input    string
}

// Exec builds a panel that runs a program. The manifest supplies the entry
// point, the placement and the declared settings.
func Exec(manifest Manifest) Panel {
	return &execPanel{
		manifest: manifest,
		argv:     manifest.Command(KindPanel),
		timeout:  DefaultTimeout,
	}
}

func (e *execPanel) Meta() Meta {
	panel := e.manifest.Panel
	title := panel.DisplayName
	if title == "" {
		title = e.manifest.Name
	}
	meta := Meta{
		ID: e.manifest.ID, Title: title, Subtitle: panel.Category,
		Role: tideui.RoleOptional, Priority: panel.Priority,
		MinWidth: panel.MinWidth, MinHeight: panel.MinHeight,
		HideBelow: panel.HideBelow, Interval: e.manifest.Interval(),
		// A freshly installed plugin should not rearrange the dashboard, so
		// it starts hidden and is enabled from the panel picker or settings.
		Hidden: true,
		// The plugin claims which metric rows it prints, so its settings page
		// offers only the styles it uses.
		Gauge: panel.Gauge,
		Spark: panel.Spark,
	}
	if meta.Priority == 0 {
		meta.Priority = 30
	}
	if meta.MinWidth == 0 {
		meta.MinWidth = 18
	}
	if meta.MinHeight == 0 {
		meta.MinHeight = 4
	}
	return meta
}

func (e *execPanel) Schema() []Field { return e.manifest.Fields() }

// AlwaysLive runs the program whatever the deck's mode: a plugin is a real
// program the user installed, not sample data, so demo mode must not starve it.
func (e *execPanel) AlwaysLive() bool { return true }

// Configure records the plugin's declared settings. Only declared keys are
// kept: a plugin gets what it asked for, and the dashboard's other settings are
// not its business. The typed input is seeded from its setting, so a configured
// default shows until something is typed.
func (e *execPanel) Configure(values Values) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settings = make(map[string]string, len(e.manifest.Panel.Schema))
	for _, declared := range e.manifest.Panel.Schema {
		value := values.String(e.manifest.SettingKey(declared.Key))
		if value == "" && declared.DefaultValue != nil {
			value = fmt.Sprintf("%v", declared.DefaultValue)
		}
		e.settings[declared.Key] = value
	}
	if input := e.manifest.Panel.Input; input != "" {
		e.input = e.settings[input]
	}
	return nil
}

// environment is the run's environment: every declared setting as
// TIDEDECK_PLUGIN_<KEY>, with the typed input standing in for its setting.
func (e *execPanel) environment() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	env := make([]string, 0, len(e.settings)+1)
	for key, value := range e.settings {
		env = append(env, "TIDEDECK_PLUGIN_"+envKey(key)+"="+value)
	}
	if input := e.manifest.Panel.Input; input != "" {
		env = append(env, "TIDEDECK_PLUGIN_"+envKey(input)+"="+e.input)
	}
	return env
}

// Type appends to the input when the rune is one the panel accepts. Anything
// else is declined, so shortcuts and focus keys still work while it is focused.
func (e *execPanel) Type(r rune) bool {
	if e.manifest.Panel.Input == "" || !strings.ContainsRune(e.manifest.Panel.InputChars, r) {
		return false
	}
	e.mu.Lock()
	e.input += string(r)
	e.mu.Unlock()
	return true
}

// Copy returns the value of the row the manifest named with "copy", so a
// plugin offers a copyable value without any code of its own.
func (e *execPanel) Copy() (string, bool) {
	label := strings.TrimSpace(e.manifest.Panel.Copy)
	if label == "" {
		return "", false
	}
	for _, row := range e.Load().Rows {
		if row.Label == label && row.Value != "" {
			return row.Value, true
		}
	}
	return "", false
}

// Backspace removes the last typed rune.
func (e *execPanel) Backspace() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.manifest.Panel.Input == "" || e.input == "" {
		return false
	}
	runes := []rune(e.input)
	e.input = string(runes[:len(runes)-1])
	return true
}

// envKey converts a setting key into an environment variable name.
func envKey(key string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(key) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out.WriteRune(r)
		default:
			out.WriteByte('_')
		}
	}
	return out.String()
}

// Refresh runs the program and stores what it printed. A failure of any kind
// - a non-zero exit, unreadable output, a timeout, too much output - leaves
// the last good document in place, so a panel that worked a minute ago keeps
// showing what it knew rather than blanking.
func (e *execPanel) Refresh(ctx context.Context) error {
	if len(e.argv) == 0 {
		return fmt.Errorf("plugin %s: no entry point for %s", e.manifest.ID, KindPanel)
	}
	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	command := exec.CommandContext(runCtx, e.argv[0], e.argv[1:]...)
	command.Dir = e.manifest.dir
	command.Env = append(os.Environ(), e.environment()...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	// Killing the entry point does not necessarily end what it started: a
	// script that runs `sleep` leaves that child holding the output pipe, and
	// Run would wait for it. WaitDelay bounds that, so a plugin cannot hold a
	// refresh open past its timeout by spawning something.
	command.WaitDelay = 2 * time.Second

	if err := command.Run(); err != nil {
		if runCtx.Err() != nil {
			return fmt.Errorf("plugin %s: timed out after %s", e.manifest.ID, e.timeout)
		}
		if message := firstLine(stderr.String()); message != "" {
			return fmt.Errorf("plugin %s: %v: %s", e.manifest.ID, err, message)
		}
		return fmt.Errorf("plugin %s: %v", e.manifest.ID, err)
	}
	if stdout.Len() > maxPluginOutput {
		return fmt.Errorf("plugin %s: printed more than %d bytes", e.manifest.ID, maxPluginOutput)
	}

	var doc Doc
	if err := json.NewDecoder(io.LimitReader(&stdout, maxPluginOutput)).Decode(&doc); err != nil {
		return fmt.Errorf("plugin %s: %w", e.manifest.ID, err)
	}
	if doc.SchemaVersion != 0 && doc.SchemaVersion != DocSchemaVersion {
		return fmt.Errorf("plugin %s: document schemaVersion %d, want %d",
			e.manifest.ID, doc.SchemaVersion, DocSchemaVersion)
	}
	e.Store(doc)
	return nil
}

func (e *execPanel) View(ctx tideui.PanelContext) string {
	return RenderDoc(ctx.Renderer, e.Load(), ctx.Width, ctx.Zoomed)
}

// Badge reports whatever badge the document carried.
func (e *execPanel) Badge() (string, tideui.Tone) {
	badge := e.Load().Badge
	if badge == nil {
		return "", tideui.ToneNeutral
	}
	return badge.Text, badge.tone()
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return strings.TrimSpace(text[:index])
	}
	return text
}

// LoadPlugins discovers plugins in a directory, one subdirectory each. A
// plugin that does not validate is skipped with its reason rather than
// stopping the others from loading.
func LoadPlugins(dir string) ([]Panel, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no plugin directory is the normal case
		}
		return nil, []error{err}
	}
	var panels []Panel
	var problems []error
	for _, entry := range entries {
		// A dot-prefixed directory is a staging or editor directory, not a
		// plugin.
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		manifest, err := LoadManifest(filepath.Join(dir, entry.Name()))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		panels = append(panels, Exec(manifest))
	}
	return panels, problems
}
