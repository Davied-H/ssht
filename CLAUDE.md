# ssht Claude Code Instructions

Use these notes when working on this repository.

## What This Project Does

`ssht` is a local Go TUI for browsing OpenSSH `Host` aliases, grouping them with `# ssht:` metadata comments, and opening selected aliases in a supported terminal. It does not implement SSH and should keep relying on OpenSSH for final connection semantics.

## Build And Test

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o /tmp/ssht ./cmd/ssht
sh scripts/preflight-release.sh
```

For release-install testing:

```bash
sh scripts/install-release.sh
```

## Repository Conventions

- Keep examples fictional. Prefer `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, `example.com`, and aliases like `demo-api`.
- Do not commit real SSH config fragments, private paths, passwords, private keys, `.env*`, `.superpowers/`, or `raycast/node_modules/`.
- Parser and writer changes belong with tests in `internal/sshconfig`.
- TUI state and rendering changes belong with tests in `internal/app`.
- Raycast integration changes belong under `raycast/` and should preserve `package-lock.json`.
