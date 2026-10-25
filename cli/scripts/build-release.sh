#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
: "${GOCACHE:=$root/.cache/go-build}"
export GOCACHE
exec go run ./cmd/release "$@"
