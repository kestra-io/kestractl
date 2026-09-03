#!/usr/bin/env bash
# Tests for check-sdk-pin.sh, the release blocker guarding the Go SDK pin.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/check-sdk-pin.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
failures=0

# check <name> <accept|reject> <go.mod body>
check() {
  local name="$1" want="$2" body="$3"
  local f="$tmp/go.mod"
  printf '%s\n' "$body" >"$f"

  local got=accept
  GO_MOD="$f" bash "$script" >/dev/null 2>&1 || got=reject

  if [ "$got" != "$want" ]; then
    printf 'FAIL %s: want %s, got %s\n' "$name" "$want" "$got"
    failures=$((failures + 1))
    return
  fi
  printf 'ok   %s -> %s\n' "$name" "$got"
}

req() { printf 'module github.com/kestra-io/kestractl\n\nrequire (\n\t%s\n)\n' "$1"; }

# Clean tags release.
check 'released tag' accept \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-rc3')"
check 'GA tag' accept \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk/v2 v2.1.0')"
check 'v1 module path' accept \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk v1.3.0')"

# The real pin this repo carried on main (commit "fix: pin go-sdk to a
# develop-tracking pseudo-version") -- exactly what must never be released.
check 'develop-tracking pseudo-version' reject \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk v1.1.1-0.20260702143038-8c3851bea2e1')"
check 'bare pseudo-version' reject \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-20260702143038-8c3851bea2e1')"
check 'snapshot' reject \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-SNAPSHOT')"
check 'incompatible' reject \
  "$(req 'github.com/kestra-io/client-sdk/go-sdk v2.0.0+incompatible')"

# A missing requirement means the grep silently matched nothing -- fail loudly
# rather than release something unverified.
check 'no go-sdk requirement' reject \
  "$(req 'github.com/spf13/cobra v1.8.0')"

# One bad pin among good ones must still block.
check 'mixed pins' reject \
  "$(printf 'module m\n\nrequire (\n\tgithub.com/kestra-io/client-sdk/go-sdk v1.3.0\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-0.20260702143038-8c3851bea2e1\n)\n')"

# A `replace` defeats the require-line check: the pin can read clean while the build
# resolves something else entirely.
check 'replace to a pseudo-version' reject \
  "$(printf 'module m\n\nrequire (\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-rc3\n)\n\nreplace github.com/kestra-io/client-sdk/go-sdk/v2 => github.com/foo/bar v0.0.0-20260702143038-8c3851bea2e1\n')"
check 'replace to a local dir' reject \
  "$(printf 'module m\n\nrequire (\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-rc3\n)\n\nreplace github.com/kestra-io/client-sdk/go-sdk/v2 => ../client-sdk/go-sdk\n')"
check 'replace inside a block' reject \
  "$(printf 'module m\n\nrequire (\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-rc3\n)\n\nreplace (\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 => ../go-sdk\n)\n')"
# An unrelated replace must not block a release.
check 'unrelated replace' accept \
  "$(printf 'module m\n\nrequire (\n\tgithub.com/kestra-io/client-sdk/go-sdk/v2 v2.0.0-rc3\n)\n\nreplace github.com/spf13/cobra => github.com/fork/cobra v1.9.0\n')"

if [ "$failures" -ne 0 ]; then
  printf '\n%d test(s) failed\n' "$failures"
  exit 1
fi
printf '\nall check-sdk-pin.sh tests passed\n'
