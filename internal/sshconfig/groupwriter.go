package sshconfig

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// BatchResult summarises a multi-file group operation.
type BatchResult struct {
	FilesChanged []string
	HostsChanged int
	Backups      []string
}

var reservedGroupNames = map[string]bool{
	"all":       true,
	"ungrouped": true,
}

// ValidateGroupName checks a candidate group name for the rules listed in the spec.
func ValidateGroupName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("group name is required")
	}
	if reservedGroupNames[trimmed] {
		return fmt.Errorf("%q is a reserved group name", trimmed)
	}
	if strings.ContainsAny(trimmed, " \t\r\n,#=") {
		return fmt.Errorf("group name must not contain whitespace, comma, hash, or equals")
	}
	return nil
}

// RenameGroup rewrites every `# ssht: group=oldName ...` comment to use newName.
// Caller is responsible for ensuring newName does not already collide with an
// existing group; use MergeGroup for that case.
func RenameGroup(sources []string, oldName, newName string) (BatchResult, error) {
	if err := ValidateGroupName(newName); err != nil {
		return BatchResult{}, err
	}
	if strings.TrimSpace(oldName) == "" {
		return BatchResult{}, fmt.Errorf("old group name is required")
	}
	if oldName == newName {
		return BatchResult{}, nil
	}
	return rewriteGroupReference(sources, oldName, newName)
}

// MergeGroup folds fromName into toName by rewriting comments.
func MergeGroup(sources []string, fromName, toName string) (BatchResult, error) {
	if err := ValidateGroupName(toName); err != nil {
		return BatchResult{}, err
	}
	if strings.TrimSpace(fromName) == "" {
		return BatchResult{}, fmt.Errorf("source group name is required")
	}
	if fromName == toName {
		return BatchResult{}, nil
	}
	return rewriteGroupReference(sources, fromName, toName)
}

// DeleteGroup strips `group=name` from every relevant comment. Hosts move into
// ungrouped automatically.
func DeleteGroup(sources []string, name string) (BatchResult, error) {
	if strings.TrimSpace(name) == "" {
		return BatchResult{}, fmt.Errorf("group name is required")
	}
	if reservedGroupNames[name] {
		return BatchResult{}, fmt.Errorf("%q cannot be deleted", name)
	}
	return rewriteGroupReference(sources, name, "")
}

// MoveHostsToGroup rewrites the metadata comment immediately preceding each
// given host's Host directive so that its group equals targetName. Pass an
// empty targetName (or "ungrouped") to move hosts out of any group.
func MoveHostsToGroup(entries []HostEntry, targetName string) (BatchResult, error) {
	target := strings.TrimSpace(targetName)
	if target == "ungrouped" {
		target = ""
	}
	if target != "" {
		if err := ValidateGroupName(target); err != nil {
			return BatchResult{}, err
		}
	}
	if len(entries) == 0 {
		return BatchResult{}, nil
	}

	byFile := map[string][]HostEntry{}
	files := []string{}
	for _, entry := range entries {
		if entry.SourceFile == "" || entry.SourceLine <= 0 {
			return BatchResult{}, fmt.Errorf("host %q has no source location", entry.Alias)
		}
		if _, ok := byFile[entry.SourceFile]; !ok {
			files = append(files, entry.SourceFile)
		}
		byFile[entry.SourceFile] = append(byFile[entry.SourceFile], entry)
	}
	sort.Strings(files)

	plans := make([]filePlan, 0, len(files))
	for _, path := range files {
		original, err := os.ReadFile(path)
		if err != nil {
			return BatchResult{}, err
		}
		newContent, hostsChanged, err := planMoveHosts(string(original), byFile[path], target)
		if err != nil {
			return BatchResult{}, fmt.Errorf("%s: %w", path, err)
		}
		plans = append(plans, filePlan{
			path:         path,
			original:     original,
			newContent:   newContent,
			hostsChanged: hostsChanged,
		})
	}

	return commitPlans(plans)
}

// rewriteGroupReference scans every source file, parses `# ssht:` metadata
// comments, and rewrites any whose `group` field equals fromName. If toName is
// empty the group is dropped (and the entire comment line is removed when no
// fields remain).
func rewriteGroupReference(sources []string, fromName, toName string) (BatchResult, error) {
	dedup := map[string]bool{}
	ordered := make([]string, 0, len(sources))
	for _, path := range sources {
		if path == "" || dedup[path] {
			continue
		}
		dedup[path] = true
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	plans := make([]filePlan, 0, len(ordered))
	for _, path := range ordered {
		original, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return BatchResult{}, err
		}
		newContent, hostsChanged := planRewriteGroupReference(string(original), fromName, toName)
		if hostsChanged == 0 {
			continue
		}
		plans = append(plans, filePlan{
			path:         path,
			original:     original,
			newContent:   newContent,
			hostsChanged: hostsChanged,
		})
	}

	return commitPlans(plans)
}

type filePlan struct {
	path         string
	original     []byte
	newContent   string
	hostsChanged int
}

func commitPlans(plans []filePlan) (BatchResult, error) {
	if len(plans) == 0 {
		return BatchResult{}, nil
	}
	result := BatchResult{
		FilesChanged: make([]string, 0, len(plans)),
		Backups:      make([]string, 0, len(plans)),
	}
	for _, plan := range plans {
		if err := backupFile(plan.path, plan.original); err != nil {
			return result, fmt.Errorf("backup %s: %w", plan.path, err)
		}
		if err := os.WriteFile(plan.path, []byte(plan.newContent), fileMode(plan.path)); err != nil {
			return result, fmt.Errorf("write %s: %w", plan.path, err)
		}
		result.FilesChanged = append(result.FilesChanged, plan.path)
		result.HostsChanged += plan.hostsChanged
	}
	return result, nil
}

// planRewriteGroupReference returns the new file content and the number of
// host metadata comments that were rewritten. toName == "" drops the group.
func planRewriteGroupReference(content, fromName, toName string) (string, int) {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	changed := 0
	for _, line := range lines {
		meta, ok := parseMetadataComment(line)
		if !ok || meta.group != fromName {
			out = append(out, line)
			continue
		}
		newLine, drop := rewriteMetadataLine(line, meta, toName)
		changed++
		if drop {
			continue
		}
		out = append(out, newLine)
	}
	return strings.Join(out, "\n"), changed
}

// rewriteMetadataLine rebuilds a metadata comment after changing the group
// value. Returns drop=true when the entire comment line should be removed
// because no fields remain.
func rewriteMetadataLine(original string, meta metadata, newGroup string) (string, bool) {
	indent := original[:len(original)-len(strings.TrimLeft(original, " \t"))]
	form := HostForm{
		Group:  newGroup,
		Hidden: meta.hidden,
		Tags:   append([]string(nil), meta.tags...),
	}
	rendered := renderMetadata(form)
	if rendered == "" {
		return "", true
	}
	return indent + rendered, false
}

// planMoveHosts edits the metadata comment immediately preceding each given
// host's Host line. If the host has no preceding metadata comment, one is
// inserted; if the host already has one, its group field is replaced (and the
// comment dropped when nothing remains).
func planMoveHosts(content string, hostsInFile []HostEntry, target string) (string, int, error) {
	lines := strings.Split(content, "\n")
	hostLines := map[int]struct{}{}
	for _, host := range hostsInFile {
		idx := host.SourceLine - 1
		if idx < 0 || idx >= len(lines) {
			return "", 0, fmt.Errorf("host %q out of range (line %d)", host.Alias, host.SourceLine)
		}
		key, _, ok := parseDirective(lines[idx])
		if !ok || !strings.EqualFold(key, "host") {
			return "", 0, fmt.Errorf("host %q at line %d is not a Host directive", host.Alias, host.SourceLine)
		}
		hostLines[idx] = struct{}{}
	}

	type edit struct {
		removeMetaIdx int
		insertAtIdx   int
		insertText    string
	}
	edits := make([]edit, 0, len(hostsInFile))
	for hostIdx := range hostLines {
		var existingMeta *metadata
		metaIdx := -1
		if hostIdx > 0 {
			if meta, ok := parseMetadataComment(lines[hostIdx-1]); ok {
				existingMeta = &meta
				metaIdx = hostIdx - 1
			}
		}

		var newMeta metadata
		if existingMeta != nil {
			newMeta = *existingMeta
		}
		newMeta.group = target

		form := HostForm{Group: newMeta.group, Hidden: newMeta.hidden, Tags: append([]string(nil), newMeta.tags...)}
		rendered := renderMetadata(form)

		switch {
		case metaIdx >= 0 && rendered == "":
			edits = append(edits, edit{removeMetaIdx: metaIdx, insertAtIdx: -1})
		case metaIdx >= 0:
			indent := lines[metaIdx][:len(lines[metaIdx])-len(strings.TrimLeft(lines[metaIdx], " \t"))]
			edits = append(edits, edit{removeMetaIdx: metaIdx, insertAtIdx: metaIdx, insertText: indent + rendered})
		case rendered != "":
			edits = append(edits, edit{removeMetaIdx: -1, insertAtIdx: hostIdx, insertText: rendered})
		}
	}

	sort.Slice(edits, func(i, j int) bool {
		ai := edits[i].insertAtIdx
		if ai < 0 {
			ai = edits[i].removeMetaIdx
		}
		aj := edits[j].insertAtIdx
		if aj < 0 {
			aj = edits[j].removeMetaIdx
		}
		return ai > aj
	})

	for _, e := range edits {
		switch {
		case e.removeMetaIdx >= 0 && e.insertAtIdx == e.removeMetaIdx:
			lines[e.removeMetaIdx] = e.insertText
		case e.removeMetaIdx >= 0 && e.insertAtIdx < 0:
			lines = append(lines[:e.removeMetaIdx], lines[e.removeMetaIdx+1:]...)
		case e.removeMetaIdx < 0 && e.insertAtIdx >= 0:
			lines = append(lines[:e.insertAtIdx], append([]string{e.insertText}, lines[e.insertAtIdx:]...)...)
		}
	}

	return strings.Join(lines, "\n"), len(hostsInFile), nil
}
