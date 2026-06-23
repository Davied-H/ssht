package doctor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dong/ssht/internal/sshconfig"
)

type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warning"
	SeverityInfo  Severity = "info"
)

type Finding struct {
	Severity Severity
	Subject  string
	Message  string
}

func Check(entries []sshconfig.HostEntry, warnings []sshconfig.Warning) []Finding {
	var findings []Finding
	for _, warning := range warnings {
		findings = append(findings, Finding{
			Severity: SeverityWarn,
			Subject:  warning.Path,
			Message:  warning.Error(),
		})
	}

	byAlias := map[string][]sshconfig.HostEntry{}
	groups := map[string]bool{}
	for _, entry := range entries {
		byAlias[entry.Alias] = append(byAlias[entry.Alias], entry)
		if strings.TrimSpace(entry.Group) != "" {
			groups[entry.Group] = true
		}
		if strings.TrimSpace(entry.HostName) == "" {
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Subject:  entry.Alias,
				Message:  "HostName is empty; OpenSSH will use the alias as target",
			})
		}
		if entry.Port != "" {
			port, err := strconv.Atoi(entry.Port)
			if err != nil || port <= 0 || port > 65535 {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Subject:  entry.Alias,
					Message:  "Port must be a number between 1 and 65535",
				})
			}
		}
		if hasDuplicateTags(entry.Tags) {
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Subject:  entry.Alias,
				Message:  "duplicate tags can be removed",
			})
		}
		if entry.ProxyJump != "" && entry.ProxyCommand != "" {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Subject:  entry.Alias,
				Message:  "both ProxyJump and ProxyCommand are set; OpenSSH precedence may be surprising",
			})
		}
	}

	for alias, matches := range byAlias {
		if len(matches) <= 1 {
			continue
		}
		var sources []string
		for _, entry := range matches {
			sources = append(sources, fmt.Sprintf("%s:%d", entry.SourceFile, entry.SourceLine))
		}
		sort.Strings(sources)
		findings = append(findings, Finding{
			Severity: SeverityWarn,
			Subject:  alias,
			Message:  "duplicate Host alias in " + strings.Join(sources, ", "),
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		if findings[i].Subject != findings[j].Subject {
			return findings[i].Subject < findings[j].Subject
		}
		return findings[i].Message < findings[j].Message
	})
	return findings
}

func Summary(findings []Finding) string {
	if len(findings) == 0 {
		return "doctor: no issues found"
	}
	counts := map[Severity]int{}
	for _, finding := range findings {
		counts[finding.Severity]++
	}
	return fmt.Sprintf("doctor: %d finding(s), %d error(s), %d warning(s), %d info", len(findings), counts[SeverityError], counts[SeverityWarn], counts[SeverityInfo])
}

func Format(findings []Finding) string {
	var b strings.Builder
	b.WriteString(Summary(findings))
	b.WriteByte('\n')
	for _, finding := range findings {
		fmt.Fprintf(&b, "[%s] %s: %s\n", finding.Severity, finding.Subject, finding.Message)
	}
	return b.String()
}

func hasDuplicateTags(tags []string) bool {
	seen := map[string]bool{}
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if key == "" {
			continue
		}
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarn:
		return 1
	default:
		return 2
	}
}
