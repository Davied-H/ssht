---
name: ssht-release-packager
description: Use when preparing ssht releases, changing install scripts, creating GitHub tags, checking release artifacts, or packaging macOS and Linux builds.
---

# ssht Release Packager

Use this skill for release and installation work.

## Release Targets

The release workflow builds:

- `darwin/amd64` for Intel macOS
- `darwin/arm64` for Apple Silicon macOS
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`

## Local Validation

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o /tmp/ssht ./cmd/ssht
sh scripts/install-release.sh --help
```

## Release Flow

1. Confirm `git status --short --branch` is understood.
2. Commit intended changes.
3. Push `main` to the GitHub remote.
4. Create an annotated `vX.Y.Z` tag.
5. Push the tag.
6. Watch the Release workflow:
   ```bash
   gh run list --repo Davied-H/ssht --workflow Release --limit 5
   gh run watch <run-id> --repo Davied-H/ssht --exit-status
   ```
7. Verify release assets:
   ```bash
   gh release view vX.Y.Z --repo Davied-H/ssht --json url,assets
   ```

## Notes

- `scripts/install-release.sh` installs from GitHub Releases and should not require Go.
- `install.sh` builds from source and does require Go.
- Keep README install commands aligned with the scripts.
