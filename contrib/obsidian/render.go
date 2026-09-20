package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

// maxMatches caps the list the pane draws while searching. A fuzzy query in a
// big vault can match hundreds of notes; the pane shows the best of them and
// says how many were left.
const maxMatches = 12

// Render turns a vault's notes into the panel document. Without a query the
// pane is a single search row - a note list is only drawn when the reader is
// actually searching. The rows that carry an id are the matches, and the
// reader's enter key loads the one under the cursor in the editor.
func Render(vault *Vault, vaults []Vault, notes []Note, query string, problem error) dash.Doc {
	doc := dash.Doc{SchemaVersion: dash.DocSchemaVersion, Options: vaultOptions(vaults)}

	if problem != nil {
		doc.Badge = &dash.DocBadge{Text: "no vault", Tone: "warning"}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "Obsidian", Value: problem.Error(), Tone: "warning",
		})
		return doc
	}

	query = strings.TrimSpace(query)
	if query == "" {
		if vault != nil && vault.Name != "" {
			doc.Badge = &dash.DocBadge{Text: vault.Name, Tone: "muted"}
		}
		// The whole pane is one row until something is searched for.
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "search", Value: "type to find a note", Tone: "muted",
		})
		return doc
	}

	doc.Badge = &dash.DocBadge{Text: fmt.Sprintf("%d match", len(notes)), Tone: "accent"}
	doc.Rows = append(doc.Rows, dash.Row{
		Type: "text", Label: "search", Value: query, Tone: "accent",
	})
	if len(notes) == 0 {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "no note", Value: "nothing matches " + query, Tone: "warning",
		})
		return doc
	}

	matches := notes
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}
	doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: "NOTES"})
	for _, note := range matches {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: note.Title, Value: folderOf(note.Rel), ID: note.Rel,
		})
	}
	if len(notes) > len(matches) {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "more", Value: fmt.Sprintf("%d more", len(notes)-len(matches)), Tone: "muted",
		})
	}
	return doc
}

// vaultOptions offers the vault setting every vault Obsidian knows, blank first
// so the default (the open one) keeps meaning something.
func vaultOptions(vaults []Vault) map[string][]string {
	if len(vaults) == 0 {
		return nil
	}
	names := make([]string, 0, len(vaults)+1)
	names = append(names, "")
	for _, vault := range vaults {
		if vault.Name != "" {
			names = append(names, vault.Name)
		}
	}
	return map[string][]string{"vault": names}
}

// folderOf is the row's second column: where the note lives, "vault root" for
// a note at the top.
func folderOf(rel string) string {
	dir := path.Dir(rel)
	if dir == "." || dir == "/" || dir == "" {
		return "vault root"
	}
	return dir
}
