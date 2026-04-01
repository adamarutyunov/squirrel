#!/bin/sh
set -e

SQUIRREL_REPO="adamarutyunov/squirrel"
LAUNCH_REPO="adamarutyunov/launch"
SQUIRREL_BIN="sq"
LAUNCH_BIN="launch"
INSTALL_DIR="/usr/local/bin"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command '$1' is not installed"
    exit 1
  fi
}

install_bin() {
  src="$1"
  dst="$2"

  if [ -w "$INSTALL_DIR" ]; then
    install -m 755 "$src" "$INSTALL_DIR/$dst"
  else
    sudo install -m 755 "$src" "$INSTALL_DIR/$dst"
  fi
}

resolve_version() {
  repo="$1"
  version=$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

  if [ -z "$version" ]; then
    echo "Error: could not determine latest version for ${repo}"
    exit 1
  fi

  echo "$version"
}

download_release() {
  repo="$1"
  version="$2"
  archive="$3"
  bin="$4"
  target="$5"
  url="https://github.com/${repo}/releases/download/${version}/${archive}_${OS}_${ARCH}.tar.gz"

  curl -fsSL "$url" | tar -xz -C "$target"
  install_bin "$target/$bin" "$bin"
}

require_cmd curl
require_cmd tar
require_cmd install

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  darwin|linux) ;;
  *) echo "Error: unsupported OS '$OS'"; exit 1 ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Error: unsupported architecture '$ARCH'"; exit 1 ;;
esac

if [ -z "$VERSION" ]; then
  VERSION=$(resolve_version "$SQUIRREL_REPO")
fi

if [ -z "$LAUNCH_VERSION" ]; then
  LAUNCH_VERSION=$(resolve_version "$LAUNCH_REPO")
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

if ! command -v "$LAUNCH_BIN" >/dev/null 2>&1; then
  echo "Installing launch dependency (${LAUNCH_VERSION})..."
  download_release "$LAUNCH_REPO" "$LAUNCH_VERSION" "$LAUNCH_BIN" "$LAUNCH_BIN" "$TMP"
else
  echo "launch already installed; skipping binary install"
fi

echo "Installing squirrel ${VERSION} (${OS}/${ARCH})..."
download_release "$SQUIRREL_REPO" "$VERSION" "squirrel" "$SQUIRREL_BIN" "$TMP"

echo "Done. Run: $SQUIRREL_BIN"
