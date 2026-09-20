#!/bin/sh
# Entry point for the PagePulse panel.
#
# This panel is a Go program, so an installed copy needs a binary. Building it
# here would make the first render wait for the compiler - long enough to outlast
# the panel's own timeout - so this says what to run instead, the way the mail
# panel names the file it could not find. The build is one command, done once.
set -eu

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BIN="$DIR/pagepulse"

if [ ! -x "$BIN" ]; then
  case "${1:-}" in
  render)
    printf '{"schemaVersion":1,"rows":['
    printf '{"type":"text","label":"pagepulse","value":"not built yet","tone":"warning"},'
    printf '{"type":"block","body":["run %s/build.sh, once"],"bodyTone":"muted"}]}\n' "$DIR"
    ;;
  *)
    echo "pagepulse: not built yet - run $DIR/build.sh once" >&2
    exit 1
    ;;
  esac
  exit 0
fi

exec "$BIN" "$@"
