package doctor

import (
	"strings"
	"testing"

	"github.com/dong/ssht/internal/sshconfig"
)

func TestCheckFindsDuplicateAliasAndInvalidPort(t *testing.T) {
	findings := Check([]sshconfig.HostEntry{
		{Alias: "prod", HostName: "192.0.2.12", Port: "22", SourceFile: "a", SourceLine: 1},
		{Alias: "prod", HostName: "192.0.2.13", Port: "bad", SourceFile: "b", SourceLine: 2},
	}, nil)

	text := Format(findings)
	for _, want := range []string{"duplicate Host alias", "Port must be a number"} {
		if !strings.Contains(text, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, text)
		}
	}
}

func TestCheckReportsWarnings(t *testing.T) {
	findings := Check(nil, []sshconfig.Warning{{Path: "config", Message: "include matched no files"}})
	if len(findings) != 1 || findings[0].Severity != SeverityWarn {
		t.Fatalf("findings = %#v, want one warning", findings)
	}
}
