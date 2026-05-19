#!/usr/bin/env sh
set -eu

REPO="${SSHT_REPO:-Davied-H/ssht}"
REF="${SSHT_REF:-main}"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
CLAUDE_HOME="${CLAUDE_HOME:-$HOME/.claude}"

usage() {
  cat <<EOF
Install ssht agent context and skills for Codex and Claude Code.

Usage:
  sh scripts/configure-agents.sh
  SSHT_REF=v0.1.0 sh scripts/configure-agents.sh

Environment:
  SSHT_REPO     GitHub repository. Defaults to Davied-H/ssht.
  SSHT_REF      Branch, tag, or commit to install from. Defaults to main.
  CODEX_HOME    Codex home. Defaults to $HOME/.codex.
  CLAUDE_HOME   Claude home. Defaults to $HOME/.claude.
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

auth_token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
if command -v curl >/dev/null 2>&1; then
  if [ -n "$auth_token" ]; then
    fetch() { curl -fsSL -H "Authorization: Bearer $auth_token" "$1" -o "$2"; }
  else
    fetch() { curl -fsSL "$1" -o "$2"; }
  fi
elif command -v wget >/dev/null 2>&1; then
  if [ -n "$auth_token" ]; then
    fetch() { wget --header="Authorization: Bearer $auth_token" -qO "$2" "$1"; }
  else
    fetch() { wget -qO "$2" "$1"; }
  fi
else
  echo "curl or wget is required." >&2
  exit 1
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

if [ -f "./AGENTS.md" ] && [ -f "./CLAUDE.md" ] && [ -d "./skills" ]; then
  src_dir=$(pwd)
  echo "Using local ssht checkout at $src_dir..."
else
  archive_url="https://codeload.github.com/$REPO/tar.gz/$REF"
  echo "Downloading ssht agent package from $REPO@$REF..."
  fetch "$archive_url" "$tmp_dir/source.tar.gz"
  tar -C "$tmp_dir" -xzf "$tmp_dir/source.tar.gz"
  src_dir=$(find "$tmp_dir" -maxdepth 1 -type d -name 'ssht-*' | head -n 1)
  if [ -z "$src_dir" ]; then
    echo "Could not find extracted ssht source." >&2
    exit 1
  fi
fi

mkdir -p "$CODEX_HOME/skills" "$CODEX_HOME/projects/ssht" "$CLAUDE_HOME/projects/ssht"

if [ -d "$src_dir/skills" ]; then
  cp -R "$src_dir/skills/." "$CODEX_HOME/skills/"
fi

if [ -f "$src_dir/AGENTS.md" ]; then
  cp "$src_dir/AGENTS.md" "$CODEX_HOME/projects/ssht/AGENTS.md"
fi

if [ -f "$src_dir/CLAUDE.md" ]; then
  cp "$src_dir/CLAUDE.md" "$CLAUDE_HOME/projects/ssht/CLAUDE.md"
fi

echo "Installed ssht Codex skills to $CODEX_HOME/skills"
echo "Installed Codex project context to $CODEX_HOME/projects/ssht/AGENTS.md"
echo "Installed Claude Code project context to $CLAUDE_HOME/projects/ssht/CLAUDE.md"
echo "In a local ssht checkout, Codex and Claude Code will also read AGENTS.md / CLAUDE.md from the repo root."
