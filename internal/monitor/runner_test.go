package monitor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildProbeCmdKeyAuth(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	cmd, err := buildProbeCmd(context.Background(), ProbeTarget{Alias: "host1"})
	if err != nil {
		t.Fatalf("buildProbeCmd error: %v", err)
	}
	args := cmd.Args
	if filepath.Base(args[0]) != "ssh" {
		t.Fatalf("argv[0] = %q, want ssh", args[0])
	}
	if !containsSequence(args, []string{"-o", "BatchMode=yes"}) {
		t.Fatalf("missing BatchMode=yes in %v", args)
	}
	if !containsSequence(args, []string{"--", "host1", "sh -s"}) {
		t.Fatalf("alias not at expected position: %v", args)
	}
	for _, a := range args {
		if strings.Contains(a, "PreferredAuthentications") {
			t.Fatalf("key-auth probe should not set PreferredAuthentications: %v", args)
		}
	}
	if containsEnv(cmd.Env, "LC_ALL") {
		t.Fatalf("key-auth probe should not inherit LC_ALL: %v", cmd.Env)
	}
}

func TestBuildProbeCmdPasswordAuth(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	dir := t.TempDir()
	stub := filepath.Join(dir, "sshpass")
	if runtime.GOOS == "windows" {
		t.Skip("sshpass path test is unix-specific")
	}
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub sshpass: %v", err)
	}
	t.Setenv("PATH", dir)

	cmd, err := buildProbeCmd(context.Background(), ProbeTarget{Alias: "host2", Password: "s3cret"})
	if err != nil {
		t.Fatalf("buildProbeCmd error: %v", err)
	}
	args := cmd.Args
	if filepath.Base(args[0]) != "sshpass" {
		t.Fatalf("argv[0] = %q, want sshpass", args[0])
	}
	if !containsSequence(args, []string{"-p", "s3cret", "ssh"}) {
		t.Fatalf("sshpass argument layout unexpected: %v", args)
	}
	required := []string{
		"NumberOfPasswordPrompts=1",
		"ConnectTimeout=4",
		"StrictHostKeyChecking=accept-new",
	}
	for _, want := range required {
		if !contains(args, want) {
			t.Fatalf("argv missing %q: %v", want, args)
		}
	}
	forbidden := []string{
		"PubkeyAuthentication=no",
		"PreferredAuthentications=password,keyboard-interactive",
		"BatchMode=yes",
	}
	for _, bad := range forbidden {
		if contains(args, bad) {
			t.Fatalf("password probe must not set %q (probe should mirror manual sshpass posture): %v", bad, args)
		}
	}
	if !containsSequence(args, []string{"--", "host2", "sh -s"}) {
		t.Fatalf("alias not at expected position: %v", args)
	}
	if containsEnv(cmd.Env, "LC_ALL") {
		t.Fatalf("password probe should not inherit LC_ALL: %v", cmd.Env)
	}
}

func TestBuildProbeCmdMissingSshpass(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := buildProbeCmd(context.Background(), ProbeTarget{Alias: "host3", Password: "x"})
	if err == nil {
		t.Fatal("expected error when sshpass is absent")
	}
	if got := err.Error(); !strings.Contains(got, "sshpass not installed") {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestSshpassExitMessage(t *testing.T) {
	cases := map[int]string{
		1:  "sshpass: invalid arguments",
		5:  "authentication failed (wrong password)",
		6:  "host key unknown; connect once manually to accept",
		12: "connection timed out",
	}
	for code, want := range cases {
		if got := sshpassExitMessage(code); got != want {
			t.Errorf("code %d => %q, want %q", code, got, want)
		}
	}
	if got := sshpassExitMessage(99); got != "" {
		t.Errorf("unknown code should return empty, got %q", got)
	}
}

func contains(slice []string, want string) bool {
	for _, v := range slice {
		if v == want {
			return true
		}
	}
	return false
}

func containsSequence(slice, want []string) bool {
	if len(want) > len(slice) {
		return false
	}
	for i := 0; i+len(want) <= len(slice); i++ {
		match := true
		for j, w := range want {
			if slice[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func containsEnv(environ []string, name string) bool {
	prefix := name + "="
	for _, entry := range environ {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
