#!/bin/sh
# Run the local, local package. No install, PATH edits, or network.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case $(uname -s) in Darwin) target=darwin ;; Linux) target=linux ;; *) echo 'Use try-local.ps1 on Windows' >&2; exit 2 ;; esac
case $(uname -m) in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo 'Unsupported architecture' >&2; exit 2 ;; esac
bin="$root/dist/release/ob_${target}_${arch}"
if [ ! -x "$bin" ]; then echo "Candidate missing: run scripts/build-release.sh first" >&2; exit 3; fi
cmd=${1:-query}
if [ "$#" -gt 0 ]; then shift; fi
case "$cmd" in query|search|show|status) exec "$bin" "$cmd" --dataset "$root/dist/release/catalog.json.gz" "$@" ;; *) exec "$bin" "$cmd" "$@" ;; esac
