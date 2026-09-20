# tidedeck.pagepulse

A PagePulse panel for TideDeck: who is on your site right now, the page they are
reading, and how the period went. It draws the live visitor count as the pane's
badge, the totals against the period before, a spark of the traffic, the hot
page, the next few pages, and the top sources.

## How it gets the numbers

It calls PagePulse's read-only JSON API — `api.php` — which returns the same
report data the PagePulse dashboard renders. The pane and the site therefore
cannot disagree about what "7 days" means.

PagePulse's production database is remote MySQL that only the host's PHP can
reach, so a plugin on your machine cannot query it directly. The API is the one
seam, and it is read-only: the panel never writes, and the key it holds opens
nothing but the reports.

### Setting up PagePulse

In PagePulse, open **Settings** from the topbar and choose **Generate key** under
*Read API keys*. The key is shown once; paste it into the plugin's API key
setting. Keys are stored hashed, can be revoked there, and with no keys the API
is switched off. The PagePulse repository's `docs/security.md` says what the
endpoint does and does not expose.

## Settings

| Setting | Default | What it does |
|---|---|---|
| `PagePulse URL` | — | Where your install lives, for example `https://stats.example.com`. |
| `API key` | — | A read key generated on PagePulse's Settings page. |
| `site` | the first site | Which site to show. **A list**, read from PagePulse. |
| `period` | 7 days | The reporting period: today, yesterday, 7 days, 30 days, this month. |

`site` and `period` are what the panel asks the API for; `site` is offered as a
list once the panel has run, because a manifest written before your account
existed cannot know which sites are in it.

The API key lives in `~/.config/tidedeck/config.json` under
`plugins.tidedeck.pagepulse.key`, in plaintext — it is a single-user dashboard,
and there is no secret field. The key is read-only and scoped to PagePulse;
rotate it in `config.php` if it leaks.

## Opening a row

`Space` gives the panel the keyboard, `↑`/`↓` walk the rows, and `Enter` opens
the page under the cursor — a tracked page, or the last row, which opens the
PagePulse dashboard at the same site and period. Only `http` and `https` are
opened, handed to `xdg-open` as a single argument.

## When it cannot reach PagePulse

The panel never goes blank. A missing URL or key, a key PagePulse refuses, the
API switched off, an unreachable host, or an answer that will not parse each
draw one warning row naming the failure, so "not switched on" reads differently
from "wrong key" from "unreachable".

## Installing

Build the binary once from a TideDeck checkout:

```bash
./contrib/pagepulse/build.sh
```

Then copy the directory into your plugins:

```bash
cp -r contrib/pagepulse ~/.config/tidedeck/plugins/tidedeck.pagepulse
```

Or install it from the Plugins page in settings, which needs no restart. A
hand-dropped directory needs one; either way the plugin starts hidden and is
enabled from the panel picker (`space`).
