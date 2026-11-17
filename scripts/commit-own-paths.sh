set -eu

usage() {
  echo "usage: commit-own-paths.sh -m MESSAGE PATH..." >&2
  exit 2
}

message=
while getopts m: opt; do
  case "$opt" in
    m) message=$OPTARG ;;
    *) usage ;;
  esac
done
shift $((OPTIND - 1))
[ -n "$message" ] || usage
[ "$#" -gt 0 ] || usage

top=$(git rev-parse --show-toplevel)
prefix=$(git rev-parse --show-prefix)
ref=$(git symbolic-ref -q HEAD) || { echo "HEAD is detached: nothing to move" >&2; exit 1; }
physical_top=$(cd "$top" && pwd -P)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
named="$work/named"
: > "$named"

for given in "$@"; do
  case "$given" in
    "$top"/*) relative=${given#"$top"/} ;;
    "$physical_top"/*) relative=${given#"$physical_top"/} ;;
    /*) echo "refused: $given is outside the repository" >&2; exit 1 ;;
    *) relative="$prefix$given" ;;
  esac
  normal=$(printf '%s\n' "$relative" | awk -F/ '
    {
      n = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "" || $i == ".") continue
        if ($i == "..") {
          if (n == 0) { print "OUTSIDE"; exit }
          n--
          continue
        }
        parts[++n] = $i
      }
      out = parts[1]
      for (i = 2; i <= n; i++) out = out "/" parts[i]
      print out
    }')
  if [ "$normal" = "OUTSIDE" ]; then
    echo "refused: $given is outside the repository" >&2
    exit 1
  fi
  if [ -z "$normal" ]; then
    echo "refused: $given names the whole repository" >&2
    exit 1
  fi
  printf '%s\n' "$normal" >> "$named"
done

trailer="Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
case "$message" in
  *"$trailer"*) body=$message ;;
  *) body=$(printf '%s\n\n%s' "$message" "$trailer") ;;
esac

cd "$top"
attempt=1
while [ "$attempt" -le 3 ]; do
  old=$(git rev-parse HEAD)
  index="$work/index-$attempt"
  GIT_INDEX_FILE=$index
  export GIT_INDEX_FILE

  git read-tree "$old"
  while IFS= read -r path; do
    git add -A -- ":(literal)$path"
  done < "$named"

  git -c core.quotepath=off diff --cached --name-only "$old" > "$work/changed"
  if [ ! -s "$work/changed" ]; then
    echo "nothing to commit for: $*" >&2
    exit 3
  fi

  strays=$(awk '
    NR == FNR { named[++count] = $0; next }
    {
      keep = 0
      for (i = 1; i <= count; i++) {
        if ($0 == named[i] || index($0, named[i] "/") == 1) { keep = 1; break }
      }
      if (!keep) print $0
    }
  ' "$named" "$work/changed")
  if [ -n "$strays" ]; then
    echo "refused: the commit would also change paths that were not named:" >&2
    printf '%s\n' "$strays" >&2
    exit 1
  fi

  tree=$(git write-tree)
  new=$(printf '%s\n' "$body" | git commit-tree "$tree" -p "$old")
  unset GIT_INDEX_FILE

  subject=$(printf '%s\n' "$message" | sed -n '1p')
  if git update-ref -m "commit-own-paths: $subject" "$ref" "$new" "$old" 2>/dev/null; then
    while IFS= read -r path; do
      shown=$(git ls-files -s -- ":(literal)$path" | awk '{ print $1, $2, $3 }')
      before=$(git ls-tree "$old" -- "$path" | awk '{ print $1, $3 }')
      if [ "$shown" != "$before 0" ]; then
        if [ -n "$shown" ] || [ -n "$before" ]; then continue; fi
      fi
      after=$(git ls-tree "$new" -- "$path" | awk '{ print $1 "," $3 }')
      tries=1
      while [ "$tries" -le 5 ]; do
        if [ -z "$after" ]; then
          git update-index --force-remove -- "$path" 2>/dev/null && break
        else
          git update-index --add --cacheinfo "$after,$path" 2>/dev/null && break
        fi
        tries=$((tries + 1))
        sleep 0.1
      done
    done < "$work/changed"
    printf '%s\n' "$new"
    exit 0
  fi
  attempt=$((attempt + 1))
done

echo "refused: $ref moved on every attempt; run it again" >&2
exit 4
