#!/usr/bin/env bash
# Prints the release notes of a version: its CHANGELOG.md section and how
# to verify the download. Fails if the section is missing, so that a
# version is not released without notes.
#
# Usage: tools/release/notes.sh <version tag, e.g. v0.4.0>
set -euo pipefail

tag=${1:?version tag}
changelog="$(dirname "$0")/../../CHANGELOG.md"
notes=$(awk -v h="## $tag" '
  index($0, h) == 1 && (length($0) == length(h) || substr($0, length(h) + 1, 1) == " ") { on = 1; next }
  on && /^## / { exit }
  on { print }
' "$changelog")
[[ -n ${notes//[[:space:]]/} ]] || { echo "CHANGELOG.md has no section for $tag" >&2; exit 1; }
printf '%s\n' "$notes"
cat <<END

### Download and verify

Pick the archive for your system: the binary needs nothing else installed.
Check it against \`SHA256SUMS\` and, with the GitHub CLI, that it was built
by this repository's release workflow:

\`\`\`
sha256sum -c SHA256SUMS --ignore-missing
gh attestation verify fbrescue_${tag}_windows_amd64.zip -R formeo/fbrescue
\`\`\`

Or build it yourself: \`go install github.com/formeo/fbrescue/cmd/fbrescue@${tag}\`
END
