# Obsidian

A pane over an Obsidian vault. Obsidian keeps every note as a plain markdown
file on disk, so the panel reads them directly and never needs Obsidian running.

## What it does

- **The pane reads a note, not a list.** It shows the note you last loaded -
  or, before that, the most recently edited one - with its body under it. The
  search row sits on top, so there is always one place to type.
- **Type to search, and the matches drop in** below: each with its folder and a
  line of its body, so two notes with similar names can be told apart. Matching
  is fuzzy over names and paths, so `prj` finds `Projects/` and `tftp` finds
  `TideFTP`.
- **`enter` loads the note** the cursor is on into the pane, and the search
  clears, so you are reading it. **`e` edits it** in a full-screen
  [Ripple](#requirements) editor: arrows and word motions, selection,
  undo/redo, clipboard. `ctrl+s` saves, `esc` leaves without saving; in `vim`
  mode `:w` and `:q` do the same.
- **A new note is one row away.** While searching, the last row offers
  `＋ new note` named after what you typed; enter makes it and opens the editor,
  because a note that does not exist yet has nothing to read.
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
| `vault` | the open vault | Which vault to read. **A list**, read from Obsidian. |
| `search` | — | What is typed in the pane. This is the input the pane takes. |
| `new note folder` | vault root | Where `＋ new note` creates a note, relative to the vault. |
| `editor mode` | plain | Ripple in `plain` (conventional) or `vim` mode. |

## Keys

| Where | Key | Does |
|---|---|---|
| Pane | type | search the vault; the matches appear |
| Pane | `↑`/`↓` | move over the matches or the current note |
| Pane | `enter` | load the note under the cursor (or make a new one) |
| Pane | `e` | edit the note under the cursor in Ripple |
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
