package sshconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type HostForm struct {
	Alias        string
	Group        string
	HostName     string
	User         string
	Port         string
	IdentityFile string
	ProxyJump    string
	ProxyCommand string
	SSHPassword  string
	Tags         []string
}

func AddHost(path string, form HostForm) error {
	if err := ValidateHostForm(form); err != nil {
		return err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := backupFile(path, content); err != nil {
		return err
	}

	text := string(content)
	if strings.TrimSpace(text) != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if strings.TrimSpace(text) != "" {
		text += "\n"
	}
	text += renderHostBlock([]string{form.Alias}, form)
	return os.WriteFile(path, []byte(text), fileMode(path))
}

func EditHost(entry HostEntry, form HostForm) error {
	if err := validateWritableEntry(entry); err != nil {
		return err
	}
	if err := ValidateHostForm(form); err != nil {
		return err
	}

	content, err := os.ReadFile(entry.SourceFile)
	if err != nil {
		return err
	}
	start, end, aliases, err := locateHostBlock(string(content), entry.SourceLine)
	if err != nil {
		return err
	}
	lines := splitLines(string(content))
	hostRel := entry.SourceLine - 1 - start
	if len(aliases) > 1 && form.Alias != entry.Alias {
		return fmt.Errorf("cannot change alias in multi-alias Host block")
	}
	if len(aliases) == 1 {
		aliases[0] = form.Alias
	}

	if err := backupFile(entry.SourceFile, content); err != nil {
		return err
	}
	updated := replaceLines(string(content), start, end, renderUpdatedHostBlock(lines[start:end], hostRel, aliases, form))
	return os.WriteFile(entry.SourceFile, []byte(updated), fileMode(entry.SourceFile))
}

func DeleteHost(entry HostEntry) error {
	if err := validateWritableEntry(entry); err != nil {
		return err
	}

	content, err := os.ReadFile(entry.SourceFile)
	if err != nil {
		return err
	}
	start, end, aliases, err := locateHostBlock(string(content), entry.SourceLine)
	if err != nil {
		return err
	}
	lines := splitLines(string(content))
	hostRel := entry.SourceLine - 1 - start

	var replacement string
	if len(aliases) > 1 {
		remaining := make([]string, 0, len(aliases)-1)
		for _, alias := range aliases {
			if alias != entry.Alias {
				remaining = append(remaining, alias)
			}
		}
		if len(remaining) == len(aliases) {
			return fmt.Errorf("alias %q not found in Host block", entry.Alias)
		}
		form := HostForm{
			Alias:        remaining[0],
			Group:        entry.Group,
			HostName:     entry.HostName,
			User:         entry.User,
			Port:         entry.Port,
			IdentityFile: entry.IdentityFile,
			ProxyJump:    entry.ProxyJump,
			ProxyCommand: entry.ProxyCommand,
			SSHPassword:  entry.SSHPassword,
			Tags:         entry.Tags,
		}
		replacement = renderUpdatedHostBlock(lines[start:end], hostRel, remaining, form)
	}

	if err := backupFile(entry.SourceFile, content); err != nil {
		return err
	}
	updated := replaceLines(string(content), start, end, replacement)
	return os.WriteFile(entry.SourceFile, []byte(updated), fileMode(entry.SourceFile))
}

func ValidateHostForm(form HostForm) error {
	if strings.TrimSpace(form.Alias) == "" {
		return fmt.Errorf("alias is required")
	}
	if strings.ContainsAny(form.Alias, "*?! \t\r\n") {
		return fmt.Errorf("alias must be a concrete Host name")
	}
	if form.Port != "" {
		port, err := strconv.Atoi(form.Port)
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("port must be a number between 1 and 65535")
		}
	}
	return nil
}

func validateWritableEntry(entry HostEntry) error {
	if entry.SourceFile == "" {
		return fmt.Errorf("source file is required")
	}
	if entry.SourceLine <= 0 {
		return fmt.Errorf("source line is required")
	}
	if entry.Alias == "" {
		return fmt.Errorf("alias is required")
	}
	return nil
}

func locateHostBlock(content string, sourceLine int) (int, int, []string, error) {
	lines := splitLines(content)
	hostStart := sourceLine - 1
	if hostStart < 0 || hostStart >= len(lines) {
		return 0, 0, nil, fmt.Errorf("source line %d is outside file", sourceLine)
	}
	key, values, ok := parseDirective(lines[hostStart])
	if !ok || !strings.EqualFold(key, "host") {
		return 0, 0, nil, fmt.Errorf("source line %d is not a Host directive", sourceLine)
	}
	start := hostStart
	for start > 0 && isMetadataLine(lines[start-1]) {
		start--
	}
	aliases := append([]string(nil), values...)
	end := len(lines)
	for i := hostStart + 1; i < len(lines); i++ {
		key, _, ok := parseDirective(lines[i])
		if ok && (strings.EqualFold(key, "host") || strings.EqualFold(key, "match")) {
			end = i
			break
		}
	}
	return start, end, aliases, nil
}

func renderHostBlock(aliases []string, form HostForm) string {
	var b strings.Builder
	writeMetadataLines(&b, form)
	writeHostHeaderAndManagedFields(&b, aliases, form)
	return b.String()
}

func renderUpdatedHostBlock(original []string, hostRel int, aliases []string, form HostForm) string {
	var b strings.Builder
	writeMetadataLines(&b, form)
	writeHostHeaderAndManagedFields(&b, aliases, form)
	for i, line := range original {
		if i <= hostRel {
			continue
		}
		if isManagedHostDirective(line) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func writeMetadataLines(b *strings.Builder, form HostForm) {
	if strings.TrimSpace(form.SSHPassword) != "" {
		fmt.Fprintf(b, "# sshpass 密码: %s\n", strings.TrimSpace(form.SSHPassword))
	}
	if metadata := renderMetadata(form); metadata != "" {
		b.WriteString(metadata)
		b.WriteByte('\n')
	}
}

func writeHostHeaderAndManagedFields(b *strings.Builder, aliases []string, form HostForm) {
	fmt.Fprintf(b, "Host %s\n", strings.Join(aliases, " "))
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "    %s %s\n", key, strings.TrimSpace(value))
		}
	}
	add("HostName", form.HostName)
	add("User", form.User)
	add("Port", form.Port)
	add("IdentityFile", form.IdentityFile)
	add("ProxyJump", form.ProxyJump)
	add("ProxyCommand", form.ProxyCommand)
}

func isManagedHostDirective(line string) bool {
	key, _, ok := parseDirective(line)
	if !ok {
		return false
	}
	switch strings.ToLower(key) {
	case "hostname", "user", "port", "identityfile", "proxyjump", "proxycommand", "tag":
		return true
	default:
		return false
	}
}

func renderMetadata(form HostForm) string {
	var parts []string
	if strings.TrimSpace(form.Group) != "" {
		parts = append(parts, "group="+strings.TrimSpace(form.Group))
	}
	if len(form.Tags) > 0 {
		parts = append(parts, "tags="+strings.Join(cleanTags(form.Tags), ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return "# ssht: " + strings.Join(parts, " ")
}

func cleanTags(tags []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func isMetadataLine(line string) bool {
	_, ok := parseMetadataComment(line)
	return ok
}

func replaceLines(content string, start, end int, replacement string) string {
	lines := splitLines(content)
	var b strings.Builder
	for i := 0; i < start; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	if replacement != "" {
		b.WriteString(replacement)
		if !strings.HasSuffix(replacement, "\n") {
			b.WriteByte('\n')
		}
	}
	for i := end; i < len(lines); i++ {
		b.WriteString(lines[i])
		if i < len(lines)-1 || strings.HasSuffix(content, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func splitLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func backupFile(path string, content []byte) error {
	backupPath := fmt.Sprintf("%s.ssht.%s.bak", path, time.Now().Format("20060102150405.000000000"))
	return os.WriteFile(backupPath, content, fileMode(path))
}

func fileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o600
	}
	return info.Mode().Perm()
}
