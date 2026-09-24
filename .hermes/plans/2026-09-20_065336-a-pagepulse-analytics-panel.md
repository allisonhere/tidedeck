# A PagePulse panel: who is on, and what they are reading

## Why this shape

PagePulse is your own analytics app — PHP 8 + MySQL on shared hosting, server-rendered
reports, no JSON read API. tidedeck already previews another app by reading a local
cache (`contrib/mail` opens TideMail's SQLite read-only), but PagePulse's production
database is remote MySQL that only the host's PHP can reach, so a plugin cannot query
it directly. The display therefore needs one small, read-only JSON endpoint on
PagePulse, and a tidedeck plugin that projects it into a panel document.

Two repos, two thin pieces:

- `~/Projects/PagePulse` — `public_html/api.php` + `app/api.php`, an authenticated
  read-only snapshot built from the report functions that already exist.
- `~/Projects/tidedeck/contrib/pagepulse/` — a Go program (`render` / `sites` /
  `open`), a manifest and a README, following `contrib/favourites`.

## What already exists (verified in both repos, not assumed)

PagePulse:

- The report functions are pure, taking `(siteId, range)`: `pp_summary`
  (`app/reports.php:85`), `pp_top` (`app/reports.php:303`), `pp_trend`
  (`app/reports.php:113`), `pp_live_visitors` (`app/reports.php:695`), and the range
  pair `pp_range` (`app/reports.php:15`) / `pp_prev_range` (`app/reports.php:65`).
- `live.php` is exactly the shape a JSON endpoint takes: bootstrap, security headers,
  `Content-Type: application/json`, `Cache-Control: no-store`, a guard, `json_encode`
  (`public_html/live.php:1-16`).
- `cron.php` shows the machine-key check: `hash_equals(PP_CRON_KEY, $_GET['key'])`
  (`public_html/cron.php:17`), over an HTTP path that is CLI-or-key (`cron.php:9-21`).
- Config constants are defined in `app/config.sample.php`; the live `app/config.php`
  is gitignored and kept out of the rsync payload (`README.md:126`).
- Tests are plain PHP over SQLite `:memory:` with an `ok()` counter
  (`scripts/tests.php:6-42`).
- `sites` rows carry id, name, domain, timezone (`app/db.php:57`;
  `public_html/index.php:20`), and the per-request dashboard already computes
  `$summary`, `$previous`, `$trend` and `$liveCount` (`public_html/index.php:105-110`).

tidedeck:

- A plugin is an argv entry point resolved against its own directory, run with
  `TIDEDECK_PLUGIN_<KEY>` environment variables (`dash/manifest.go:34`,
  `dash/exec.go:167-176`), printing a `dash.Doc` (`dash/doc.go:23`).
- `open` / `edit` / `copy` are declared argv templates carrying `{id}`
  (`dash/manifest.go:75-90`); a manifest naming no `{id}` is refused.
- Bundled plugins go through the real loader and must wear a two-cell colour glyph
  (`dash/bundled_test.go:15-55`).
- `contrib/favourites` is the Go precedent: one binary with `render`/`open` verbs, a
  `run()` seam so every path is testable without a terminal
  (`contrib/favourites/main.go:39`), a `run.sh` that explains a missing binary rather
  than failing silently, and a `build.sh` that builds inside this module
  (`github.com/allisonhere/tideui`, `go.mod:1`).
- The root README's plugin examples are compiled and rendered by
  `dash/readme_test.go`.

## Decisions

1. **PagePulse gains a read-only JSON API**, not a second data path. It reuses the
   report functions above and never writes.
2. **Auth is a dedicated `PP_API_KEY`** compared with `hash_equals`, accepted as
   `Authorization: Bearer <key>` or `?key=`. If the constant is undefined the
   endpoint answers `503 {"error":"api disabled"}` — "not switched on" is a different
   message from "wrong key", and the plugin draws a different row for each.
3. **Two resources.** `resource=sites` returns id/name/domain/timezone for the
   `site` setting's list; `resource=summary` returns everything the panel draws. The
   plugin makes the summary call to render and the sites call only to populate
   `options`.
4. **The panel is read-only.** v1 declares `open`, not `copy` or `edit`. A row's id
   is an absolute https URL — the tracked page, or the PagePulse dashboard — and one
   `open` verb opens either after a scheme check, the favourites rule.
5. **The panel is never blank.** A missing URL or key, a 401, an unreachable host or
   unparseable JSON each render one warning row naming the failure plus a muted block
   with the tool's own words, and a badge that says so.
6. **`site` is a populated list** (`options.site`), the mail precedent: a manifest is
   static JSON written before the plugin was installed and cannot know the sites; the
   program does, every run.
7. Glyph `📈`, category `Analytics`, `priority` 45, `refreshSeconds` 60 — the API is
   `no-store` and the live count is the point.
8. **Values are formatted once, in Go**: thousands separators, `▲/▼ n%` deltas, `·`
   between counts. The pane is a display, not a spreadsheet.

## Tasks

### Part A — the PagePulse read API

#### A1. The pure payload builder

Files: `PagePulse/public_html/app/api.php`, `PagePulse/scripts/tests.php`.

Test first, appended to `scripts/tests.php` after the existing sections, reusing the
seeded `events` rows and the `ok()` counter:

- `TestApiAuthorizedByHeaderOrQuery` — the exact key passes as a Bearer header and as
  `?key=`; a wrong or absent key fails.
- `TestApiDisabledWithoutAKey` — with no `PP_API_KEY` defined the answer is 503, and
  its body says `api disabled`.
- `TestApiSitesPayload` — the seeded sites come back with id, name, domain, timezone,
  and never the write-key hash.
- `TestApiSummaryPayload` — for a seeded site and `7d`: `live`, `summary`, `previous`,
  bounded `topPages`, and a zero-filled `trend` whose length matches the range.
- `TestApiUnknownSiteIs404` and `TestApiUnknownPresetFallsBack` — a bad `site` is
  404, a bad `preset` is treated as `7d` rather than erroring.

Shape, all pure and returning `['status' => int, 'body' => array]`:

```php
function pp_api_key(): ?string;                       // defined PP_API_KEY, or null
function pp_api_authorized(array $server, ?string $key, array $query): bool;
function pp_api_sites(array $sites): array;           // resource=sites
function pp_api_summary(array $site, string $preset, array $range, array $prev): array;
```

`pp_api_summary` calls `pp_summary`, `pp_top(..., 'pages', 8)`, `pp_top(..., 'sources',
5)`, `pp_trend` and `pp_live_visitors`; it does not re-implement SQL.

Verify: `php scripts/tests.php` exits 0. Commit:
`Answer with a read-only snapshot of the reports, behind a key`.

#### A2. The HTTP front, the key, and the docs

Files: `PagePulse/public_html/api.php`, `PagePulse/public_html/app/config.sample.php`,
`PagePulse/README.md`, `PagePulse/docs/security.md`.

- `public_html/api.php` is thin: `bootstrap.php`, `pp_security_headers()`, JSON and
  `no-store` headers, load `sites`, dispatch to A1 by `resource`, echo
  `json_encode($body)` with the status (`live.php:6-16` is the template).
- `config.sample.php` gains `define('PP_API_KEY', 'change-me');` beside `PP_CRON_KEY`
  (`config.sample.php:18`), with the one-liner to generate a real value:
  `php -r 'echo bin2hex(random_bytes(32)), "\n";'`.
- `README.md` documents the endpoint, its two resources and the key; `security.md`
  notes it is read-only, keyed, and that the key travels in a header by preference.

Verify: run `php -S 127.0.0.1:8899` over SQLite, seed with
`php scripts/seed.php 1 30 120`, then
`curl -H 'Authorization: Bearer <key>' '127.0.0.1:8899/api.php?resource=summary&site=1&preset=7d'`
and a `resource=sites` call; confirm a wrong key is 401. Commit:
`Reach the reports over JSON without a session`.

### Part B — the tidedeck plugin

#### B1. The client

Files: `contrib/pagepulse/client.go`, `contrib/pagepulse/client_test.go`.

Test first, against `net/http/httptest` — no network:

- `TestClientParsesASummary` — every field arrives, top pages in order.
- `TestClientReportsItsStatus` — a 401 names the key; a 500 and a non-JSON 200 both
  name the status; none of them panic on an empty body.
- `TestClientPassesThroughANetworkError` — a closed server is an error, not an empty
  snapshot.
- `TestClientListsSites` — `resource=sites` parses.

Shape:

```go
type Client struct{ BaseURL, Key string; HTTP *http.Client }
func (c Client) Summary(ctx context.Context, site, preset string) (Snapshot, error)
func (c Client) Sites(ctx context.Context) ([]Site, error)
```

Verify: `go test -count=1 ./contrib/pagepulse/` → `ok`. Commit:
`Ask PagePulse for a snapshot, and say what went wrong`.

#### B2. The document the pane draws

Files: `contrib/pagepulse/render.go`, `contrib/pagepulse/render_test.go`.

Test first:

- `TestRenderDrawsThePulse` — live becomes the badge and an accent metric; visitors,
  views and sessions become text rows carrying `▲/▼` deltas.
- `TestRenderNormalisesTheSpark` — the trend buckets become a `spark` row whose
  history is 0..1, downsampled when longer than ~30 buckets.
- `TestRenderListsHotPages` — each top page is a row whose id is
  `domain + pathname`, most-visited first.
- `TestRenderListsSources` — the source rows, muted.
- `TestRenderWithoutSettings` — no URL, or no key, yields one warning row naming what
  is missing and a badge that says so, never an empty document.
- `TestRenderOnAFailedCall` — an API error yields a warning row plus a muted body
  carrying the error text.
- `TestRenderAlwaysOffersTheDashboard` — the document ends with a row whose id is the
  dashboard URL.

Shape: `func Render(snapshot Snapshot, sites []Site, dashboard string, problem error)
dash.Doc`, emitting `dash.DocSchemaVersion` with `Options["site"]` from `sites`.

Verify: `go test -count=1 ./contrib/pagepulse/` → `ok`. Commit:
`Draw the pulse: who is on now, and the hot page`.

#### B3. The program

Files: `contrib/pagepulse/main.go`, `contrib/pagepulse/main_test.go`,
`contrib/pagepulse/run.sh`, `contrib/pagepulse/build.sh`.

Entry points, mirroring `contrib/favourites`:

```
pagepulse render        print the panel document
pagepulse sites         print only the site list, as options
pagepulse open <url>    open a http/https URL in the browser
```

Test first, with a fake client injected through `run`:

- `TestMainRenderPrintsADocument` — parses as JSON with `schemaVersion: 1`.
- `TestMainWithoutUrlSaysSo` — the missing setting becomes the warning row, exit 0
  (the pane must draw the explanation).
- `TestMainOpenRefusesNonWebSchemes` — `javascript:alert(1)` is refused with the
  opener never called (the favourites rule, `contrib/favourites/render.go:99`).
- `TestMainUnknownVerb` — prints the verbs and exits 2.

`run.sh` mirrors `contrib/favourites/run.sh:9-28`: when the binary is absent, `render`
prints a "not built yet" document and everything else says to run `build.sh`.
`build.sh` mirrors `contrib/favourites/build.sh`.

Verify: `go build ./... && go test -count=1 ./contrib/pagepulse/` → `ok`. Commit:
`Add the program the panel runs`.

#### B4. The manifest, glyph and README

Files: `contrib/pagepulse/manifest.json`, `contrib/pagepulse/manifest_test.go`,
`contrib/pagepulse/README.md`.

Manifest: id `tidedeck.pagepulse`, name `PagePulse`, glyph `📈`,
`entryPoints.panel: ["./run.sh","render"]`, `open: ["./run.sh","open","{id}"]`,
`refreshSeconds` 60, `minWidth` 24, `minHeight` 5, `priority` 45, and four settings:
`url` (string, `https://stats.alliehere.com` as the placeholder), `key` (string),
`site` (string, populated by `options`), `preset` (choice: today / yesterday / 7d /
30d / month, default `7d`).

- `TestManifestIsValid` — parses, declares the panel entry point, its `open` names
  `{id}`, the site row is a string (so options may promote it), and the glyph is two
  cells wide.

README: what the panel shows, the PagePulse endpoint it needs (and the `PP_API_KEY`
this requires), the failure states, and how to build and install.

Verify: `go test -count=1 ./contrib/pagepulse/ ./dash/` → `ok`. Commit:
`Describe the PagePulse panel so the dashboard can run it`.

#### B5. End to end, through a pty

Not a commit — evidence.

1. Run PagePulse locally with `PP_API_KEY` set (`php -S 127.0.0.1:8899` + seed), then
   `render` against it: the real rows, live count, hot pages, dashboard row.
2. Install into a scratch profile's plugin directory and drive the dashboard through
   a pty; read the frame and confirm the pane's rows.
3. Stop the server; confirm the pane shows the unreachable warning and keeps its
   shape. Repeat with a wrong key and with `PP_API_KEY` removed, and confirm the three
   read differently.
4. Confirm `enter` on a hot-page row hands `xdg-open` exactly the composed URL, and
   that the dashboard row opens PagePulse.

## Risks and notes

- **The key lives in `~/.config/tidedeck/config.json`** under
  `plugins.tidedeck.pagepulse.key`, in plaintext. It is read-only and scoped to
  PagePulse; rotate it in `config.php` if it leaks. There is no password-style field
  in the settings screen, and adding one is out of scope.
- **`PP_API_KEY` must be added to the live `config.php` by hand** — it is
  deploy-excluded (`README.md:126`). The 503 path is what makes that discoverable from
  the pane rather than from a log.
- **The endpoint has no rate limit.** Reuse `rate_limits` if that becomes real; not
  v1. It is behind the host's own auth-less URL, so the key is the only gate — the
  same trade `cron.php` already makes.
- **A 30-day trend is 30 buckets and "today" is 24.** `pp_trend` zero-fills, so the
  spark downsamples by summing adjacent buckets; a single-bucket day shows one point
  and the row should degrade to a `text` rather than a flat line.
- **Absolute page URLs are composed from the site domain.** `pp_top` returns paths
  only (`app/reports.php:305`); a site tracked across a subpath is still
  `domain + pathname`. A domain served over http not https is rare; the scheme check
  refuses anything that is neither.
- **Two HTTP calls per render** (summary + sites). At a 60-second interval the sites
  call is negligible; if it ever matters, fold the list into the summary response.
- **This is a two-repo change.** PagePulse must be deployed before the plugin can be
  pointed at it, which is the ordering B5 verifies.
