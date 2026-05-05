package terminal

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Runner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (ExecRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Manager struct {
	Runner     Runner
	Preference string
	OpenMode   OpenMode
	selected   *Backend
}

type Target struct {
	Alias       string
	SSHPassword string
}

type OpenMode string

const (
	OpenModeWindow OpenMode = "window"
	OpenModeTab    OpenMode = "tab"
)

type Backend struct {
	ID       string
	Name     string
	Check    func(Runner) error
	Supports func(OpenMode) bool
	Connect  func(Runner, OpenMode, Target) error
}

func New() Manager {
	return Manager{Runner: ExecRunner{}, OpenMode: OpenModeTab}
}

func (m *Manager) CheckAvailable() error {
	_, err := m.selectBackend()
	return err
}

func (m *Manager) BackendName() string {
	if m.selected != nil {
		return m.selected.Name
	}
	if backend, err := m.selectBackend(); err == nil {
		return backend.Name
	}
	return ""
}

func (m *Manager) Connect(target any) error {
	backend, err := m.selectBackend()
	if err != nil {
		return err
	}
	normalized, err := normalizeTarget(target)
	if err != nil {
		return err
	}
	return backend.Connect(m.runner(), m.openMode(), normalized)
}

func normalizeTarget(value any) (Target, error) {
	switch target := value.(type) {
	case string:
		return Target{Alias: target}, nil
	case Target:
		return target, nil
	default:
		return Target{}, fmt.Errorf("unsupported connect target %T", value)
	}
}

func (m *Manager) OpenModeName() string {
	return string(m.openMode())
}

func (m *Manager) selectBackend() (*Backend, error) {
	if m.selected != nil {
		return m.selected, nil
	}
	mode := m.openMode()
	if !ValidOpenMode(mode) {
		return nil, fmt.Errorf("unsupported open mode %q; supported modes: window, tab", mode)
	}
	backends := defaultBackends()
	if preference := normalizePreference(m.Preference); preference != "" && preference != "auto" {
		backend, ok := findBackend(backends, preference)
		if !ok {
			return nil, fmt.Errorf("unsupported terminal %q; supported terminals: %s", m.Preference, supportedBackends(backends))
		}
		if !backend.supports(mode) {
			return nil, fmt.Errorf("%s does not support %s mode", backend.Name, mode)
		}
		if err := backend.Check(m.runner()); err != nil {
			return nil, fmt.Errorf("%s is not available", backend.Name)
		}
		m.selected = &backend
		return &backend, nil
	}
	var tried []string
	for _, backend := range backends {
		if !backend.supports(mode) {
			continue
		}
		tried = append(tried, backend.Name)
		if err := backend.Check(m.runner()); err == nil {
			m.selected = &backend
			return &backend, nil
		}
	}
	return nil, fmt.Errorf("no supported terminal is available; tried: %s", strings.Join(tried, ", "))
}

func defaultBackends() []Backend {
	return []Backend{
		{
			ID:       "iterm",
			Name:     "iTerm2",
			Check:    appCheck("iTerm"),
			Supports: supportsWindowAndTab,
			Connect: func(runner Runner, mode OpenMode, target Target) error {
				command := sshCommand(target)
				if mode == OpenModeTab {
					if !hasAppWindow(runner, "iTerm") {
						return runner.Run("osascript",
							"-e", `tell application "iTerm"`,
							"-e", "activate",
							"-e", `create window with default profile`,
							"-e", `tell current session of current window to write text `+appleScriptString(command),
							"-e", "end tell",
						)
					}
					return runner.Run("osascript",
						"-e", `tell application "iTerm"`,
						"-e", "activate",
						"-e", `tell current window to create tab with default profile`,
						"-e", `tell current session of current window to write text `+appleScriptString(command),
						"-e", "end tell",
					)
				}
				return runner.Run("osascript",
					"-e", `tell application "iTerm"`,
					"-e", "activate",
					"-e", `create window with default profile`,
					"-e", `tell current session of current window to write text `+appleScriptString(command),
					"-e", "end tell",
				)
			},
		},
		{
			ID:       "terminal",
			Name:     "Terminal.app",
			Check:    appCheck("Terminal"),
			Supports: supportsWindowAndTab,
			Connect: func(runner Runner, mode OpenMode, target Target) error {
				command := sshCommand(target)
				if mode == OpenModeTab && hasAppWindow(runner, "Terminal") {
					return runner.Run("osascript",
						"-e", `tell application "Terminal"`,
						"-e", "activate",
						"-e", `do script `+appleScriptString(command)+` in front window`,
						"-e", "end tell",
					)
				}
				return runner.Run("osascript",
					"-e", `tell application "Terminal"`,
					"-e", "activate",
					"-e", `do script `+appleScriptString(command),
					"-e", "end tell",
				)
			},
		},
		tabbedCLIBackend("wezterm", "WezTerm", "wezterm", []string{"start", "--new-window", "--"}, []string{"start", "--new-tab", "--"}),
		tabbedCLIBackend("kitty", "kitty", "kitty", []string{"--detach"}, []string{"@", "launch", "--type=tab"}),
		windowOnlyCLIBackend("alacritty", "Alacritty", "alacritty", "-e"),
		ghosttyBackend(),
	}
}

func appCheck(name string) func(Runner) error {
	return func(runner Runner) error {
		_, err := runner.Output("osascript", "-e", `id of application "`+name+`"`)
		return err
	}
}

func supportsWindowAndTab(mode OpenMode) bool {
	return mode == OpenModeWindow || mode == OpenModeTab
}

func windowOnlyCLIBackend(id, name, command string, args ...string) Backend {
	return Backend{
		ID:   id,
		Name: name,
		Check: func(runner Runner) error {
			_, err := runner.Output("which", command)
			return err
		},
		Supports: func(mode OpenMode) bool {
			return mode == OpenModeWindow
		},
		Connect: func(runner Runner, mode OpenMode, target Target) error {
			runArgs := append([]string{}, args...)
			runArgs = append(runArgs, sshCommandArgs(target)...)
			return runner.Run(command, runArgs...)
		},
	}
}

func tabbedCLIBackend(id, name, command string, windowArgs, tabArgs []string) Backend {
	return Backend{
		ID:   id,
		Name: name,
		Check: func(runner Runner) error {
			_, err := runner.Output("which", command)
			return err
		},
		Supports: func(mode OpenMode) bool {
			return mode == OpenModeWindow || mode == OpenModeTab
		},
		Connect: func(runner Runner, mode OpenMode, target Target) error {
			runArgs := append([]string{}, windowArgs...)
			if mode == OpenModeTab {
				runArgs = append([]string{}, tabArgs...)
			}
			runArgs = append(runArgs, sshCommandArgs(target)...)
			return runner.Run(command, runArgs...)
		},
	}
}

func ghosttyBackend() Backend {
	return Backend{
		ID:   "ghostty",
		Name: "Ghostty",
		Check: func(runner Runner) error {
			_, err := runner.Output("which", "ghostty")
			return err
		},
		Supports: func(mode OpenMode) bool {
			return mode == OpenModeWindow || mode == OpenModeTab
		},
		Connect: func(runner Runner, mode OpenMode, target Target) error {
			if mode == OpenModeTab {
				return connectGhosttyTab(runner, target)
			}
			runArgs := append([]string{"-e"}, sshCommandArgs(target)...)
			return runner.Run("ghostty", runArgs...)
		},
	}
}

func findBackend(backends []Backend, id string) (Backend, bool) {
	for _, backend := range backends {
		if backend.ID == id {
			return backend, true
		}
	}
	return Backend{}, false
}

func normalizePreference(preference string) string {
	return strings.ToLower(strings.TrimSpace(preference))
}

func ValidOpenMode(mode OpenMode) bool {
	return mode == OpenModeWindow || mode == OpenModeTab
}

func supportedBackends(backends []Backend) string {
	names := make([]string, 0, len(backends)+1)
	names = append(names, "auto")
	for _, backend := range backends {
		names = append(names, backend.ID)
	}
	return strings.Join(names, ", ")
}

func (m *Manager) runner() Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return ExecRunner{}
}

func (m *Manager) openMode() OpenMode {
	if m.OpenMode == "" {
		return OpenModeTab
	}
	return OpenMode(strings.ToLower(strings.TrimSpace(string(m.OpenMode))))
}

func (b Backend) supports(mode OpenMode) bool {
	if b.Supports == nil {
		return mode == OpenModeWindow
	}
	return b.Supports(mode)
}

func hasAppWindow(runner Runner, appName string) bool {
	output, err := runner.Output("osascript", "-e", `tell application "`+appName+`" to count windows`)
	if err != nil {
		return false
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	return err == nil && count > 0
}

func connectGhosttyTab(runner Runner, target Target) error {
	command := sshCommand(target)
	operation := `new window with configuration cfg`
	if hasAppWindow(runner, "Ghostty") {
		operation = `tell front window to new tab with configuration cfg`
	}
	return runner.Run("osascript",
		"-e", `tell application "Ghostty"`,
		"-e", "activate",
		"-e", `set cfg to default configuration`,
		"-e", `set command of cfg to `+appleScriptString(command),
		"-e", operation,
		"-e", "end tell",
	)
}

func sshCommand(target Target) string {
	if target.SSHPassword == "" {
		return "ssh " + shellQuote(target.Alias)
	}
	return "sshpass -p " + shellQuote(target.SSHPassword) + " ssh " + shellQuote(target.Alias)
}

func sshCommandArgs(target Target) []string {
	if target.SSHPassword == "" {
		return []string{"ssh", target.Alias}
	}
	return []string{"sshpass", "-p", target.SSHPassword, "ssh", target.Alias}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func appleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
