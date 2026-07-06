#!/usr/bin/env sh
set -eu

BIN_NAME="ssht"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<EOF
Install $BIN_NAME.

Usage:
  ./install.sh
  ./install.sh --raycast
  INSTALL_DIR=/usr/local/bin ./install.sh

Environment:
  INSTALL_DIR  Directory to install $BIN_NAME into. Defaults to $HOME/.local/bin.

Options:
  --raycast    Also register the Raycast extension from raycast/.
EOF
}

INSTALL_RAYCAST=0
case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  --raycast)
    INSTALL_RAYCAST=1
    ;;
  "")
    ;;
  *)
    echo "Unknown argument: $1" >&2
    usage >&2
    exit 2
    ;;
esac

if [ "${2:-}" != "" ]; then
  echo "Unknown argument: $2" >&2
  usage >&2
  exit 2
fi

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

if [ "$INSTALL_RAYCAST" -eq 1 ]; then
  if [ ! -d "$SCRIPT_DIR/raycast" ]; then
    echo "raycast/ directory was not found next to install.sh." >&2
    exit 1
  fi
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required to install the Raycast extension." >&2
    echo "Install Node.js/npm, then run ./install.sh --raycast again." >&2
    exit 1
  fi
  echo "Installing Raycast extension dependencies..."
  (
    cd "$SCRIPT_DIR/raycast"
    if [ -f package-lock.json ]; then
      npm ci
    else
      npm install
    fi
  )

  echo "Registering Raycast extension..."
  raycast_log="$TMP_DIR/raycast-develop.log"
  (
    cd "$SCRIPT_DIR/raycast"
    npm run dev -- --non-interactive
  ) >"$raycast_log" 2>&1 &
  raycast_pid=$!
  raycast_ready=0
  attempts=0
  while [ "$attempts" -lt 30 ]; do
    if grep -q "ready  - built extension successfully" "$raycast_log"; then
      raycast_ready=1
      break
    fi
    if ! kill -0 "$raycast_pid" >/dev/null 2>&1; then
      break
    fi
    attempts=$((attempts + 1))
    sleep 1
  done
  if kill -0 "$raycast_pid" >/dev/null 2>&1; then
    kill "$raycast_pid" >/dev/null 2>&1 || true
    wait "$raycast_pid" >/dev/null 2>&1 || true
  fi
  if [ "$raycast_ready" -ne 1 ]; then
    echo "Raycast extension did not finish registering." >&2
    cat "$raycast_log" >&2
    exit 1
  fi
  echo "Registered Raycast extension. Search for: Search SSH Hosts"
fi
