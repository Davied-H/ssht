---
name: ssht-operator
description: Use when helping a user operate ssht, the Go CLI/TUI for browsing OpenSSH Host aliases and opening SSH sessions. Trigger for user-facing ssht tasks such as installing or updating ssht, running health checks, listing hosts, opening a host, troubleshooting terminal behavior, configuring groups/tags, explaining ssht commands and shortcuts, or safely reviewing ssht-related SSH config snippets without exposing credentials.
---

# ssht Operator

## Overview

Use this skill to help a user operate `ssht` safely from their machine. Treat `ssht` as a wrapper around OpenSSH config discovery and terminal launching; actual connections must remain delegated to OpenSSH with `ssh <alias>` or ssht's existing sshpass wrapper.

## Safety Rules

- Do not ask the user to paste private keys, passwords, full personal SSH configs, or production HostName values.
- If examples are needed, use RFC 5737 documentation IPs such as `192.0.2.12`, domains under `example.com`, and generic users such as `deploy`, `demo`, or `git`.
- Do not persist SSH credentials, HostName values, IdentityFile values, or copied SSH config contents outside the user's existing SSH config.
- Inspect repository fixtures or user-provided redacted snippets by default. Only read a real SSH config path when the user explicitly asks for that path to be inspected.
- When showing `--print-hosts` output, avoid exposing sensitive fields. ssht intentionally omits `SSHPassword`, but HostName/User/IdentityFile may still be private.
- Never reimplement SSH config evaluation. Use `ssht`, `ssh -G <alias>` when appropriate, or OpenSSH itself.

## First Checks

1. Check whether `ssht` is installed:
   ```bash
   command -v ssht
   ssht --version
   ```
2. If working inside the ssht repository, prefer local commands:
   ```bash
   go test ./...
   go build -trimpath -ldflags="-s -w" -o /tmp/ssht ./cmd/ssht
   ```
3. For an installed binary, run:
   ```bash
   ssht doctor
   ssht --print-hosts
   ```
4. If the user gives a custom SSH config path, pass it explicitly:
   ```bash
   ssht --config /path/to/config doctor
   ssht --config /path/to/config --print-hosts
   ```

## Install Or Update

Use the release installer when the user wants normal installation and does not need to build from source:

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/install-release.sh | sh
```

Useful options:

```bash
SSHT_VERSION=v0.1.0 sh scripts/install-release.sh
INSTALL_DIR=/usr/local/bin sh scripts/install-release.sh
```

Use source installation only from a local checkout:

```bash
./install.sh
```

After installation, verify:

```bash
command -v ssht
ssht doctor
```

## Operate ssht

Common commands:

```bash
ssht
ssht --config ~/.ssh/config
ssht --no-include
ssht --debug
ssht doctor
ssht --print-hosts
ssht --connect demo-api
ssht --terminal auto --open-mode tab
```

Important behavior:

- `--print-hosts` prints discovered concrete Host aliases as JSON for other tools.
- `--connect <alias>` skips the TUI and opens the selected alias through the configured terminal.
- `--no-include` disables recursive `Include` parsing.
- `--debug` prints parse warnings to stderr.
- `--terminal` supports `auto`, `iterm`, `terminal`, `wezterm`, `kitty`, `alacritty`, `ghostty`, and `warp`.
- `--open-mode` supports `auto`, `window`, `tab`, and `split`.

## Diagnose Problems

For config parsing or host discovery issues:

```bash
ssht doctor
ssht --debug --print-hosts
```

For a custom config:

```bash
ssht --config /path/to/config doctor
ssht --config /path/to/config --debug --print-hosts
```

Interpret common findings:

- Duplicate Host aliases can cause confusing selection and connection behavior.
- Wildcard or negated Host patterns such as `Host *`, `Host *.internal`, and `Host !blocked` are not concrete connectable entries.
- `Include` warnings often mean an included path does not exist, a glob matches nothing, or a file cannot be read.
- Invalid ports, empty HostName values, and malformed metadata should be fixed in the user's SSH config.

For terminal launch issues, check the requested backend and mode first:

```bash
ssht --terminal auto --open-mode auto --connect demo-api
ssht --terminal iterm --open-mode split --connect demo-api
```

Use `split` only where supported. iTerm2 split behavior depends on the current iTerm2 session; Warp split requires macOS Accessibility permission for the calling app.

## Groups And Tags

ssht stores groups and tags in comments immediately before a Host entry:

```sshconfig
# ssht: group=prod tags=api,critical
Host demo-api
    HostName 192.0.2.12
    User deploy
```

Use `group` for the sidebar grouping and `tags` for search filters. Search examples:

```text
tag:api
group:prod
user:deploy
port:22
fav:
recent:
prod -db
```

To hide a Host from ssht:

```sshconfig
# ssht: hidden=true
Host demo-hidden
    HostName 192.0.2.13
    User demo
```

## TUI Shortcuts

Use these when guiding a user through the interactive UI:

- `/`: search
- `:` or `Ctrl+K`: command palette
- `o`: settings
- `Enter`: open selected host
- `Space`: mark host for batch actions
- `[` / `]`: switch group
- `g`: move host to group
- `f`: toggle favorite
- `H`: local connection history
- `W`: parse warnings
- `A` / `e` / `d`: add, edit, delete host
- `?`: help
- `q` / `Esc`: quit

## State And Privacy

ssht may save favorites, recent connection timestamps, connection counts, group order, empty groups, and TUI preferences. It must not save HostName, User, IdentityFile, passwords, private keys, or other SSH credentials.

State paths:

- With `XDG_STATE_HOME`: `$XDG_STATE_HOME/ssht/state.json`
- macOS default: `~/Library/Application Support/ssht/state.json`
- Other systems: `~/.local/state/ssht/state.json`

When debugging state, inspect structure and timestamps without copying sensitive surrounding files into notes or reports.

## Raycast Companion

If the user operates ssht through Raycast, remember that the extension calls:

```bash
ssht --print-hosts
ssht --connect <alias>
```

If Raycast cannot find `ssht`, ask the user to set the extension preference `ssht Path` to the installed binary path from `command -v ssht`. Keep the Raycast config aligned with any custom SSH config path, terminal, or open mode the user expects.
