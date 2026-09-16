package dash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestSchemaVersion is the manifest contract this build understands. It
// describes this format only. The shape deliberately resembles the one
// Omarchy shell plugins use, because a format someone already knows is worth
// more than a better one they do not - but the plugins are not
// interchangeable, and nothing here should imply that they are.
const ManifestSchemaVersion = 1

// ManifestFile is the file discovered inside a plugin directory.
const ManifestFile = "manifest.json"

// Manifest describes a plugin: who wrote it, what it provides, and how to run
// it.
type Manifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Author        string   `json:"author"`
	License       string   `json:"license"`
	Description   string   `json:"description"`
	Kinds         []string `json:"kinds"`

	// EntryPoints maps a kind to the argv that produces it. Unlike a shell
	// plugin, whose entry point is a file loaded into the host, a tidedeck
	// entry point is a program: it prints a document and exits.
	EntryPoints map[string][]string `json:"entryPoints"`

	Panel PanelManifest `json:"panel"`

	// dir is where the manifest was read from; entry points resolve against
	// it so a plugin can ship "./render.sh".
	dir string
}

// PanelManifest is the per-kind block for a panel, carrying how it should be
// placed and what it lets you configure.
type PanelManifest struct {
	DisplayName    string         `json:"displayName"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	RefreshSeconds int            `json:"refreshSeconds"`
	MinWidth       int            `json:"minWidth"`
	MinHeight      int            `json:"minHeight"`
	Priority       int            `json:"priority"`
	HideBelow      int            `json:"hideBelow"`
	Schema         []SchemaField  `json:"schema"`
	Defaults       map[string]any `json:"defaults"`
}

// SchemaField is one declared setting, in the shape shell plugin manifests
// already use.
type SchemaField struct {
	Key          string   `json:"key"`
	Type         string   `json:"type"` // string, boolean, choice
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	DefaultValue any      `json:"defaultValue"`
	Options      []string `json:"options"`
}

// KindPanel is the only kind this build supports.
const KindPanel = "panel"

// minRefresh keeps a plugin from being run many times a second. A program
// that wants to be sampled faster than this is the wrong shape for a
// subprocess.
const minRefresh = 2 * time.Second

// LoadManifest reads and validates a plugin directory.
func LoadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", filepath.Join(dir, ManifestFile), err)
	}
	manifest.dir = dir
	if problems := manifest.Validate(); len(problems) > 0 {
		return Manifest{}, fmt.Errorf("%s: %s", dir, strings.Join(problems, "; "))
	}
	return manifest, nil
}

// Validate reports everything wrong with a manifest rather than only the
// first problem, so one run of the checker is enough to fix it.
func (m Manifest) Validate() []string {
	var problems []string
	if m.SchemaVersion != ManifestSchemaVersion {
		problems = append(problems, fmt.Sprintf("schemaVersion is %d, want %d", m.SchemaVersion, ManifestSchemaVersion))
	}
	for _, required := range []struct{ name, value string }{
		{"id", m.ID}, {"name", m.Name}, {"version", m.Version},
		{"author", m.Author}, {"description", m.Description},
	} {
		if strings.TrimSpace(required.value) == "" {
			problems = append(problems, required.name+" is required")
		}
	}
	// A namespaced id keeps two authors' plugins from colliding, and matches
	// how panel ids are already written.
	if m.ID != "" && !strings.Contains(m.ID, ".") {
		problems = append(problems, fmt.Sprintf("id %q should be namespaced, like \"author.name\"", m.ID))
	}
	if len(m.Kinds) == 0 {
		problems = append(problems, "kinds is required")
	}
	for _, kind := range m.Kinds {
		if kind != KindPanel {
			problems = append(problems, fmt.Sprintf("kind %q is not supported; this build renders %q, not shell surfaces", kind, KindPanel))
			continue
		}
		entry := m.EntryPoints[kind]
		if len(entry) == 0 {
			problems = append(problems, fmt.Sprintf("entryPoints.%s is required", kind))
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry[0]), ".qml") {
			problems = append(problems, fmt.Sprintf("entryPoints.%s is a QML file; a tidedeck entry point is a program that prints a document", kind))
		}
	}
	for i, field := range m.Panel.Schema {
		if strings.TrimSpace(field.Key) == "" {
			problems = append(problems, fmt.Sprintf("panel.schema[%d].key is required", i))
		}
		switch field.Type {
		case "string", "boolean", "choice", "":
		default:
			problems = append(problems, fmt.Sprintf("panel.schema[%d].type %q is not string, boolean or choice", i, field.Type))
		}
		if field.Type == "choice" && len(field.Options) == 0 {
			problems = append(problems, fmt.Sprintf("panel.schema[%d] is a choice with no options", i))
		}
	}
	return problems
}

// Command resolves a kind's entry point against the plugin directory, so a
// manifest can name "./render.sh".
func (m Manifest) Command(kind string) []string {
	entry := m.EntryPoints[kind]
	if len(entry) == 0 {
		return nil
	}
	argv := append([]string(nil), entry...)
	// A relative entry point is relative to the plugin, not to wherever the
	// dashboard was started. Resolving it to an absolute path here means the
	// command does not also depend on the working directory it is run in.
	if strings.HasPrefix(argv[0], "./") || strings.HasPrefix(argv[0], "../") {
		resolved := filepath.Join(m.dir, argv[0])
		if absolute, err := filepath.Abs(resolved); err == nil {
			resolved = absolute
		}
		argv[0] = resolved
	}
	return argv
}

// Interval is how often the panel should run, floored so a plugin cannot ask
// to be executed many times a second.
func (m Manifest) Interval() time.Duration {
	if m.Panel.RefreshSeconds <= 0 {
		return time.Minute
	}
	if interval := time.Duration(m.Panel.RefreshSeconds) * time.Second; interval > minRefresh {
		return interval
	}
	return minRefresh
}

// Fields converts the manifest's declared settings into the form the settings
// screen builds its page from. Keys are namespaced under the plugin id, so a
// plugin cannot collide with a built-in panel's key or another plugin's.
func (m Manifest) Fields() []Field {
	fields := make([]Field, 0, len(m.Panel.Schema))
	for _, declared := range m.Panel.Schema {
		field := Field{
			Key:   m.SettingKey(declared.Key),
			Label: declared.Label,
			Kind:  FieldText,
		}
		switch declared.Type {
		case "boolean":
			field.Kind = FieldBool
		case "choice":
			field.Kind, field.Options = FieldChoice, declared.Options
		}
		if declared.DefaultValue != nil {
			field.Default = fmt.Sprintf("%v", declared.DefaultValue)
		}
		if field.Label == "" {
			field.Label = declared.Key
		}
		fields = append(fields, field)
	}
	return fields
}

// SettingKey is where a plugin's setting lives in the configuration document.
func (m Manifest) SettingKey(key string) string {
	return "plugins." + m.ID + "." + key
}
