#!/usr/bin/env bash
# Builds the short release notes for one version, mirroring the format of the
# hand-written releases:
#
#   ## RouteBox vX.Y.Z
#
#   <the "## [X.Y.Z]" section of CHANGELOG.md>
#
#   Full changelog: <link>
#
# When CHANGELOG.md has no section for the version (tag cut before the
# changelog was updated), falls back to the commit subjects since the previous
# tag so the release is never published with an empty body.
set -euo pipefail

VERSION="${1:?usage: extract-changelog.sh <version-without-v>}"

# Everything between "## [X.Y.Z]" and the next "## [" heading.
section=$(awk -v ver="$VERSION" '
	index($0, "## [" ver "]") == 1 { grab = 1; next }
	/^## \[/                       { grab = 0 }
	grab                           { print }
' CHANGELOG.md)

if [ -z "$(printf '%s' "$section" | tr -d '[:space:]')" ]; then
	prev=$(git describe --tags --abbrev=0 "v${VERSION}^" 2>/dev/null || echo "")
	if [ -n "$prev" ]; then
		range="${prev}..v${VERSION}"
	else
		range="v${VERSION}"
	fi
	commits=$(git log --no-merges --pretty='- %s' "$range" 2>/dev/null || true)
	if [ -z "$commits" ]; then
		commits="- See the linked changelog."
	fi
	section="### Changes"$'\n\n'"$commits"
fi

echo "## RouteBox v${VERSION}"
echo
# Trim leading blank lines; trailing ones are harmless in markdown.
printf '%s\n' "$section" | awk 'NF { found = 1 } found'
echo
echo "Full changelog: [CHANGELOG.md](https://github.com/hoaxisr/routebox/blob/source/CHANGELOG.md)"
