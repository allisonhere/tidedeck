#!/bin/sh
# Adapts `ai-usagebar usage --json` to a tidedeck panel document. The two
# formats already agree on type/label/value/percent/severity, so this is a
# projection rather than a translation.
set -eu
BIN="${TIDEDECK_PLUGIN_BINARY:-ai-usagebar}"
"$BIN" usage --json | jq -c '{
  rows: [ .entries[] | select(.status == "ready") |
          ({type:"divider", label:.display_name}),
          (.sections[] | select(.type != "spacer")) ],
  badge: ( [ .entries[] | select(.status=="ready") | .metrics[]? | .percent ] | max // 0
           | if . >= 80 then {text:(.|tostring)+"%", tone:"danger"}
             elif . >= 50 then {text:(.|tostring)+"%", tone:"warning"}
             else {text:(.|tostring)+"%", tone:"good"} end )
}'
