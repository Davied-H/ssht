# Host Management Dashboard Design

## Goal

Add a complete SSH Host manager to `ssht`: a top dashboard with key counts, plus create, edit, and delete flows for OpenSSH `Host` blocks.

## Current Context

`ssht` is a Go Bubble Tea TUI. It currently reads SSH config entries, filters and previews hosts, and connects through tmux. It does not write SSH config. Parsed entries already include source file, source line, raw block, and common fields.

The project is not currently inside a Git repository, so this design is saved but not committed.

## User-Approved Direction

- Build the complete manager option: create, edit, and delete Host entries.
- Add a compact top dashboard with four metrics: total hosts, matched hosts, active tmux sessions, and warnings.
- For hosts from included files, write back to the entry's `SourceFile`, but make the target path explicit in the confirmation flow.
- Do not ask for additional confirmation before implementation.

## Interface Design

The default screen keeps the existing search, list, preview, status, and help behavior. A dashboard line appears near the top:

```text
Hosts 24 | Matched 8 | Active 3 | Warnings 1
```

New shortcuts:

- `e`: edit the selected Host.
- `A`: add a new Host.
- `d`: delete the selected Host.
- `s`: save from a form or confirm from a confirmation screen.
- `Esc`: cancel the current form or confirmation screen.

The form edits these fields:

- Alias
- HostName
- User
- Port
- IdentityFile
- ProxyJump
- ProxyCommand
- Tags

The confirmation screen shows the operation, write target file, affected alias or aliases, and a compact summary of field changes.

## Data And State

The app model gains explicit mode state:

- Browse mode: current list/search/preview behavior.
- Form mode: add or edit a host.
- Confirm mode: save or delete confirmation.

The parsed `HostEntry` remains the display source of truth. Write operations reload config after success, so the list, preview, dashboard, and warnings use freshly parsed data.

Active sessions are counted by checking whether each host's stable tmux session exists. To avoid per-render command calls, the model stores an `activeAliases` map and refreshes it after initial load, reload, connect, add, edit, and delete.

## SSH Config Write Strategy

Create a new focused package file under `internal/sshconfig` for write operations. It will expose a small API for add, edit, and delete.

Write constraints:

- Preserve unrelated blocks and comments.
- Create a timestamped backup in the same directory before writing.
- Only edit blocks that start with a concrete `Host` directive.
- For a single-alias block, editing can update Alias and fields.
- For a multi-alias block, editing the Alias is rejected unless the alias is unchanged; fields are shared by the block.
- For deleting from a single-alias block, remove the whole block.
- For deleting from a multi-alias block, remove only the selected alias from the `Host` line.
- Return clear errors for malformed source data, missing source file, and file write failures.

The writer will operate on source file text by locating the Host block at `SourceLine`, then replacing the block with canonical field output. This is intentionally conservative and easier to test than a full OpenSSH AST formatter.

## Error Handling

Validation happens before confirmation:

- Alias is required for add/edit.
- Port, when present, must be numeric.
- Edit/delete requires `SourceFile` and `SourceLine`.
- Multi-alias alias changes are rejected with a message explaining that fields can still be edited.

Write failures return to the current form or browse screen with status text.

## Testing

Add focused tests before implementation:

- Config writer adds a Host block and parser can read it.
- Config writer edits a single-alias block and preserves unrelated content.
- Multi-alias edit rejects alias changes but allows shared field changes.
- Delete removes a single-alias block.
- Delete removes one alias from a multi-alias block.
- Model enters add/edit/delete modes and applies validation.
- Dashboard counts total, filtered, warnings, and active aliases.

Run `go test ./...` as final verification.
