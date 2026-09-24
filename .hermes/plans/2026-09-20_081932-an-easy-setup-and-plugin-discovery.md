# An easy setup, and plugins you can find

## Why this shape

The dashboard is not yet something a person can install, launch, and understand
in a minute. It exists as `examples/workspace`, run with
`go run ./examples/workspace` (`examples/workspace/main.go:1`), under a module
named `github.com/allisonhere/tideui` while the repository is
`github.com/allisonhere/tidedeck`. There is no release, no binary, no app
README, and no first-run experience. A plugin is installed by pasting a git URL
into a text box (`examples/workspace/settings.go:895`) and then enabling it, and
it **starts hidden** (`dash/exec.go:80`), so even a successful install looks
like nothing happened.

Discovery is half-built. `plugins/index.json` exists and is generated from every
bundled manifest by `go run ./cmd/plugincatalog`, and `dash/catalog.go` defines
its schema — but nothing in the app reads it. It is a file for a site that does
not exist yet.

This plan starts the smallest useful version of both: make the app installable
and welcoming, then make the catalogue it already ships readable and
installable from inside the app.

## What exists (verified in the repo, not assumed)

- **The app**: `examples/workspace/main.go`, package `main`, entrypoint comment
  "Run with: go run ./examples/workspace". Config and plugins live under
  `~/.config/tidedeck` (`examples/workspace/config.go:108`); defaults are a Go
  value (`config.go:88`).
- **It already starts without config**: `loadConfig` falls back to defaults when
  the file is missing or unreadable (`examples/workspace/config.go:115`). So
  there is a first launch, but its only surface is a settings screen (`s`).
- **Plugins**: `dash.Install` clones/copies and validates (`dash/install.go:49`),
  `dash.Installed` lists (`:94`), `Remove`/`Update` (`:127`, `:138`),
  `LoadPlugins` discovers on startup (`dash/exec.go:432`). The settings page is a
  paste box plus per-plugin update/remove rows (`settings.go:895`), and the
  install runs off the UI goroutine (`settings.go:946`).
- **The catalogue**: `dash.Catalog`/`CatalogEntry` with a schema version, one
  entry per `contrib/*` manifest (`dash/catalog.go:23`), written to
  `plugins/index.json`; `dash/catalog_test.go` fails when it drifts. Entries
  carry identity, pane presentation, placement, settings, and an `install`
  source of the form `repository#subdir`.
- **Install sources**: `parseSource` accepts `source#subdir` with several
  segments (`dash/install.go:182`); `fetchSource` git-clones a URL or copies a
  local path (`dash/install.go:196`).
- **CI**: tests and vet only (`.github/workflows/ci.yml`). No release workflow,
  no Makefile, no binary name.

## Decisions

1. **The app gets a real identity and a command.** A `cmd/tidedeck` (or an
   equivalent `go build -o tidedeck`) with `--version`, `--config`, `--demo`,
   `-h`, and an app README written for a person, not an embedder. This is the
   prerequisite for every other step: without a binary there is nothing to
   release, sign, or point a site at.
2. **First run is a screen, not a silent default.** A one-time welcome overlay
   that says what the app is, the three keys that matter (`s` settings, `space`
   panels, `?` help), the live/demo switch, and a way into the plugin browser.
   Dismissed state is remembered in config.
3. **Discovery is in-app, from a catalogue the app already ships.** The Plugins
   page gains a browser that reads `plugins/index.json`, with search and
   category grouping; selecting an entry installs it, one step, no URL to paste.
4. **The catalogue grows fields a browsable list needs**: `tags`, `homepage`,
   `readme`, `screenshot` (a repo-relative path), and later `verified` and
   `minAppVersion`. It stays generated from manifests; tags/homepage live in the
   manifest so a plugin describes itself once.
5. **Third-party plugins are one more catalogue, not a second mechanism.** A
   remote `plugins.json` (a URL in settings) uses the same schema and the same
   `install` source. The bundled file is the offline default; the remote file is
   fetched, validated, cached, and ignored on failure.
6. **Discovery never hides the trust boundary.** Plugins run unsandboxed with
   the user's permissions (README:1366). The browser shows the author, the
   source, and what the plugin asks for (settings, open/edit commands) before an
   install, and the install row is a deliberate keypress, not a hover.

## Tasks

### Phase 1 — An installable, welcoming app

#### 1.1 A real command

Files: `cmd/tidedeck/main.go`, `examples/workspace/README.md`, root `README.md`.

- The command is the current dashboard: either move the model into an importable
  package (`examples/workspace` → `deckapp`, with `cmd/tidedeck` a thin `main`)
  or build `./examples/workspace` as `tidedeck`. Moving is cleaner and stops the
  app from being "an example"; do it behind one commit that only moves code.
- Flags: `--version` (module version or build stamp), `--config <dir>`, `--demo`
  (force the fake feed), `--plugins <dir>`.
- `-h` prints what the app is and the main keys.
- The app README is user-facing: install, run, the keys, where config lives,
  how to add a plugin. The root README is the library's and stays separate.

Verify: `go build ./... && go test ./...`; `./tidedeck --version` and `-h` print
without a terminal. Commit: `Ship the dashboard as a command, with a README for
people who run it`.

#### 1.2 Releases and install

Files: `.goreleaser.yaml`, `.github/workflows/release.yml`, `install.sh`.

- goreleaser builds `linux/amd64`, `linux/arm64`, `darwin/*`, `windows/*`, with
  a checksums file and a version stamp from the git tag.
- The release workflow runs on `v*` tags and attaches archives.
- `install.sh` is `curl | sh` friendly: detect OS/arch, download, verify the
  checksum, install to `~/.local/bin`.
- Decide and record the module-path question: the module is
  `github.com/allisonhere/tideui` and the repo is `.../tidedeck`. `go install`
  and the module proxy need these to agree. Either rename the module to match
  the repo, or split the app into its own module under `cmd/tidedeck`. This is a
  decision, not a task to guess at.

Verify: goreleaser `--snapshot` builds locally; the workflow is checked in CI on
a dry run. Commit: `Build and publish a tidedeck binary`.

#### 1.3 First run

Files: `examples/workspace/welcome.go` (new), `config.go`, `main.go`,
`settings.go`.

- On start, if config has never been written and the plugins dir is empty, draw
  a centered soft-panel welcome: what the app does, the key map, a theme choice
  drawn live, live/demo, and two buttons — **Open settings** and **Browse
  plugins** — plus **Start**.
- Persist `seen_welcome` in config so it appears once.
- Keep it a document like everything else, so it is testable by rendering a
  frame at 80x24 and reading it.

Verify: a cascade test renders the first frame with no config and asserts the
welcome text and the three keys; a second run with `seen_welcome` renders the
dashboard. Commit: `Welcome a first-time reader instead of dropping them into a
dashboard`.

### Phase 2 — Plugins you can find

#### 2.1 A catalogue a browser can read

Files: `dash/catalog.go`, `dash/catalog_test.go`, `dash/manifest.go`,
`cmd/plugincatalog/main.go`, `plugins/index.json`, `contrib/*/manifest.json`.

- Add to `CatalogEntry`: `Tags []string`, `Homepage string`, `Readme string`
  (repo-relative), `Screenshot string`, `MinAppVersion string`.
- Source the new fields from `PanelManifest` (`tags`, `homepage`, `readme`,
  `screenshot`, `minAppVersion`) so a plugin describes itself once and the
  generator only projects it.
- `Readme`/`Screenshot` are resolved by checking the file exists beside the
  manifest, so the catalogue cannot point at nothing.
- Regenerate; the drift test keeps it honest.

Verify: `go run ./cmd/plugincatalog && go test ./dash/`. Commit: `Let a plugin
describe itself well enough for a browser`.

#### 2.2 An in-app plugin browser

Files: `examples/workspace/settings.go`, `dash/catalog.go`, new
`examples/workspace/pluginbrowser.go`, tests.

- A new "Browse plugins" action on the Plugins page opens a list of catalogue
  entries: glyph, display name, author, one-line description, tags.
- Search (`/`), category grouping (`←/→` or tag chips), and a detail view that
  shows the description, the settings it declares, the source, and the author
  before install.
- `Enter` installs through the existing `dash.Install` path and the existing
  off-thread `pluginOp`; the row then reads "installed" and the panel appears in
  the picker, still hidden until enabled.
- Already-installed entries show update/remove instead of install.

Verify: a test builds a catalogue fixture, drives the browser's `Update` with
keys, and asserts the queued `pluginOp`; a rendered frame asserts a search
result. Commit: `Browse and install plugins from inside the dashboard`.

#### 2.3 A remote catalogue

Files: `examples/workspace/settings.go`, `examples/workspace/config.go`,
`dash/catalog.go`, `provider/` (a fetch helper), tests.

- A `catalog_url` setting (blank = bundled only). Fetch with a timeout, validate
  `schemaVersion` and every entry, and cache to
  `~/.cache/tidedeck/catalog.json`; on failure use the cache, then the bundled
  file. Never block startup.
- The browser merges bundled and remote entries, newest version winning, and
  marks the source of each.

Verify: a test serves a catalog over `httptest`, checks merge and a bad-schema
refusal; offline falls back to cache. Commit: `Read a plugin catalogue from a
URL, and keep the last good one`.

### Phase 3 — Setup that meets the machine

#### 3.1 Environment check

Files: `examples/workspace/welcome.go`, a new `provider/doctor.go`, tests.

- A short checklist with plain answers: terminal colour (truecolor / 256 / none),
  box-drawing and emoji width support, `git`, `docker`, `sqlite3`, `jq`, the
  OS/AUR helper, and the config/plugins dirs.
- Each item is pass/info/fail with one line of what to do; failures never block
  the app.
- Offer to set `icons`/`PlainUI` from the result, since a font without emoji is
  the most common first-run surprise.

Verify: table-driven `provider` tests for the parsing; a rendered welcome frame
asserts the colour row. Commit: `Tell a new reader what their terminal can do`.

#### 3.2 Recommended setup

Files: `examples/workspace/welcome.go`, `dash/panels` detection, tests.

- From the doctor's answers, offer a "recommended" set of panels (no Docker →
  skip the containers panel; no GPU → skip GPU) and apply it to the layout on
  one keypress.
- Reuse the existing preset mechanism rather than a second layout path.

Verify: a test feeds a doctor result and asserts the chosen panel ids. Commit:
`Offer a layout that fits the machine, not the demo`.

### Phase 4 — Publishing, so discovery stays alive

#### 4.1 A plugin index repository

Files: in `allisonhere/tidedeck-plugins` (or a new `plugins` branch): one
directory per plugin, plus a generated `plugins.json`.

- The same manifest shape; the same `cmd/plugincatalog` generator, pointed at
  the repo root, writing `plugins.json` at the publish URL.
- A submission template (a PR with one directory) and a CI job that validates
  every manifest and refuses drift between `plugins.json` and the directories.

Verify: the CI job is the contract; a test in the repo runs `plugincatalog`
in check mode. Commit: `Publish a validated plugin index`.

#### 4.2 Point the app at it

Files: `config.go` default `catalog_url`, docs.

- Default to the published URL once it exists; keep the bundled catalogue as the
  offline floor.

Verify: the remote-catalogue test plus a documented override. Commit: `Discover
the published index by default`.

### Phase 5 — A site (later, not this plan)

A static page generated from the same `plugins.json` (search, categories,
per-plugin pages, copyable install string), hosted on Pages. The catalogue is
the single source; the site is a renderer of it. Out of scope until Phase 2's
schema is settled.

## Risks and notes

- **Module identity is the first decision, not a later cleanup.** `go install`
  and the module proxy need the declared path to resolve to the repository. Pick
  "rename the module to `tidedeck`" or "split the app into its own module"
  before building releases.
- **Discovery widens an unsandboxed surface.** A stranger's plugin can do
  anything the user can. The browser must show the author and source, and the
  install must be an explicit keypress. Do not add one-key "install all".
- **A remote catalogue is a new network dependency.** Validate the schema, cap
  the size, cache the last good copy, and never let it delay or fail startup.
- **Moving `examples/workspace` touches many tests.** Do the package move as a
  pure move first, with no behaviour change, so a green suite proves it.
- **The catalogue is generated, so it can only be as rich as manifests.** Tags
  and screenshots have to live in the manifest — one place to edit, and the
  drift test keeps the projection honest.
- **Plugins start hidden by design** (`dash/exec.go:80`). The browser should
  say so at the moment of install, or the first install will look like it did
  nothing.

## The first three commits

1. `cmd/tidedeck` + `--version`/`-h` + a user README (no behaviour change).
2. `dash.CatalogEntry` gains `tags`/`homepage`/`readme`/`screenshot`, sourced
   from the manifest; regenerate; drift test.
3. Read-only in-app Plugins browser over the bundled catalogue, with install
   wired to the existing `pluginOp`.
