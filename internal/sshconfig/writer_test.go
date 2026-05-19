package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddHostWritesBlockAndBackup(t *testing.T) {
	path := writeTempConfig(t, `
Host existing
    HostName 10.0.0.1
`)

	err := AddHost(path, HostForm{
		Alias:        "new-host",
		Group:        "prod",
		HostName:     "new.example.com",
		User:         "deploy",
		Port:         "2222",
		IdentityFile: "~/.ssh/new",
		ProxyJump:    "bastion",
		Tags:         []string{"prod", "api"},
	})
	if err != nil {
		t.Fatalf("AddHost returned error: %v", err)
	}

	content := readFile(t, path)
	for _, want := range []string{
		"Host existing",
		"# ssht: group=prod tags=prod,api",
		"Host new-host",
		"    HostName new.example.com",
		"    User deploy",
		"    Port 2222",
		"    IdentityFile ~/.ssh/new",
		"    ProxyJump bastion",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	assertBackupExists(t, path)
}

func TestAddHostWritesHiddenMetadata(t *testing.T) {
	path := writeTempConfig(t, "")

	err := AddHost(path, HostForm{
		Alias:    "git-only",
		Hidden:   true,
		HostName: "github.example.com",
		User:     "git",
	})
	if err != nil {
		t.Fatalf("AddHost returned error: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "# ssht: hidden=true") {
		t.Fatalf("hidden metadata missing:\n%s", content)
	}
}

func TestEditHostUpdatesSingleAliasBlockAndPreservesOtherContent(t *testing.T) {
	path := writeTempConfig(t, `
# before
Host prod
    HostName old.example.com
    User root

Host other
    HostName other.example.com
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{
		Alias:    "prod-api",
		Group:    "prod",
		HostName: "new.example.com",
		User:     "deploy",
		Port:     "22",
		Tags:     []string{"api"},
	})
	if err != nil {
		t.Fatalf("EditHost returned error: %v", err)
	}

	content := readFile(t, path)
	if strings.Contains(content, "Host prod\n") {
		t.Fatalf("old alias remains:\n%s", content)
	}
	for _, want := range []string{
		"# before",
		"# ssht: group=prod tags=api",
		"Host prod-api",
		"    HostName new.example.com",
		"    User deploy",
		"    Port 22",
		"Host other",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestEditHostUpdatesExistingSshtMetadata(t *testing.T) {
	path := writeTempConfig(t, `
# ssht: group=old tags=legacy
Host prod
    HostName old.example.com
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{
		Alias:    "prod",
		Group:    "prod",
		HostName: "new.example.com",
		Tags:     []string{"api", "critical"},
	})
	if err != nil {
		t.Fatalf("EditHost returned error: %v", err)
	}

	content := readFile(t, path)
	if strings.Contains(content, "group=old") || strings.Contains(content, "legacy") {
		t.Fatalf("old metadata remains:\n%s", content)
	}
	if !strings.Contains(content, "# ssht: group=prod tags=api,critical") {
		t.Fatalf("new metadata missing:\n%s", content)
	}
}

func TestEditHostPreservesSSHPasswordMetadata(t *testing.T) {
	path := writeTempConfig(t, `
# sshpass 密码: secret value
Host prod
    HostName old.example.com
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{
		Alias:       "prod",
		HostName:    "new.example.com",
		SSHPassword: entry.SSHPassword,
	})
	if err != nil {
		t.Fatalf("EditHost returned error: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "# sshpass 密码: secret value") {
		t.Fatalf("sshpass metadata missing:\n%s", content)
	}
	if !strings.Contains(content, "Host prod") || !strings.Contains(content, "HostName new.example.com") {
		t.Fatalf("edited host missing:\n%s", content)
	}
}

func TestEditHostPreservesUnmodeledDirectivesAndComments(t *testing.T) {
	path := writeTempConfig(t, `
Host prod
    HostName old.example.com
    User root
    LocalForward 8080 localhost:80
    # keep this note
    ForwardAgent yes
    ServerAliveInterval 30
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{
		Alias:    "prod-api",
		HostName: "new.example.com",
		User:     "deploy",
		Port:     "2222",
	})
	if err != nil {
		t.Fatalf("EditHost returned error: %v", err)
	}

	content := readFile(t, path)
	for _, want := range []string{
		"Host prod-api",
		"    HostName new.example.com",
		"    User deploy",
		"    Port 2222",
		"    LocalForward 8080 localhost:80",
		"    # keep this note",
		"    ForwardAgent yes",
		"    ServerAliveInterval 30",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestEditHostRejectsAliasChangeInMultiAliasBlock(t *testing.T) {
	path := writeTempConfig(t, `
Host prod prod.internal
    HostName old.example.com
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{Alias: "prod-api", HostName: "new.example.com"})
	if err == nil {
		t.Fatal("expected alias change to be rejected")
	}
	if !strings.Contains(err.Error(), "multi-alias") {
		t.Fatalf("error = %q, want multi-alias context", err.Error())
	}
}

func TestEditHostAllowsSharedFieldsInMultiAliasBlock(t *testing.T) {
	path := writeTempConfig(t, `
Host prod prod.internal
    HostName old.example.com
    User root
`)
	entry := entryForAlias(t, path, "prod")

	err := EditHost(entry, HostForm{Alias: "prod", HostName: "new.example.com", User: "deploy"})
	if err != nil {
		t.Fatalf("EditHost returned error: %v", err)
	}

	content := readFile(t, path)
	for _, want := range []string{
		"Host prod prod.internal",
		"    HostName new.example.com",
		"    User deploy",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestDeleteHostRemovesSingleAliasBlock(t *testing.T) {
	path := writeTempConfig(t, `
# ssht: group=prod tags=api
Host prod
    HostName prod.example.com

Host other
    HostName other.example.com
`)
	entry := entryForAlias(t, path, "prod")

	if err := DeleteHost(entry); err != nil {
		t.Fatalf("DeleteHost returned error: %v", err)
	}

	content := readFile(t, path)
	if strings.Contains(content, "Host prod") || strings.Contains(content, "prod.example.com") {
		t.Fatalf("deleted block remains:\n%s", content)
	}
	if strings.Contains(content, "ssht: group=prod") {
		t.Fatalf("metadata for deleted block remains:\n%s", content)
	}
	if !strings.Contains(content, "Host other") {
		t.Fatalf("unrelated block missing:\n%s", content)
	}
}

func TestDeleteHostRemovesAliasFromMultiAliasBlock(t *testing.T) {
	path := writeTempConfig(t, `
Host prod prod.internal
    HostName prod.example.com
    LocalForward 8080 localhost:80
    ForwardAgent yes
`)
	entry := entryForAlias(t, path, "prod")

	if err := DeleteHost(entry); err != nil {
		t.Fatalf("DeleteHost returned error: %v", err)
	}

	content := readFile(t, path)
	if strings.Contains(content, "Host prod prod.internal") {
		t.Fatalf("old host line remains:\n%s", content)
	}
	if !strings.Contains(content, "Host prod.internal") || !strings.Contains(content, "HostName prod.example.com") {
		t.Fatalf("remaining alias block malformed:\n%s", content)
	}
	for _, want := range []string{
		"    LocalForward 8080 localhost:80",
		"    ForwardAgent yes",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q after deleting alias:\n%s", want, content)
		}
	}
}

func TestDeleteHostStopsAtMatchBlock(t *testing.T) {
	path := writeTempConfig(t, `
Host prod
    HostName prod.example.com

Match user git
    IdentityFile ~/.ssh/git

Host other
    HostName other.example.com
`)
	entry := entryForAlias(t, path, "prod")

	if err := DeleteHost(entry); err != nil {
		t.Fatalf("DeleteHost returned error: %v", err)
	}

	content := readFile(t, path)
	if strings.Contains(content, "Host prod") || strings.Contains(content, "prod.example.com") {
		t.Fatalf("deleted Host block remains:\n%s", content)
	}
	for _, want := range []string{
		"Match user git",
		"    IdentityFile ~/.ssh/git",
		"Host other",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func entryForAlias(t *testing.T, path, alias string) HostEntry {
	t.Helper()
	entries, warnings, err := ParseFile(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for _, entry := range entries {
		if entry.Alias == alias {
			return entry
		}
	}
	t.Fatalf("alias %q not found in %#v", alias, entries)
	return HostEntry{}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertBackupExists(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(path + ".ssht.*.bak")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected backup matching %s.ssht.*.bak", path)
	}
}
