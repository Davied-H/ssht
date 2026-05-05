# Host Management Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top dashboard and complete create/edit/delete Host management to the SSH tmux TUI.

**Architecture:** Add a focused SSH config writer beside the parser, then wire it into Bubble Tea modes for browse, form, and confirmation. Keep tmux session status cached in the app model and refresh it through commands instead of shelling out during render.

**Tech Stack:** Go 1.22, Bubble Tea, Lip Gloss, standard `testing` package.

---

## File Structure

- Create `internal/sshconfig/writer.go`: canonical host form type, validation, add/edit/delete/write-back operations, backups.
- Create `internal/sshconfig/writer_test.go`: fixture-based tests for add, edit, delete, multi-alias behavior, and backups.
- Modify `internal/app/model.go`: app modes, form state, validation, write commands, active-session refresh messages.
- Modify `internal/app/view.go`: dashboard, form view, confirmation view, updated help.
- Modify `internal/app/model_test.go`: state-machine tests for dashboard and management modes.
- Modify `README.md`: document dashboard and new shortcuts.

## Task 1: SSH Config Writer

- [ ] Write failing writer tests in `internal/sshconfig/writer_test.go` for adding, editing, deleting, multi-alias rejection, and backup creation.
- [ ] Run `go test ./internal/sshconfig` and verify the tests fail because writer symbols do not exist.
- [ ] Implement `HostForm`, `WriteOperation`, `AddHost`, `EditHost`, `DeleteHost`, block location, validation, and backup creation in `internal/sshconfig/writer.go`.
- [ ] Run `go test ./internal/sshconfig` and verify it passes.

## Task 2: App State And Dashboard

- [ ] Add failing model tests for dashboard counts and active alias state in `internal/app/model_test.go`.
- [ ] Run `go test ./internal/app` and verify failure.
- [ ] Add model fields for active aliases, dashboard counts, active refresh command/message, and key handling that does not call tmux while rendering.
- [ ] Update `internal/app/view.go` to render `Hosts`, `Matched`, `Active`, and `Warnings` above search.
- [ ] Run `go test ./internal/app`.

## Task 3: Add/Edit/Delete Modes

- [ ] Add failing model tests for `A`, `e`, `d`, form validation, confirmation, and cancellation.
- [ ] Run `go test ./internal/app` and verify failure.
- [ ] Implement browse/form/confirm modes and write commands in `internal/app/model.go`.
- [ ] Implement form and confirmation rendering in `internal/app/view.go`.
- [ ] Run `go test ./internal/app`.

## Task 4: Documentation And Final Verification

- [ ] Update `README.md` with dashboard metrics and management shortcuts.
- [ ] Run `gofmt` on modified Go files.
- [ ] Run `go test ./...` and ensure all packages pass.
- [ ] Because the project is not a Git repository, skip commit steps and report the changed files.

## Self-Review

Spec coverage is complete: the plan includes dashboard metrics, add/edit/delete flows, source-file writeback, backups, validation, tmux active caching, tests, and documentation.

No placeholder tasks remain. Function and type names are consistent across tasks.
