package main

import "testing"

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
	if options.OpenMode != "tab" {
		t.Fatalf("open mode = %q, want tab", options.OpenMode)
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
	t.Setenv("SSHT_OPEN_MODE", "tab")

	options, err := parseOptions([]string{})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.OpenMode != "tab" {
		t.Fatalf("open mode = %q, want tab", options.OpenMode)
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

func TestParseOptionsRejectsInvalidOpenMode(t *testing.T) {
	t.Setenv("SSHT_TERMINAL", "")
	t.Setenv("SSHT_OPEN_MODE", "")

	_, err := parseOptions([]string{"--open-mode", "pane"})
	if err == nil {
		t.Fatal("expected invalid open mode error")
	}
}
