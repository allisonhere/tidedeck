package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
	form2 "github.com/allisonhere/tideui/form"
)

// mailForm opens a settings form with the mail plugin registered and run once
// against a fixture mailbox.
func mailForm(t *testing.T) *settingsForm {
	t.Helper()
	mailFixtureHome(t)
	m, err := dash.LoadManifest("../../contrib/mail")
	if err != nil {
		t.Skip("no mail plugin:", err)
	}
	panel := dash.Exec(m)
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panel)
	deck.Attach(ws)
	deck.SetMode(dash.ModeLive)
	deck.Refresh(context.Background(), time.Now())

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)
	return form
}

func fieldNamed(form *settingsForm, label string) *formField {
	for i := range form.currentFields() {
		if form.currentFields()[i].label == label {
			return &form.currentFields()[i]
		}
	}
	return nil
}

// A setting whose useful values the program discovered is a list, not an empty
// box: a manifest is static JSON and cannot know what accounts exist here.
func TestMailSettingsAreLists(t *testing.T) {
	form := mailForm(t)
	openCategory(t, form, "Mail")

	for _, label := range []string{"account", "mailbox"} {
		field := fieldNamed(form, label)
		if field == nil {
			t.Fatalf("no %s field", label)
		}
		if field.kind != fieldChoice {
			t.Errorf("%s is kind %v, want a list", label, field.kind)
		}
		if len(field.options) < 2 {
			t.Errorf("%s options = %v, want the discovered values", label, field.options)
		}
		if field.options[0] != "" {
			t.Errorf("%s first option = %q, want blank to stay choosable", label, field.options[0])
		}
	}
}

// Choosing an account narrows the mailbox list to that account's folders.
// Offering every folder of every account at once is what made it a haystack.
func TestMailMailboxListFollowsTheAccount(t *testing.T) {
	form := mailForm(t)
	openCategory(t, form, "Mail")

	before := len(fieldNamed(form, "mailbox").options)

	account := fieldNamed(form, "account")
	if len(account.options) < 2 {
		t.Skip("the fixture offers one account")
	}
	// Move onto the account row and pick the first real account.
	for form.currentField() != nil && form.currentField().label != "account" {
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	chosen := fieldNamed(form, "account").choiceValue()
	if chosen == "" {
		t.Fatal("stepping did not choose an account")
	}

	// The panel re-runs on the next tick, then the open page follows.
	form.deck.Refresh(context.Background(), time.Now())
	form.SyncOptions()

	after := fieldNamed(form, "mailbox").options
	if len(after) >= before {
		t.Errorf("mailbox options %d -> %d for %q, want the list narrowed",
			before, len(after), chosen)
	}
	// And the account choice survived the refresh.
	if got := fieldNamed(form, "account").choiceValue(); got != chosen {
		t.Errorf("account = %q, want the choice kept at %q", got, chosen)
	}
}

// Opening settings before the panel has run must not leave its fields as text
// boxes for good. The plugin runs a second after launch; a page built in that
// window used to be stuck, because the sync only refreshed rows that were
// already lists.
func TestMailFieldsUpgradeWhenThePanelRunsLate(t *testing.T) {
	mailFixtureHome(t)
	m, err := dash.LoadManifest("../../contrib/mail")
	if err != nil {
		t.Skip("no mail plugin:", err)
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(dash.Exec(m))
	deck.Attach(ws)
	deck.SetMode(dash.ModeLive)

	// Settings opens first, with the panel not yet run.
	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)
	openCategory(t, form, "Mail")

	if got := fieldNamed(form, "account"); got == nil || got.kind != fieldText {
		t.Fatalf("account = %v before the panel ran, want a text box", got)
	}

	// The panel runs on the next tick, and the open page follows.
	deck.Refresh(context.Background(), time.Now())
	form.SyncOptions()

	account := fieldNamed(form, "account")
	if account.kind != fieldChoice {
		t.Fatal("account stayed a text box after the panel reported its options")
	}
	if len(account.options) < 2 {
		t.Errorf("account options = %v, want the discovered list", account.options)
	}
	// The promoted row still writes to the same buffer, so it saves correctly.
	if account.choice == nil {
		t.Fatal("the promoted row lost its buffer")
	}
	account.setChoice("Gmail")
	if got := form.state.panelText[account.key]; got == nil || *got != "Gmail" {
		t.Error("choosing on a promoted row did not reach the saved buffer")
	}
}

// Every test above checks a field's kind, which is not what anyone looks at.
// This one checks the screen: the row has to read as a list, and enter has to
// put the discovered accounts on it.
func TestMailAccountRowDrawsAsAList(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	form := mailForm(t)
	openCategory(t, form, "Mail")
	moveTo(t, form, "account")

	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
	idle := ansi.Strip(form.RenderWorkspace(r, 120, 30))
	if !strings.Contains(idle, "every account ▾") {
		t.Errorf("the account row does not draw as a list:\n%s", idle)
	}

	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	open := ansi.Strip(form.RenderWorkspace(r, 120, 30))
	if !strings.Contains(open, "tide · account") {
		t.Errorf("enter did not open the picker:\n%s", open)
	}
	for _, option := range fieldNamed(form, "account").options[1:] {
		if !strings.Contains(open, option) {
			t.Errorf("the picker is missing %q", option)
		}
	}
}

// A row promoted from text box to list while it is being edited has to swap
// the control too. The editor is only built when an edit starts, so without
// this the reader who opened settings before the panel ran, then pressed
// enter, kept a text box for the rest of the session however many times the
// options were synced.
func TestMailEditorFollowsAPromotedRow(t *testing.T) {
	mailFixtureHome(t)
	m, err := dash.LoadManifest("../../contrib/mail")
	if err != nil {
		t.Skip("no mail plugin:", err)
	}
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(dash.Exec(m))
	deck.Attach(ws)
	deck.SetMode(dash.ModeLive)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	form.Open(cfg)
	openCategory(t, form, "Mail")
	moveTo(t, form, "account")

	// Edit it while it is still a text box.
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := form.editor.(*form2.Text); !ok {
		t.Fatalf("editor = %T before the panel ran, want a text control", form.editor)
	}

	// The panel reports its accounts a moment later.
	deck.Refresh(context.Background(), time.Now())
	form.SyncOptions()

	if form.editing {
		t.Fatalf("the text editor survived the promotion: %T", form.editor)
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
	if got := ansi.Strip(form.RenderWorkspace(r, 120, 30)); !strings.Contains(got, "▾") {
		t.Errorf("the row still does not draw as a list:\n%s", got)
	}
}

// moveTo puts the cursor on a named row.
func moveTo(t *testing.T, form *settingsForm, label string) {
	t.Helper()
	for i := 0; i < len(form.currentFields()); i++ {
		if f := form.currentField(); f != nil && f.label == label {
			return
		}
		form.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	t.Fatalf("no %s row to move to", label)
}

// cascadePlugin writes a plugin whose reported list narrows when a sibling
// setting is set, which is the mail plugin's cascade in miniature: the
// mailboxes of the account just chosen. Its lists have enough entries either
// way for a choice to be a list rather than something you step through blind.
func cascadePlugin(t *testing.T) dash.Manifest {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"schemaVersion":1,"id":"test.cascade","name":"Cascade","version":"1.0.0",` +
		`"author":"test","description":"a list that narrows","kinds":["panel"],` +
		`"entryPoints":{"panel":["./run.sh"]},"panel":{"displayName":"Cascade","refreshSeconds":300,` +
		`"schema":[{"key":"account","type":"string","label":"account"},` +
		`{"key":"mailbox","type":"string","label":"mailbox"}]}}`
	// With no account the list is one account's folders; with one, it is the
	// other's - and the value the reader saved is not in the second.
	script := `#!/bin/sh
if [ -z "${TIDEDECK_PLUGIN_ACCOUNT:-}" ]; then
  printf '%s' '{"rows":[],"options":{"account":["","work","personal"],` +
		`"mailbox":["","INBOX","Receipts","Archive","Sent","Work"]}}'
else
  printf '%s' '{"rows":[],"options":{"account":["","work","personal"],` +
		`"mailbox":["","INBOX.Sent","INBOX.Archive","INBOX.Junk","INBOX.Trash"]}}'
fi
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	loaded, err := dash.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// A panel re-reads its options on every run, and a list that narrows can leave
// the saved value out of it. form.Choice draws the first option for a value it
// was not given, because a control cannot show what it does not have: the row
// said "the inbox" while the setting said "Receipts", and confirming the list
// wrote that over the value the reader had chosen. Switching account should not
// cost the mailbox that belongs to the other one.
func TestSavedChoiceSurvivesAListThatNoLongerOffersIt(t *testing.T) {
	panel := dash.Exec(cascadePlugin(t))
	ws := tideui.NewWorkspace()
	deck := dash.New()
	deck.Register(panel)
	deck.Attach(ws)
	deck.SetMode(dash.ModeLive)

	form := newSettingsForm()
	form.SetWorkspace(ws)
	form.SetDeck(deck)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	cfg.doc.Set("plugins.test.cascade.mailbox", "Receipts")
	deck.Configure(cfg.doc)
	deck.Refresh(context.Background(), time.Now())
	form.Open(cfg)
	openCategory(t, form, "Cascade")

	if got := fieldNamed(form, "mailbox").choiceValue(); got != "Receipts" {
		t.Fatalf("saved mailbox = %q, want Receipts", got)
	}

	// Choose an account. The panel re-runs with it and reports the other
	// account's folders, which is the cascade the mail plugin offers.
	moveTo(t, form, "account")
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := fieldNamed(form, "account").choiceValue(); got == "" {
		t.Fatal("stepping did not choose an account")
	}
	deck.Refresh(context.Background(), time.Now())
	form.SyncOptions()

	row := fieldNamed(form, "mailbox")
	if !slices.Contains(row.options, "Receipts") {
		t.Errorf("mailbox options = %v, want the saved value still offered", row.options)
	}
	if got := row.choiceValue(); got != "Receipts" {
		t.Errorf("mailbox = %q, want the choice kept", got)
	}
	moveTo(t, form, "mailbox")
	if got := rendered(t, form); !strings.Contains(got, "Receipts ▾") {
		t.Errorf("the row does not draw the value that is saved:\n%s", got)
	}

	// and the list opens on it, so confirming keeps it rather than writing
	// the first option over it.
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !form.editing {
		t.Fatal("enter did not open the list")
	}
	if got := form.editor.Value(); got != "Receipts" {
		t.Errorf("the list opened on %q, want the saved value", got)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := fieldNamed(form, "mailbox").choiceValue(); got != "Receipts" {
		t.Errorf("confirming the list wrote %q over the saved value", got)
	}
}

// rendered draws the settings screen and strips its colour, so a test can read
// what the reader would see rather than what the model holds.
func rendered(t *testing.T, form *settingsForm) string {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
	return ansi.Strip(form.RenderWorkspace(r, 120, 30))
}

// recordingPlugin writes a plugin that appends a line to a log on every run, so
// a test can count what was actually executed rather than what a model says.
func recordingPlugin(t *testing.T, id, title, log string) dash.Manifest {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"schemaVersion":1,"id":"` + id + `","name":"` + title + `","version":"1.0.0",` +
		`"author":"test","description":"records its runs","kinds":["panel"],` +
		`"entryPoints":{"panel":["./run.sh"]},"panel":{"displayName":"` + title + `","refreshSeconds":300,` +
		`"schema":[{"key":"mode","type":"string","label":"mode"}]}}`
	script := "#!/bin/sh\nprintf '%s\\n' run >> " + log + "\n" +
		`printf '%s' '{"rows":[],"options":{"mode":["","a","b","c","d","e"]}}'` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	loaded, err := dash.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// runs counts how many times a plugin has been executed.
func runs(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return strings.Count(string(data), "run\n")
}

// Editing one panel's settings must not throw away every other panel's cache.
// A full Configure forgets every interval, so the next tick re-fetches the
// whole dashboard - news, weather, markets, updates - and the settings screen
// stalls once per typed character. That stall is what made the mail dropdown
// look broken: the frame waited on unrelated sources before the page could
// follow the lists the panel had just reported.
func TestAKeystrokeRefetchesOnlyThePanelBeingEdited(t *testing.T) {
	alphaLog, bravoLog := filepath.Join(t.TempDir(), "alpha"), filepath.Join(t.TempDir(), "bravo")
	alpha := dash.Exec(recordingPlugin(t, "test.alpha", "Alpha", alphaLog))
	bravo := dash.Exec(recordingPlugin(t, "test.bravo", "Bravo", bravoLog))

	deck := dash.New()
	deck.Register(alpha, bravo)
	deck.Attach(tideui.NewWorkspace())
	deck.SetMode(dash.ModeLive)
	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	deck.Configure(cfg.doc)
	deck.Refresh(context.Background(), time.Now())

	form := newSettingsForm()
	form.SetWorkspace(tideui.NewWorkspace())
	form.SetDeck(deck)
	form.Open(cfg)
	openCategory(t, form, "Alpha")

	before, other := runs(t, alphaLog), runs(t, bravoLog)
	if before == 0 || other == 0 {
		t.Fatalf("neither panel ran to begin with: alpha=%d bravo=%d", before, other)
	}

	// Change a setting on Alpha's page, then let the tick do its work.
	moveTo(t, form, "mode")
	form.Update(tea.KeyMsg{Type: tea.KeyRight})
	deck.Refresh(context.Background(), time.Now())

	if got := runs(t, alphaLog); got != before+1 {
		t.Errorf("alpha ran %d times, want %d: the edited panel must re-read its source", got, before+1)
	}
	if got := runs(t, bravoLog); got != other {
		t.Errorf("editing Alpha re-ran Bravo %d times: a keystroke must not invalidate the deck", got-other)
	}
}

// mailDeck registers the real mail plugin on a deck of its own, pointed at a
// database path, and runs it once the way the dashboard would.
func mailDeck(t *testing.T, db string) (dash.Panel, *dash.Deck) {
	t.Helper()
	m, err := dash.LoadManifest("../../contrib/mail")
	if err != nil {
		t.Skip("no mail plugin:", err)
	}
	panel := dash.Exec(m)
	deck := dash.New()
	deck.Register(panel)
	deck.Attach(tideui.NewWorkspace())
	deck.SetMode(dash.ModeLive)

	cfg := defaultConfig()
	cfg.doc = dash.NewValues()
	if db != "" {
		cfg.doc.Set("plugins.tidedeck.mail.db", db)
	}
	deck.Configure(cfg.doc)
	deck.Refresh(context.Background(), time.Now())
	return panel, deck
}

// mailOptions reads the list the settings screen would offer for one of the
// plugin's own settings.
func mailOptions(t *testing.T, deck *dash.Deck, label string) []string {
	t.Helper()
	for _, category := range deck.Schema() {
		if category.PanelID != "tidedeck.mail" {
			continue
		}
		for _, field := range category.Fields {
			if field.Label == label {
				return field.Options
			}
		}
	}
	t.Fatalf("the mail panel declares no %q setting", label)
	return nil
}

// writeMailFixture builds the smallest database this plugin queries, so a test
// can point it at a mailbox of its own instead of the machine's real one: four
// accounts, so a long list opens a picker, with different folders in each, so
// choosing one narrows the mailbox list.
func writeMailFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	sql := `
CREATE TABLE accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0);
CREATE TABLE mailboxes (id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL, name TEXT NOT NULL);
CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, mailbox_id INTEGER NOT NULL, subject TEXT NOT NULL DEFAULT '',
  from_addr TEXT NOT NULL DEFAULT '', date INTEGER NOT NULL DEFAULT 0, read INTEGER NOT NULL DEFAULT 0,
  starred INTEGER NOT NULL DEFAULT 0, has_attachment INTEGER NOT NULL DEFAULT 0);
INSERT INTO accounts (id, name, position) VALUES (1, 'work', 0), (2, 'personal', 1), (3, 'school', 2), (4, 'junk', 3);
INSERT INTO mailboxes (id, account_id, name) VALUES
  (1, 1, 'INBOX'), (2, 1, 'Receipts'), (3, 1, 'Projects'),
  (4, 2, 'INBOX'), (5, 2, 'Sent'), (6, 2, 'Archive'),
  (7, 3, 'INBOX'), (8, 3, 'Courses'),
  (9, 4, 'INBOX'), (10, 4, 'Spam');
INSERT INTO messages (mailbox_id, subject, from_addr, date, read, starred, has_attachment) VALUES
  (1, 'standup notes', 'ana@example.com', strftime('%s','now') - 300, 0, 0, 0),
  (1, 'invoice 41', 'billing@example.com', strftime('%s','now') - 4000, 0, 0, 1),
  (2, 'receipt for the thing', 'shop@example.com', strftime('%s','now') - 90000, 1, 1, 0),
  (4, 'dinner?', 'sam@example.com', strftime('%s','now') - 60, 0, 0, 0),
  (5, 're: dinner?', 'sam@example.com', strftime('%s','now') - 30, 1, 0, 0);
`
	cmd := exec.Command("sqlite3", path)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building an sqlite fixture: %v: %s", err, out)
	}
}

// mailFixtureHome points the plugin's default database location at a fixture
// mailbox. A test that reads whatever mail the machine happens to have passes
// here and fails on a fresh clone, which is where the mail tests were: CI has
// no mailbox at all, so they asserted against nothing.
func mailFixtureHome(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"sqlite3", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	writeMailFixture(t, filepath.Join(data, "tidemail", "mail.db"))
}

// A path is written the way a shell writes it, and a plugin receives it as an
// environment variable, where ~ means itself. A configured
// "~/.local/share/tidemail/mail.db" was therefore a file that did not exist:
// the panel reported "TideMail not set up" with the database right there, and
// because it reported no options either, the account and mailbox settings - the
// two whose values it had just discovered - stayed empty boxes.
func TestMailSettingResolvesATildePath(t *testing.T) {
	for _, tool := range []string{"sqlite3", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	writeMailFixture(t, filepath.Join(home, "mailboxes", "mail.db"))

	t.Run("the lists come from the database the setting names", func(t *testing.T) {
		_, deck := mailDeck(t, "~/mailboxes/mail.db")
		if got := mailOptions(t, deck, "account"); !slices.Contains(got, "work") {
			t.Errorf("account options = %v, want the account the fixture holds", got)
		}
		if got := mailOptions(t, deck, "mailbox"); !slices.Contains(got, "Archive") {
			t.Errorf("mailbox options = %v, want the fixture's folders", got)
		}
	})

	t.Run("a path with no database names the path, not TideMail", func(t *testing.T) {
		panel, _ := mailDeck(t, "~/nowhere/mail.db")
		r := tideui.NewRenderer(tideui.BuiltinThemes[0], tideui.StyleOptions{})
		body := ansi.Strip(panel.View(tideui.PanelContext{ID: "tidedeck.mail", Width: 70, Renderer: r}))
		if !strings.Contains(body, filepath.Join(home, "nowhere", "mail.db")) {
			t.Errorf("the panel does not say which path it looked for:\n%s", body)
		}
		if strings.Contains(body, "TideMail not set up") {
			t.Errorf("a path the reader set is reported as TideMail being absent:\n%s", body)
		}
	})
}
