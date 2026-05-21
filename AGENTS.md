# ssht Agent Instructions

These instructions are for Codex and other code agents working in this repository.

## Project Shape

- `ssht` is a Go CLI/TUI that reads OpenSSH config files and opens selected hosts through the user's terminal.
- Connection behavior must always delegate to OpenSSH with `ssh <alias>` or the existing sshpass wrapper. Do not reimplement SSH config evaluation.
- The tool must not persist passwords, private keys, HostName values, IdentityFile values, or other SSH credentials outside the user's existing SSH config.
- `raycast/` contains the companion Raycast extension. Keep its dependency lockfile in sync when changing package metadata.

## Common Commands

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o /tmp/ssht ./cmd/ssht
sh scripts/preflight-release.sh
./install.sh
sh scripts/install-release.sh
```

## Release Notes

- Release archives are produced by `.github/workflows/release.yml`.
- Supported release targets are `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, plus Windows amd64.
- Prefer `vX.Y.Z` tags. Pushing a tag triggers the release workflow.

## Safety Rules

- Treat real SSH config contents as sensitive. Use RFC 5737 example IPs such as `192.0.2.12` in docs and tests.
- Do not commit `.superpowers/`, local state files, generated backups, `raycast/node_modules/`, `.env*`, private keys, or personal SSH config data.
- When editing parser/writer behavior, add focused tests under `internal/sshconfig`.
- When editing TUI behavior, add focused tests under `internal/app`.
