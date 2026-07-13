package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Runner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

// TempScriptRunner is required only by terminal backends that open scripts as files.
type TempScriptRunner interface {
	Runner
	WriteTempScript(content string) (string, error)
	Remove(path string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (ExecRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (ExecRunner) WriteTempScript(content string) (string, error) {
	file, err := os.CreateTemp("", "ssht-warp-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Chmod(0o700); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (ExecRunner) Remove(path string) error {
	return os.Remove(path)
}

type Manager struct {
	Runner     Runner
	Preference string
	OpenMode   OpenMode
	Env        EnvLookup
	selected   *Backend
	lastMode   OpenMode
}

type Target struct {
	Alias       string
	SSHPassword string
}

type OpenMode string

const (
	OpenModeAuto   OpenMode = "auto"
	OpenModeWindow OpenMode = "window"
	OpenModeTab    OpenMode = "tab"
	OpenModeSplit  OpenMode = "split"
)

type EnvLookup func(string) string

type Backend struct {
	ID       string
	Name     string
	Check    func(Runner) error
	Supports func(OpenMode) bool
	Connect  func(Runner, OpenMode, Target, EnvLookup) (OpenMode, error)
}

func New() Manager {
	return Manager{Runner: ExecRunner{}, OpenMode: OpenModeAuto}
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
	actualMode, err := backend.Connect(m.runner(), m.openMode(), normalized, m.env())
	if err != nil {
		return err
	}
	m.lastMode = actualMode
	return nil
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
	if m.lastMode != "" {
		return string(m.lastMode)
	}
	return string(m.openMode())
}

func (m *Manager) selectBackend() (*Backend, error) {
	if m.selected != nil {
		return m.selected, nil
	}
	mode := m.openMode()
	if !ValidOpenMode(mode) {
		return nil, fmt.Errorf("unsupported open mode %q; supported modes: auto, window, tab, split", mode)
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
	backends = prioritizeCurrentBackend(backends, currentTerminalBackendID(m.env()))
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
			Supports: supportsITermModes,
			Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
				command := sshCommand(target)
				hasWindow := false
				if mode == OpenModeAuto || mode == OpenModeTab || mode == OpenModeSplit {
					hasWindow = hasAppWindow(runner, "iTerm")
				}
				if mode == OpenModeSplit {
					if !isITermSession(env) || !hasWindow {
						return "", fmt.Errorf("iTerm2 split mode requires an active iTerm2 session")
					}
					return OpenModeSplit, runner.Run("osascript",
						"-e", `tell application "iTerm"`,
						"-e", "activate",
						"-e", `tell current session of current window`,
						"-e", `set newSession to (split vertically with default profile)`,
						"-e", `tell newSession to write text `+appleScriptString(command),
						"-e", "end tell",
						"-e", "end tell",
					)
				}
				if mode == OpenModeAuto && isITermSession(env) && hasWindow {
					return OpenModeSplit, runner.Run("osascript",
						"-e", `tell application "iTerm"`,
						"-e", "activate",
						"-e", `tell current session of current window`,
						"-e", `set newSession to (split vertically with default profile)`,
						"-e", `tell newSession to write text `+appleScriptString(command),
						"-e", "end tell",
						"-e", "end tell",
					)
				}
				if mode == OpenModeAuto || mode == OpenModeTab {
					if !hasWindow {
						return OpenModeWindow, runner.Run("osascript",
							"-e", `tell application "iTerm"`,
							"-e", "activate",
							"-e", `create window with default profile`,
							"-e", `tell current session of current window to write text `+appleScriptString(command),
							"-e", "end tell",
						)
					}
					return OpenModeTab, runner.Run("osascript",
						"-e", `tell application "iTerm"`,
						"-e", "activate",
						"-e", `tell current window to create tab with default profile`,
						"-e", `tell current session of current window to write text `+appleScriptString(command),
						"-e", "end tell",
					)
				}
				return OpenModeWindow, runner.Run("osascript",
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
			Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
				command := sshCommand(target)
				if (mode == OpenModeAuto || mode == OpenModeTab) && hasAppWindow(runner, "Terminal") {
					return OpenModeTab, runner.Run("osascript",
						"-e", `tell application "Terminal"`,
						"-e", "activate",
						"-e", `do script `+appleScriptString(command)+` in front window`,
						"-e", "end tell",
					)
				}
				return OpenModeWindow, runner.Run("osascript",
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
		warpBackend(),
	}
}

func prioritizeCurrentBackend(backends []Backend, id string) []Backend {
	if id == "" {
		return backends
	}
	idx := -1
	for i, backend := range backends {
		if backend.ID == id {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return backends
	}
	out := make([]Backend, 0, len(backends))
	out = append(out, backends[idx])
	out = append(out, backends[:idx]...)
	out = append(out, backends[idx+1:]...)
	return out
}

func currentTerminalBackendID(env EnvLookup) string {
	if env == nil {
		return ""
	}
	termProgram := strings.ToLower(strings.TrimSpace(env("TERM_PROGRAM")))
	switch termProgram {
	case "iterm.app":
		return "iterm"
	case "apple_terminal":
		return "terminal"
	case "wezterm":
		return "wezterm"
	case "ghostty":
		return "ghostty"
	case "warpterminal":
		return "warp"
	}
	if env("KITTY_WINDOW_ID") != "" {
		return "kitty"
	}
	if env("ALACRITTY_WINDOW_ID") != "" {
		return "alacritty"
	}
	if env("WEZTERM_PANE") != "" {
		return "wezterm"
	}
	if env("GHOSTTY_RESOURCES_DIR") != "" || env("GHOSTTY_BIN_DIR") != "" {
		return "ghostty"
	}
	return ""
}

func appCheck(name string) func(Runner) error {
	return func(runner Runner) error {
		_, err := runner.Output("osascript", "-e", `id of application "`+name+`"`)
		return err
	}
}

func supportsWindowAndTab(mode OpenMode) bool {
	return mode == OpenModeAuto || mode == OpenModeWindow || mode == OpenModeTab
}

func supportsITermModes(mode OpenMode) bool {
	return supportsWindowAndTab(mode) || mode == OpenModeSplit
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
		Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
			runArgs := append([]string{}, args...)
			runArgs = append(runArgs, sshCommandArgs(target)...)
			return OpenModeWindow, runner.Run(command, runArgs...)
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
			return mode == OpenModeAuto || mode == OpenModeWindow || mode == OpenModeTab
		},
		Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
			runArgs := append([]string{}, windowArgs...)
			actualMode := OpenModeWindow
			if mode == OpenModeAuto || mode == OpenModeTab {
				runArgs = append([]string{}, tabArgs...)
				actualMode = OpenModeTab
			}
			runArgs = append(runArgs, sshCommandArgs(target)...)
			return actualMode, runner.Run(command, runArgs...)
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
			return mode == OpenModeAuto || mode == OpenModeWindow || mode == OpenModeTab
		},
		Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
			if mode == OpenModeAuto || mode == OpenModeTab {
				return connectGhosttyTab(runner, target)
			}
			runArgs := append([]string{"-e"}, sshCommandArgs(target)...)
			return OpenModeWindow, runner.Run("ghostty", runArgs...)
		},
	}
}

func warpBackend() Backend {
	return Backend{
		ID:       "warp",
		Name:     "Warp",
		Check:    appCheck("Warp"),
		Supports: supportsWarpModes,
		Connect: func(runner Runner, mode OpenMode, target Target, env EnvLookup) (OpenMode, error) {
			if mode == OpenModeSplit {
				return connectWarpSplit(runner, target)
			}
			scriptRunner, ok := runner.(TempScriptRunner)
			if !ok {
				return "", fmt.Errorf("Warp launcher cannot create a temporary command script")
			}
			path, err := scriptRunner.WriteTempScript(warpCommandScript(sshCommand(target)))
			if err != nil {
				return "", fmt.Errorf("create Warp command script: %w", err)
			}

			args := []string{"-a", "Warp", path}
			actualMode := OpenModeTab
			if mode == OpenModeWindow {
				args = []string{"-n", "-a", "Warp", path}
				actualMode = OpenModeWindow
			}
			if err := runner.Run("open", args...); err != nil {
				_ = scriptRunner.Remove(path)
				return "", err
			}
			return actualMode, nil
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
	return mode == OpenModeAuto || mode == OpenModeWindow || mode == OpenModeTab || mode == OpenModeSplit
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

func (m *Manager) env() EnvLookup {
	if m.Env != nil {
		return m.Env
	}
	return os.Getenv
}

func (m *Manager) openMode() OpenMode {
	if m.OpenMode == "" {
		return OpenModeAuto
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

func connectGhosttyTab(runner Runner, target Target) (OpenMode, error) {
	command := sshCommand(target)
	operation := `new window with configuration cfg`
	actualMode := OpenModeWindow
	if hasAppWindow(runner, "Ghostty") {
		operation = `tell front window to new tab with configuration cfg`
		actualMode = OpenModeTab
	}
	return actualMode, runner.Run("osascript",
		"-e", `tell application "Ghostty"`,
		"-e", "activate",
		"-e", `set cfg to default configuration`,
		"-e", `set command of cfg to `+appleScriptString(command),
		"-e", operation,
		"-e", "end tell",
	)
}

func warpCommandScript(command string) string {
	return "#!/bin/sh\nrm -f -- \"$0\"\nexec " + command + "\n"
}

func supportsWarpModes(mode OpenMode) bool {
	return supportsWindowAndTab(mode) || mode == OpenModeSplit
}

func connectWarpSplit(runner Runner, target Target) (OpenMode, error) {
	scriptRunner, ok := runner.(TempScriptRunner)
	if !ok {
		return "", fmt.Errorf("Warp launcher cannot create a temporary command script")
	}
	path, err := scriptRunner.WriteTempScript(warpCommandScript(sshCommand(target)))
	if err != nil {
		return "", fmt.Errorf("create Warp command script: %w", err)
	}

	err = runner.Run("osascript",
		"-e", `tell application "Warp" to activate`,
		"-e", "delay 0.2",
		"-e", `tell application "System Events"`,
		"-e", `tell process "Warp"`,
		"-e", "set frontmost to true",
		"-e", `keystroke "d" using command down`,
		"-e", "delay 0.5",
		"-e", `keystroke `+appleScriptString(shellQuote(path)),
		// Warp processes synthetic text input asynchronously before it accepts Return.
		"-e", "delay 0.3",
		"-e", "key code 36",
		"-e", "end tell",
		"-e", "end tell",
	)
	if err != nil {
		_ = scriptRunner.Remove(path)
		return "", fmt.Errorf("Warp split failed; allow the calling app to control your computer in macOS Privacy & Security > Accessibility: %w", err)
	}
	return OpenModeSplit, nil
}

func isITermSession(env EnvLookup) bool {
	if env == nil {
		return false
	}
	return env("TERM_PROGRAM") == "iTerm.app" || env("ITERM_SESSION_ID") != ""
}

func sshCommand(target Target) string {
	if target.SSHPassword == "" {
		return "env -u LC_ALL ssh " + shellQuote(target.Alias)
	}
	return "env -u LC_ALL sshpass -p " + shellQuote(target.SSHPassword) + " ssh " + shellQuote(target.Alias)
}

func sshCommandArgs(target Target) []string {
	if target.SSHPassword == "" {
		return []string{"env", "-u", "LC_ALL", "ssh", target.Alias}
	}
	return []string{"env", "-u", "LC_ALL", "sshpass", "-p", target.SSHPassword, "ssh", target.Alias}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func appleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
