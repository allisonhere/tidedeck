// Command obsidian is a tidedeck panel over an Obsidian vault. Obsidian keeps
// its notes as plain markdown on disk, so the pane reads them directly: a fuzzy
// search filters the vault, and enter loads the note under the cursor in a
// full-screen Ripple editor.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/allisonhere/tideui/dash"
)

const usage = `obsidian - search and edit notes in your Obsidian vault

  obsidian render       print the panel document
  obsidian edit <note>  open the note in the Ripple editor
  obsidian vaults       print the vaults Obsidian knows
  obsidian path         print the vault the pane will use
`

// settings are what the dashboard passes in as TIDEDECK_PLUGIN_* variables.
type settings struct {
	vault string
	query string
	mode  string
}

func settingsFromEnv() settings {
	cfg := settings{
		vault: os.Getenv("TIDEDECK_PLUGIN_VAULT"),
		query: os.Getenv("TIDEDECK_PLUGIN_QUERY"),
		mode:  os.Getenv("TIDEDECK_PLUGIN_MODE"),
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

// renderDoc prints the panel document. A vault that cannot be found is drawn
// rather than returned: the pane has to explain itself, and a non-zero exit
// would leave its last good content up instead.
func renderDoc(stdout, stderr io.Writer, cfg settings) int {
	vaults, _ := Vaults()
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		return writeDoc(stdout, stderr, Render(nil, vaults, nil, cfg.query, err))
	}
	notes, err := ScanNotes(vault.Path)
	if err != nil {
		return writeDoc(stdout, stderr, Render(vault, vaults, nil, cfg.query, err))
	}
	return writeDoc(stdout, stderr, Render(vault, vaults, RankNotes(notes, cfg.query), cfg.query, nil))
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

func editCmd(stdout, stderr io.Writer, cfg settings, id string) int {
	vault, err := ResolveVault(cfg.vault)
	if err != nil {
		fmt.Fprintf(stderr, "obsidian: %v\n", err)
		return 1
	}
	if err := editNote(vault, id, cfg.mode); err != nil {
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
