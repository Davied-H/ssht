---
name: ssht-config-auditor
description: Use when auditing or editing SSH config examples for ssht, adding ssht group/tag metadata, checking for sensitive SSH data before publishing, or explaining how ssht parses Host entries.
---

# ssht Config Auditor

Use this skill when the task involves SSH config fixtures, README examples, privacy review, or `# ssht:` metadata.

## Workflow

1. Inspect only repository files unless the user explicitly provides a real SSH config path.
2. Run the repository preflight before publishing:
   ```bash
   sh scripts/preflight-release.sh
   ```
3. Replace real-looking examples with documentation-safe values:
   - IPs: `192.0.2.0/24`, `198.51.100.0/24`, or `203.0.113.0/24`
   - Domains: `example.com`
   - Users: `deploy`, `demo`, `git`
   - Aliases: `demo-api`, `staging-db`, `lab-box`
4. Keep OpenSSH semantics intact. `ssht` discovers concrete `Host` aliases and connects with `ssh <alias>`.
5. For metadata examples, use comments immediately before the `Host` line:
   ```sshconfig
   # ssht: group=prod tags=api,critical
   Host demo-api
       HostName 192.0.2.12
       User deploy
   ```

## Validation

Run:

```bash
sh scripts/preflight-release.sh
```

For parser/writer changes, make sure affected tests under `internal/sshconfig` cover the new behavior.
