// Command obsidian is a tidedeck panel over an Obsidian vault. Obsidian keeps
// its notes as plain markdown on disk, so the pane reads them directly: it
// shows the note the reader last loaded, a fuzzy search finds another one, and
// enter loads it (or e edits it in a full-screen Ripple editor).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

const usage = `obsidian - search and edit notes in your Obsidian vault

  obsidian render       print the panel document
  obsidian open <note>  load the note into the pane ("new" makes one)
  obsidian edit <note>  edit the note in the Ripple editor
  obsidian vaults       print the vaults Obsidian knows
  obsidian path         print the vault the pane will use
`

// settings are what the dashboard passes in as TIDEDECK_PLUGIN_* variables.
type settings struct {
	vault  string
	query  string
	mode   string
	folder string
}

func settingsFromEnv() settings {
	cfg := settings{
		vault:  os.Getenv("TIDEDECK_PLUGIN_VAULT"),
		query:  os.Getenv("TIDEDECK_PLUGIN_QUERY"),
		mode:   os.Getenv("TIDEDECK_PLUGIN_MODE"),
		folder: strings.TrimSpace(os.Getenv("TIDEDECK_PLUGIN_FOLDER")),
	}
	if cfg.mode != "vim" {
		cfg.mode = "plain"
	}
	return cfg
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the program without os.Exit in it, so every verb can be tested.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cfg := settingsFromEnv()
	switch args[0] {
	case "render":
		return renderDoc(stdout, stderr, cfg)
	case "open":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "obsidian: open needs a note")
			return 2
		}
		return openCmd(stdout, stderr, cfg, args[1])
	case "edit":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "obsidian: edit needs a note")
			return 2
		}
		return editCmd(stdout, stderr, cfg, args[1])
	case "vaults":
		return listVaults(stdout, stderr)
	case "path":
		return vaultPath(stdout, stderr, cfg)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "obsidian: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// renderDoc prints the panel document. Without a query the pane is a reader,
// showing the note the reader last loaded; with one it is the fuzzy matches.
// A vault that cannot be found is drawn rather than returned, so the pane
// explains itself instead of keeping its last good content.
func renderDoc(stdout, stderr io.Writer, cfg settings) int {
	vaults, _ := Vaults()
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		return writeDoc(stdout, stderr, Render(nil, vaults, cfg.query, nil, nil, nil, err))
	}
	notes, err := ScanNotes(vault.Path)
	if err != nil {
		return writeDoc(stdout, stderr, Render(vault, vaults, cfg.query, nil, nil, nil, err))
	}

	if strings.TrimSpace(cfg.query) == "" {
		current := currentNote(vault, notes)
		var body []string
		if current != nil {
			if data, readErr := ReadNote(vault, current.Rel); readErr == nil {
				body = BodyLines(data, previewLines)
			}
		}
		return writeDoc(stdout, stderr, Render(vault, vaults, "", nil, current, body, nil))
	}

	matches := make([]Match, 0, len(notes))
	for _, note := range RankNotes(notes, cfg.query) {
		excerpt := ""
		if data, readErr := ReadNote(vault, note.Rel); readErr == nil {
			excerpt = Excerpt(data, 90)
		}
		matches = append(matches, Match{Note: note, Excerpt: excerpt})
	}
	return writeDoc(stdout, stderr, Render(vault, vaults, cfg.query, matches, nil, nil, nil))
}

// currentNote is the note the reader view shows: the one the reader last
// loaded, if it is still in the vault, then the most recently edited.
func currentNote(vault *Vault, notes []Note) *Note {
	if state := LoadState(vault.Path); state.Note != "" {
		for i := range notes {
			if notes[i].Rel == state.Note {
				return &notes[i]
			}
		}
	}
	if len(notes) == 0 {
		return nil
	}
	newest := 0
	for i := 1; i < len(notes); i++ {
		if notes[i].Mod.After(notes[newest].Mod) {
			newest = i
		}
	}
	return &notes[newest]
}

func writeDoc(stdout, stderr io.Writer, doc dash.Doc) int {
	data, err := json.Marshal(doc)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(data))
	return 0
}

// openCmd loads a note into the pane by remembering it, so the next render
// shows it. The "new" id makes a note from the query and edits it instead,
// because a note that does not exist yet has nothing to read.
func openCmd(stdout, stderr io.Writer, cfg settings, id string) int {
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if id == newNoteID {
		return createAndEdit(stdout, stderr, vault, cfg)
	}
	if _, err := ReadNote(vault, id); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if err := SaveState(vault.Path, id); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	return 0
}

func editCmd(stdout, stderr io.Writer, cfg settings, id string) int {
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if id == newNoteID {
		return createAndEdit(stdout, stderr, vault, cfg)
	}
	if err := SaveState(vault.Path, id); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if err := editNote(vault, id, cfg.mode); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	return 0
}

// createAndEdit makes the note named by the query and opens it in Ripple.
func createAndEdit(stdout, stderr io.Writer, vault *Vault, cfg settings) int {
	rel, err := CreateNote(vault, cfg.folder, cfg.query)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if err := SaveState(vault.Path, rel); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if err := editNote(vault, rel, cfg.mode); err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	return 0
}

func listVaults(stdout, stderr io.Writer) int {
	vaults, err := Vaults()
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if len(vaults) == 0 {
		fmt.Fprintln(stdout, "Obsidian knows no vaults.")
		return 0
	}
	for _, vault := range vaults {
		mark := " "
		if vault.Open {
			mark = "*"
		}
		fmt.Fprintf(stdout, "%s %s\t%s\n", mark, vault.Name, vault.Path)
	}
	return 0
}

func vaultPath(stdout, stderr io.Writer, cfg settings) int {
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, vault.Path)
	return 0
}
