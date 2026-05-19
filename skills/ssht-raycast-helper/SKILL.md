---
name: ssht-raycast-helper
description: Use when working on the ssht Raycast extension, its preferences, host search behavior, connect action, package metadata, or extension build checks.
---

# ssht Raycast Helper

Use this skill for changes under `raycast/`.

## Key Behavior

- The extension calls `ssht --print-hosts` to get JSON host entries.
- The extension calls `ssht --connect <alias>` to open a selected host.
- Preferences should keep supporting:
  - `ssht Path`
  - `SSH Config Path`
  - `Disable Include Parsing`
  - `Terminal`
  - `Open Mode`

## Workflow

1. Read `raycast/package.json` and `raycast/src/search-hosts.tsx`.
2. Preserve shell argument safety when adding command execution.
3. Avoid exposing sshpass values. `ssht --print-hosts` intentionally omits `SSHPassword`.
4. If package metadata or dependencies change, update `raycast/package-lock.json`.

## Validation

```bash
cd raycast
npm install
npm run build
```

Also run the Go suite from the repo root when CLI behavior changes:

```bash
go test ./...
```
