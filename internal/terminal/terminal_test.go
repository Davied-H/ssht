package terminal

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAutoDetectSelectsFirstAvailableBackend(t *testing.T) {
	runner := &recordingRunner{failOutputs: map[string]bool{
		`osascript|-e|id of application "iTerm"`: true,
	}}
	manager := Manager{Runner: runner, Env: mapEnv()}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}
	if got, want := manager.BackendName(), "Terminal.app"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
}

func TestExecRunnerWritesAndRemovesPrivateTempScript(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	runner := ExecRunner{}

	path, err := runner.WriteTempScript("#!/bin/sh\nexit 0\n")
	if err != nil {
		t.Fatalf("WriteTempScript returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat temp script: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("permissions = %o, want %o", got, want)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp script: %v", err)
	}
	if got, want := string(contents), "#!/bin/sh\nexit 0\n"; got != want {
		t.Fatalf("contents = %q, want %q", got, want)
	}
	if err := runner.Remove(path); err != nil {
		t.Fatalf("remove temp script: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp script still exists or stat returned unexpected error: %v", err)
	}
}

func TestAutoDetectPrefersCurrentTerminalApp(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Env: mapEnv("TERM_PROGRAM", "Apple_Terminal")}

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

func TestAutoDetectPrefersCurrentWezTerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Env: mapEnv("TERM_PROGRAM", "WezTerm")}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}

	want := [][]string{
		{"output", "which", "wezterm"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.BackendName(), "WezTerm"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
}

func TestAutoDetectPrefersCurrentWarp(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Env: mapEnv("TERM_PROGRAM", "WarpTerminal")}

	if err := manager.CheckAvailable(); err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Warp"`},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.BackendName(), "Warp"; got != want {
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
			"-e", `do script "env -u LC_ALL ssh 'prod-api'"`,
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
			"-e", `tell current session of current window to write text "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHExplicitITermWindowDoesNotSplitInsideITerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{
		Runner:     runner,
		Preference: "iterm",
		OpenMode:   OpenModeWindow,
		Env:        mapEnv("ITERM_SESSION_ID", "w0t0p0"),
	}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	got := strings.Join(runner.calls[1], "\n")
	if strings.Contains(got, "split vertically") {
		t.Fatalf("explicit window should not split:\n%s", got)
	}
	if !strings.Contains(got, "create window with default profile") {
		t.Fatalf("explicit window did not create a window:\n%s", got)
	}
	if got, want := manager.OpenModeName(), "window"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
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
			"-e", `tell current session of current window to write text "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestOpenSSHUsesITermSplitForAutoModeInsideITerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{
		Runner:     runner,
		Preference: "iterm",
		OpenMode:   OpenModeAuto,
		Env:        mapEnv("TERM_PROGRAM", "iTerm.app"),
	}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "iTerm"`},
		{"output", "osascript", "-e", `tell application "iTerm" to count windows`},
		{"run", "osascript",
			"-e", `tell application "iTerm"`,
			"-e", "activate",
			"-e", `tell current session of current window`,
			"-e", `set newSession to (split vertically with default profile)`,
			"-e", `tell newSession to write text "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell",
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.OpenModeName(), "split"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestValidOpenModeAcceptsSplit(t *testing.T) {
	if !ValidOpenMode(OpenModeSplit) {
		t.Fatal("split should be a valid open mode")
	}
}

func TestOpenSSHUsesExplicitITermSplitInsideITerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{
		Runner:     runner,
		Preference: "iterm",
		OpenMode:   OpenModeSplit,
		Env:        mapEnv("ITERM_SESSION_ID", "w0t0p0"),
	}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "iTerm"`},
		{"output", "osascript", "-e", `tell application "iTerm" to count windows`},
		{"run", "osascript",
			"-e", `tell application "iTerm"`,
			"-e", "activate",
			"-e", `tell current session of current window`,
			"-e", `set newSession to (split vertically with default profile)`,
			"-e", `tell newSession to write text "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell",
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.OpenModeName(), "split"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestOpenSSHUsesITermTabForAutoModeOutsideITerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{
		Runner:     runner,
		Preference: "iterm",
		OpenMode:   OpenModeAuto,
		Env:        mapEnv("TERM_PROGRAM", "Apple_Terminal"),
	}

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
			"-e", `tell current session of current window to write text "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.OpenModeName(), "tab"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestOpenSSHExplicitITermTabDoesNotSplitInsideITerm(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{
		Runner:     runner,
		Preference: "iterm",
		OpenMode:   OpenModeTab,
		Env:        mapEnv("ITERM_SESSION_ID", "w0t0p0"),
	}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	got := strings.Join(runner.calls[2], "\n")
	if strings.Contains(got, "split vertically") {
		t.Fatalf("explicit tab should not split:\n%s", got)
	}
	if !strings.Contains(got, "create tab with default profile") {
		t.Fatalf("explicit tab did not create a tab:\n%s", got)
	}
	if got, want := manager.OpenModeName(), "tab"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
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
			"-e", `tell current session of current window to write text "env -u LC_ALL ssh 'prod-api'"`,
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
			"-e", `do script "env -u LC_ALL ssh 'prod-api'"`,
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestTerminalAppSplitReturnsUnsupportedError(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "terminal", OpenMode: OpenModeSplit}

	err := manager.Connect("prod-api")
	if err == nil {
		t.Fatal("expected unsupported split error")
	}
	if !strings.Contains(err.Error(), "Terminal.app does not support split mode") {
		t.Fatalf("error = %q, want unsupported split mode", err)
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
			"-e", `do script "env -u LC_ALL sshpass -p 'p@ss \"word\"' ssh 'prod-api'"`,
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
			"-e", `do script "env -u LC_ALL ssh 'prod-api'" in front window`,
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
			"-e", `do script "env -u LC_ALL ssh 'prod-api'"`,
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
		{"run", "wezterm", "start", "--new-window", "--", "env", "-u", "LC_ALL", "ssh", `prod"api'one`},
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
		{"run", "wezterm", "start", "--new-tab", "--", "env", "-u", "LC_ALL", "ssh", "prod-api"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestWarpTabRunsSelfRemovingCommandScript(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "warp", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Warp"`},
		{"run", "open", "-a", "Warp", "/tmp/ssht-warp"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := runner.scriptContents, []string{"#!/bin/sh\nrm -f -- \"$0\"\nexec env -u LC_ALL ssh 'prod-api'\n"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("script contents = %#v, want %#v", got, want)
	}
	if got, want := manager.OpenModeName(), "tab"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestWarpWindowStartsNewAppInstance(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "warp", OpenMode: OpenModeWindow}

	if err := manager.Connect("prod-api"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Warp"`},
		{"run", "open", "-n", "-a", "Warp", "/tmp/ssht-warp"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := manager.OpenModeName(), "window"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestWarpSplitCreatesPaneAndRunsPrivateScript(t *testing.T) {
	runner := &recordingRunner{}
	manager := Manager{Runner: runner, Preference: "warp", OpenMode: OpenModeSplit}

	if err := manager.Connect(Target{Alias: "prod-api", SSHPassword: "example-password"}); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := [][]string{
		{"output", "osascript", "-e", `id of application "Warp"`},
		{"run", "osascript",
			"-e", `tell application "Warp" to activate`,
			"-e", "delay 0.2",
			"-e", `tell application "System Events"`,
			"-e", `tell process "Warp"`,
			"-e", "set frontmost to true",
			"-e", `keystroke "d" using command down`,
			"-e", "delay 0.5",
			"-e", `keystroke "'/tmp/ssht-warp'"`,
			"-e", "delay 0.3",
			"-e", "key code 36",
			"-e", "end tell",
			"-e", "end tell"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if got, want := runner.scriptContents, []string{"#!/bin/sh\nrm -f -- \"$0\"\nexec env -u LC_ALL sshpass -p 'example-password' ssh 'prod-api'\n"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("script contents = %#v, want %#v", got, want)
	}
	if got, want := manager.OpenModeName(), "split"; got != want {
		t.Fatalf("open mode name = %q, want %q", got, want)
	}
}

func TestWarpRemovesScriptWhenOpenFails(t *testing.T) {
	runner := &recordingRunner{runErr: errRecordingRun}
	manager := Manager{Runner: runner, Preference: "warp", OpenMode: OpenModeTab}

	if err := manager.Connect("prod-api"); err == nil {
		t.Fatal("expected Connect to return the open error")
	}
	if got, want := runner.removedPaths, []string{"/tmp/ssht-warp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed paths = %#v, want %#v", got, want)
	}
}

func TestWarpRemovesScriptWhenSplitAutomationFails(t *testing.T) {
	runner := &recordingRunner{runErr: errRecordingRun}
	manager := Manager{Runner: runner, Preference: "warp", OpenMode: OpenModeSplit}

	err := manager.Connect("prod-api")
	if err == nil {
		t.Fatal("expected Connect to return the accessibility error")
	}
	if !strings.Contains(err.Error(), "Accessibility") {
		t.Fatalf("error = %q, want accessibility guidance", err)
	}
	if got, want := runner.removedPaths, []string{"/tmp/ssht-warp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed paths = %#v, want %#v", got, want)
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
		{"run", "kitty", "@", "launch", "--type=tab", "env", "-u", "LC_ALL", "ssh", "prod-api"},
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
			"-e", `set command of cfg to "env -u LC_ALL ssh 'prod-api'"`,
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
			"-e", `set command of cfg to "env -u LC_ALL ssh 'prod-api'"`,
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
	for _, want := range []string{"iTerm2", "Terminal.app", "WezTerm", "kitty", "Alacritty", "Ghostty", "Warp"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error %q does not contain %q", got, want)
		}
	}
}

func mapEnv(values ...string) EnvLookup {
	env := map[string]string{}
	for i := 0; i+1 < len(values); i += 2 {
		env[values[i]] = values[i+1]
	}
	return func(name string) string {
		return env[name]
	}
}

type recordingRunner struct {
	calls          [][]string
	outputs        map[string][]byte
	failOutputs    map[string]bool
	failAllOutputs bool
	scriptContents []string
	removedPaths   []string
	runErr         error
}

func (r *recordingRunner) Run(name string, args ...string) error {
	call := append([]string{"run", name}, args...)
	r.calls = append(r.calls, call)
	return r.runErr
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

func (r *recordingRunner) WriteTempScript(content string) (string, error) {
	r.scriptContents = append(r.scriptContents, content)
	return "/tmp/ssht-warp", nil
}

func (r *recordingRunner) Remove(path string) error {
	r.removedPaths = append(r.removedPaths, path)
	return nil
}

var errRecordingOutput = &recordingError{}
var errRecordingRun = &recordingError{}

type recordingError struct{}

func (*recordingError) Error() string { return "not available" }
