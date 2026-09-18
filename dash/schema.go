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

	// Default is the value to show when the key is absent from the document,
	// so a fresh install displays what the panel will actually use rather
	// than an empty row.
	Default string

	// Description explains what the setting does, in one sentence. The
	// settings screen shows it under the field it belongs to. A label alone
	// cannot say that a docker socket of "1" means the default one, or what a
	// blank value falls back to, and that is exactly what a reader needs.
	Description string

	// Placeholder is shown in place of an empty value, so a field that does
	// something sensible when blank can say so rather than looking unset.
	Placeholder string

	// Normalize tidies a value on save, as the repository list does.
	Normalize func(string) string
	// Summary renders a long value as something that fits one row, as the
	// repository list does; the raw value is still what gets edited.
	Summary func(string) string
	// Validate reports whether a value is usable, as it is typed. It is the
	// counterpart to Normalize: Normalize tidies a value that is already
	// acceptable, Validate says a value is not. Returning an error marks the
	// field rather than rejecting the keystroke, so a half-typed value is
	// still editable - "47." is not a number yet, but deleting it to get
	// there would make the field impossible to fill in.
	Validate func(string) error
	// Run performs a FieldAction and returns the message to show.
	Run func() string

	// Unit, Min, Max and Step describe a FieldFloat. Step is how far an arrow
	// key moves the value; zero leaves the field typed rather than stepped.
	// Min and Max bound it, and are ignored when equal.
	Unit string
	Min  float64
	Max  float64
	Step float64
}

// Bounded reports whether a numeric field has a range to clamp to. Min and Max
// both left at zero means "unbounded", which is the useful default: a field
// that wanted 0..0 has nothing to edit.
func (f Field) Bounded() bool { return f.Min != f.Max }

// Category is one page of the settings screen: a panel's fields under its
// title. A panel with no settings still gets a page, because the screen adds
// the visibility toggle and, when the panel uses them, the metric styles.
type Category struct {
	PanelID string
	Name    string
	Fields  []Field
	// Gauge and Spark mirror the panel's Meta, so the settings screen knows
	// whether to offer that style for this panel.
	Gauge bool
	Spark bool
}
