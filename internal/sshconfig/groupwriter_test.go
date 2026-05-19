package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSingleFile = `# ssht: group=prod tags=api,critical
Host prod-api-01
    HostName 10.0.1.12
    User deploy

# ssht: group=prod
Host prod-db-01
    HostName 10.0.1.20

# ssht: group=dev
Host dev-box
    HostName 192.168.1.50
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFileGW(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func parseGroupForAlias(t *testing.T, path, alias string) string {
	t.Helper()
	entries, _, err := ParseFile(path, Options{NoInclude: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Alias == alias {
			return entry.Group
		}
	}
	t.Fatalf("alias %q not found in %s", alias, path)
	return ""
}

func TestRenameGroupSingleFileRewritesAllReferences(t *testing.T) {
	path := writeTemp(t, sampleSingleFile)

	res, err := RenameGroup([]string{path}, "prod", "production")
	if err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if res.HostsChanged != 2 {
		t.Fatalf("HostsChanged = %d, want 2", res.HostsChanged)
	}
	if got := parseGroupForAlias(t, path, "prod-api-01"); got != "production" {
		t.Fatalf("prod-api-01 group = %q, want production", got)
	}
	if got := parseGroupForAlias(t, path, "dev-box"); got != "dev" {
		t.Fatalf("dev-box group = %q, want dev (untouched)", got)
	}
	dir := filepath.Dir(path)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.bak"))
	if len(matches) == 0 {
		t.Fatalf("no backup created in %s", dir)
	}
}

func TestRenameGroupMultiFile(t *testing.T) {
	a := writeTemp(t, "# ssht: group=prod\nHost a\n    HostName 1.1.1.1\n")
	b := writeTemp(t, "# ssht: group=prod tags=db\nHost b\n    HostName 2.2.2.2\n")

	res, err := RenameGroup([]string{a, b}, "prod", "production")
	if err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if res.HostsChanged != 2 {
		t.Fatalf("HostsChanged = %d, want 2", res.HostsChanged)
	}
	if len(res.FilesChanged) != 2 {
		t.Fatalf("FilesChanged = %v, want 2 files", res.FilesChanged)
	}
	if got := parseGroupForAlias(t, a, "a"); got != "production" {
		t.Fatalf("a group = %q, want production", got)
	}
	if got := parseGroupForAlias(t, b, "b"); got != "production" {
		t.Fatalf("b group = %q, want production", got)
	}
	if !strings.Contains(readFileGW(t, b), "tags=db") {
		t.Fatalf("tags lost when renaming b: %s", readFileGW(t, b))
	}
}

func TestRenameGroupPreservesHiddenMetadata(t *testing.T) {
	path := writeTemp(t, "# ssht: group=prod hidden=true tags=db\nHost hidden-db\n    HostName 1.1.1.1\n")

	res, err := RenameGroup([]string{path}, "prod", "production")
	if err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if res.HostsChanged != 1 {
		t.Fatalf("HostsChanged = %d, want 1", res.HostsChanged)
	}
	content := readFileGW(t, path)
	if !strings.Contains(content, "group=production") || !strings.Contains(content, "hidden=true") || !strings.Contains(content, "tags=db") {
		t.Fatalf("metadata not preserved:\n%s", content)
	}
}

func TestMergeGroupCollisionsResolved(t *testing.T) {
	path := writeTemp(t, `# ssht: group=staging
Host stg-a
    HostName 1.1.1.1

# ssht: group=dev
Host dev-a
    HostName 2.2.2.2

# ssht: group=staging tags=api
Host stg-b
    HostName 3.3.3.3
`)

	res, err := MergeGroup([]string{path}, "staging", "dev")
	if err != nil {
		t.Fatalf("MergeGroup: %v", err)
	}
	if res.HostsChanged != 2 {
		t.Fatalf("HostsChanged = %d, want 2", res.HostsChanged)
	}
	for _, alias := range []string{"stg-a", "stg-b"} {
		if got := parseGroupForAlias(t, path, alias); got != "dev" {
			t.Fatalf("%s group = %q, want dev", alias, got)
		}
	}
	if got := parseGroupForAlias(t, path, "dev-a"); got != "dev" {
		t.Fatalf("dev-a group = %q, want dev (untouched)", got)
	}
	if !strings.Contains(readFileGW(t, path), "tags=api") {
		t.Fatalf("tags=api lost during merge: %s", readFileGW(t, path))
	}
}

func TestDeleteGroupStripsCommentLineWhenOnlyGroupPresent(t *testing.T) {
	path := writeTemp(t, `# ssht: group=lab
Host lab-a
    HostName 1.1.1.1

# ssht: group=lab tags=experimental
Host lab-b
    HostName 2.2.2.2
`)

	res, err := DeleteGroup([]string{path}, "lab")
	if err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if res.HostsChanged != 2 {
		t.Fatalf("HostsChanged = %d, want 2", res.HostsChanged)
	}
	if got := parseGroupForAlias(t, path, "lab-a"); got != "" {
		t.Fatalf("lab-a group = %q, want empty (ungrouped)", got)
	}
	if got := parseGroupForAlias(t, path, "lab-b"); got != "" {
		t.Fatalf("lab-b group = %q, want empty (ungrouped)", got)
	}
	content := readFileGW(t, path)
	if strings.Contains(content, "group=lab") {
		t.Fatalf("group=lab still present:\n%s", content)
	}
	if !strings.Contains(content, "tags=experimental") {
		t.Fatalf("tags=experimental should be preserved:\n%s", content)
	}
	if strings.Contains(content, "# ssht: \n") || strings.Contains(content, "# ssht: $") {
		t.Fatalf("empty ssht comment leaked:\n%s", content)
	}
}

func TestRenameGroupRejectsReservedNames(t *testing.T) {
	path := writeTemp(t, "# ssht: group=prod\nHost a\n")
	for _, bad := range []string{"all", "ungrouped", "", "  ", "with space", "with,comma", "with#hash", "with=eq"} {
		if _, err := RenameGroup([]string{path}, "prod", bad); err == nil {
			t.Fatalf("RenameGroup accepted invalid newName %q", bad)
		}
	}
}

func TestDeleteGroupRejectsReservedNames(t *testing.T) {
	path := writeTemp(t, "# ssht: group=prod\nHost a\n")
	for _, bad := range []string{"all", "ungrouped", ""} {
		if _, err := DeleteGroup([]string{path}, bad); err == nil {
			t.Fatalf("DeleteGroup accepted reserved name %q", bad)
		}
	}
}

func TestRenameGroupIsNoopWhenSameName(t *testing.T) {
	path := writeTemp(t, "# ssht: group=prod\nHost a\n    HostName 1.1.1.1\n")
	original := readFileGW(t, path)
	res, err := RenameGroup([]string{path}, "prod", "prod")
	if err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if res.HostsChanged != 0 || len(res.FilesChanged) != 0 {
		t.Fatalf("noop result = %#v", res)
	}
	if readFileGW(t, path) != original {
		t.Fatalf("file changed after noop rename")
	}
}

func TestMoveHostsToGroupAddsNewMetadataComment(t *testing.T) {
	path := writeTemp(t, "Host orphan\n    HostName 1.1.1.1\n")
	entries, _, err := ParseFile(path, Options{NoInclude: true})
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Group != "" {
		t.Fatalf("orphan group = %q, want empty", entries[0].Group)
	}

	res, err := MoveHostsToGroup(entries[:1], "prod")
	if err != nil {
		t.Fatalf("MoveHostsToGroup: %v", err)
	}
	if res.HostsChanged != 1 {
		t.Fatalf("HostsChanged = %d, want 1", res.HostsChanged)
	}
	if got := parseGroupForAlias(t, path, "orphan"); got != "prod" {
		t.Fatalf("orphan group = %q, want prod", got)
	}
}

func TestMoveHostsToGroupReplacesExistingGroupKeepingTags(t *testing.T) {
	path := writeTemp(t, "# ssht: group=staging tags=api\nHost api-1\n    HostName 1.1.1.1\n")
	entries, _, err := ParseFile(path, Options{NoInclude: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := MoveHostsToGroup(entries[:1], "prod"); err != nil {
		t.Fatalf("MoveHostsToGroup: %v", err)
	}
	if got := parseGroupForAlias(t, path, "api-1"); got != "prod" {
		t.Fatalf("api-1 group = %q, want prod", got)
	}
	if !strings.Contains(readFileGW(t, path), "tags=api") {
		t.Fatalf("tags=api lost: %s", readFileGW(t, path))
	}
}

func TestMoveHostsToGroupEmptyTargetMovesToUngrouped(t *testing.T) {
	path := writeTemp(t, "# ssht: group=staging\nHost api-1\n    HostName 1.1.1.1\n")
	entries, _, err := ParseFile(path, Options{NoInclude: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MoveHostsToGroup(entries[:1], "ungrouped"); err != nil {
		t.Fatalf("MoveHostsToGroup: %v", err)
	}
	if got := parseGroupForAlias(t, path, "api-1"); got != "" {
		t.Fatalf("api-1 group = %q, want empty (ungrouped)", got)
	}
	if strings.Contains(readFileGW(t, path), "group=") {
		t.Fatalf("group= leaked: %s", readFileGW(t, path))
	}
}

func TestValidateGroupNameAccepted(t *testing.T) {
	for _, name := range []string{"prod", "prod-east", "prod_1", "team.api"} {
		if err := ValidateGroupName(name); err != nil {
			t.Fatalf("ValidateGroupName(%q) returned %v", name, err)
		}
	}
}
