#!/usr/bin/env bash
# Usage: extract-changelog.sh <version|vX.Y.Z> (falls back to $VERSION).
set -euo pipefail

version="${1:-${VERSION:-}}"
version="${version#v}"
changelog="${CHANGELOG:-CHANGELOG.md}"

if [ -z "$version" ]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

awk -v ver="$version" '
  $0 ~ ("^## \\[" ver "\\]") { flag = 1; next }
  flag && (/^## \[/ || /^\[[^]]*\]: http/) { exit }
  flag { print }
' "$changelog" | sed '1{/^$/d}'
