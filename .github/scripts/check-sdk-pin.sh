#!/usr/bin/env bash
# Refuse to release while the Kestra Go SDK is pinned to anything but a clean tag.
#
# Per AGENTS.md a floating / pseudo-version / -SNAPSHOT / develop-tracking SDK pin is
# a release blocker: pseudo-versions sort BELOW tagged releases in Go semver, so a
# release built from one is not reproducible and `go get -u` can silently move it.
#
# Env override, used by check-sdk-pin_test.sh:
#   GO_MOD  path to the go.mod to check (default: repo root go.mod)
set -euo pipefail

go_mod="${GO_MOD:-$(cd "$(dirname "$0")/../.." && pwd)/go.mod}"
[ -f "$go_mod" ] || { printf 'check-sdk-pin: %s not found\n' "$go_mod" >&2; exit 1; }

# Every go-sdk requirement, majors included (go-sdk, go-sdk/v2, ...).
# A while-read pipeline rather than `mapfile`, which macOS's bash 3.2 lacks.
pins="$(grep -oE 'github\.com/kestra-io/client-sdk/go-sdk(/v[0-9]+)?[[:space:]]+v[^[:space:]/]+' "$go_mod" || true)"

if [ -z "$pins" ]; then
  printf 'check-sdk-pin: no kestra-io/client-sdk/go-sdk requirement found in %s\n' "$go_mod" >&2
  exit 1
fi

# A here-doc, not a pipe, so the loop runs in this shell and $status survives it.
status=0
while IFS= read -r pin; do
  [ -n "$pin" ] || continue
  module="${pin%%[[:space:]]*}"
  version="${pin##*[[:space:]]}"

  reason=""
  # Pseudo-version. Go writes three shapes, and the timestamp is preceded by "-"
  # in the bare form but by "." in the two that build on a base version:
  #   v2.0.0-<ts>-<sha>  |  v1.1.1-0.<ts>-<sha>  |  v1.1.1-rc1.0.<ts>-<sha>
  if printf '%s' "$version" | grep -qE -- '[-.][0-9]{14}-[0-9a-f]{12}$'; then
    reason="a pseudo-version (tracks a branch, not a release)"
  elif printf '%s' "$version" | grep -qiE -- '-SNAPSHOT'; then
    reason="a -SNAPSHOT build"
  elif printf '%s' "$version" | grep -qE -- '\+incompatible$'; then
    reason="an +incompatible version"
  elif ! printf '%s' "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    reason="not a vX.Y.Z[-prerelease] version"
  fi

  if [ -n "$reason" ]; then
    printf 'check-sdk-pin: %s is pinned to %s, which is %s.\n' "$module" "$version" "$reason" >&2
    printf '               Pin a released tag before releasing (see AGENTS.md "Branching & Releases").\n' >&2
    status=1
  else
    printf 'check-sdk-pin: %s %s ok\n' "$module" "$version"
  fi
done <<PINS
$pins
PINS

exit "$status"
