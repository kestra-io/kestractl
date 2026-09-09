#!/usr/bin/env bash
# Tests for render-compat-badges.sh, which keeps the README's compatibility
# badges in sync with COMPATIBLE_KESTRA_VERSION.properties.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/render-compat-badges.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
failures=0

fail() { printf 'FAIL %s: %s\n' "$1" "$2"; failures=$((failures + 1)); }
pass() { printf 'ok   %s\n' "$1"; }

# readme <marker-body> -- a minimal README with the markers in place.
readme() {
  printf '# kestractl\n\n<!-- compat-badges:start -->\n%s<!-- compat-badges:end -->\n\nprose below\n' "$1"
}

# render <name> <properties-body> <expected-badge-lines>
render() {
  local name="$1" props="$2" want="$3"
  local p="$tmp/props" r="$tmp/README.md"
  printf '%s' "$props" >"$p"
  readme '' >"$r"

  if ! PROPERTIES="$p" README="$r" bash "$script" >/dev/null 2>&1; then
    fail "$name" 'script exited non-zero'
    return
  fi

  local got
  got="$(sed -n '/compat-badges:start/,/compat-badges:end/p' "$r" | sed -e '1d' -e '$d')"
  if [ "$got" != "$want" ]; then
    fail "$name" "badges differ
--- want ---
$want
--- got ----
$got"
    return
  fi
  pass "$name"
}

# badge <alt-text> <label-and-message> <color> -- one expected markdown line.
badge() {
  printf '[![%s](https://img.shields.io/badge/%s-%s?style=for-the-badge&labelColor=15112B)](COMPATIBLE_KESTRA_VERSION.properties)' \
    "$1" "$2" "$3"
}
versions() { badge 'Compatible Kestra versions' "Kestra-$1" '8405FF'; }
tracked() { badge "Kestra $1" "$2-tracked" '6B4BA8'; }

# --- rendering -------------------------------------------------------------

# The file as it actually ships: ascending versions, `develop` last, and no
# trailing newline on that last line.
render 'repo shape' 'v1.0
v1.1
v1.2
v1.3
v2.0
develop' \
"$(versions '2.0%20%7C%201.3%20%7C%201.2%20%7C%201.1%20%7C%201.0')
$(tracked develop develop)"

# Newest first regardless of how many lines, `v` prefix stripped, and a file
# that does end in a newline works the same.
render 'trailing newline and single version' 'v2.0
' \
"$(versions '2.0')"

render 'blank lines and comments skipped' '# only these matter

v1.3

v2.0
' \
"$(versions '2.0%20%7C%201.3')"

render 'surrounding whitespace trimmed' '  v1.3
	v2.0
' \
"$(versions '2.0%20%7C%201.3')"

# shields.io treats `-` as its field separator and `_` as a space, so both must
# be doubled or the badge renders as the wrong number of fields.
render 'shields metacharacters escaped' 'v2.0
develop-nightly
some_branch
' \
"$(versions '2.0')
$(tracked 'develop-nightly' 'develop--nightly')
$(tracked 'some_branch' 'some__branch')"

# --- README handling -------------------------------------------------------

p="$tmp/props"; r="$tmp/README.md"
printf 'v2.0\ndevelop' >"$p"

# Content outside the markers must survive untouched.
readme '' >"$r"
PROPERTIES="$p" README="$r" bash "$script" >/dev/null 2>&1
if grep -qF '# kestractl' "$r" && grep -qF 'prose below' "$r"; then
  pass 'preserves content outside the markers'
else
  fail 'preserves content outside the markers' 'surrounding lines lost'
fi

# Running twice must not append or drift.
first="$(cat "$r")"
PROPERTIES="$p" README="$r" bash "$script" >/dev/null 2>&1
if [ "$first" = "$(cat "$r")" ]; then
  pass 'idempotent'
else
  fail 'idempotent' 'second run changed the README'
fi

# --check on the freshly rendered README.
if PROPERTIES="$p" README="$r" bash "$script" --check >/dev/null 2>&1; then
  pass '--check accepts a fresh README'
else
  fail '--check accepts a fresh README' 'reported stale'
fi

# --check must fail, and must not rewrite, when the badges are stale.
readme 'stale badge\n' >"$r"
before="$(cat "$r")"
if PROPERTIES="$p" README="$r" bash "$script" --check >/dev/null 2>&1; then
  fail '--check rejects a stale README' 'reported in sync'
elif [ "$before" != "$(cat "$r")" ]; then
  fail '--check rejects a stale README' '--check rewrote the file'
else
  pass '--check rejects a stale README'
fi

# A README that predates the markers must fail loudly, not silently no-op.
printf '# kestractl\n\nno markers here\n' >"$r"
if PROPERTIES="$p" README="$r" bash "$script" >/dev/null 2>&1; then
  fail 'missing markers rejected' 'accepted a README without markers'
else
  pass 'missing markers rejected'
fi

# --- bad input -------------------------------------------------------------

readme '' >"$r"
printf '# comments only\n\n' >"$p"
if PROPERTIES="$p" README="$r" bash "$script" >/dev/null 2>&1; then
  fail 'empty properties rejected' 'accepted a file with no versions'
else
  pass 'empty properties rejected'
fi

printf 'v2.0\n' >"$p"
if PROPERTIES="$p" README="$r" bash "$script" --oops >/dev/null 2>&1; then
  fail 'unknown flag rejected' 'accepted --oops'
else
  pass 'unknown flag rejected'
fi

if PROPERTIES="$tmp/nope" README="$r" bash "$script" >/dev/null 2>&1; then
  fail 'missing properties file rejected' 'accepted a nonexistent path'
else
  pass 'missing properties file rejected'
fi

# --- the real files --------------------------------------------------------

# The committed README must already be in sync, so CI's --check is meaningful.
if bash "$script" --check >/dev/null 2>&1; then
  pass 'committed README is in sync'
else
  fail 'committed README is in sync' 'run .github/scripts/render-compat-badges.sh'
fi

if [ "$failures" -ne 0 ]; then
  printf '\n%d test(s) failed\n' "$failures"
  exit 1
fi
printf '\nall tests passed\n'
