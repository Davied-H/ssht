package terminal

import (
	"reflect"
	"strings"
	"testing"
)

func TestAutoDetectSelectsFirstAvailableBackend(t *testing.T) {
	runner := &recordingRunner{failOutputs: map[string]bool{
		`osascript|-e|id of application "iTerm"`: true,
	}}
	manager := Manager{Runner: runner}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}
	if got, want := manager.BackendName(), "Terminal.app"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
}

func TestPreferenceUsesOnlyRequestedBackend(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal"}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.BackendName(), "Terminal.app"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
}

func TestConnectUsesBackendSelectedByAvailabilityCheck(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeWindow}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}
	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
		{"run", "osascript",
			"-e", `tell application "Terminal"`,
			"-e", "activate",
			"-e", `do script "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesITermWindowWithoutTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "iterm", OpenMode: OpenModeWindow}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "iTerm"`},
		{"run", "osascript",
			"-e", `tell application "iTerm"`,
			"-e", "activate",
			"-e", `create window with default profile`,
			"-e", `tell current session of current window to write text "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesITermTabWhenWindowExists(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "iterm", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "iTerm"`},
		{"output", "osascript", "-e", `tell application "iTerm" to count windows`},
		{"run", "osascript",
			"-e", `tell application "iTerm"`,
			"-e", "activate",
			"-e", `tell current window to create tab with default profile`,
			"-e", `tell current session of current window to write text "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesITermWindowForTabWhenNoWindowExists(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		`osascript|-e|tell application "iTerm" to count windows`: []byte("0\n"),
	}}
	manager := Manager{Runner: runner, Preference: "iterm", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "iTerm"`},
		{"output", "osascript", "-e", `tell application "iTerm" to count windows`},
		{"run", "osascript",
			"-e", `tell application "iTerm"`,
			"-e", "activate",
			"-e", `create window with default profile`,
			"-e", `tell current session of current window to write text "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHQuotesAliasForShellAndAppleScript(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "iterm", OpenMode: OpenModeWindow}

	if err := manager.Connect(`prod"api'one`); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	got := strings.Join(runner.calls[1], "\n")
	if !strings.Contains(got, `ssh 'prod\"api'\\''one'`) {
		t.Fatalf("command did not quote alias safely:\n%s", got)
	}
}

func TestOpenSSHUsesTerminalAppWindow(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeWindow}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
		{"run", "osascript",
			"-e", `tell application "Terminal"`,
			"-e", "activate",
			"-e", `do script "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesSshpassWhenPasswordIsProvided(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeWindow}

	if err := manager.Connect(Target{Alias: "prod-api", SSHPassword: `p@ss "word"`}); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
		{"run", "osascript",
			"-e", `tell application "Terminal"`,
			"-e", "activate",
			"-e", `do script "sshpass -p 'p@ss \"word\"' ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesTerminalAppTabWhenWindowExists(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
		{"output", "osascript", "-e", `tell application "Terminal" to count windows`},
		{"run", "osascript",
			"-e", `tell application "Terminal"`,
			"-e", "activate",
			"-e", `do script "ssh 'prod-api'" in front window`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesTerminalAppWindowForTabWhenNoWindowExists(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		`osascript|-e|tell application "Terminal" to count windows`: []byte("0\n"),
	}}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Terminal"`},
		{"output", "osascript", "-e", `tell application "Terminal" to count windows`},
		{"run", "osascript",
			"-e", `tell application "Terminal"`,
			"-e", "activate",
			"-e", `do script "ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCLIBackendUsesArgumentArray(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "wezterm", OpenMode: OpenModeWindow}

	if err := manager.Connect(`prod"api'one`); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "wezterm"},
		{"run", "wezterm", "start", "--new-window", "--", "ssh", `prod"api'one`},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestWezTermTabUsesNewTabFlag(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "wezterm", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "wezterm"},
		{"run", "wezterm", "start", "--new-tab", "--", "ssh", "prod-api"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestKittyTabUsesRemoteControlLaunch(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "kitty", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "kitty"},
		{"run", "kitty", "@", "launch", "--type=tab", "ssh", "prod-api"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestGhosttyTabUsesAppleScriptWhenWindowExists(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "ghostty", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "ghostty"},
		{"output", "osascript", "-e", `tell application "Ghostty" to count windows`},
		{"run", "osascript",
			"-e", `tell application "Ghostty"`,
			"-e", "activate",
			"-e", `set cfg to default configuration`,
			"-e", `set command of cfg to "ssh 'prod-api'"`,
			"-e", `tell front window to new tab with configuration cfg`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestGhosttyTabUsesWindowWhenNoWindowExists(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		`osascript|-e|tell application "Ghostty" to count windows`: []byte("0\n"),
	}}
	manager := Manager{Runner: runner, Preference: "ghostty", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "ghostty"},
		{"output", "osascript", "-e", `tell application "Ghostty" to count windows`},
		{"run", "osascript",
			"-e", `tell application "Ghostty"`,
			"-e", "activate",
			"-e", `set cfg to default configuration`,
			"-e", `set command of cfg to "ssh 'prod-api'"`,
			"-e", `new window with configuration cfg`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestAlacrittyTabReturnsUnsupportedError(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "alacritty", OpenMode: OpenModeTab}

	err := manager.Connect("prod-api")
	if err == nil {
		t.Fatal("expected unsupported tab error")
	}
	if !strings.Contains(err.Error(), "Alacritty does not support tab mode") {
		t.Fatalf("error = %q, want unsupported tab mode", err)
	}
}

func TestAutoDetectTabSkipsWindowOnlyBackends(t *testing.T) {
	runner := &recordingRunner{failOutputs: map[string]bool{
		`osascript|-e|id of application "iTerm"`:    true,
		`osascript|-e|id of application "Terminal"`: true,
		`which|wezterm`: true,
		`which|kitty`:   true,
	}}
	manager := Manager{Runner: runner, OpenMode: OpenModeTab}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}
	if got, want := manager.BackendName(), "Ghostty"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}

	for _, call := range runner.calls {
		if reflect.DeepEqual(call, []string{"output", "which", "alacritty"}) {
			t.Fatalf("auto tab mode should skip Alacritty check, calls = %#v", runner.calls)
		}
	}
}

func TestUnavailableBackendsReturnSupportedList(t *testing.T) {
	runner := &recordingRunner{failAllOutputs: true}
	manager := Manager{Runner: runner, OpenMode: OpenModeWindow}

	err := manager.CheckAvailable()
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	for _, want := range []string{"iTerm2", "Terminal.app", "WezTerm", "kitty", "Alacritty", "Ghostty"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error %q does not contain %q", got, want)
		}
	}
}

type recordingRunner struct {
	calls          [][]string
	outputs        map[string][]byte
	failOutputs    map[string]bool
	failAllOutputs bool
}

func (r *recordingRunner) Run(name string, args ...string) error {
	call := append([]string{"run", name}, args...)
	r.calls = append(r.calls, call)
	return nil
}

func (r *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	call := append([]string{"output", name}, args...)
	r.calls = append(r.calls, call)
	key := strings.Join(append([]string{name}, args...), "|")
	if r.failAllOutputs || r.failOutputs[key] {
		return nil, errRecordingOutput
	}
	if r.outputs != nil {
		if output, ok := r.outputs[key]; ok {
			return output, nil
		}
	}
	return []byte("1\n"), nil
}

var errRecordingOutput = &recordingError{}

type recordingError struct{}

func (*recordingError) Error() string { return "not available" }
