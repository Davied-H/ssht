package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/dong/ssht/internal/sshconfig"
)

func TestParseOptionsDefaultsTerminalToAuto(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	options, err := parseOptions([]string{})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.Terminal != "auto" {
		t.Fatalf("terminal = %q, want auto", options.Terminal)
	}
	if options.OpenMode != "auto" {
		t.Fatalf("open mode = %q, want auto", options.OpenMode)
	}
}

func TestParseOptionsUsesTerminalEnvironment(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "terminal")
	t.Setenv("SSHT_OPEN_MODE", "")

	options, err := parseOptions([]string{})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.Terminal != "terminal" {
		t.Fatalf("terminal = %q, want terminal", options.Terminal)
	}
}

func TestParseOptionsFlagOverridesTerminalEnvironment(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "terminal")
	t.Setenv("SSHT_OPEN_MODE", "")

	options, err := parseOptions([]string{"--terminal", "iterm"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.Terminal != "iterm" {
		t.Fatalf("terminal = %q, want iterm", options.Terminal)
	}
}

func TestParseOptionsRejectsInvalidTerminal(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	_, err := parseOptions([]string{"--terminal", "bad"})
	if err == nil {
		t.Fatal("expected invalid terminal error")
	}
}

func TestParseOptionsUsesOpenModeEnvironment(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "auto")

	options, err := parseOptions([]string{})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.OpenMode != "auto" {
		t.Fatalf("open mode = %q, want auto", options.OpenMode)
	}
}

func TestParseOptionsFlagOverridesOpenModeEnvironment(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "tab")

	options, err := parseOptions([]string{"--open-mode", "window"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.OpenMode != "window" {
		t.Fatalf("open mode = %q, want window", options.OpenMode)
	}
}

func TestParseOptionsAcceptsSplitOpenMode(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	options, err := parseOptions([]string{"--open-mode", "split"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.OpenMode != "split" {
		t.Fatalf("open mode = %q, want split", options.OpenMode)
	}
}

func TestParseOptionsRejectsInvalidOpenMode(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	_, err := parseOptions([]string{"--open-mode", "pane"})
	if err == nil {
		t.Fatal("expected invalid open mode error")
	}
	if got := err.Error(); !strings.Contains(got, "auto, window, tab, split") {
		t.Fatalf("error = %q, want supported modes", got)
	}
}

func TestParseOptionsAcceptsConnectHost(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	options, err := parseOptions([]string{"--connect", "prod-api-01"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.ConnectHost != "prod-api-01" {
		t.Fatalf("connect host = %q, want prod-api-01", options.ConnectHost)
	}
}

func TestParseOptionsAcceptsDoctorSubcommand(t *testing.T) {
	options, err := parseOptions([]string{"doctor", "--config", "/tmp/config"})
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if !options.Doctor {
		t.Fatal("Doctor = false, want true")
	}
	if options.ConfigPath != "/tmp/config" {
		t.Fatalf("ConfigPath = %q, want /tmp/config", options.ConfigPath)
	}
}

func TestEnableTUIColorProfileForcesTrueColor(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previous)
	})

	lipgloss.SetColorProfile(termenv.ANSI256)
	enableTUIColorProfile()

	if got := lipgloss.ColorProfile(); got != termenv.TrueColor {
		t.Fatalf("color profile = %v, want %v", got, termenv.TrueColor)
	}
}

func TestFindHostByAlias(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "dev"},
		{Alias: "prod"},
	}

	entry, ok := findHost(entries, "prod")
	if !ok {
		t.Fatal("expected host to be found")
	}
	if entry.Alias != "prod" {
		t.Fatalf("alias = %q, want prod", entry.Alias)
	}
}

func TestFindHostRejectsEmptyAlias(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "prod"}}

	if _, ok := findHost(entries, ""); ok {
		t.Fatal("expected empty alias to be rejected")
	}
}
