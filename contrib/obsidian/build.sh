#!/bin/sh
# Builds the obsidian program.
#
# Run it from a TideDeck checkout: the panel imports the app's own library
# packages (and Ripple), so it can only be compiled inside the module that holds
# them. An installed copy does not need this - the released app is built with its
# bundled plugins, and the binary travels with it.
set -eu

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$DIR/../.." && pwd)

if [ ! -f "$ROOT/go.mod" ]; then
  echo "build.sh: no go.mod above $DIR." >&2
  echo "build.sh: run this from a TideDeck checkout - the panel imports the app's own packages." >&2
  exit 1
fi

command -v go >/dev/null 2>&1 || { echo "build.sh: go is not on PATH" >&2; exit 1; }
cd "$ROOT"
exec go build -o "$DIR/obsidian" ./contrib/obsidian
