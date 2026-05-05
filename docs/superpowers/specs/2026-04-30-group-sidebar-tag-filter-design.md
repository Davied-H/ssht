# Group Sidebar And Tag Filter Design

## Goal

Add a left group sidebar to `ssht` so users can browse hosts by a primary group, while keeping tags as a separate filtering feature.

## User Decisions

- Use the left sidebar layout for grouped browsing.
- Do not use OpenSSH `Tag` as the primary group.
- Store `ssht` metadata in comments near each `Host` block:

```sshconfig
# ssht: group=prod tags=api,critical
Host prod-api-01
    HostName 10.0.1.12
    User deploy
```

## Behavior

- `Group` is a single primary grouping value used by the left sidebar.
- `Tags` are separate labels used by filtering and preview.
- Hosts without metadata appear under `ungrouped`.
- The sidebar includes `all`, one row per group, and `ungrouped` when needed.
- The selected group and the search filter are combined.
- Search supports tag tokens:
  - `tag:api` matches hosts containing tag `api`.
  - Multiple tag tokens are ANDed, such as `tag:api tag:critical`.
  - Non-tag search terms still match alias, host name, user, port, proxy, source file, group, and tags.

## SSH Config Write Strategy

The writer renders metadata comments immediately before the `Host` line when either group or tags are present. Editing a host rewrites the metadata comment and canonical host block together. Deleting a host removes the metadata comment only when it directly belongs to that block.

This keeps OpenSSH compatibility because comments are ignored by `ssh`.

## TUI Design

The browse view becomes a two-column layout:

- Left: group list with counts and active selection.
- Right: current dashboard, search, host list, preview, and status.

Keyboard additions:

- `[` or left arrow: move to previous group.
- `]` or right arrow: move to next group.
- `/`: clear text filter without changing the selected group.

Existing keys continue to work.

## Testing

- Parser reads `# ssht: group=... tags=...` metadata into `HostEntry`.
- Writer adds and edits metadata comments.
- Delete removes attached metadata comments.
- Model filters by selected group.
- Model supports `tag:` query tokens.
- View renders group sidebar and selected group counts.
