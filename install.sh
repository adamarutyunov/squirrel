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
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/' || true)

  if [ -n "$version" ]; then
    echo "$version"
  else
    echo "main"
  fi
}

clone_repo() {
  repo="$1"
  version="$2"
  target="$3"

  if [ "$version" = "main" ]; then
    git clone --depth 1 "https://github.com/${repo}.git" "$target"
  else
    git clone --depth 1 --branch "$version" "https://github.com/${repo}.git" "$target"
  fi
}

require_cmd curl
require_cmd git
require_cmd go

if [ -z "$VERSION" ]; then
  VERSION=$(resolve_version "$SQUIRREL_REPO")
fi

if [ -z "$LAUNCH_VERSION" ]; then
  LAUNCH_VERSION=$(resolve_version "$LAUNCH_REPO")
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Fetching sources..."
clone_repo "$LAUNCH_REPO" "$LAUNCH_VERSION" "$TMP/launch"
clone_repo "$SQUIRREL_REPO" "$VERSION" "$TMP/squirrel"

if ! command -v "$LAUNCH_BIN" >/dev/null 2>&1; then
  echo "Installing launch dependency (${LAUNCH_VERSION})..."
  (
    cd "$TMP/launch"
    GOCACHE="${GOCACHE:-$TMP/.gocache}" go build -o "$TMP/$LAUNCH_BIN" .
  )
  install_bin "$TMP/$LAUNCH_BIN" "$LAUNCH_BIN"
else
  echo "launch already installed; skipping binary install"
fi

echo "Installing squirrel ${VERSION}..."
(
  cd "$TMP/squirrel"
  GOCACHE="${GOCACHE:-$TMP/.gocache}" go build -o "$TMP/$SQUIRREL_BIN" .
)
install_bin "$TMP/$SQUIRREL_BIN" "$SQUIRREL_BIN"

echo "Done. Run: $SQUIRREL_BIN"
