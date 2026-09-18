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
	Glyph          string         `json:"glyph"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	RefreshSeconds int            `json:"refreshSeconds"`
	MinWidth       int            `json:"minWidth"`
	MinHeight      int            `json:"minHeight"`
	Priority       int            `json:"priority"`
	HideBelow      int            `json:"hideBelow"`
	Schema         []SchemaField  `json:"schema"`
	Defaults       map[string]any `json:"defaults"`

	// Gauge and Spark say whether the document this plugin prints contains
	// gauge or spark rows, so its settings page offers the matching style and
	// not the other. A plugin that only prints text leaves both off.
	Gauge bool `json:"gauge"`
	Spark bool `json:"spark"`

	// Input names a declared setting that receives typing while the panel is
	// focused, so a plugin can offer an input - a calculator's expression, a
	// search box. What the user types is passed to the program as that
	// setting's environment variable on every run. InputChars lists the runes
	// that are accepted, so the rest still reach the application's shortcuts.
	Input      string `json:"input"`
	InputChars string `json:"inputChars"`

	// Copy names the row whose value the copy key puts on the clipboard, so a
	// plugin can offer a copyable value (a token balance, a URL) without any
	// code of its own.
	Copy string `json:"copy"`

	// Open is the argv that opens a row the reader selected: the program that
	// owns the data this panel previews. OpenIDPlaceholder stands for that
	// row's id, so one template serves every row, and the command is run with
	// the terminal handed to it.
	Open []string `json:"open"`
}

// OpenIDPlaceholder is the token a manifest's open command puts where the id of
// the row the reader picked belongs.
const OpenIDPlaceholder = "{id}"

// SchemaField is one declared setting, in the shape shell plugin manifests
// already use.
type SchemaField struct {
	Key          string   `json:"key"`
	Type         string   `json:"type"` // string, boolean, choice
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Placeholder  string   `json:"placeholder"`
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
		case "string", "boolean", "choice", "number", "":
		default:
			problems = append(problems, fmt.Sprintf("panel.schema[%d].type %q is not string, boolean, choice or number", i, field.Type))
		}
		if field.Type == "choice" && len(field.Options) == 0 {
			problems = append(problems, fmt.Sprintf("panel.schema[%d] is a choice with no options", i))
		}
	}
	// An input has to say which setting receives the typing, and which runes it
	// accepts, so an unlisted key (q, s) still reaches the application.
	if input := strings.TrimSpace(m.Panel.Input); input != "" {
		declared := false
		for _, field := range m.Panel.Schema {
			if field.Key == input {
				declared = true
				break
			}
		}
		if !declared {
			problems = append(problems, fmt.Sprintf("panel.input %q is not a declared schema key", input))
		}
		if strings.TrimSpace(m.Panel.InputChars) == "" {
			problems = append(problems, "panel.inputChars is required when panel.input is set")
		}
	}
	// An open command has to say where the id goes. Without a placeholder it
	// would run the same thing whatever the reader picked, which looks like it
	// worked and opens the wrong thing.
	if len(m.Panel.Open) > 0 {
		if strings.TrimSpace(m.Panel.Open[0]) == "" {
			problems = append(problems, "panel.open[0] is empty")
		}
		named := false
		for _, arg := range m.Panel.Open {
			if strings.Contains(arg, OpenIDPlaceholder) {
				named = true
			}
		}
		if !named {
			problems = append(problems, fmt.Sprintf("panel.open names no %s placeholder", OpenIDPlaceholder))
		}
	}
	return problems
}

// Command resolves a kind's entry point against the plugin directory, so a
// manifest can name "./render.sh".
func (m Manifest) Command(kind string) []string {
	return m.resolve(m.EntryPoints[kind])
}

// OpenArgv is the manifest's open command, resolved against the plugin
// directory. The placeholder stays in place: the panel substitutes the selected
// row's id at launch, so one command is resolved once and used for every row.
// The second result is false when the plugin declares no open command, which is
// a panel with no primary action rather than an error.
func (m Manifest) OpenArgv() ([]string, bool) {
	if len(m.Panel.Open) == 0 {
		return nil, false
	}
	return m.resolve(m.Panel.Open), true
}

// resolve makes a command's first element absolute against the plugin, so a
// manifest can name "./render.sh" and the command does not also depend on the
// working directory the dashboard was started in.
func (m Manifest) resolve(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	out := append([]string(nil), argv...)
	if strings.HasPrefix(out[0], "./") || strings.HasPrefix(out[0], "../") {
		resolved := filepath.Join(m.dir, out[0])
		if absolute, err := filepath.Abs(resolved); err == nil {
			resolved = absolute
		}
		out[0] = resolved
	}
	return out
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
			// A manifest has always been able to describe a setting; until
			// now the description was parsed and then dropped on the floor,
			// so a plugin could explain itself and never be heard.
			Description: declared.Description,
			Placeholder: declared.Placeholder,
		}
		switch declared.Type {
		case "boolean":
			field.Kind = FieldBool
		case "choice":
			field.Kind, field.Options = FieldChoice, declared.Options
		case "number":
			field.Kind = FieldFloat
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

// settingName is the inverse of SettingKey: the name the plugin declared, back
// out of the namespaced configuration key.
func (m Manifest) settingName(key string) string {
	return strings.TrimPrefix(key, "plugins."+m.ID+".")
}

// SettingKey is where a plugin's setting lives in the configuration document.
func (m Manifest) SettingKey(key string) string {
	return "plugins." + m.ID + "." + key
}
