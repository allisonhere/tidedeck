#!/bin/sh
# A calculator you type into. The dashboard passes what you type at the panel as
# TIDEDECK_PLUGIN_EXPRESSION, so this program just evaluates it and prints the
# result as a document. awk does the arithmetic so operands can be fractional.
set -eu

expr="${TIDEDECK_PLUGIN_EXPRESSION:-}"

# Only the runes the manifest declares are expected, but the environment can be
# set by hand, so keep just the arithmetic characters before handing the
# expression to awk.
clean=$(printf '%s' "$expr" | tr -cd '0-9.+*/() -')
[ "$clean" = "$expr" ] || expr=""

result=""
tone="muted"
if [ -n "$expr" ]; then
  result=$(awk "BEGIN { printf \"%g\", ($expr) }" 2>/dev/null) || result=""
  if [ -z "$result" ]; then
    result="?"
    tone="danger"
  else
    tone="good"
  fi
fi

printf '{"schemaVersion":1'
printf ',"badge":{"text":"%s","tone":"%s"}' "${result:-—}" "$tone"
printf ',"rows":['
printf '{"type":"text","label":"expression","value":"%s","tone":"muted"},' "${expr:-type an expression}"
printf '{"type":"divider","label":"result"},'
printf '{"type":"text","label":"value","value":"%s","tone":"%s"}' "${result:-—}" "$tone"
printf ']}\n'
