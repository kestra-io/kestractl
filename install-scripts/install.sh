#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="${BINARY_NAME:-kestractl}"
GITHUB_REPO="${GITHUB_REPO:-kestra-io/kestractl}"
INSTALL_DIR="${INSTALL_DIR:-}"
VERSION="${VERSION:-}"

err() {
  printf "Error: %s\n" "$1" >&2
  exit 1
}

require() {
  command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"
}

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    err "curl or wget is required"
  fi
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin|linux)
    ;;
  *)
    err "Unsupported OS: $os"
    ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)
    arch="amd64"
    ;;
  arm64|aarch64)
    arch="arm64"
    ;;
  *)
    err "Unsupported architecture: $arch"
    ;;
esac


require awk
release_json="$(mktemp)"
if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ]; then
  download "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" "$release_json"
else
  download "https://api.github.com/repos/${GITHUB_REPO}/releases/tags/${VERSION}" "$release_json"
fi
VERSION="$(awk -F '"' '/"tag_name":/ {print $4; exit}' "$release_json")"


asset_name="${BINARY_NAME}_${VERSION}_${os}_${arch}"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

asset_url="$(awk -v name="$asset_name" -F '"' '$2=="name" && $4==name {found=1} found && $2=="browser_download_url" {print $4; exit}' "$release_json")"
checksums_url="$(awk -F '"' '$2=="name" && $4=="checksums.txt" {found=1} found && $2=="browser_download_url" {print $4; exit}' "$release_json")"
rm -f "$release_json"

[ -n "$asset_url" ] || err "Release asset not found for tag ${VERSION}"
[ -n "$checksums_url" ] || err "checksums.txt not found for tag ${VERSION}"

download "$asset_url" "$tmp_dir/$asset_name"
download "$checksums_url" "$tmp_dir/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  checksum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  checksum_cmd="shasum -a 256"
else
  err "sha256sum or shasum is required for verification"
fi

expected_sum="$(awk -v name="$asset_name" '$2 == name {print $1}' "$tmp_dir/checksums.txt")"
[ -n "$expected_sum" ] || err "Checksum not found for ${asset_name}"
actual_sum="$($checksum_cmd "$tmp_dir/$asset_name" | awk '{print $1}')"

if [ "$expected_sum" != "$actual_sum" ]; then
  err "Checksum verification failed"
fi

if [ -z "$INSTALL_DIR" ]; then
  if [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

mkdir -p "$INSTALL_DIR"
install "$tmp_dir/$asset_name" "$INSTALL_DIR/$BINARY_NAME"

printf "%s installed to %s/%s\n" "$BINARY_NAME" "$INSTALL_DIR" "$BINARY_NAME"

if [ "$INSTALL_DIR" = "${HOME}/.local/bin" ]; then
  printf "Add %s to your PATH if needed:\n" "$INSTALL_DIR"
  printf "  export PATH=\"%s:$PATH\"\n" "$INSTALL_DIR"
fi
