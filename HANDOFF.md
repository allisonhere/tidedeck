# Handoff — panel registry migration

Branch `dashboard-panels-and-glyphs`, head `d1c1260`, `go test ./...` green.
Readable version: https://claude.ai/artifact/KcbtET7fmfS71VdHbvXCVh

Every dashboard panel is moving onto the `dash` registry, where a panel's data,
rendering and settings live in one file.

## Where it stands

**Migrated (8):** weather, gpu, updates, clock, system, agenda, git, news.
**Remaining (6):** network, storage, services, tasks, notes, markets.

The old path still stands beside the new one: the closures in
`examples/workspace/panels.go`, the `dataSource` interface and `liveSource` in
`live.go`, `demoFeed` in `feed.go`, and `provider.Dashboard`. Each migration
deletes its own slice of that; the last one deletes the rest.

## Migrating a panel

This order came out of four migrations, and the config step has to come before
anything reads the key.

1. Write `dash/panels/<id>.go`: embed `dash.State[T]`, implement `Meta` and
   `View`, then add `Fetcher`, `Configurable`, `Demoable`, `Ticker`, `Badger`,
   `Actor` as the panel actually needs them. Copy the sizing from the old
   `ws.Panel(...)` call verbatim or the layout moves.
2. Declare `Schema()` with the keys already in `config.json` (`aur_helper`,
   `weather.latitude`, `zones`). Nothing is renamed by a migration.
3. Add it to `deck.Register(...)` in `main.go`; delete the panel's
   `ws.Panel(...)` block from `registerPanels`.
4. Delete the old path for that panel: the `panels.go` closure, the
   `dataSource` method, the `liveSource` method and its `provider.Dashboard`
   field, the `demoFeed` method, any `demoState` fields. Move the demo body
   into the panel's `Demo(now)` rather than deleting it.
5. Remove the hand-written settings category in `settings.go` — or keep only
   what a panel cannot own, as Weather keeps the geocoder. `mergeCategories`
   appends a panel's declared fields to a hand-written category with the same
   `panelID`.
6. Drop the field from the typed `config` struct. `configKeys()` then stops
   claiming the key and `dash.Values` preserves it across load and save.
7. Write `dash/panels/<id>_test.go`: bounded output at widths 4/12/20/40,
   last-good-value on a failed refresh, defaults when keys are absent, and one
   pass through a real `dash.Deck`.

## Traps

- **An absent key is not false.** `Values.Bool` returns `false` for a key that
  was never written, so a setting defaulting to *true* reads as a deliberate
  off on a fresh config. Use `Values.Has` in the panel and
  `Field.Default == "true"` in `loadPanelFields`. Bit the clock and all three
  weather booleans.
- **Numbers stay numbers.** `FieldFloat` is parsed and written back as a JSON
  number in `applyPanelFields`; a typo is refused by field name.
- **Presets declare what they *hide*.** A panel neither placed nor listed shows
  up in every preset. `TestNewPanelsArePlacedOrHiddenInEveryPreset` catches it.
- **A failed fetch keeps the last value.** Return the error and store nothing.
  A provider meaning "nothing to update" returns `(zero, nil)`.
- **One key per field.** A panel's schema maps one setting to one config key.
  The news catalogue is a tick list plus free text over the single
  comma-separated `feeds` key, which that model cannot express, so the News
  category stays hand-written and the panel only reads `feeds` in `Configure`.
- **`tideui` can never import `provider`** — `provider` imports `tideui`, so
  `dash` sits below both.
- **`Deck.Refresh` is synchronous.** Bounded at 12s per panel, but run in the
  calling goroutine; it belongs in a `tea.Cmd`.

## Open work

- **`deck.Refresh` runs on the UI goroutine** — `examples/workspace/main.go:448`
  calls it inline on the tick, so an updates fetch stalls the frame for up to
  12s. Move it into a command.
- **Invert the layout presets** so each declares what it shows.
- **Delete the old collector** once the last panel is across:
  `provider.Dashboard`, `Snapshot`, `dataSource`, `liveSource`, `demoFeed`. The
  per-source `provider.X()` constructors stay.
- **Plugins are built but not wired**: `dash.LoadPlugins` is never called by the
  app, and `cmd/tideplug validate` was planned and never written.
- **Git's `r` key is fixed** — migrating the panel gave it
  `dash.Action{Refresh: true}`, so it refetches instead of only claiming to.
- **Allie's config still says `Sydney`**, which is not an IANA name, so the
  clock resolves two zones. The shipped default is now `Australia/Sydney`;
  existing configs keep their value on purpose.
- Offered, never built: RRULE expansion for recurring events, `repo_root`
  auto-discovery for the git panel.

## Checking the work

```
go test ./... && go vet ./... && gofmt -l .
```

No new dependencies — `provider` is standard-library only by design.

Two habits caught what tests did not:

- **Run the real binary in a real terminal.** Bubble Tea needs a controlling
  TTY: `script -q -c "stty rows 45 cols 150; ./ws" out.txt`, then strip escapes
  and read the frame. This is how live weather was confirmed against Austin.
- **Prove a new test fails without its fix.** Revert, watch it go red, restore
  *from a copy* — `git checkout` takes uncommitted work with it.

The README's plugin examples are held to the format by `dash/readme_test.go`,
which extracts the JSON, validates the manifest and renders the document.

Next panel: network — one setting, a badge, and a provider fetcher; then
the list panels (storage, services, tasks, notes, markets).
