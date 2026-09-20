package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

const (
	// maxMatches caps the list the pane draws; a vault can match many notes.
	maxMatches = 8
	// newNoteID is the row that makes a note from the query. It cannot collide
	// with a note id, which is always a vault-relative .md path.
	newNoteID = "new"
)

// Match is a note the search found, with a line of its body so two notes with
// similar names can be told apart.
type Match struct {
	Note
	Excerpt string
}

// Render draws the pane. With no query it is a reader - the search row, the
// current note and its body. With a query it is the fuzzy matches, each
// openable, and a row to make a new note from the query.
func Render(vault *Vault, vaults []Vault, query string, matches []Match, current *Note, body []string, problem error) dash.Doc {
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
		doc.Badge = &dash.DocBadge{Text: name, Tone: "muted"}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "search", Value: "type to find a note", Tone: "muted",
		})
		if current == nil {
			doc.Rows = append(doc.Rows, dash.Row{
				Type: "text", Label: "no notes", Value: "this vault is empty", Tone: "warning",
			})
			return doc
		}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: current.Title, Value: folderOf(current.Rel), ID: current.Rel, Tone: "accent",
		})
		if len(body) > 0 {
			doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: "NOTE"})
			doc.Rows = append(doc.Rows, dash.Row{Type: "block", Body: body})
		}
		return doc
	}

	doc.Badge = &dash.DocBadge{Text: fmt.Sprintf("%d match", len(matches)), Tone: "accent"}
	doc.Rows = append(doc.Rows, dash.Row{Type: "text", Label: "search", Value: query, Tone: "accent"})
	if len(matches) == 0 {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "no note", Value: "nothing matches " + query, Tone: "muted",
		})
		doc.Rows = append(doc.Rows, newNoteRow(query))
		return doc
	}

	shown := matches
	if len(shown) > maxMatches {
		shown = shown[:maxMatches]
	}
	doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: "NOTES"})
	for _, match := range shown {
		row := dash.Row{Type: "block", Label: match.Title, Value: folderOf(match.Rel), ID: match.Rel}
		if match.Excerpt != "" {
			row.Body = []string{match.Excerpt}
			row.BodyTone = "muted"
		}
		doc.Rows = append(doc.Rows, row)
	}
	if len(matches) > len(shown) {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "more", Value: fmt.Sprintf("%d more", len(matches)-len(shown)), Tone: "muted",
		})
	}
	doc.Rows = append(doc.Rows, newNoteRow(query))
	return doc
}

// newNoteRow makes a note named after the query. Enter opens it in the editor,
// because a note that does not exist yet has nothing to read.
func newNoteRow(query string) dash.Row {
	return dash.Row{Type: "text", Label: "＋ new note", Value: capText(query, 40), ID: newNoteID, Tone: "accent"}
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
