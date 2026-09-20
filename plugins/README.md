# Plugin catalogue

`index.json` is the machine-readable list of the plugins bundled with tidedeck,
so a site (or a script) can discover and describe them without re-reading every
manifest. The plugins themselves live in `contrib/`; this folder only points at
them, so there is one copy of each plugin and no second place to update.

It is generated, never hand-edited:

```bash
go run ./cmd/plugincatalog
```

That reads every `contrib/*/manifest.json`, builds one entry per plugin, and
writes `index.json`. `dash`'s `catalog_test.go` rebuilds it the same way and
fails whenever the file and the manifests drift, so a plugin added or renamed
without regenerating the catalogue is a failing test rather than a stale site.

## What an entry carries

| Field | Meaning |
|---|---|
| `id` | Namespaced plugin id, e.g. `tidedeck.mail`. |
| `name`, `displayName` | The plugin's name and the title its pane draws. |
| `version`, `author`, `license`, `description` | From the manifest. |
| `glyph`, `category`, `kinds` | How the pane presents itself. |
| `source` | The plugin's directory in this repository, e.g. `contrib/mail`. |
| `install` | The source a reader pastes into settings → Plugins: `repository#source`. |
| `refreshSeconds`, `minWidth`, `minHeight`, `priority` | Placement and cadence. |
| `gauge`, `spark` | Whether the document carries those metric row styles. |
| `settings` | Each declared setting's key, type, label and description. |

## Install sources

Every `install` value is the `repository#subdir` form the Plugins page accepts,
for example:

```
https://github.com/allisonhere/tidedeck#contrib/mail
```

A source names the directory inside the repository that holds one plugin, so
this repository can host all of them.
