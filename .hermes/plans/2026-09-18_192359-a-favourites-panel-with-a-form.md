# A favourites panel, with a form to keep it

## Why this shape

The original plan was to read Firefox's `places.sqlite` (918 bookmarks, 848 of them
http, 160 duplicate URLs). That was dropped in favour of a plugin that owns its own list
and ships a form to maintain it. Less machinery, nothing to parse, and no question about
reading a live browser database.

## What the contract already gives us

Verified in this repo, not assumed:

- **A plugin is a program that prints a document.** `dash/manifest.go:37`:
  `EntryPoints map[string][]string` - an entry point is an *argv*, resolved against the
  manifest's own directory, so `["./favourites", "render"]` is a valid panel entry point.
  The program is run with `TIDEDECK_PLUGIN_<KEY>` environment variables, one per declared
  setting (`dash/exec.go:167-176`), and with a timeout (`dash/exec.go:242`).
- **A row with an `id` is openable** (`dash/doc.go:85-89`); the cursor stops only on such
  rows.
- **`open` is "the program that owns the data this panel previews"**
  (`dash/manifest.go:80-84`), run with `{id}` substituted and **with the terminal handed
  to it**. The pane refreshes when it returns - the path the mail panel already takes into
  TideMail.
- **`copy` names the row whose value the copy key takes** (`dash/manifest.go:75-78`).
- **The document vocabulary** is `metric`, `gauge`, `spark`, `text`, `block`, `divider`,
  `spacer` (and `image`), with `label`, `value`, `body`, `tone`, `bodyTone`, `id`,
  `options` (`dash/doc.go:23-101`). `{"schemaVersion":1,"rows":[...]}`.
- **The form controls already exist**: `form.Text`, `form.Number`, `form.Toggle`,
  `form.Choice`, `form.Button`, each a `Control` with `Value/SetValue/Update/View/Editing/
  Err/Hints` (`form/form.go:52-66`), rendering through a `tideui.Renderer` so the editor
  inherits the theme. The screen owns rows, rails and selection; a control draws only its
  value cell.

## Decisions

1. **The plugin owns one file**: `${XDG_DATA_HOME:-~/.local/share}/tidedeck/favourites.json`,
   overridable with the `path` setting, with a leading `~` expanded (the mail plugin sets
   that precedent). Data, not config.
2. **One binary, two entry points**: `favourites render` prints the panel document;
   `favourites edit <id>` opens the form. The form is the only writer.
3. **The list is never a dead end.** The renderer always ends with a row labelled
   `＋ add a favourite` carrying the id `add`, which the editor reads as "blank form".
   Without it, an empty list has no openable row and `enter` can only say "select one
   first" - on a list with nothing to select.
4. **A row is `type: text`**, label = title, value = tags (or the URL's host when there are
   no tags, so the row is never half-empty), id = the URL.
5. **Only `http`/`https` favourites are openable.** Anything else (`javascript:`, `file:`)
   renders as text with a warning tone and gets no id, so `xdg-open` is never handed a
   scheme it should not open.
6. **Duplicate URLs are refused, not silently merged**: saving a URL another entry already
   uses fails with a message naming that entry.
7. **A store that cannot be parsed is never overwritten.** `render` prints a notice row
   naming the file and the parse error; `edit` refuses to start. Losing a hand-maintained
   list to a stray comma is the one failure worth being paranoid about.
8. **Glyph `🔖`** - two cells, colour, like every other pane's.
9. **Read-only from the dashboard's side.** The pane previews; the form edits.

## Tasks

### F1. The store

Files: `contrib/favourites/store.go`, `contrib/favourites/store_test.go`.

Test first:

- `TestStoreRoundTripsFavourites` - a list with two entries saves and loads back equal,
  tags intact, order preserved.
- `TestStoreSavesAtomically` - a save leaves no temporary file behind, and the file it
  writes parses.
- `TestStoreRefusesToReadRubbish` - a file containing `{` returns an error naming the
  path, and the file is still there afterwards, byte for byte.
- `TestStoreExpandsHome` - `~/x.json` resolves against `$HOME`.

Shape:

```go
type Favourite struct {
    Title string   `json:"title"`
    URL   string   `json:"url"`
    Tags  []string `json:"tags,omitempty"`
    Added time.Time `json:"added"`
}

type Store struct{ Path string }

func (s Store) Load() ([]Favourite, error)   // missing file = empty list, no error
func (s Store) Save(list []Favourite) error  // temp file + rename, mode 0600
func ExpandHome(path string) string          // ~ and ~/ prefixes
```

Verify: `go test -count=1 ./contrib/favourites/` → `ok`. Commit:
`Keep a favourites list in one file, and refuse to lose it`.

### F2. The document the pane draws

Files: `contrib/favourites/render.go`, `contrib/favourites/render_test.go`.

Test first:

- `TestRenderPrintsOneRowPerFavourite` - two entries produce two `text` rows whose ids are
  the URLs, with the add row last.
- `TestRenderAlwaysOffersAdding` - an empty list still prints exactly one row, id `add`.
- `TestRenderSkipsNonWebURLs` - a `javascript:` entry keeps its row and loses its id.
- `TestRenderExplainsAStoreItCannotRead` - the bad-file case yields one muted notice row
  naming the path, not an empty document.
- `TestRenderSortsNewestFirst` - the most recently added entry is the first row.

Shape: `func Render(list []Favourite, problem error) Doc` emitting
`{"schemaVersion":1,"rows":[...]}`; `Problem` (if non-nil) becomes a single `warning` text
row and the rows are still printed.

Verify: `go test -count=1 ./contrib/favourites/` → `ok`. Commit:
`Print a row per favourite, and always a way to add another`.

### F3. The form

Files: `contrib/favourites/edit.go`, `contrib/favourites/edit_test.go`.

Test first, driving `Update` with `tea.KeyMsg` values:

- `TestFormStartsBlankForAdd` - `edit add` opens with empty fields and no delete button.
- `TestFormLoadsTheEntryItWasGiven` - a title, URL and tags arrive in the fields.
- `TestFormRefusesAnEmptyTitleOrURL` - saving with either blank reports a problem and
  writes nothing.
- `TestFormRefusesANonWebURL` - `file:///etc/passwd` is rejected with a message naming the
  schemes that work.
- `TestFormRefusesADuplicate` - a URL already in the store fails, naming the entry.
- `TestFormTrimsTagsAndDropsTheEmptyOnes` - `" news , , tech "` becomes `["news","tech"]`.
- `TestFormDeletesOnRequest` - deleting removes the entry and saves.

Shape: a `model` holding `form.Text` controls, a cursor over rows, the store, and the
entry being edited; `Save`/`Delete`/`Cancel` as methods so the logic is testable without a
terminal. Keys: `tab`/`shift+tab` move, `ctrl+s` save, `ctrl+d` delete, `esc` cancel.

Verify: `go test -count=1 ./contrib/favourites/` → `ok`. Commit:
`Give the favourites list a form that edits one entry at a time`.

### F4. The program

Files: `contrib/favourites/main.go`, `contrib/favourites/main_test.go`.

Test first:

- `TestMainRenderPrintsADocument` - `run([]string{"render"})` writes a document that parses
  as JSON with `schemaVersion: 1`.
- `TestMainEditNeedsAnID` - `edit` with no id exits non-zero and says so.
- `TestMainUnknownVerb` - prints the verbs and exits non-zero.

Shape: `func run(args []string, stdout io.Writer, stdin io.Reader) int` so tests never need
a terminal; `main` calls it. `render` prints the document; `edit <id>` builds the store,
runs the bubbletea program with the terminal, then re-renders nothing - the dashboard
refreshes the pane itself.

Verify: `go build ./... && go test -count=1 ./contrib/favourites/` → `ok`. Commit:
`Add the program the panel and the form both run`.

### F5. The manifest

Files: `contrib/favourites/manifest.json`, `contrib/favourites/README.md`.

Manifest: id `tidedeck.favourites`, name `Favourites`, glyph `🔖`, category `Favourites`,
`entryPoints.panel: ["./favourites","render"]`, `open: ["./favourites","edit","{id}"]`,
one string setting `path` (blank = the default location), `refreshSeconds` 60,
`minWidth` 24, `minHeight` 5, `priority` 45.

- `TestManifestIsValid` (in `contrib/favourites/manifest_test.go`) - parses, declares the
  panel entry point, its `open` names `{id}`, and the glyph is two cells wide (the rule
  the mail pane's plain `✉` broke).

README: what it is, the file it owns, the two entry points, how to install
(`go build -o ~/.config/tidedeck/plugins/tidedeck.favourites/favourites ./contrib/favourites`).

Verify: `go test -count=1 ./contrib/favourites/` → `ok`. Commit:
`Describe the favourites panel so the dashboard can run it`.

### F6. End to end, through a pty

Not a commit - evidence.

1. Build the binary and install it into a scratch profile's plugin directory.
2. Drive the dashboard through a pty and confirm the pane's rows, including the add row.
3. Drive `favourites edit add` through a pty: type a title, tab, type a URL, tab, type
   tags, `ctrl+s`.
4. Confirm the store file now holds the entry, and that a fresh `render` shows it as a row
   whose id is the URL.
5. Confirm the dashboard pane shows it after a refresh.

## Risks and notes

- **The editor's theme.** The editor renders through `tideui`; if the dashboard's theme is
  resolved from the config file, the editor should read the same file and fall back to the
  default theme rather than inventing one. If that seam is not importable from a plugin,
  the editor uses the default theme and says so in its README - a mismatch is cosmetic and
  not worth duplicating the theme resolver for.
- **A store file edited by hand** is a supported case: `render` shows whatever is in it,
  and a syntax error is reported rather than swallowed.
- **No locking.** Two forms open at once could lose one save. It is a single-user list and
  the last save wins; a lock file is not worth its failure modes here.

## Notes from implementing it

Six commits on `feat/favourites-panel`. Everything below was checked by running
it, not by reading it.

**The contract is what made this small.** `entryPoints` is an argv, and `open`
resolves against the plugin's own directory, so one binary does both jobs:

```
"entryPoints": {"panel": ["./favourites", "render"]},
"open": ["./favourites", "edit", "{id}"]
```

**The add row is the whole trick.** `render` always ends with a row whose id is
`add`. Without it an empty list has no openable row, `enter` could only ever say
"select one first", and a new reader would have no way in at all.

**Scheme filtering is not theoretical.** The form refuses anything that is not
http or https, naming the scheme it saw, and the renderer only ever puts a web URL
in a row's id - so a stored `javascript:` entry can never reach `xdg-open`.

**Two deliberate deviations from the plan above:**

- A blank title is *not* refused. It is filled in from the site, so saving never
  requires inventing a name; refusing it would be the form being fussy about
  something it can work out itself.
- The form runs on the alternate screen, so the dashboard the reader left is still
  underneath when the form closes.

**The verification that mattered was the pty.** Driving the real binary through a
real terminal showed three things no unit test could:

- the form drew, took the keys, and wrote the file: title, link, two tags trimmed
  out of "news, tech", and a timestamp;
- run a second time against the same list, the duplicate rule appeared in the UI
  itself - `"Hacker News" already saves that link`;
- the dashboard's own loader reads the installed plugin as
  `id="tidedeck.favourites" title="Favourites" glyph="🔖" hidden=true`, which is
  every plugin's starting state: enabled from the panel picker, not by default.

**Two traps worth remembering**, both of which cost a run:

- A TUI asks its terminal questions before it draws - the background colour and
  the cursor position - and a bare pty never answers them. The first run produced
  nothing at all and looked like a crash; it was a form politely waiting.
- A pty has no size until it is given one, and a dashboard with no size draws an
  empty frame.

## After the first use: enter opened the form, and it should not have

The pane went up with one action, so `enter` ran `favourites edit <id>` and the
reader's first instinct - press `enter` on a bookmark - opened the form instead of
the site. One action per pane is not enough for a list you maintain as well as use.

What changed:

- **`panel.open` now points at `./favourites open {id}`**, and the program decides
  what a row's enter key means: a link goes to the browser through `xdg-open` as a
  single argument (never a shell, because the link comes out of a file the reader
  owns), and the add row opens a blank form. One template still serves every row.
- **`panel.edit` is new**, run with the same `{id}` and the same terminal handover,
  and the dashboard binds it to `e` inside an entered pane. A panel that declares no
  edit command leaves `e` to the application, exactly as one that declares no open
  command leaves `enter`. `dash.Editor` is the interface, `Manifest.EditArgv` the
  accessor, and both commands are checked by one validator (`commandProblems`) so a
  command naming no `{id}` is refused whichever key it hangs off.
- **The scheme rule is enforced in two places on purpose**: the renderer gives a
  non-web row no id at all (so the cursor never lands on it), and the program
  refuses one that arrives anyway, naming the scheme.

Verified: the stub-opener run above shows `open <link>` handing exactly the link to
`xdg-open`, and `open javascript:alert(1)` refusing it with the opener never called;
the app's own key path has a test (`e` edits the row under the cursor while `enter`
still opens it), as do the manifest validator, `execPanel.Edit`, and the twelve
plugin tests.
