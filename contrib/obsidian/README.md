# Obsidian

A pane over an Obsidian vault. Obsidian keeps every note as a plain markdown
file on disk, so the panel reads them directly and never needs Obsidian running.

## What it does

- **The pane is one search row.** Until you type, there is no list of notes - a
  note app should not greet you with a wall of filenames. Type and the fuzzy
  matches drop in below.
- **Fuzzy search**: note names and paths match as a subsequence, so `prj` finds
  `Projects/` and `tftp` finds `TideFTP`.
- **`enter` loads the note under the cursor in the editor** - a full-screen
  [Ripple](#requirements) surface: arrows and word motions, selection, undo/redo,
  clipboard copy/cut/paste. `ctrl+s` writes the note and leaves, `esc` leaves
  without saving; in `vim` mode `:w` and `:q` do the same.
- **Notes are written atomically** - a temporary file renamed over the original -
  so a crash half way through leaves the note intact, and only a `.md` file
  inside the vault can be written at all.

## Finding the vault

Obsidian records its vaults in `~/.config/obsidian/obsidian.json` (`$XDG_CONFIG_HOME`
aware). The pane reads that list, so the `vault` setting is a **populated list**
rather than a path to type. Blank means the vault Obsidian has open, then the
first it knows about; a path to a folder Obsidian has not been told about also
works.

## Settings

| Setting | Default | What it does |
|---|---|---|
| `vault` | the open vault | Which vault to search. **A list**, read from Obsidian. |
| `search` | — | What is typed in the pane. This is the input the pane takes. |
| `editor mode` | plain | Ripple in `plain` (conventional) or `vim` mode. |

## Keys

| Where | Key | Does |
|---|---|---|
| Pane | type | filter the vault; the matches appear |
| Pane | `↑`/`↓` | move over the matches |
| Pane | `enter` | load the note in the Ripple editor |
| Editor | `ctrl+s` | save and leave (or `:w` in vim mode) |
| Editor | `esc` | leave without saving (or `:q` in vim mode) |

The pane is entered with `space` first, the way every tidedeck list panel is.

## Requirements

- **Obsidian** for the vault list. The pane still reads and edits notes without
  it if you set `vault` to a path.
- [Ripple](https://github.com/allisonhere/ripple) is compiled into the panel, so
  there is nothing to install for editing.

## Installing

The panel is a Go program that imports TideDeck's own library packages and Ripple,
so it is compiled inside a checkout. It is in the repository's plugin catalogue
(`plugins/index.json`), so the Plugins page can install it:

```
https://github.com/allisonhere/tidedeck#contrib/obsidian
```

To build and install it by hand, from a checkout:

```
./contrib/obsidian/build.sh
mkdir -p ~/.config/tidedeck/plugins/tidedeck.obsidian
cp -r contrib/obsidian/. ~/.config/tidedeck/plugins/tidedeck.obsidian/
```

The editor draws in TideDeck's default theme: a plugin program cannot ask the
dashboard which theme it resolved, and a mismatch is cosmetic.
