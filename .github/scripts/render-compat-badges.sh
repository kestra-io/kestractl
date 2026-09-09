#!/usr/bin/env bash
# Render the README's compatible-Kestra-version badges from
# COMPATIBLE_KESTRA_VERSION.properties, the single source of truth that also
# drives the e2e matrix (see .github/workflows/list-versions.yml).
#
# Usage:
#   render-compat-badges.sh            rewrite the README block in place
#   render-compat-badges.sh --check    exit 1 if the README block is stale (CI)
#
# Env overrides, used by render-compat-badges_test.sh:
#   PROPERTIES  path to the properties file (default: repo root)
#   README      path to the README to rewrite (default: repo root)
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
properties="${PROPERTIES:-$root/COMPATIBLE_KESTRA_VERSION.properties}"
readme="${README:-$root/README.md}"

start_marker='<!-- compat-badges:start -->'
end_marker='<!-- compat-badges:end -->'

# Kept in one place so a rebrand is a one-line change.
released_color='8405FF'   # Kestra purple: version lines with a release behind them
moving_color='6B4BA8'     # dimmed: moving targets like `develop`, not a release
label_color='15112B'
badge_style='for-the-badge'
properties_link='COMPATIBLE_KESTRA_VERSION.properties'

for f in "$properties" "$readme"; do
  [ -f "$f" ] || { printf 'render-compat-badges: %s not found\n' "$f" >&2; exit 1; }
done

mode=rewrite
case "${1:-}" in
  --check) mode=check ;;
  '') ;;
  *) printf 'render-compat-badges: unknown argument %s (want --check or nothing)\n' "$1" >&2; exit 1 ;;
esac

# shields.io reserves `-` (field separator) and `_` (space); both are escaped by
# doubling. A literal space becomes %20 and the separator pipe %7C.
shields_escape() {
  printf '%s' "$1" | sed -e 's/-/--/g' -e 's/_/__/g' -e 's/|/%7C/g' -e 's/ /%20/g'
}

# Split the file into release lines (numeric, e.g. v1.3) and moving targets
# (anything else, e.g. develop). `|| [ -n "$line" ]` so the last line still
# counts when the file has no trailing newline -- this one currently doesn't.
releases=''
moving=''
while IFS= read -r line || [ -n "$line" ]; do
  # Trim surrounding whitespace, skip blanks and `#` comments.
  version="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  [ -n "$version" ] || continue
  case "$version" in \#*) continue ;; esac

  if printf '%s' "$version" | grep -qE '^v?[0-9]'; then
    # Display without the `v` prefix; newest first, so prepend.
    releases="${version#v}${releases:+ | }$releases"
  else
    moving="${moving:+$moving }$version"
  fi
done <"$properties"

[ -n "$releases" ] || { printf 'render-compat-badges: %s lists no version lines\n' "$properties" >&2; exit 1; }

# Newest-first ordering comes from reversing the file, which is maintained in
# ascending order. Deliberately NOT a version sort: neither `sort -V` nor
# `sort -t. -k1,1n` is semver-correct on prerelease suffixes (see AGENTS.md).
block="$start_marker
[![Compatible Kestra versions](https://img.shields.io/badge/Kestra-$(shields_escape "$releases")-${released_color}?style=${badge_style}&labelColor=${label_color})](${properties_link})"

for target in $moving; do
  block="$block
[![Kestra $target](https://img.shields.io/badge/$(shields_escape "$target")-tracked-${moving_color}?style=${badge_style}&labelColor=${label_color})](${properties_link})"
done

block="$block
$end_marker"

grep -qF "$start_marker" "$readme" || {
  printf 'render-compat-badges: %s has no %s marker\n' "$readme" "$start_marker" >&2
  exit 1
}
grep -qF "$end_marker" "$readme" || {
  printf 'render-compat-badges: %s has no %s marker\n' "$readme" "$end_marker" >&2
  exit 1
}

# Replace everything between the markers, inclusive. Done in awk rather than
# sed so the multi-line replacement needs no escaping of its own.
rendered="$(BLOCK="$block" awk -v s="$start_marker" -v e="$end_marker" '
  index($0, s) { print ENVIRON["BLOCK"]; skip = 1; next }
  index($0, e) { skip = 0; next }
  !skip
' "$readme")"

if [ "$mode" = check ]; then
  if printf '%s\n' "$rendered" | diff -u "$readme" - >/dev/null; then
    printf 'render-compat-badges: README badges match %s\n' "$(basename "$properties")"
    exit 0
  fi
  printf 'render-compat-badges: README badges are stale (want <, got >):\n' >&2
  printf '%s\n' "$rendered" | diff -u "$readme" - >&2 || true
  printf 'render-compat-badges: run .github/scripts/render-compat-badges.sh to regenerate.\n' >&2
  exit 1
fi

printf '%s\n' "$rendered" >"$readme"
printf 'render-compat-badges: wrote %s\n' "$readme"
