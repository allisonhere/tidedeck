#!/bin/sh
# Previews recent mail as a tidedeck panel document, read straight from
# TideMail's SQLite cache. The database is opened read-only, so this runs
# safely while TideMail itself is open: the file is in WAL mode, which admits
# one writer and any number of readers.
#
# Reading the cache rather than talking to TideMail is what lets the panel show
# real mail when TideMail is closed, which is most of the time - it is a
# foreground TUI, not a daemon.
#
# Every setting has a working default, so the common case - one account, one
# inbox - needs no configuration at all. When there is nothing to show the
# panel explains itself and names the accounts it can see, because a blank
# panel and a misconfigured one look identical otherwise.
set -eu

DB="${TIDEDECK_PLUGIN_DB:-}"
# A path is written the way a shell writes one, and a value read out of a
# config file arrives here as an environment variable, where ~ means itself.
# The rest of the dashboard resolves a leading ~ for the paths it owns (the git
# panel, the calendar); without the same courtesy here, a configured
# "~/.local/share/tidemail/mail.db" names a file that does not exist, the panel
# explains itself as though TideMail were missing, and the account and mailbox
# settings - whose values this script found out by looking - stay empty boxes.
case "$DB" in
  "~")   DB="$HOME" ;;
  "~/"*) DB="$HOME/${DB#\~/}" ;;
esac
# Kept to tell "no TideMail here" apart from "not at the path you set", which
# are different problems with different answers.
CONFIGURED_DB="$DB"
[ -n "$DB" ] || DB="${XDG_DATA_HOME:-$HOME/.local/share}/tidemail/mail.db"
MAILBOX="${TIDEDECK_PLUGIN_MAILBOX:-}"
ACCOUNT="${TIDEDECK_PLUGIN_ACCOUNT:-}"
COUNT="${TIDEDECK_PLUGIN_COUNT:-6}"
UNREAD="${TIDEDECK_PLUGIN_UNREAD:-false}"

# notice prints a one-row document. A panel that cannot read its source should
# say so in its own body rather than failing the run, which would leave the
# last good document on screen and never explain why it stopped moving.
notice() {
  printf '{"schemaVersion":1,"rows":[{"type":"text","label":"mail","value":"%s","tone":"muted"}]}\n' "$1"
  exit 0
}

# escape makes a string safe to sit inside the JSON a notice prints. A path is
# the one value here that comes from the reader rather than from this script,
# and a quote in it would otherwise end the document early.
escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# notice_path names the file that was not there. "TideMail not set up" is the
# right thing to say about the default location; saying it about a path the
# reader set sends them to look at TideMail instead of at the setting they can
# fix. The path gets its own row rather than a place in the sentence, so a long
# one is not the part that gets cut off.
notice_path() {
  printf '{"schemaVersion":1,"rows":[{"type":"text","label":"mail","value":"no database at the path set","tone":"warning"},{"type":"block","label":"","body":["%s"],"tone":"muted"}]}\n' "$(escape "$1")"
  exit 0
}

if [ ! -f "$DB" ]; then
  [ -z "$CONFIGURED_DB" ] || notice_path "$DB"
  notice "TideMail not set up"
fi
command -v sqlite3 >/dev/null 2>&1 || notice "sqlite3 missing"
command -v jq >/dev/null 2>&1 || notice "jq missing"

# A count that is not a plain number would be interpolated into the LIMIT
# clause, so it is checked rather than trusted.
case "$COUNT" in
  ''|*[!0-9]*) COUNT=6 ;;
esac
[ "$COUNT" -gt 0 ] 2>/dev/null || COUNT=6
[ "$COUNT" -le 50 ] || COUNT=50

# quote doubles any single quote, so a mailbox or account name containing one
# cannot end the SQL string literal it sits in.
quote() { printf "%s" "$1" | sed "s/'/''/g"; }

# A blank mailbox means "the inbox", matched case-insensitively rather than by
# the literal name. Servers disagree about capitalisation, and TideMail does
# not record the IMAP \Inbox special-use flag, so the name is all there is.
if [ -n "$MAILBOX" ]; then
  BOX_SQL="b.name = '$(quote "$MAILBOX")'"
  BOX_SQL_OUTER="b2.name = '$(quote "$MAILBOX")'"
else
  BOX_SQL="UPPER(b.name) = 'INBOX'"
  BOX_SQL_OUTER="UPPER(b2.name) = 'INBOX'"
fi

# The mailbox list is scoped to the chosen account. Offering every folder from
# every account at once is what made the list useless: forty entries, most of
# them belonging to an account the reader is not looking at.
ACCOUNT_SCOPE="1=1"
if [ -n "$ACCOUNT" ]; then
  ACCOUNT_SCOPE="a.name = '$(quote "$ACCOUNT")'"
fi

WHERE="$BOX_SQL"
if [ -n "$ACCOUNT" ]; then
  WHERE="$WHERE AND a.name = '$(quote "$ACCOUNT")'"
fi
# Written as an if rather than a trailing && so that a false test cannot take
# the exit status of the whole script with it under set -e.
if [ "$UNREAD" = "true" ]; then
  WHERE="$WHERE AND m.read = 0"
fi

# One query returns everything the panel can need: the unread tally for the
# badge, the messages themselves, and - for the empty case - what accounts
# exist and how much inbox mail each has cached. The third query costs nothing
# next to the second and is what turns a blank panel into a setup hint.
#
# body_text and body_html are deliberately never selected: every message row
# carries its full body, and a preview needs none of it.
SQL="
SELECT (
  SELECT COUNT(*) FROM messages m
  JOIN mailboxes b ON b.id = m.mailbox_id
  JOIN accounts  a ON a.id = b.account_id
  WHERE $WHERE AND m.read = 0
);
SELECT COALESCE((SELECT json_group_array(json_object(
  'from', f, 'subject', s, 'read', r, 'starred', st, 'attach', at,
  'age', ag, 'account', acct))
FROM (
  SELECT m.from_addr AS f, m.subject AS s, m.read AS r, m.starred AS st,
         m.has_attachment AS at, a.name AS acct,
         (strftime('%s','now') - m.date) AS ag
  FROM messages m
  JOIN mailboxes b ON b.id = m.mailbox_id
  JOIN accounts  a ON a.id = b.account_id
  WHERE $WHERE
  ORDER BY m.date DESC
  LIMIT $COUNT
)), '[]');
SELECT COALESCE((SELECT json_group_array(json_object('name', nm, 'inbox', ib))
FROM (
  SELECT a.name AS nm,
         (SELECT COUNT(*) FROM messages m
          JOIN mailboxes b2 ON b2.id = m.mailbox_id
          WHERE b2.account_id = a.id AND $BOX_SQL_OUTER) AS ib
  FROM accounts a ORDER BY a.position, a.name
)), '[]');
SELECT COALESCE((SELECT json_group_array(nm) FROM (
  SELECT DISTINCT a.name AS nm FROM accounts a ORDER BY a.position, a.name
)), '[]');
SELECT COALESCE((SELECT json_group_array(nm) FROM (
  SELECT b.name AS nm,
         (SELECT COUNT(*) FROM messages m WHERE m.mailbox_id = b.id) AS msgs
  FROM mailboxes b
  JOIN accounts a ON a.id = b.account_id
  WHERE $ACCOUNT_SCOPE
  GROUP BY b.name
  -- The inbox first, then whatever actually holds mail: a list of forty
  -- folders in alphabetical order is not a list, it is a haystack.
  ORDER BY (UPPER(b.name) = 'INBOX') DESC, SUM(msgs) DESC, b.name COLLATE NOCASE
)), '[]');
"

OUT=$(sqlite3 "file:$DB?mode=ro" "$SQL" 2>/dev/null) || notice "unreadable"
UNREAD_N=$(printf '%s\n' "$OUT" | sed -n 1p)
ITEMS=$(printf '%s\n' "$OUT" | sed -n 2p)
ACCTS=$(printf '%s\n' "$OUT" | sed -n 3p)
# The values the two name settings can usefully take. A manifest is written
# before the machine it runs on exists, so it cannot list them; this can,
# because it just read them.
ACCOUNT_OPTS=$(printf '%s\n' "$OUT" | sed -n 4p)
MAILBOX_OPTS=$(printf '%s\n' "$OUT" | sed -n 5p)
[ -n "$ITEMS" ] || ITEMS='[]'
[ -n "$ACCTS" ] || ACCTS='[]'
[ -n "$ACCOUNT_OPTS" ] || ACCOUNT_OPTS='[]'
[ -n "$MAILBOX_OPTS" ] || MAILBOX_OPTS='[]'
case "$UNREAD_N" in ''|*[!0-9]*) UNREAD_N=0 ;; esac

printf '%s' "$ITEMS" | jq -c \
  --argjson unread "$UNREAD_N" \
  --argjson accounts "$ACCTS" \
  --argjson accountOpts "$ACCOUNT_OPTS" \
  --argjson mailboxOpts "$MAILBOX_OPTS" \
  --arg account "$ACCOUNT" \
  --arg mailbox "$MAILBOX" \
  --arg unreadonly "$UNREAD" '

  # sender reduces "Display Name <addr@host>" to what a narrow panel can show.
  # The address is the fallback, not the preference: a name is what the reader
  # recognises, and the address rarely fits.
  def sender:
    . as $raw
    | ($raw | capture("^\\s*\"?(?<n>[^<\"]*?)\"?\\s*<"; "x").n? // "") as $name
    | if ($name | length) > 0 then $name
      else ($raw | capture("<(?<a>[^>]+)>").a? // $raw)
      end
    | gsub("^\\s+|\\s+$"; "");

  # age is written the way the rest of the dashboard writes it: one unit, no
  # decimal, widest unit that still reads as a number.
  def age:
    if   . < 60      then "now"
    elif . < 3600    then "\((./60)|floor)m"
    elif . < 86400   then "\((./3600)|floor)h"
    elif . < 2592000 then "\((./86400)|floor)d"
    else                  "\((./2592000)|floor)mo"
    end;

  # A read message is muted so the unread ones carry the panel. Starred wins
  # over unread: saving something is a stronger signal than not having opened it.
  def tone:
    if   .starred == 1 then "accent"
    elif .read == 0    then "good"
    else                    "muted"
    end;

  # The subject sits under its sender rather than beside it, so a long subject
  # wraps into the panel width instead of being cut at a label column.
  #
  # The age joins the sender line rather than taking the value column. A block
  # row pads its label only up to a cap, so senders longer than the cap push
  # their age out of line with the rest - one left-aligned line reads straight
  # where two ragged columns did not.
  def rows:
    [ .[]
      | { type: "block",
          label: ((.from | sender)
                  + (if .attach == 1 then " @" else "" end)
                  + " · " + (.age | age)),
          body: [ (if (.subject | length) > 0 then .subject else "(no subject)" end) ],
          tone: tone },
        { type: "spacer" } ]
    | .[:-1];

  # The empty panel is the one a new user sees, so it is the one that has to
  # teach. It names every account it found and how much inbox mail each has,
  # which is exactly what the account setting wants typed into it.
  def setup:
    ($accounts | map(select(.inbox > 0))) as $withmail
    | ($accounts | length) as $n
    | if $n == 0 then
        [ { type: "text", label: "mail", value: "no accounts yet", tone: "muted" },
          { type: "spacer" },
          { type: "block", label: "Set one up in TideMail",
            body: [ "then this panel fills itself in" ],
            tone: "muted", bodyTone: "muted" } ]
      elif ($account | length) > 0
           and ($accounts | map(.name) | index($account) | not) then
        [ { type: "text", label: "account", value: "not found", tone: "warning" },
          { type: "divider", label: "AVAILABLE" } ]
        + [ $accounts[] | { type: "text",
                            label: (.name + " \u00b7 "
                                    + (if .inbox > 0 then "\(.inbox)" else "empty" end)),
                            tone: (if .inbox > 0 then "good" else "muted" end) } ]
      else
        [ { type: "text", label: "mail",
            value: (if $unreadonly == "true" then "nothing unread" else "empty" end),
            tone: "muted" } ]
        + (if ($withmail | length) > 0 and ($account | length) > 0 then
             [ { type: "divider", label: "HAS MAIL" } ]
             + [ $withmail[] | { type: "text",
                                 label: (.name + " \u00b7 \(.inbox)"), tone: "good" } ]
           else [] end)
      end;

  # Offered to the settings screen so the two name fields are lists rather
  # than empty boxes. "" is included so "every account" stays choosable.
  { account: ([""] + $accountOpts), mailbox: ([""] + $mailboxOpts) } as $options

  | if length == 0 then
    { schemaVersion: 1, rows: setup, options: $options }
  else
    { schemaVersion: 1,
      rows: rows,
      options: $options,
      detail: ([ { type: "divider",
                   label: (if ($mailbox | length) > 0
                           then ($mailbox | ascii_upcase) else "INBOX" end) } ] + rows),
      badge: ( if $unread > 0
               then { text: ($unread | tostring), tone: "warning" }
               else null end ) }
  end'
