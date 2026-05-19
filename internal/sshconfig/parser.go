package sshconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultMaxDepth = 16
	defaultMaxFiles = 256
)

type Options struct {
	NoInclude bool
	MaxDepth  int
	MaxFiles  int
}

type HostEntry struct {
	Alias        string   `json:"alias"`
	Group        string   `json:"group,omitempty"`
	HostName     string   `json:"hostName,omitempty"`
	User         string   `json:"user,omitempty"`
	Port         string   `json:"port,omitempty"`
	IdentityFile string   `json:"identityFile,omitempty"`
	ProxyJump    string   `json:"proxyJump,omitempty"`
	ProxyCommand string   `json:"proxyCommand,omitempty"`
	SSHPassword  string   `json:"-"`
	Tags         []string `json:"tags,omitempty"`
	SourceFile   string   `json:"sourceFile"`
	SourceLine   int      `json:"sourceLine"`
	RawBlock     string   `json:"rawBlock,omitempty"`
}

type Warning struct {
	Path    string
	Line    int
	Message string
}

func (w Warning) Error() string {
	if w.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", w.Path, w.Line, w.Message)
	}
	return fmt.Sprintf("%s: %s", w.Path, w.Message)
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.ssh/config"
	}
	return filepath.Join(home, ".ssh", "config")
}

func ParseFile(path string, opts Options) ([]HostEntry, []Warning, error) {
	if opts.MaxDepth == 0 {
		opts.MaxDepth = defaultMaxDepth
	}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = defaultMaxFiles
	}

	parser := fileParser{
		opts:    opts,
		visited: map[string]bool{},
	}
	entries, warnings := parser.parse(expandPath(path), 0)
	return entries, warnings, nil
}

type fileParser struct {
	opts      Options
	visited   map[string]bool
	fileCount int
}

type block struct {
	aliases []string
	group   string
	hidden  bool
	fields  map[string]string
	sshPass string
	tags    []string
	source  string
	line    int
	raw     []string
}

func (p *fileParser) parse(path string, depth int) ([]HostEntry, []Warning) {
	if depth > p.opts.MaxDepth {
		return nil, []Warning{{Path: path, Message: "include depth limit exceeded"}}
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if p.visited[abs] {
		return nil, nil
	}
	p.visited[abs] = true
	p.fileCount++
	if p.fileCount > p.opts.MaxFiles {
		return nil, []Warning{{Path: path, Message: "include file limit exceeded"}}
	}

	file, err := os.Open(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, []Warning{{Path: abs, Message: "config file does not exist"}}
	}
	if err != nil {
		return nil, []Warning{{Path: abs, Message: err.Error()}}
	}
	defer file.Close()

	var entries []HostEntry
	var warnings []Warning
	var current *block
	var pending metadata
	flush := func() {
		if current == nil {
			return
		}
		entries = append(entries, current.entries()...)
		current = nil
	}

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		rawLine := scanner.Text()
		if meta, ok := parseMetadataComment(rawLine); ok {
			pending = mergeMetadata(pending, meta)
			continue
		}
		key, values, ok := parseDirective(rawLine)
		if !ok {
			if current != nil {
				current.raw = append(current.raw, rawLine)
			}
			continue
		}

		switch strings.ToLower(key) {
		case "include":
			if current != nil {
				current.raw = append(current.raw, rawLine)
			}
			if p.opts.NoInclude {
				continue
			}
			for _, pattern := range values {
				childEntries, childWarnings := p.parseInclude(abs, pattern, depth+1, lineNo)
				entries = append(entries, childEntries...)
				warnings = append(warnings, childWarnings...)
			}
		case "host":
			flush()
			current = &block{
				aliases: values,
				group:   pending.group,
				hidden:  pending.hidden,
				fields:  map[string]string{},
				sshPass: pending.sshPass,
				tags:    append([]string(nil), pending.tags...),
				source:  abs,
				line:    lineNo,
				raw:     []string{rawLine},
			}
			pending = metadata{}
		case "match":
			flush()
			pending = metadata{}
		default:
			if current == nil {
				continue
			}
			current.raw = append(current.raw, rawLine)
			if len(values) > 0 {
				current.fields[strings.ToLower(key)] = strings.Join(values, " ")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, Warning{Path: abs, Message: err.Error()})
	}
	flush()
	return entries, warnings
}

func (p *fileParser) parseInclude(parent, pattern string, depth, line int) ([]HostEntry, []Warning) {
	resolved := expandPath(pattern)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(parent), resolved)
	}

	matches, err := filepath.Glob(resolved)
	if err != nil {
		return nil, []Warning{{Path: parent, Line: line, Message: "invalid include pattern: " + pattern}}
	}
	if len(matches) == 0 {
		return nil, []Warning{{Path: parent, Line: line, Message: "include matched no files: " + pattern}}
	}
	sort.Strings(matches)

	var entries []HostEntry
	var warnings []Warning
	for _, match := range matches {
		childEntries, childWarnings := p.parse(match, depth)
		entries = append(entries, childEntries...)
		warnings = append(warnings, childWarnings...)
	}
	return entries, warnings
}

func (b block) entries() []HostEntry {
	if b.hidden {
		return nil
	}
	var entries []HostEntry
	for _, alias := range b.aliases {
		if !isConcreteAlias(alias) {
			continue
		}
		entries = append(entries, HostEntry{
			Alias:        alias,
			Group:        b.group,
			HostName:     b.fields["hostname"],
			User:         b.fields["user"],
			Port:         b.fields["port"],
			IdentityFile: b.fields["identityfile"],
			ProxyJump:    b.fields["proxyjump"],
			ProxyCommand: b.fields["proxycommand"],
			SSHPassword:  b.sshPass,
			Tags:         mergeTags(b.tags, splitTags(b.fields["tag"])),
			SourceFile:   b.source,
			SourceLine:   b.line,
			RawBlock:     strings.Join(b.raw, "\n"),
		})
	}
	return entries
}

func parseDirective(line string) (string, []string, bool) {
	trimmed := strings.TrimSpace(stripComment(line))
	if trimmed == "" {
		return "", nil, false
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

func stripComment(line string) string {
	var out strings.Builder
	quote := rune(0)
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			out.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			out.WriteRune(r)
		case r == '#':
			return out.String()
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func isConcreteAlias(alias string) bool {
	return alias != "" && !strings.ContainsAny(alias, "*?!")
}

func splitTags(value string) []string {
	if value == "" {
		return nil
	}
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
}

type metadata struct {
	group   string
	hidden  bool
	tags    []string
	sshPass string
}

func mergeMetadata(existing, next metadata) metadata {
	if next.group != "" {
		existing.group = next.group
	}
	if next.hidden {
		existing.hidden = true
	}
	if len(next.tags) > 0 {
		existing.tags = mergeTags(existing.tags, next.tags)
	}
	if next.sshPass != "" {
		existing.sshPass = next.sshPass
	}
	return existing
}

func parseMetadataComment(line string) (metadata, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return metadata{}, false
	}
	body := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	if password, ok := parseSSHPasswordComment(body); ok {
		return metadata{sshPass: password}, true
	}
	if !strings.HasPrefix(body, "ssht:") {
		return metadata{}, false
	}
	body = strings.TrimSpace(strings.TrimPrefix(body, "ssht:"))
	var meta metadata
	for _, field := range strings.Fields(body) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "group":
			meta.group = strings.TrimSpace(value)
		case "hidden":
			meta.hidden = parseBoolMetadata(value)
		case "tags":
			meta.tags = splitTags(value)
		}
	}
	return meta, true
}

func parseBoolMetadata(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseSSHPasswordComment(body string) (string, bool) {
	prefixes := []string{"sshpass 密码:", "sshpass password:"}
	lower := strings.ToLower(body)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return strings.TrimSpace(body[len(prefix):]), true
		}
	}
	return "", false
}

func mergeTags(primary, secondary []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, tag := range append(append([]string{}, primary...), secondary...) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func expandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}
