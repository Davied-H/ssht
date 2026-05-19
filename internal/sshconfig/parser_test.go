package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigDiscoversConcreteHostsAndIncludes(t *testing.T) {
	dir := t.TempDir()
	includePath := filepath.Join(dir, "included.conf")
	mainPath := filepath.Join(dir, "config")

	if err := os.WriteFile(includePath, []byte(`
Host staging-api !blocked *.wild
    HostName staging.example.com
    User deploy
    Port 2222
    ProxyJump bastion
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(mainPath, []byte(`
Include included.conf

Host prod-api prod-api.internal
    HostName 10.0.1.12
    User deploy
    IdentityFile ~/.ssh/prod

Host *
    User ignored

Host *.internal
    User ignored
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(mainPath, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	wantAliases := []string{"staging-api", "prod-api", "prod-api.internal"}
	if len(entries) != len(wantAliases) {
		t.Fatalf("got %d entries, want %d: %#v", len(entries), len(wantAliases), entries)
	}
	for i, alias := range wantAliases {
		if entries[i].Alias != alias {
			t.Fatalf("entry %d alias = %q, want %q", i, entries[i].Alias, alias)
		}
	}

	staging := entries[0]
	if staging.HostName != "staging.example.com" || staging.User != "deploy" || staging.Port != "2222" || staging.ProxyJump != "bastion" {
		t.Fatalf("included host fields not parsed: %#v", staging)
	}
	if staging.SourceFile != includePath || staging.SourceLine == 0 {
		t.Fatalf("source tracking missing: %#v", staging)
	}
	if staging.RawBlock == "" {
		t.Fatal("RawBlock is empty")
	}

	prod := entries[1]
	if prod.HostName != "10.0.1.12" || prod.IdentityFile != "~/.ssh/prod" {
		t.Fatalf("prod fields not parsed: %#v", prod)
	}
}

func TestParseMissingConfigReturnsEmptyWarning(t *testing.T) {
	entries, warnings, err := ParseFile(filepath.Join(t.TempDir(), "missing"), Options{})
	if err != nil {
		t.Fatalf("missing file should not be fatal: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got entries for missing file: %#v", entries)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for missing file")
	}
}

func TestParseSshtMetadataCommentAddsGroupAndTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
# unrelated comment
# ssht: group=prod tags=api,critical
Host prod-api
    HostName 10.0.1.12

# ssht: group=dev
Host dev-box
    HostName 127.0.0.1

Host ungrouped
    HostName example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %#v, want 3", entries)
	}

	if entries[0].Group != "prod" {
		t.Fatalf("prod group = %q, want prod", entries[0].Group)
	}
	if got := entries[0].Tags; len(got) != 2 || got[0] != "api" || got[1] != "critical" {
		t.Fatalf("prod tags = %#v, want api critical", got)
	}
	if entries[1].Group != "dev" {
		t.Fatalf("dev group = %q, want dev", entries[1].Group)
	}
	if entries[2].Group != "" || len(entries[2].Tags) != 0 {
		t.Fatalf("ungrouped metadata = group %q tags %#v, want empty", entries[2].Group, entries[2].Tags)
	}
}

func TestParseSshtHiddenSkipsHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
# ssht: hidden=true
Host git-only
    HostName github.example.com
    User git

# ssht: hidden=false
Host visible
    HostName visible.example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 1 || entries[0].Alias != "visible" {
		t.Fatalf("entries = %#v, want only visible host", entries)
	}
}

func TestParseSshpassCommentAddsPasswordToNextHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
# sshpass 密码: p@ss word
Host password-host
    HostName 10.0.1.12

Host key-host
    HostName 10.0.1.13
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2", entries)
	}
	if entries[0].SSHPassword != "p@ss word" {
		t.Fatalf("password host SSHPassword = %q, want p@ss word", entries[0].SSHPassword)
	}
	if entries[1].SSHPassword != "" {
		t.Fatalf("key host SSHPassword = %q, want empty", entries[1].SSHPassword)
	}
}

func TestParseAdjacentSshpassAndSshtMetadataCombinesBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
# sshpass 密码: p@ss word
# ssht: group=prod tags=api,critical
Host password-host
    HostName 10.0.1.12
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one password-host", entries)
	}
	entry := entries[0]
	if entry.SSHPassword != "p@ss word" {
		t.Fatalf("SSHPassword = %q, want p@ss word", entry.SSHPassword)
	}
	if entry.Group != "prod" {
		t.Fatalf("Group = %q, want prod", entry.Group)
	}
	if got := strings.Join(entry.Tags, ","); got != "api,critical" {
		t.Fatalf("Tags = %q, want api,critical", got)
	}
}

func TestParseRawBlockDoesNotIncludeNextHostSSHPasswordComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
Host key-host
    HostName 10.0.1.13

# sshpass 密码: secret
Host password-host
    HostName 10.0.1.12
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2", entries)
	}
	if strings.Contains(entries[0].RawBlock, "secret") {
		t.Fatalf("first raw block leaked next host password:\n%s", entries[0].RawBlock)
	}
	if entries[1].SSHPassword != "secret" {
		t.Fatalf("second host SSHPassword = %q, want secret", entries[1].SSHPassword)
	}
}

func TestParseMatchBlockDoesNotBleedIntoPreviousHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(`
Host prod
    HostName prod.example.com

Match user git
    IdentityFile ~/.ssh/git
`), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want prod only", entries)
	}
	if entries[0].IdentityFile != "" {
		t.Fatalf("IdentityFile = %q, want empty because it belongs to Match", entries[0].IdentityFile)
	}
	if strings.Contains(entries[0].RawBlock, "Match user git") {
		t.Fatalf("RawBlock should stop before Match:\n%s", entries[0].RawBlock)
	}
}
