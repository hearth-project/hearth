#!/usr/bin/env bash
# Print the CHANGELOG.md section for a given version, for use as a GitHub release body.
# Usage: extract-changelog.sh <version|vX.Y.Z>   (falls back to $VERSION)
set -euo pipefail

version="${1:-${VERSION:-}}"
version="${version#v}"
changelog="${CHANGELOG:-CHANGELOG.md}"

if [ -z "$version" ]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

# Print everything between "## [<version>]" and the next "## [" heading or the
# link-reference block at the bottom of the file.
awk -v ver="$version" '
  $0 ~ ("^## \\[" ver "\\]") { flag = 1; next }
  flag && (/^## \[/ || /^\[[^]]*\]: http/) { exit }
  flag { print }
' "$changelog" | sed '1{/^$/d}'
