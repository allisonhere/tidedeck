# tidedeck.mail

An email preview panel for TideDeck. It shows what is in your mailbox — sender,
subject and age, unread carried and read muted — and it does that without a
password, without a network connection, and without anything running.

## How it gets the mail

TideMail is a foreground TUI, not a daemon, so for most of the day there is
nothing to ask. What there is, is its cache: `mail.db`, SQLite in WAL mode,
which admits one writer and any number of readers. This plugin opens that file
**read-only** and queries it.

That is the whole design, and the constraints it buys are worth stating
plainly:

- **No credentials live here.** Account setup stays in TideMail, where the
  passwords already are. The dashboard never holds a second copy, because a
  second copy is a second thing to leak.
- **No network.** The plugin issues SQL and exits. It cannot fetch mail, mark
  anything read, or tell a server you were looking.
- **Nothing to break.** Opening TideMail while the panel is up changes nothing
  in either direction; read-only WAL readers do not block the writer.
- **No message bodies are ever read.** The queries select senders, subjects,
  flags and dates. `body_text` and `body_html` are never named, so the body of
  a message cannot reach the dashboard even by accident.

It follows that the panel is as fresh as TideMail's last sync, and no fresher.
That is the trade: a panel that is always right about what TideMail knows,
rather than one that needs your password to be right about the server.

## Requirements

`sqlite3` and `jq`. If either is missing, or the database is not there yet, the
panel says which — it does not go blank. A blank panel cannot be told apart
from a broken one.

## Settings

| Setting | Default | What it does |
|---|---|---|
| `account` | every account | Which account to preview. **A list**, read from your mailbox. |
| `mailbox` | the inbox | Which mailbox. Narrows to the chosen account. **A list.** |
| `messages shown` | 6 | How many messages the panel previews, 1 to 50. |
| `unread only` | off | Hide mail you have already read. |
| `mail.db path` | TideMail's own location | Only needed if the database is somewhere unusual. A leading `~` is resolved, so `~/.local/share/tidemail/mail.db` works. |

Every setting has a working default, so one account with one inbox needs no
configuration at all: a blank mailbox finds the inbox by name, matched
case-insensitively because servers disagree about capitalisation.

`~` needs saying because it is not a shell here: a setting reaches the plugin as
an environment variable, where a `~` means itself. The rest of the dashboard
resolves it for the paths it owns, and a path that names nothing is reported as
a path — "no database at the path set", with the path under it — rather than as
TideMail being absent, because those are different problems and only one of them
is in this setting.

`account` and `mailbox` are **populated lists, not text boxes.** A manifest is
static JSON written before the plugin was ever installed — it cannot know which
accounts exist on this machine. The program does, every time it runs, so it
reports them back in the document's `options` and the settings screen builds the
list from that. Choosing an account narrows the mailbox list to that account's
folders on the next run, inbox first and then by how much mail each holds;
offering every folder of every account at once is what made it a haystack.

If the panel has never run, those rows are text boxes for a moment and become
lists as soon as it has. That is the only time you should see a box there.

## Installing

```bash
cp -r contrib/mail ~/.config/tidedeck/plugins/tidedeck.mail
```

Or from the Plugins page in settings, which installs without a restart. A
hand-dropped directory needs one.
