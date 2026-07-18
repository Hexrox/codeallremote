#!/usr/bin/env bash
# Documentation link validation.
#
# Checks that internal markdown links (relative paths) resolve to existing
# files. Used by CI (M4-04) so that broken-link PRs fail.
set -euo pipefail

cd "$(dirname "$0")/.."

status=0
checked=0

# Match [text](path) where path does not start with http/https/#/mailto:.
link_re='\[([^]]+)\]\(([^)]+)\)'

while IFS= read -r -d '' file; do
  while IFS= read -r line; do
    # Extract link targets.
    while [[ $line =~ $link_re ]]; do
      target="${BASH_REMATCH[2]}"
      line="${line#*"${BASH_REMATCH[0]}"}"
      # Strip optional anchor.
      path="${target%%#*}"
      case "$path" in
        http://*|https://*|mailto://*|\#*) continue ;;
        "") continue ;;
      esac
      checked=$((checked + 1))
      if [ ! -e "$path" ]; then
        echo "  FAIL: $file -> $path (missing)"
        status=1
      fi
    done
  done < "$file"
done < <(find . -name '*.md' -not -path './vendor/*' -not -path './.git/*' -print0)

echo "Checked $checked internal links."
exit $status
