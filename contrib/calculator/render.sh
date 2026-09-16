#!/bin/sh
# A two-operand calculator panel. The dashboard passes the declared settings as
# environment variables (TIDEDECK_PLUGIN_*), the program computes, and prints a
# document the panel draws. awk does the arithmetic so the operands can be
# fractional; shell arithmetic is integers only.
set -eu

a="${TIDEDECK_PLUGIN_A:-0}"
op="${TIDEDECK_PLUGIN_OP:-+}"
b="${TIDEDECK_PLUGIN_B:-0}"
decimals="${TIDEDECK_PLUGIN_DECIMALS:-2}"

result=$(awk -v a="$a" -v b="$b" -v op="$op" -v p="$decimals" 'BEGIN {
  if (op == "+")      r = a + b
  else if (op == "-") r = a - b
  else if (op == "*") r = a * b
  else if (op == "/") {
    if (b == 0) { print "divide by zero"; exit }
    r = a / b
  }
  else { print "unknown operator"; exit }
  printf "%.*f", p, r
}')

# A failed calculation is worth flagging, so the badge and the value carry it.
case "$result" in
  "divide by zero" | "unknown operator") tone="danger" ;;
  *)                                     tone="good" ;;
esac

# A value could contain a quote or backslash; strip them so the document stays
# valid JSON. The settings are text, and a broken document costs the panel.
clean() { printf '%s' "$1" | tr -d '"\\'; }

a=$(clean "$a")
op=$(clean "$op")
b=$(clean "$b")
result=$(clean "$result")

printf '{"schemaVersion":1,"badge":{"text":"%s","tone":"%s"},"rows":[' "$result" "$tone"
printf '{"type":"text","label":"expression","value":"%s %s %s","tone":"muted"},' "$a" "$op" "$b"
printf '{"type":"divider","label":"result"},'
printf '{"type":"text","label":"value","value":"%s","tone":"%s"}' "$result" "$tone"
printf ',{"type":"text","label":"edit operands","value":"settings (s)","tone":"muted"}'
printf '],"detail":['
printf '{"type":"text","label":"a","value":"%s"},' "$a"
printf '{"type":"text","label":"operation","value":"%s"},' "$op"
printf '{"type":"text","label":"b","value":"%s"},' "$b"
printf '{"type":"text","label":"decimals","value":"%s"},' "$decimals"
printf '{"type":"text","label":"result","value":"%s","tone":"%s"}' "$result" "$tone"
printf ']}\n'
