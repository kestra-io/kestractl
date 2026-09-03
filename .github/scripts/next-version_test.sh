#!/usr/bin/env bash
# Table-driven tests for next-version.sh. Release-critical math, so it is unit
# tested rather than discovered in production.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/next-version.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
failures=0

# check <name> <latest-tag> <expected-tag-or-empty> <commit messages...>
# Each trailing argument is ONE whole commit message, newlines and all.
check() {
  local name="$1" latest="$2" want="$3"
  shift 3
  local f="$tmp/records"
  printf '%s\0' "$@" >"$f"

  local got status
  got="$(LATEST_TAG="$latest" COMMIT_MESSAGES_FILE="$f" bash "$script" 2>/dev/null)"
  status=$?

  if [ "$status" -ne 0 ]; then
    printf 'FAIL %s: exited %d\n' "$name" "$status"
    failures=$((failures + 1))
    return
  fi
  if [ "$got" != "$want" ]; then
    printf 'FAIL %s: want %s, got %s\n' "$name" "${want:-<none>}" "${got:-<none>}"
    failures=$((failures + 1))
    return
  fi
  printf 'ok   %s -> %s\n' "$name" "${got:-<none>}"
}

# check_fails <name> <latest-tag> <commit messages...>
check_fails() {
  local name="$1" latest="$2"
  shift 2
  local f="$tmp/records"
  printf '%s\0' "$@" >"$f"
  if LATEST_TAG="$latest" COMMIT_MESSAGES_FILE="$f" bash "$script" >/dev/null 2>&1; then
    printf 'FAIL %s: expected a non-zero exit\n' "$name"
    failures=$((failures + 1))
    return
  fi
  printf 'ok   %s -> refused\n' "$name"
}

# --- prerelease base: the counter advances, the semver triple is frozen ---
check 'rc2 + feat'                v2.0.0-rc2  v2.0.0-rc.3  'feat(cli): add a thing'
check 'rc2 + fix'                 v2.0.0-rc2  v2.0.0-rc.3  'fix(cli): fix a thing'
check 'legacy rc1 normalises'     v2.0.0-rc1  v2.0.0-rc.2  'feat: x'
check 'dotted rc.9 -> rc.10'      v2.0.0-rc.9 v2.0.0-rc.10 'fix: x'
check 'breaking stays on rc line' v2.0.0-rc2  v2.0.0-rc.3  'feat(cli)!: drop a flag'
check 'BREAKING CHANGE body'      v2.0.0-rc2  v2.0.0-rc.3  "$(printf 'feat: x\n\nBREAKING CHANGE: nope')"

# --- release base: standard semver ---
check 'GA + feat'  v2.1.0 v2.2.0 'feat: x'
check 'GA + fix'   v2.1.0 v2.1.1 'fix: x'
check 'GA + perf'  v2.1.0 v2.1.1 'perf: x'
check 'GA + build' v2.1.0 v2.1.1 'build: x'
check 'GA + feat!' v2.1.0 v3.0.0 'feat!: x'
check 'GA + BREAKING CHANGE body' v2.1.0 v3.0.0 "$(printf 'fix: x\n\nBREAKING CHANGE: nope')"
check 'double-digit minor' v2.9.0  v2.10.0 'feat: x'
check 'double-digit patch' v2.1.9  v2.1.10 'fix: x'

# --- multi-commit ranges: every record must be classified, not just the newest ---
# These inject records directly, so they cover the ranking loop, NOT the git output
# format -- the real `git log` invocation is pinned by the fixture test at the end.
check 'feat behind a chore'  v2.0.0-rc2 v2.0.0-rc.3 \
  "$(printf 'chore: tidy\n\nbody\n')" "$(printf 'feat(cli): a real feature\n\nbody\n')"
check 'feat behind two docs' v2.1.0 v2.2.0 \
  "$(printf 'docs: a\n\nbody\n')" "$(printf 'docs: b\n\nbody\n')" "$(printf 'feat: c\n\nbody\n')"
check 'breaking last'        v2.1.0 v3.0.0 \
  "$(printf 'fix: a\n\nbody\n')" "$(printf 'feat!: b\n\nbody\n')"

# --- highest bump in the range wins, regardless of order ---
check 'feat then fix' v2.1.0 v2.2.0 'fix: a' 'feat: b'
check 'fix then feat' v2.1.0 v2.2.0 'feat: b' 'fix: a'
check 'feat then breaking' v2.1.0 v3.0.0 'feat: b' 'fix!: a'

# --- no release ---
check 'docs only'             v2.0.0-rc2 '' 'docs: readme'
check 'chore/test/ci only'    v2.0.0-rc2 '' 'chore: tidy' 'test: more' 'ci: tweak'
check 'style only'            v2.0.0-rc2 '' 'style: gofmt'
check 'no conventional match' v2.0.0-rc2 '' 'Merge branch main' 'wip'
check 'empty range'           v2.0.0-rc2 '' ''

# --- only the SUBJECT carries the type; a body that discusses types must not ---
# Regression: this project's own "chore(ci): ..." commit has a body reading
# "computes v2.0.0 for both / feat: and fix:, which would ...". Classifying per
# line rather than per commit read that as a feat and cut a spurious release.
check 'body mentioning feat:' v2.0.0-rc2 '' "$(printf 'chore(ci): wire up the tagger\n\ncomputes v2.0.0 for both\nfeat: and fix:, which would silently release.\n')"
check 'body quoting fix:'     v2.0.0-rc2 '' "$(printf 'docs: explain the mapping\n\nfix: maps to patch\nfeat: maps to minor\n')"
# ...but a real feat subject with a chatty body still releases.
check 'feat with chatty body' v2.0.0-rc2 v2.0.0-rc.3 "$(printf 'feat(cli): add a flag\n\ndocs: not a real subject\nchore: nor this\n')"

# --- chore(deps) ships, bare chore does not ---
check 'chore(deps)'      v2.1.0 v2.1.1 'chore(deps): bump go-sdk to v2.0.0-rc3'
check 'chore(sdk)'       v2.1.0 ''     'chore(sdk): unrelated scope'
check 'chore(ci)'        v2.1.0 ''     'chore(ci): a CI change does not release'

# --- the [skip release] escape hatch beats every other commit ---
check 'skip in subject'       v2.1.0 '' 'feat: big thing [skip release]'
check 'skip on its own line'  v2.1.0 '' "$(printf 'feat: big thing\n\n[skip release]\n')"
check 'skip indented line'    v2.1.0 '' "$(printf 'feat: big thing\n\n  [skip release]  \n')"
check 'skip in another commit' v2.1.0 '' 'feat: shipped' 'chore: oops [skip release]'

# Prose that merely MENTIONS the marker must not suppress a real release. The
# commit introducing this feature documents "[skip release]" in its own body, which
# is how the too-broad match was noticed.
check 'marker mentioned in prose' v2.1.0 v2.2.0 \
  "$(printf 'feat(cli): add a flag\n\nbodies are scanned for [skip release] only on\ntheir own line, not anywhere.\n')"
check 'marker mid-sentence'       v2.1.0 v2.2.0 \
  "$(printf 'feat: a thing\n\nnot using [skip release] here, we want this out.\n')"

# --- refuse to guess rather than invent a version ---
check_fails 'unknown prerelease suffix' v2.0.0-beta7 'feat: x'
check_fails 'unparseable base tag'      not-a-tag    'feat: x'
# ...but a no-op range is fine on any base, since no math is needed.
check 'unknown suffix, no-op range' v2.0.0-beta7 '' 'docs: x'

# --- the real `git log` path, against a throwaway repo ---
# This is the only test that exercises how commits are read out of git, and the
# reason it exists: `git log --format=%B%x00` joins entries with a newline, so every
# record after the first began with "\n", its subject read as empty, and only the
# newest commit in the range was ever classified. `git log -z` terminates records
# with NUL and has no such seam. Record-injection tests cannot catch that.
check_repo() {
  local name="$1" want="$2"
  shift 2
  local repo="$tmp/repo"
  rm -rf "$repo"
  mkdir -p "$repo"

  (
    cd "$repo"
    git init -q
    git config user.email t@example.com
    git config user.name test
    git config commit.gpgsign false
    git commit -q --allow-empty -m "chore: base"
    git tag v2.0.0-rc2
    for msg in "$@"; do
      git commit -q --allow-empty -m "$msg"
    done
  )

  local got
  got="$(cd "$repo" && bash "$script" 2>/dev/null)"
  if [ "$got" != "$want" ]; then
    printf 'FAIL %s: want %s, got %s\n' "$name" "${want:-<none>}" "${got:-<none>}"
    failures=$((failures + 1))
    return
  fi
  printf 'ok   %s -> %s\n' "$name" "${got:-<none>}"
}

# A feat behind two non-releasing commits: only reachable if every record parses.
check_repo 'git: feat behind chores' v2.0.0-rc.3 \
  "feat(cli): the real feature" "chore: tidy up" "docs: explain it"
# The base tag is found by git describe, and a no-op range stays a no-op.
check_repo 'git: no-op range'       ''          "docs: only" "chore: only"
# Multi-paragraph bodies must not shift the record boundaries.
check_repo 'git: multiline bodies'  v2.0.0-rc.3 \
  "$(printf 'fix(cli): a fix\n\nfirst para\n\nsecond para\n')" \
  "$(printf 'chore: tidy\n\nfeat: a body line that must be ignored\n')"

if [ "$failures" -ne 0 ]; then
  printf '\n%d test(s) failed\n' "$failures"
  exit 1
fi
printf '\nall next-version.sh tests passed\n'
