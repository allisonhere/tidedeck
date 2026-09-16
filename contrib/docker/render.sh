#!/bin/sh
# Containers: docker ps as a tidedeck panel.
#
# Prints one document describing the containers on this host: a row per
# container with its state as the tone, a copyable id once a filter narrows to
# one, and the detail rows shown when the panel is zoomed.
set -eu

emit() { printf '%s\n' "$1"; exit 0; }

# A missing docker or a stopped daemon still leaves a panel worth looking at:
# a single row that says what is wrong, in the danger tone.
if ! command -v docker >/dev/null 2>&1; then
	emit '{"schemaVersion":1,"rows":[{"type":"text","label":"docker","value":"not installed","tone":"danger"}]}'
fi

if ! docker info >/dev/null 2>&1; then
	emit '{"schemaVersion":1,"rows":[{"type":"text","label":"docker","value":"daemon not running","tone":"danger"}]}'
fi

all="${TIDEDECK_PLUGIN_ALL:-true}"
filter="${TIDEDECK_PLUGIN_FILTER:-}"

if [ "$all" = "true" ]; then
	docker ps -a --format '{{json .}}'
else
	docker ps --format '{{json .}}'
fi | jq -s --arg filter "$filter" '
  # Tone by state and health, so a crashed container and a cleanly stopped one
  # do not look alike.
  def tone:
    (.HealthStatus // "") as $health
    | if .State == "running" and ($health == "" or $health == "healthy") then "good"
      elif .State == "running" then "warning"
      elif .State == "exited" and (.Status | startswith("Exited (0)")) then "muted"
      elif .State == "exited" or .State == "dead" then "danger"
      else "muted" end;

  def matches($f):
    $f == "" or ((.Names | ascii_downcase | contains($f))
                 or (.Image | ascii_downcase | contains($f)));

  def row: { "type": "text", "label": .Names, "value": .Status, "tone": tone };
  def header($title; $n): { "type": "divider", "label": ($title + " (" + ($n | tostring) + ")") };
  def pair($label; $value; $tone): { "type": "text", "label": $label, "value": $value, "tone": $tone };
  def dash: if . == "" then "—" else . end;

  ($filter | ascii_downcase) as $f
  | [ .[] | select(matches($f)) ] as $cs
  | [ $cs[] | select(.State == "running") ] as $up
  | [ $cs[] | select(.State != "running") ] as $down
  | {
      "schemaVersion": 1,
      "rows": (
        (if ($up | length) > 0 then [header("running"; $up | length)] + ($up | map(row)) else [] end)
        + (if ($down | length) > 0 then [header("stopped"; $down | length)] + ($down | map(row)) else [] end)
        + (if ($cs | length) == 0 then [pair(""; "no containers"; "muted")] else [] end)
        + (if ($cs | length) == 1 then [pair("id"; $cs[0].ID[0:12]; "accent")] else [] end)
      ),
      "detail": [
        $cs[] as $c
        | { "type": "divider", "label": $c.Names },
          pair("image"; $c.Image; ""),
          pair("state"; $c.Status; $c | tone),
          pair("ports"; $c.Ports | dash; ""),
          pair("mounts"; $c.Mounts | dash; ""),
          pair("created"; $c.CreatedAt; ""),
          pair("command"; $c.Command; "")
      ]
    }
'
