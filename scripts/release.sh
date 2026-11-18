#!/usr/bin/env bash
# Tags plugins for publishing: scripts/release.sh <plugin…|all> [--push]
# Creates <plugin>/v<version> from each manifest's version (skips tags that
# exist and the SDK example de.example.ups); with --push the tags go to
# origin, and .github/workflows/publish.yml submits each plugin to the store.
set -euo pipefail
cd "$(dirname "$0")/.."
push=false
names=()
for a in "$@"; do
  case "$a" in
    --push) push=true ;;
    all) for d in plugins/*/; do names+=("$(basename "$d")"); done ;;
    *) names+=("$a") ;;
  esac
done
[ ${#names[@]} -gt 0 ] || { echo "usage: scripts/release.sh <plugin…|all> [--push]" >&2; exit 2; }
git fetch -q --tags origin
tags=()
for n in "${names[@]}"; do
  m="plugins/$n/phoedox-plugin.json"
  [ -f "$m" ] || { echo "no plugin $n" >&2; exit 1; }
  id=$(jq -r .id "$m"); v=$(jq -r .version "$m")
  [[ "$id" == com.phoedox.* ]] || { echo "skip $n ($id is not an official plugin)"; continue; }
  t="$n/v$v"
  if git rev-parse -q --verify "refs/tags/$t" >/dev/null; then echo "exists $t"; continue; fi
  git tag -a "$t" -m "$id $v"
  tags+=("$t"); echo "tagged $t"
done
if $push && [ ${#tags[@]} -gt 0 ]; then
  for t in "${tags[@]}"; do git push -q origin "refs/tags/$t"; done
  echo "pushed ${#tags[@]} tags"
fi
