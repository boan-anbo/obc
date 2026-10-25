#!/bin/sh
# Offline bootstrap only. Installation, validation and filesystem ownership are Go.
set -eu
package=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
destination=
package_seen=0
dir_seen=0
while [ "$#" -gt 0 ]; do
  key=$1
  case "$key" in
    --help) echo 'Usage: sh install.sh [--package DIR] --dir NEW_DIR'; exit 0 ;;
    --package|--dir)
      [ "$#" -ge 2 ] || { echo "Missing value for $key" >&2; exit 2; }
      value=$2; shift 2
      case "$key" in
        --package) [ "$package_seen" -eq 0 ] || { echo 'Repeated --package' >&2; exit 2; }; package_seen=1; package=$value ;;
        --dir) [ "$dir_seen" -eq 0 ] || { echo 'Repeated --dir' >&2; exit 2; }; dir_seen=1; destination=$value ;;
      esac ;;
    *) echo "Unknown option: $key" >&2; exit 2 ;;
  esac
done
[ -n "$destination" ] || { echo '--dir NEW_DIR is required' >&2; exit 2; }
case $(uname -s) in Darwin) target=darwin ;; Linux) target=linux ;; *) echo 'Unsupported OS; use install.ps1 on Windows' >&2; exit 2 ;; esac
case $(uname -m) in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo 'Unsupported architecture' >&2; exit 2 ;; esac
package=$(CDPATH= cd -- "$package" && pwd) || { echo 'Cannot access package directory' >&2; exit 4; }
name="ob_${target}_${arch}"
[ -f "$package/$name" ] && [ ! -L "$package/$name" ] || { echo "Missing regular binary: $name" >&2; exit 4; }
[ -f "$package/CLI-SHA256SUMS" ] && [ ! -L "$package/CLI-SHA256SUMS" ] || { echo 'Missing regular SHA256SUMS' >&2; exit 4; }
expected=$(awk -v target="$name" '$2==target {count++; hash=$1} END {if(count!=1) exit 1; print hash}' "$package/CLI-SHA256SUMS") || { echo 'Missing/duplicate binary checksum' >&2; exit 4; }
case "$expected" in ''|*[!0-9a-f]*) echo 'Invalid checksum' >&2; exit 4 ;; esac
[ "${#expected}" -eq 64 ] || { echo 'Invalid checksum length' >&2; exit 4; }
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$package/$name" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then actual=$(shasum -a 256 "$package/$name" | awk '{print $1}')
else echo 'A system SHA256 verifier (sha256sum or shasum) is required' >&2; exit 4
fi
[ "$actual" = "$expected" ] || { echo 'Binary checksum mismatch; nothing executed or installed' >&2; exit 4; }
bootstrap_tmp=$(mktemp -d "${TMPDIR:-/tmp}/ob-install.XXXXXX")
trap 'rm -f "$bootstrap_tmp/ob"; rmdir "$bootstrap_tmp"' EXIT
cp "$package/$name" "$bootstrap_tmp/ob"
chmod 700 "$bootstrap_tmp/ob"
if command -v sha256sum >/dev/null 2>&1; then copied=$(sha256sum "$bootstrap_tmp/ob" | awk '{print $1}')
else copied=$(shasum -a 256 "$bootstrap_tmp/ob" | awk '{print $1}')
fi
[ "$copied" = "$expected" ] || { echo 'Bootstrap copy checksum mismatch' >&2; exit 4; }
"$bootstrap_tmp/ob" install --package "$package" --dir "$destination"
