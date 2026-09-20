package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

// maxMatches caps the list the pane draws. A fuzzy query in a big vault can
// match hundreds of notes; the pane shows the best of them and says how many
// were left.
const maxMatches = 12

// Render turns a vault's notes into the panel document: the fuzzy matches, and
// a preview of the best one so a query loads a note as it is typed.
func Render(vault *Vault, vaults []Vault, notes []Note, query string, preview []string, previewTitle string, problem error) dash.Doc {
	doc := dash.Doc{SchemaVersion: dash.DocSchemaVersion, Options: vaultOptions(vaults)}

	if problem != nil {
		doc.Badge = &dash.DocBadge{Text: "no vault", Tone: "warning"}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "Obsidian", Value: problem.Error(), Tone: "warning",
		})
		return doc
	}

	name := "vault"
	if vault != nil && vault.Name != "" {
		name = vault.Name
	}
	query = strings.TrimSpace(query)
	if query == "" {
		doc.Badge = &dash.DocBadge{Text: fmt.Sprintf("%d notes", len(notes)), Tone: "muted"}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "search", Value: "type to filter " + name, Tone: "muted",
		})
	} else {
		doc.Badge = &dash.DocBadge{Text: fmt.Sprintf("%d match", len(notes)), Tone: "accent"}
	}

	matches := notes
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}
	if len(notes) == 0 {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "no note", Value: "nothing matches " + query, Tone: "warning",
		})
		return doc
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

	if len(preview) > 0 {
		label := "PREVIEW"
		if previewTitle != "" {
			label = "PREVIEW · " + strings.ToUpper(capLabel(previewTitle, 24))
		}
		doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: label})
		doc.Rows = append(doc.Rows, dash.Row{Type: "block", Body: preview})
	}
	return doc
}

// capLabel keeps a preview divider from being swallowed by a long note title.
func capLabel(title string, limit int) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
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
