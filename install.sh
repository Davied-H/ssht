#!/usr/bin/env sh
set -eu

BIN_NAME="ssht"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<EOF
Install $BIN_NAME.

Usage:
  ./install.sh
  INSTALL_DIR=/usr/local/bin ./install.sh

Environment:
  INSTALL_DIR  Directory to install $BIN_NAME into. Defaults to $HOME/.local/bin.
EOF
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    echo "Unknown argument: $1" >&2
    usage >&2
    exit 2
    ;;
esac

if ! command -v go >/dev/null 2>&1; then
  echo "go is required but was not found on PATH." >&2
  echo "Install Go 1.22 or newer, then run ./install.sh again." >&2
  exit 1
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

mkdir -p "$INSTALL_DIR"

echo "Building $BIN_NAME..."
(
  cd "$SCRIPT_DIR"
  go build -trimpath -ldflags="-s -w" -o "$TMP_DIR/$BIN_NAME" ./cmd/ssht
)

install -m 0755 "$TMP_DIR/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"

echo "Installed $BIN_NAME to $INSTALL_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    echo "Run it with: $BIN_NAME"
    ;;
  *)
    echo "Add this directory to PATH to run it as '$BIN_NAME':"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if ! osascript -e 'id of application "iTerm"' >/dev/null 2>&1 \
  && ! osascript -e 'id of application "Terminal"' >/dev/null 2>&1 \
  && ! command -v wezterm >/dev/null 2>&1 \
  && ! command -v kitty >/dev/null 2>&1 \
  && ! command -v alacritty >/dev/null 2>&1 \
  && ! command -v ghostty >/dev/null 2>&1; then
  echo "Note: no supported terminal was found. $BIN_NAME can list hosts, but connecting needs iTerm2, Terminal.app, WezTerm, kitty, Alacritty, or Ghostty."
fi
