# Favourites

A pane that previews the links you keep, and a form that keeps them.

```
${XDG_DATA_HOME:-~/.local/share}/tidedeck/favourites.json
```

The list is one plain JSON file, and the form is a convenience rather than the
only way in: edit the file by hand and the pane shows what you wrote.

## What it does

- **`enter`** on a row opens that link in your browser, through `xdg-open`.
- **`e`** on a row opens the form for that entry. Enter acts on a row, `e` changes
  it; the pane previews and the form is the only thing that writes.
- The last row, `＋ add a favourite`, opens a blank form. It is there even when
  the list is empty, which is what stops an empty pane being a dead end: with no
  openable row, `enter` could only ever say "select one first".
- The form has three fields - title, link, tags. `ctrl+s` saves, `ctrl+d`
  deletes, `esc` leaves without saving, and moving with `tab` or the arrows keeps
  what you typed.
- Only `http` and `https` links are openable. A `javascript:` or `file:` entry is
  stored, shown and marked, but carries no row id, so the pane can never hand one
  to a browser opener.
- A blank title is filled in from the site, so saving does not require inventing
  one. A link another entry already uses is refused, naming that entry.
- `favourites path` prints the file it is using, which is the first question when
  a form saves somewhere unexpected.
- The verbs are `render`, `open <link>` (a link to the browser, `add` to the form),
  `edit <link>` and `path`.

## Installing

The panel is a Go program that imports TideDeck's own library packages, so it is
compiled inside a checkout and ships with the app: nothing to do when you install
TideDeck. It is deliberately **not** in the plugin catalogue - the shell plugins
beside it install and run as they are, and this one cannot.

To build and install it by hand, from a checkout:

```
./contrib/favourites/build.sh
mkdir -p ~/.config/tidedeck/plugins/tidedeck.favourites
cp -r contrib/favourites/. ~/.config/tidedeck/plugins/tidedeck.favourites/
```

`build.sh` explains itself if it is run anywhere without a `go.mod` above it, and
the pane does the same until the binary is there.

## Settings

| setting | what it does |
| --- | --- |
| `path` | Where the list is kept. Blank uses `$XDG_DATA_HOME/tidedeck/favourites.json`. A leading `~` is expanded, because a path typed into a settings field is written the way a shell writes one. |

## Notes

- **The file is written atomically** - a temporary file in the same directory,
  renamed over the old one - so a crash half way through leaves the previous list
  intact, and a reader never sees half a file.
- **A list that cannot be parsed is never overwritten.** The pane says so, naming
  the file and the parse error, and the form refuses to start. Losing a list that
  is maintained by hand is the one failure worth being paranoid about.
- **The dashboard's side is read-only.** The pane previews; only the form writes.
- **The form draws in TideDeck's default theme.** A plugin program cannot ask the
  dashboard which theme it resolved, and duplicating a theme resolver per plugin
  is not worth a cosmetic mismatch.
