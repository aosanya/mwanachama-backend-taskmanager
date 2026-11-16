#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
module="$(awk '/^module /{print $2}' "$repo/go.mod")"
name="${module##*-}"
hub="${1:-$repo/../projects/hub-steward/code/wakala-api/modules/$name}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

copy_tree() {
  local from="$1" to="$2"
  mkdir -p "$to"
  (cd "$from" && find . \( -name '*.go' -o -name '*.json' \) \
    -not -path './cmd/*' -not -path './documentation/*' -not -path './scripts/*' \
    -not -path './.git/*' -print0 | tar -cf - --null -T -) | tar -xf - -C "$to"
}

copy_tree "$repo" "$work/backend"
copy_tree "$hub" "$work/hub"

find "$work/hub" -name '*.go' -print0 | WAKALA_MODULE="wakalaserver/modules/$name" BACKEND_MODULE="$module" xargs -0 perl -pi -e '
  s{"wakalaserver/modules/shared/}{"github.com/aosanya/mwanachama-backend-shared/}g;
  s{"\Q$ENV{WAKALA_MODULE}\E}{"$ENV{BACKEND_MODULE}}g;
'

(cd "$repo" && go run ./scripts/hubdiff "$work/backend")
(cd "$repo" && go run ./scripts/hubdiff "$work/hub")

cd "$work"
diff -r backend hub | sed "s#^diff -r backend/#diff #"
