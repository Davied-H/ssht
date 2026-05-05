package monitor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ProbeTarget describes the host to probe. Password-auth hosts populate Password
// so the runner can wrap the ssh invocation with sshpass.
type ProbeTarget struct {
	Alias    string
	Password string
}

// ProbeFunc is the contract for fetching a snapshot. Injectable so tests can swap it.
type ProbeFunc func(ctx context.Context, target ProbeTarget) (*Snapshot, error)

// Probe runs the embedded probe script against the target via SSH. The remote shell is
// invoked as `sh -s` and the script is fed over stdin so quoting stays simple.
//
// Key-auth hosts use BatchMode=yes to suppress prompts and ConnectTimeout=4 to cap
// DNS/TCP wait. Password-auth hosts (Password != "") are wrapped with sshpass -p;
// BatchMode is dropped because it would block sshpass from injecting the password.
// Auth method selection is left to ssh's default negotiation so the probe mirrors
// the actual terminal connection in internal/terminal/terminal.go (plain
// `sshpass -p PWD ssh ALIAS`); whichever method connects first wins, and probe
// success matches "Enter would connect". NumberOfPasswordPrompts=1 still bounds
// the failure case so a wrong password fails fast instead of hanging the probe.
func Probe(ctx context.Context, target ProbeTarget) (*Snapshot, error) {
	cmd, err := buildProbeCmd(ctx, target)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = strings.NewReader(probeScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	fetchedAt := time.Now()
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.New("ssh probe timeout")
		}
		return nil, errors.New(formatProbeError(target, runErr, stderr.String()))
	}
	return Parse(target.Alias, fetchedAt, &stdout), nil
}

func buildProbeCmd(ctx context.Context, target ProbeTarget) (*exec.Cmd, error) {
	if target.Password == "" {
		return exec.CommandContext(ctx, "ssh",
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=4",
			"-o", "StrictHostKeyChecking=accept-new",
			"--",
			target.Alias, "sh -s",
		), nil
	}
	if _, err := exec.LookPath("sshpass"); err != nil {
		return nil, errors.New("sshpass not installed")
	}
	return exec.CommandContext(ctx, "sshpass",
		"-p", target.Password,
		"ssh",
		"-o", "ConnectTimeout=4",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "NumberOfPasswordPrompts=1",
		"--",
		target.Alias, "sh -s",
	), nil
}

// formatProbeError turns sshpass/ssh failures into a single short line suitable for
// the preview pane. sshpass exposes specific exit codes that are far more useful than
// the generic "exit status N" message Go would otherwise show.
func formatProbeError(target ProbeTarget, runErr error, stderr string) string {
	if target.Password != "" {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			if msg := sshpassExitMessage(exitErr.ExitCode()); msg != "" {
				return msg
			}
		}
	}
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = runErr.Error()
	}
	msg = firstLine(msg)
	if len(msg) > 140 {
		msg = msg[:140] + "…"
	}
	return msg
}

// sshpassExitMessage maps sshpass exit codes to human-readable explanations.
// See sshpass(1): 1 invalid args, 2 conflicting args, 3 read-pwd error, 4 unknown
// auth method (often = no password prompt seen), 5 invalid/incorrect password,
// 6 host public key unknown, 12 timeout / network unreachable.
func sshpassExitMessage(code int) string {
	switch code {
	case 1, 2:
		return "sshpass: invalid arguments"
	case 3:
		return "sshpass: cannot read password"
	case 4:
		return "no password prompt from server"
	case 5:
		return "authentication failed (wrong password)"
	case 6:
		return "host key unknown; connect once manually to accept"
	case 12:
		return "connection timed out"
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
