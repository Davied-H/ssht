package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dong/ssh-config-tmux-tui/internal/app"
	"github.com/dong/ssh-config-tmux-tui/internal/monitor"
	"github.com/dong/ssh-config-tmux-tui/internal/sshconfig"
	"github.com/dong/ssh-config-tmux-tui/internal/state"
	"github.com/dong/ssh-config-tmux-tui/internal/terminal"
)

type options struct {
	ConfigPath     string
	NoInclude      bool
	PrintHosts     bool
	Debug          bool
	Terminal       string
	OpenMode       string
	Monitor        bool
	MonitorTTL     time.Duration
	MonitorTimeout time.Duration
}

func main() {
	options, err := parseOptions(os.Args[1:])
	if err != nil {
		exitErr(err)
	}

	load := func() ([]sshconfig.HostEntry, []sshconfig.Warning, error) {
		return sshconfig.ParseFile(options.ConfigPath, sshconfig.Options{NoInclude: options.NoInclude})
	}

	entries, warnings, err := load()
	if err != nil {
		exitErr(err)
	}
	if options.Debug {
		for _, warning := range warnings {
			fmt.Fprintln(os.Stderr, warning.Error())
		}
	}

	if options.PrintHosts {
		if err := json.NewEncoder(os.Stdout).Encode(entries); err != nil {
			exitErr(err)
		}
		return
	}

	manager := terminal.New()
	manager.Preference = options.Terminal
	manager.OpenMode = terminal.OpenMode(options.OpenMode)
	if err := manager.CheckAvailable(); err != nil {
		warnings = append(warnings, sshconfig.Warning{Path: "terminal", Message: err.Error()})
	}
	statePath := state.DefaultPath()
	store, err := state.Load(statePath)
	if err != nil {
		warnings = append(warnings, sshconfig.Warning{Path: statePath, Message: err.Error()})
		store = state.NewStore()
	}

	monitorCache := monitor.NewCache(options.MonitorTTL, options.MonitorTimeout)
	probeFn := monitor.Probe
	if options.Monitor && hasPasswordHost(entries) {
		if _, err := exec.LookPath("sshpass"); err != nil {
			warnings = append(warnings, sshconfig.Warning{
				Path:    "monitor",
				Message: "sshpass not installed; password-auth hosts will report errors instead of stats",
			})
		}
	}

	model := app.NewModel(app.Config{
		ConfigPath:     options.ConfigPath,
		Entries:        entries,
		Warnings:       warnings,
		Manager:        manager,
		Load:           load,
		StatePath:      statePath,
		State:          store,
		Monitor:        monitorCache,
		Probe:          probeFn,
		MonitorVisible: options.Monitor,
	})
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		exitErr(err)
	}
}

func hasPasswordHost(entries []sshconfig.HostEntry) bool {
	for _, entry := range entries {
		if entry.SSHPassword != "" {
			return true
		}
	}
	return false
}

func parseOptions(args []string) (options, error) {
	opts := options{
		ConfigPath:     sshconfig.DefaultPath(),
		Terminal:       terminalPreferenceDefault(),
		OpenMode:       openModeDefault(),
		MonitorTTL:     30 * time.Second,
		MonitorTimeout: 5 * time.Second,
	}
	flags := flag.NewFlagSet("ssht", flag.ContinueOnError)
	flags.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to ssh config")
	flags.BoolVar(&opts.NoInclude, "no-include", false, "disable Include parsing")
	flags.BoolVar(&opts.PrintHosts, "print-hosts", false, "print discovered hosts as JSON")
	flags.BoolVar(&opts.Debug, "debug", false, "print parser warnings to stderr")
	flags.StringVar(&opts.Terminal, "terminal", opts.Terminal, "terminal backend: auto, iterm, terminal, wezterm, kitty, alacritty, ghostty")
	flags.StringVar(&opts.OpenMode, "open-mode", opts.OpenMode, "terminal open mode: window, tab")
	flags.BoolVar(&opts.Monitor, "monitor", false, "show the SSH monitoring panel on startup (toggle anytime with M)")
	flags.DurationVar(&opts.MonitorTTL, "monitor-ttl", opts.MonitorTTL, "how long a monitor snapshot stays fresh")
	flags.DurationVar(&opts.MonitorTimeout, "monitor-timeout", opts.MonitorTimeout, "hard timeout for one monitor SSH probe")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts.Terminal = strings.ToLower(strings.TrimSpace(opts.Terminal))
	if opts.Terminal == "" {
		opts.Terminal = "auto"
	}
	if !validTerminalPreference(opts.Terminal) {
		return options{}, errors.New("unsupported terminal " + opts.Terminal + "; supported terminals: auto, iterm, terminal, wezterm, kitty, alacritty, ghostty")
	}
	opts.OpenMode = strings.ToLower(strings.TrimSpace(opts.OpenMode))
	if opts.OpenMode == "" {
		opts.OpenMode = string(terminal.OpenModeTab)
	}
	if !terminal.ValidOpenMode(terminal.OpenMode(opts.OpenMode)) {
		return options{}, errors.New("unsupported open mode " + opts.OpenMode + "; supported modes: window, tab")
	}
	return opts, nil
}

func terminalPreferenceDefault() string {
	if value := strings.TrimSpace(os.Getenv("SSHT_TERMINAL")); value != "" {
		return value
	}
	return "auto"
}

func openModeDefault() string {
	if value := strings.TrimSpace(os.Getenv("SSHT_OPEN_MODE")); value != "" {
		return value
	}
	return string(terminal.OpenModeTab)
}

func validTerminalPreference(value string) bool {
	switch value {
	case "auto", "iterm", "terminal", "wezterm", "kitty", "alacritty", "ghostty":
		return true
	default:
		return false
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
