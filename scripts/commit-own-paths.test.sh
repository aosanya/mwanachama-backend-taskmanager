set -eu

here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
real_git=$(command -v git)

fail() { echo "FAIL: $*" >&2; exit 1; }

new_repo() {
  repo="$work/$1"
  git init -q -b dev "$repo"
  git -C "$repo" config user.name tester
  git -C "$repo" config user.email tester@example.invalid
  mkdir -p "$repo/dir with space" "$repo/sub"
  printf 'a0\n' > "$repo/a.txt"
  printf 'b0\n' > "$repo/b.txt"
  printf 'f0\n' > "$repo/dir with space/f.txt"
  printf 'd0\n' > "$repo/sub/d.txt"
  git -C "$repo" add -A
  git -C "$repo" commit -q -m initial
}

commit_own() {
  (cd "$repo" && sh "$here/commit-own-paths.sh" "$@")
}

head_of() { git -C "$repo" rev-parse HEAD; }

changed_in_head() { git -C "$repo" diff --name-status HEAD~1 HEAD; }

new_repo one
printf 'a1\n' > "$repo/a.txt"
printf 'b1\n' > "$repo/b.txt"
commit_own -m "change a" a.txt >/dev/null 2>&1 || fail "committing a named changed file failed"
[ "$(changed_in_head)" = "$(printf 'M\ta.txt')" ] || fail "the commit touched more than the named path: $(changed_in_head)"
[ "$(git -C "$repo" show HEAD:a.txt)" = "a1" ] || fail "the committed content of a.txt is wrong"
[ "$(git -C "$repo" show HEAD:b.txt)" = "b0" ] || fail "b.txt was committed without being named"
[ "$(cat "$repo/b.txt")" = "b1" ] || fail "the working tree b.txt was changed"
git -C "$repo" log -1 --format=%B | grep -q '^change a$' || fail "the message is missing"
git -C "$repo" log -1 --format=%B | grep -q '^Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>$' || fail "the trailer is missing"

before=$(head_of)
if commit_own -m "nothing" a.txt >"$work/out" 2>&1; then fail "a path with no changes was accepted"; fi
[ "$(head_of)" = "$before" ] || fail "a commit was made for a path with no changes"
grep -q 'a.txt' "$work/out" || fail "the refusal does not name the path"

before=$(head_of)
if commit_own -m "outside" ../outside.txt >"$work/out" 2>&1; then fail "a path outside the repo was accepted"; fi
[ "$(head_of)" = "$before" ] || fail "a commit was made for a path outside the repo"
grep -q 'outside.txt' "$work/out" || fail "the refusal does not name the outside path"

if commit_own a.txt >"$work/out" 2>&1; then fail "a missing message was accepted"; fi
if commit_own -m "no paths" >"$work/out" 2>&1; then fail "a missing path list was accepted"; fi

new_repo two
printf 'n\n' > "$repo/new.txt"
printf 'o\n' > "$repo/other-new.txt"
commit_own -m "add new" new.txt >/dev/null 2>&1 || fail "adding a named untracked file failed"
git -C "$repo" cat-file -e HEAD:new.txt || fail "the named untracked file was not committed"
if git -C "$repo" cat-file -e HEAD:other-new.txt 2>/dev/null; then fail "an untracked file that was not named was committed"; fi
[ -z "$(git -C "$repo" status --porcelain -- new.txt)" ] || fail "new.txt still shows in git status after its commit"
[ "$(git -C "$repo" status --porcelain -- other-new.txt)" = "?? other-new.txt" ] || fail "other-new.txt lost its untracked state"

new_repo three
printf 'a1\n' > "$repo/a.txt"
printf 'b1\n' > "$repo/b.txt"
git -C "$repo" add b.txt
staged_b=$(git -C "$repo" ls-files -s b.txt)
commit_own -m "change a" a.txt >/dev/null 2>&1 || fail "the commit failed with another path staged"
[ "$(git -C "$repo" ls-files -s b.txt)" = "$staged_b" ] || fail "the staged b.txt entry in the real index was touched"
git -C "$repo" diff --cached --quiet HEAD -- a.txt || fail "the real index still differs from HEAD for the committed a.txt"
[ -z "$(git -C "$repo" status --porcelain -- a.txt)" ] || fail "a.txt still shows in git status after its commit"
git -C "$repo" diff --cached --name-only HEAD | grep -q '^b.txt$' || fail "the staged b.txt was lost from the real index"
if git -C "$repo" cat-file -e HEAD:b.txt 2>/dev/null && [ "$(git -C "$repo" show HEAD:b.txt)" = "b1" ]; then fail "the staged b.txt was committed"; fi

new_repo three_b
printf 'a1\n' > "$repo/a.txt"
printf 'a-staged-by-someone\n' > "$repo/a.txt"
git -C "$repo" add a.txt
printf 'a1\n' > "$repo/a.txt"
staged_a=$(git -C "$repo" ls-files -s a.txt)
commit_own -m "change a" a.txt >/dev/null 2>&1 || fail "the commit failed when a different version of the path was staged"
[ "$(git -C "$repo" ls-files -s a.txt)" = "$staged_a" ] || fail "a different staged version of the named path was overwritten in the real index"

new_repo four
rm "$repo/a.txt"
commit_own -m "remove a" a.txt >/dev/null 2>&1 || fail "committing the removal of a named file failed"
[ -z "$(git -C "$repo" ls-files -- a.txt)" ] || fail "the removed a.txt is still in the real index"
[ -z "$(git -C "$repo" status --porcelain -- a.txt)" ] || fail "a.txt still shows in git status after its removal"
[ "$(changed_in_head)" = "$(printf 'D\ta.txt')" ] || fail "the removal commit touched more than a.txt: $(changed_in_head)"

new_repo five
printf 'f1\n' > "$repo/dir with space/f.txt"
printf 'd1\n' > "$repo/sub/d.txt"
commit_own -m "spaces" "dir with space/f.txt" >/dev/null 2>&1 || fail "a path with spaces failed"
[ "$(changed_in_head)" = "$(printf 'M\tdir with space/f.txt')" ] || fail "the spaces commit touched more: $(changed_in_head)"
(cd "$repo/sub" && sh "$here/commit-own-paths.sh" -m "relative" d.txt >/dev/null 2>&1) || fail "a path relative to a subfolder failed"
[ "$(changed_in_head)" = "$(printf 'M\tsub/d.txt')" ] || fail "the relative commit touched more: $(changed_in_head)"

new_repo six
mkdir -p "$work/bin"
cat > "$work/bin/git" <<SHIM
#!/bin/sh
if [ "\$1" = "update-ref" ] && [ ! -e "$work/moved" ]; then
  : > "$work/moved"
  printf 'z\n' > "$repo/z.txt"
  env -u GIT_INDEX_FILE "$real_git" -C "$repo" add z.txt
  env -u GIT_INDEX_FILE "$real_git" -C "$repo" commit -q -m "competing" -- z.txt
fi
exec "$real_git" "\$@"
SHIM
chmod +x "$work/bin/git"
printf 'a1\n' > "$repo/a.txt"
(cd "$repo" && PATH="$work/bin:$PATH" sh "$here/commit-own-paths.sh" -m "after the move" a.txt >/dev/null 2>&1) || fail "the commit failed when HEAD moved"
[ -e "$work/moved" ] || fail "the competing commit was never made"
git -C "$repo" cat-file -e HEAD:z.txt || fail "the competing commit was lost"
[ "$(git -C "$repo" show HEAD:a.txt)" = "a1" ] || fail "the retried commit is missing"
[ "$(git -C "$repo" log --format=%s -3 | tr '\n' ',')" = "after the move,competing,initial," ] || fail "the history is wrong: $(git -C "$repo" log --format=%s -3 | tr '\n' ',')"
rm -f "$work/bin/git"

new_repo seven
cat > "$work/bin/git" <<SHIM
#!/bin/sh
if [ "\$1" = "read-tree" ]; then exit 0; fi
exec "$real_git" "\$@"
SHIM
chmod +x "$work/bin/git"
printf 'a1\n' > "$repo/a.txt"
before=$(head_of)
if (cd "$repo" && PATH="$work/bin:$PATH" sh "$here/commit-own-paths.sh" -m "empty index" a.txt >"$work/out" 2>&1); then fail "a commit that deletes other files was accepted"; fi
[ "$(head_of)" = "$before" ] || fail "a commit that deletes other files was made"
grep -q 'b.txt' "$work/out" || fail "the refusal does not name a file it would delete"
rm -f "$work/bin/git"

new_repo eight
round=1
while [ "$round" -le 10 ]; do
  printf 'one-%s\n' "$round" > "$repo/a.txt"
  printf 'two-%s\n' "$round" > "$repo/b.txt"
  (cd "$repo" && sh "$here/commit-own-paths.sh" -m "one $round" a.txt >/dev/null 2>"$work/err1") &
  p1=$!
  (cd "$repo" && sh "$here/commit-own-paths.sh" -m "two $round" b.txt >/dev/null 2>"$work/err2") &
  p2=$!
  wait "$p1" || fail "caller one failed in round $round: $(cat "$work/err1")"
  wait "$p2" || fail "caller two failed in round $round: $(cat "$work/err2")"
  [ "$(git -C "$repo" show HEAD:a.txt)" = "one-$round" ] || fail "a.txt at HEAD is wrong in round $round"
  [ "$(git -C "$repo" show HEAD:b.txt)" = "two-$round" ] || fail "b.txt at HEAD is wrong in round $round"
  [ "$(git -C "$repo" ls-tree -r --name-only HEAD | wc -l | tr -d ' ')" -eq 4 ] || fail "a file was lost in round $round"
  round=$((round + 1))
done
[ "$(git -C "$repo" rev-list --count HEAD)" -eq 21 ] || fail "the history does not hold every commit: $(git -C "$repo" rev-list --count HEAD)"

echo "ok"
