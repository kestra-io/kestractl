#!/usr/bin/env bash
# Installs the latest kestractl v2 release (prereleases included) by default.
# Thin wrapper around install.sh: defaults VERSION=2, everything else
# (VERSION, INSTALL_DIR, BINARY_NAME, GITHUB_REPO) can still be overridden
# via environment variables exactly like install.sh.
set -euo pipefail

INSTALL_SCRIPT_URL="${INSTALL_SCRIPT_URL:-https://raw.githubusercontent.com/kestra-io/kestractl/main/install-scripts/install.sh}"

err() {
  printf "Error: %s\n" "$1" >&2
  exit 1
}

export VERSION="${VERSION:-2}"

# When executed as a file (not piped), prefer the sibling install.sh from the
# same checkout over fetching it from the network.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [ -f "$script_dir/install.sh" ]; then
    exec bash "$script_dir/install.sh"
  fi
fi

# Piped via curl/wget: fetch install.sh and run it.
if command -v curl >/dev/null 2>&1; then
  script="$(curl -fsSL "$INSTALL_SCRIPT_URL")" || err "Download failed: $INSTALL_SCRIPT_URL"
elif command -v wget >/dev/null 2>&1; then
  script="$(wget -qO- "$INSTALL_SCRIPT_URL")" || err "Download failed: $INSTALL_SCRIPT_URL"
else
  err "curl or wget is required"
fi
[ -n "$script" ] || err "Downloaded install script is empty: $INSTALL_SCRIPT_URL"

exec bash -c "$script"
