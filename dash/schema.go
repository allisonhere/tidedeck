package dash

// FieldKind is how a setting is edited. The kinds mirror the ones the
// settings screen already draws, because this changes who owns the list of
// fields, not how a field looks.
type FieldKind int

const (
	FieldText   FieldKind = iota // free text
	FieldBool                    // a tick
	FieldChoice                  // one of Options
	FieldFloat                   // a number, validated on save
	FieldAction                  // a button
)

// Field declares one setting.
//
// Key is the dotted path into the configuration document, and it names the
// key that is already in the file - "aur_helper", "weather.latitude". Nothing
// is renamed by this design, so a user's existing config keeps working.
type Field struct {
	Key   string // "" for an action, which stores nothing
	Label string
	Kind  FieldKind

	Options []string // FieldChoice

	// Normalize tidies a value on save, as the repository list does.
	Normalize func(string) string
	// Summary renders a long value as something that fits one row, as the
	// repository list does; the raw value is still what gets edited.
	Summary func(string) string
	// Run performs a FieldAction and returns the message to show.
	Run func() string
}

// Category is one page of the settings screen: a panel's fields under its
// title. A panel with no settings still gets a page, because the screen adds
// the visibility toggle and metric styles to every panel.
type Category struct {
	PanelID string
	Name    string
	Fields  []Field
}
