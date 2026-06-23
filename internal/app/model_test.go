package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dong/ssht/internal/monitor"
	"github.com/dong/ssht/internal/sshconfig"
	"github.com/dong/ssht/internal/state"
	"github.com/dong/ssht/internal/terminal"
)

func typeSearch(model Model, query string) Model {
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if query != "" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(query)})
	}
	return model
}

func TestModelFiltersAndReloadsHosts(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "prod-api", HostName: "192.0.2.12"},
		{Alias: "prod-db", HostName: "192.0.2.13"},
		{Alias: "staging-api", HostName: "staging.example.com"},
	}
	model := NewModel(Config{Entries: entries})

	model = typeSearch(model, "pr")

	if got := model.Filter(); got != "pr" {
		t.Fatalf("filter = %q, want pr", got)
	}
	if got := len(model.FilteredEntries()); got != 2 {
		t.Fatalf("filtered count = %d, want 2", got)
	}

	model, _ = model.update(ReloadedMsg{Entries: []sshconfig.HostEntry{{Alias: "new-host"}}})
	if got := model.FilteredEntries()[0].Alias; got != "new-host" {
		t.Fatalf("reload did not replace entries, got %q", got)
	}
}

func TestModelStructuredSearchAndNegation(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod", HostName: "192.0.2.12", User: "deploy", Port: "22", ProxyJump: "bastion", SourceFile: "/tmp/work"},
		{Alias: "prod-db", Group: "prod", HostName: "192.0.2.13", User: "root", Port: "2200", SourceFile: "/tmp/work"},
		{Alias: "dev-api", Group: "dev", HostName: "dev.example.com", User: "deploy", Port: "22", SourceFile: "/tmp/dev"},
	}})

	model = typeSearch(model, "user:deploy group:prod -db jump:bastion")
	filtered := model.FilteredEntries()
	if len(filtered) != 1 || filtered[0].Alias != "prod-api" {
		t.Fatalf("filtered = %#v, want prod-api", filtered)
	}
}

func TestSearchRequiresEveryTextTerm(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api", HostName: "api.example.com"},
		{Alias: "prod-db", HostName: "db.example.com"},
	}})

	model = typeSearch(model, "prod missing")

	if got := len(model.FilteredEntries()); got != 0 {
		t.Fatalf("filtered count = %d, want 0", got)
	}
}

func TestSearchContextExplainsStructuredMatch(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api", User: "deploy", HostName: "api.example.com"},
	}})

	if got := model.searchContext(model.entries[0], "host:api"); got != "host api.example.com" {
		t.Fatalf("search context = %q, want host api.example.com", got)
	}
}

func TestCommandPaletteOpensHistory(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{{Alias: "prod-api"}}})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if cmd != nil || model.mode != modeCommandPalette {
		t.Fatalf("mode=%v cmd=%v, want command palette", model.mode, cmd)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("history")})
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || model.mode != modeHistory {
		t.Fatalf("mode=%v cmd=%v, want history", model.mode, cmd)
	}
}

func TestRiskyConnectOpensWithoutConfirmation(t *testing.T) {
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "prod-api", Group: "prod", User: "root"}},
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
			OpenMode:   terminal.OpenModeWindow,
		},
	})

	updated, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("risky connect should return connect command")
	}
	if model.mode != modeBrowse || model.pending.entry.Alias != "" || len(model.pending.movingHosts) != 0 {
		t.Fatalf("mode=%v pending=%#v, want browse with no pending connect", model.mode, model.pending)
	}
	model, _ = model.update(cmd())
	if runner.runCalls != 1 {
		t.Fatalf("run calls = %d, want 1", runner.runCalls)
	}
}

func TestSearchModeKeepsActionKeysAsFilterText(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "test-host"},
		{Alias: "prod-api"},
	}})

	model = typeSearch(model, "test")
	if model.mode != modeBrowse || model.Filter() != "test" {
		t.Fatalf("after test mode=%v filter=%q, want browse/test", model.mode, model.Filter())
	}

	model = NewModel(Config{Entries: []sshconfig.HostEntry{{Alias: "prod-api"}}})
	model = typeSearch(model, "prod")
	if model.mode != modeBrowse || model.Filter() != "prod" {
		t.Fatalf("after prod mode=%v filter=%q, want browse/prod", model.mode, model.Filter())
	}
}

func TestPlainTypingDoesNotEnterSearchMode(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api"},
		{Alias: "dev-db"},
	}})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prod")})
	if model.Filter() != "" {
		t.Fatalf("plain typing should not filter, got %q", model.Filter())
	}
	if len(model.FilteredEntries()) != 2 {
		t.Fatalf("plain typing should keep all entries, got %#v", model.FilteredEntries())
	}
}

func TestMouseClickFilterBarStartsSearch(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{{Alias: "prod-api"}}})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 18})

	model, _ = model.update(tea.MouseMsg(tea.MouseEvent{
		X:      10,
		Y:      1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}))

	if !model.searchActive {
		t.Fatal("clicking filter bar should activate search")
	}
}

func TestMouseClickHostRowSelectsHost(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod"},
		{Alias: "dev-db", Group: "dev"},
	}})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 24})
	sidebarW, _, _ := model.splitThreeColumns()

	model, _ = model.update(tea.MouseMsg(tea.MouseEvent{
		X:      sidebarW + 4,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}))

	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want clicked second row", model.cursor)
	}
	if model.focus != focusList {
		t.Fatalf("focus = %d, want focusList", model.focus)
	}
}

func TestMouseClickGroupSelectsGroup(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod"},
		{Alias: "dev-db", Group: "dev"},
	}})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 24})

	model, _ = model.update(tea.MouseMsg(tea.MouseEvent{
		X:      3,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}))

	if got := model.selectedGroup(); got != "prod" {
		t.Fatalf("clicked group = %q, want prod", got)
	}
	if model.focus != focusSidebar {
		t.Fatalf("focus = %d, want focusSidebar", model.focus)
	}
}

func TestModelFormAllowsSAsTextAndCtrlSReviewsSave(t *testing.T) {
	model := NewModel(Config{})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if cmd != nil || model.mode != modeForm {
		t.Fatalf("expected add form, mode=%v cmd=%v", model.mode, cmd)
	}

	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd != nil {
		t.Fatalf("plain s should edit text, got cmd=%v", cmd)
	}
	if model.mode != modeForm {
		t.Fatalf("plain s should stay in form mode, got %v", model.mode)
	}
	if got := model.form.values.Alias; got != "s" {
		t.Fatalf("alias = %q, want s", got)
	}

	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		t.Fatalf("ctrl+s should prepare confirmation without async command, got %v", cmd)
	}
	if model.mode != modeConfirm {
		t.Fatalf("ctrl+s mode = %v, want confirm", model.mode)
	}
	if got := model.pending.values.Alias; got != "s" {
		t.Fatalf("pending alias = %q, want s", got)
	}
}

func TestConnectOpensSelectedHostInTerminalCommand(t *testing.T) {
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "prod-api"}},
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
			OpenMode:   terminal.OpenModeWindow,
		},
	})

	updated, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected connect command")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected connect message from command")
	}
	updated, _ = model.update(msg)
	model = updated
	if runner.runCalls != 1 {
		t.Fatalf("run calls = %d, want 1", runner.runCalls)
	}
	if !strings.Contains(model.status, "Terminal.app") {
		t.Fatalf("status = %q, want backend name", model.status)
	}
	if !strings.Contains(model.status, "window") {
		t.Fatalf("status = %q, want open mode", model.status)
	}
}

func TestConnectPassesSSHPasswordToTerminalManager(t *testing.T) {
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "password-host", SSHPassword: "secret"}},
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
		},
	})

	updated, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected connect command")
	}
	model, _ = model.update(cmd())

	if runner.lastRunJoined == "" {
		t.Fatal("expected terminal command to run")
	}
	if !strings.Contains(runner.lastRunJoined, "env -u LC_ALL sshpass -p 'secret' ssh 'password-host'") {
		t.Fatalf("terminal command = %q, want sshpass command", runner.lastRunJoined)
	}
}

func TestConnectStatusIncludesTabOpenMode(t *testing.T) {
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "prod-api"}},
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
			OpenMode:   terminal.OpenModeTab,
		},
	})

	updated, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected connect command")
	}
	updated, _ = model.update(cmd())
	model = updated

	if !strings.Contains(model.status, "Terminal.app tab") {
		t.Fatalf("status = %q, want backend and tab open mode", model.status)
	}
}

func TestDashboardCountsHostsMatchesAndWarnings(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "prod-api", HostName: "192.0.2.12"},
		{Alias: "prod-db", HostName: "192.0.2.13"},
		{Alias: "staging-api", HostName: "staging.example.com"},
	}
	model := NewModel(Config{
		Entries:  entries,
		Warnings: []sshconfig.Warning{{Path: "config", Message: "warning"}},
	})

	model = typeSearch(model, "prod")

	counts := model.dashboardCounts()
	if counts.Hosts != 3 || counts.Matched != 2 || counts.Warnings != 1 || counts.Favorites != 0 || counts.Recent != 0 || counts.Selected != 0 {
		t.Fatalf("counts = %#v, want hosts=3 matched=2 favorites=0 recent=0 selected=0 warnings=1", counts)
	}
	view := model.View()
	if !strings.Contains(view, "Hosts 3") || !strings.Contains(view, "Matched 2") || !strings.Contains(view, "⚠ 1") {
		t.Fatalf("status line missing host/match/warning counts:\n%s", view)
	}
}

func TestModelTogglesFavoriteAndSavesState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	model := NewModel(Config{
		Entries:   []sshconfig.HostEntry{{Alias: "prod-api"}},
		StatePath: statePath,
		State:     state.NewStore(),
	})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd == nil {
		t.Fatal("expected state save command")
	}
	model, _ = model.update(cmd())

	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !loaded.Hosts["prod-api"].Favorite {
		t.Fatalf("state = %#v, want prod-api favorite", loaded)
	}
	if !strings.Contains(model.View(), "★") {
		t.Fatalf("favorite marker missing from view:\n%s", model.View())
	}
}

func TestModelFiltersByFavoriteAndRecentQueries(t *testing.T) {
	store := state.NewStore()
	store.Hosts["prod-api"] = state.HostState{Favorite: true, LastConnectedAt: "2026-05-01T10:30:00Z"}
	store.Hosts["prod-db"] = state.HostState{LastConnectedAt: "2026-05-01T10:00:00Z"}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "prod-api", Tags: []string{"api"}},
			{Alias: "prod-db", Tags: []string{"db"}},
			{Alias: "dev-api", Tags: []string{"api"}},
		},
		State: store,
	})

	model = typeSearch(model, "fav: tag:api")
	filtered := model.FilteredEntries()
	if len(filtered) != 1 || filtered[0].Alias != "prod-api" {
		t.Fatalf("favorite filtered = %#v, want prod-api", filtered)
	}

	model.filter = ""
	model.searchActive = false
	model.applyFilter()
	model = typeSearch(model, "recent: prod")
	filtered = model.FilteredEntries()
	if got := aliases(filtered); strings.Join(got, ",") != "prod-api,prod-db" {
		t.Fatalf("recent filtered aliases = %v, want prod-api,prod-db", got)
	}
}

func TestModelSortsByFavoriteRecentCountAndAlias(t *testing.T) {
	store := state.NewStore()
	store.Hosts["mid"] = state.HostState{ConnectCount: 8}
	store.Hosts["z-recent"] = state.HostState{ConnectCount: 1, LastConnectedAt: "2026-05-01T10:00:00Z"}
	store.Hosts["a-favorite"] = state.HostState{Favorite: true}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "z-plain"},
			{Alias: "mid"},
			{Alias: "z-recent"},
			{Alias: "a-favorite"},
			{Alias: "a-plain"},
		},
		State: store,
	})

	if got := aliases(model.FilteredEntries()); strings.Join(got, ",") != "a-favorite,z-recent,mid,a-plain,z-plain" {
		t.Fatalf("aliases = %v, want favorite, recent, count, alias order", got)
	}
}

func TestModelConnectRecordsSuccessfulAliasOnly(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries:   []sshconfig.HostEntry{{Alias: "prod-api"}},
		StatePath: statePath,
		State:     state.NewStore(),
		Now:       func() time.Time { return time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC) },
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
		},
	})

	updated, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected connect command")
	}
	var saveCmd tea.Cmd
	model, saveCmd = model.update(cmd())
	if saveCmd == nil {
		t.Fatal("expected state save command after successful connect")
	}
	model, _ = model.update(saveCmd())

	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := loaded.Hosts["prod-api"].ConnectCount; got != 1 {
		t.Fatalf("connect count = %d, want 1", got)
	}

	runner.runErr = errors.New("connect failed")
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	model, saveCmd = model.update(cmd())
	if saveCmd != nil {
		t.Fatal("failed single connect should not save state")
	}
	loaded, err = state.Load(statePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := loaded.Hosts["prod-api"].ConnectCount; got != 1 {
		t.Fatalf("connect count after failure = %d, want 1", got)
	}
}

func TestModelBatchConnectsSelectedHostsAndRecordsPartialSuccess(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := &countingRunner{failRunCall: 2}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "prod-a"},
			{Alias: "prod-b"},
		},
		StatePath: statePath,
		State:     state.NewStore(),
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
		},
	})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd != nil {
		t.Fatal("space should not create command")
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeySpace})
	if model.dashboardCounts().Selected != 2 {
		t.Fatalf("selected count = %d, want 2", model.dashboardCounts().Selected)
	}

	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected batch connect command")
	}
	var saveCmd tea.Cmd
	model, saveCmd = model.update(cmd())
	if saveCmd == nil {
		t.Fatal("expected state save command after partial success")
	}
	model, _ = model.update(saveCmd())

	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Hosts["prod-a"].ConnectCount != 1 {
		t.Fatalf("prod-a state = %#v, want recorded success", loaded.Hosts["prod-a"])
	}
	if loaded.Hosts["prod-b"].ConnectCount != 0 {
		t.Fatalf("prod-b state = %#v, want no failed connect record", loaded.Hosts["prod-b"])
	}
	if !strings.Contains(model.status, "opened 1 connection; 1 failed") {
		t.Fatalf("status = %q, want partial failure summary", model.status)
	}
}

func TestModelFiltersBySelectedGroupAndTagQuery(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod", Tags: []string{"api", "critical"}},
		{Alias: "prod-db", Group: "prod", Tags: []string{"db"}},
		{Alias: "dev-api", Group: "dev", Tags: []string{"api"}},
		{Alias: "personal", Tags: []string{"lab"}},
	}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 24})

	groups := model.groupItems()
	wantGroups := []string{"all", "dev", "prod", "ungrouped"}
	if len(groups) != len(wantGroups) {
		t.Fatalf("groups = %#v, want %v", groups, wantGroups)
	}
	for i, want := range wantGroups {
		if groups[i].Name != want {
			t.Fatalf("group %d = %q, want %q", i, groups[i].Name, want)
		}
	}

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRight})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRight})
	if got := model.selectedGroup(); got != "prod" {
		t.Fatalf("selected group = %q, want prod", got)
	}
	model = typeSearch(model, "tag:api")

	filtered := model.FilteredEntries()
	if len(filtered) != 1 || filtered[0].Alias != "prod-api" {
		t.Fatalf("filtered = %#v, want only prod-api", filtered)
	}

	view := model.View()
	if !strings.Contains(view, "prod") || !strings.Contains(view, "dev") || !strings.Contains(view, "ungrouped") {
		t.Fatalf("group names missing from view:\n%s", view)
	}
}

func TestGroupItemsRespectsGroupOrder(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "dev"},
		{Alias: "b", Group: "prod"},
		{Alias: "c", Group: "staging"},
	}
	store := state.NewStore()
	store.GroupOrder = []string{"prod", "dev"}
	model := NewModel(Config{Entries: entries, State: store})

	groups := model.groupItems()
	want := []string{"all", "prod", "dev", "staging"}
	if len(groups) != len(want) {
		t.Fatalf("groups = %#v, want %v", groups, want)
	}
	for i, name := range want {
		if groups[i].Name != name {
			t.Fatalf("group %d = %q, want %q", i, groups[i].Name, name)
		}
	}
}

func TestGroupItemsIncludesEmptyGroupsWithZeroCount(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod"},
	}
	store := state.NewStore()
	store.EmptyGroups = []string{"future-east", "lab"}
	model := NewModel(Config{Entries: entries, State: store})

	groups := model.groupItems()
	names := make([]string, len(groups))
	counts := make(map[string]int, len(groups))
	for i, g := range groups {
		names[i] = g.Name
		counts[g.Name] = g.Count
	}
	want := []string{"all", "future-east", "lab", "prod"}
	if !equalStringSliceApp(names, want) {
		t.Fatalf("groups = %v, want %v", names, want)
	}
	if counts["future-east"] != 0 || counts["lab"] != 0 {
		t.Fatalf("empty group counts = %v, want 0", counts)
	}
	if counts["prod"] != 1 {
		t.Fatalf("prod count = %d, want 1", counts["prod"])
	}
}

func TestGroupItemsIgnoresReservedNamesInOrderAndEmpty(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "dev"}}
	store := state.NewStore()
	store.GroupOrder = []string{"all", "ungrouped", "dev"}
	store.EmptyGroups = []string{"all", "ungrouped", "lab"}
	model := NewModel(Config{Entries: entries, State: store})

	groups := model.groupItems()
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	want := []string{"all", "dev", "lab"}
	if !equalStringSliceApp(names, want) {
		t.Fatalf("groups = %v, want %v", names, want)
	}
}

func TestGroupItemsDropsStaleOrderEntries(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "dev"}}
	store := state.NewStore()
	store.GroupOrder = []string{"deleted-group", "dev"}
	model := NewModel(Config{Entries: entries, State: store})

	groups := model.groupItems()
	for _, g := range groups {
		if g.Name == "deleted-group" {
			t.Fatalf("stale order entry leaked: %v", groups)
		}
	}
}

func equalStringSliceApp(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTabSwitchesFocusToSidebar(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "a", Group: "prod"}},
	})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	if model.focus != focusSidebar {
		t.Fatalf("focus = %d, want focusSidebar", model.focus)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	if model.focus != focusList {
		t.Fatalf("after second Tab focus = %d, want focusList", model.focus)
	}
}

func TestSidebarFocusJKMovesGroupCursor(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "alpha"},
			{Alias: "b", Group: "beta"},
		},
	})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.selectedGroup(); got != "all" {
		t.Fatalf("initial selection = %q, want all", got)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := model.selectedGroup(); got != "alpha" {
		t.Fatalf("after j selection = %q, want alpha", got)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := model.selectedGroup(); got != "beta" {
		t.Fatalf("after second j selection = %q, want beta", got)
	}
}

func TestCreateEmptyGroupPersistsToState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	model := NewModel(Config{
		Entries:   []sshconfig.HostEntry{{Alias: "a", Group: "prod"}},
		StatePath: statePath,
		State:     state.NewStore(),
	})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if model.mode != modeGroupInline {
		t.Fatalf("mode = %d, want modeGroupInline", model.mode)
	}
	for _, r := range "lab" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	cmd := func() tea.Msg { return nil }
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.mode != modeBrowse {
		t.Fatalf("mode after commit = %d, want modeBrowse", model.mode)
	}
	if !isEmptyGroup(model.state.EmptyGroups, "lab") {
		t.Fatalf("EmptyGroups = %v, want to contain 'lab'", model.state.EmptyGroups)
	}
	// drain the saveStateCmd to actually persist
	if cmd != nil {
		msg := cmd()
		if msg != nil {
			model, _ = model.update(msg)
		}
	}
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !isEmptyGroup(loaded.EmptyGroups, "lab") {
		t.Fatalf("persisted EmptyGroups = %v", loaded.EmptyGroups)
	}
}

func TestCreateEmptyGroupRejectsReservedName(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "a", Group: "prod"}},
	})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	for _, r := range "all" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.mode != modeGroupInline {
		t.Fatalf("mode = %d, want still modeGroupInline (rejected)", model.mode)
	}
	if model.status == "" || !strings.Contains(strings.ToLower(model.status), "reserved") {
		t.Fatalf("status = %q, want reserved-name complaint", model.status)
	}
}

func TestRenameEmptyGroupKeepsItStateOnly(t *testing.T) {
	store := state.NewStore()
	store.EmptyGroups = []string{"lab"}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "a", Group: "prod"}},
		State:   store,
	})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	// Move to "lab" in sidebar (sorted: all, lab, prod).
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := model.selectedGroup(); got != "lab" {
		t.Fatalf("expected to land on lab, got %q", got)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	// buffer pre-fills with "lab"; clear it and type "future-east"
	for i := 0; i < 3; i++ {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "future-east" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.mode != modeBrowse {
		t.Fatalf("mode after rename = %d, want modeBrowse", model.mode)
	}
	if isEmptyGroup(model.state.EmptyGroups, "lab") {
		t.Fatalf("old name still present: %v", model.state.EmptyGroups)
	}
	if !isEmptyGroup(model.state.EmptyGroups, "future-east") {
		t.Fatalf("new name missing: %v", model.state.EmptyGroups)
	}
}

func TestMovingMarkedHostsBuildsConfirmPanel(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
			{Alias: "b", Group: "prod", SourceFile: "/tmp/x", SourceLine: 5},
			{Alias: "c", Group: "dev", SourceFile: "/tmp/x", SourceLine: 9},
		},
	})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model.selected = map[string]bool{"a": true, "b": true}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	// move sidebar to "dev" (sorted: all, dev, prod)
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := model.selectedGroup(); got != "dev" {
		t.Fatalf("sidebar at %q, want dev", got)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if model.mode != modeConfirm {
		t.Fatalf("mode = %d, want modeConfirm", model.mode)
	}
	if model.pending.operation != operationGroupMoveHosts {
		t.Fatalf("operation = %d, want operationGroupMoveHosts", model.pending.operation)
	}
	if model.pending.groupTo != "dev" {
		t.Fatalf("groupTo = %q, want dev", model.pending.groupTo)
	}
	if len(model.pending.movingHosts) != 2 {
		t.Fatalf("movingHosts count = %d, want 2", len(model.pending.movingHosts))
	}
}

func TestGroupPickerMovesCurrentHostWhenNoneMarked(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
			{Alias: "b", Group: "prod", SourceFile: "/tmp/x", SourceLine: 5},
			{Alias: "c", Group: "dev", SourceFile: "/tmp/x", SourceLine: 9},
		},
	})
	model.cursor = 1

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if model.mode != modeGroupPicker {
		t.Fatalf("mode = %d, want modeGroupPicker", model.mode)
	}
	if len(model.groupPicker.movingHosts) != 1 || model.groupPicker.movingHosts[0].Alias != "b" {
		t.Fatalf("movingHosts = %#v, want current host b", model.groupPicker.movingHosts)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.mode != modeConfirm {
		t.Fatalf("mode = %d, want modeConfirm", model.mode)
	}
	if model.pending.operation != operationGroupMoveHosts {
		t.Fatalf("operation = %d, want operationGroupMoveHosts", model.pending.operation)
	}
	if model.pending.groupTo != "dev" {
		t.Fatalf("groupTo = %q, want first existing group dev", model.pending.groupTo)
	}
	if len(model.pending.movingHosts) != 1 || model.pending.movingHosts[0].Alias != "b" {
		t.Fatalf("pending movingHosts = %#v, want host b", model.pending.movingHosts)
	}
}

func TestGroupPickerMovesMarkedHostsToSelectedGroup(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
			{Alias: "b", Group: "prod", SourceFile: "/tmp/x", SourceLine: 5},
			{Alias: "c", Group: "dev", SourceFile: "/tmp/x", SourceLine: 9},
		},
	})
	model.selected = map[string]bool{"a": true, "b": true}

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.mode != modeConfirm {
		t.Fatalf("mode = %d, want modeConfirm", model.mode)
	}
	if model.pending.groupTo != "prod" {
		t.Fatalf("groupTo = %q, want prod", model.pending.groupTo)
	}
	if len(model.pending.movingHosts) != 2 {
		t.Fatalf("movingHosts count = %d, want 2", len(model.pending.movingHosts))
	}
}

func TestGroupPickerMovesToNewGroupName(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
		},
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	for _, r := range "lab" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.mode != modeConfirm {
		t.Fatalf("mode = %d, want modeConfirm", model.mode)
	}
	if model.pending.groupTo != "lab" {
		t.Fatalf("groupTo = %q, want lab", model.pending.groupTo)
	}
	if len(model.state.EmptyGroups) != 0 {
		t.Fatalf("EmptyGroups = %v, want unchanged", model.state.EmptyGroups)
	}
}

func TestGroupPickerRejectsInvalidGroupName(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
		},
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	for _, r := range "bad group" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.mode != modeGroupPicker {
		t.Fatalf("mode = %d, want modeGroupPicker", model.mode)
	}
	if model.pending.operation == operationGroupMoveHosts {
		t.Fatalf("pending operation should not be move on invalid input")
	}
	if model.status == "" {
		t.Fatalf("status empty, want validation error")
	}
}

func TestGroupPickerEscCancelsWithoutPending(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
		},
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEsc})

	if model.mode != modeBrowse {
		t.Fatalf("mode = %d, want modeBrowse", model.mode)
	}
	if model.pending.operation != operationAdd {
		t.Fatalf("pending changed: %#v", model.pending)
	}
	if len(model.groupPicker.movingHosts) != 0 {
		t.Fatalf("groupPicker still populated: %#v", model.groupPicker)
	}
}

func TestGroupPickerMoveToEmptyGroupDropsPlaceholderAfterWrite(t *testing.T) {
	store := state.NewStore()
	store.EmptyGroups = []string{"lab"}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "prod", SourceFile: "/tmp/x", SourceLine: 1},
		},
		State: store,
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	for _, r := range "lab" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.pending.groupTo != "lab" {
		t.Fatalf("groupTo = %q, want lab", model.pending.groupTo)
	}
	model, _ = model.update(writeConfigMsg{op: operationGroupMoveHosts, groupTo: "lab"})
	if isEmptyGroup(model.state.EmptyGroups, "lab") {
		t.Fatalf("EmptyGroups still contains lab: %v", model.state.EmptyGroups)
	}
}

func TestShiftGroupOrderInjectsAndSwaps(t *testing.T) {
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "a", Group: "alpha"},
			{Alias: "b", Group: "beta"},
			{Alias: "c", Group: "gamma"},
		},
	})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	// move to gamma (all, alpha, beta, gamma → 3 j presses)
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := model.selectedGroup(); got != "gamma" {
		t.Fatalf("sidebar at %q, want gamma", got)
	}
	// K shifts up. With empty GroupOrder, gamma is injected then moved up.
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	if len(model.state.GroupOrder) == 0 {
		t.Fatalf("GroupOrder still empty after shift")
	}
	if model.state.GroupOrder[len(model.state.GroupOrder)-1] == "gamma" {
		t.Fatalf("gamma still at end: %v", model.state.GroupOrder)
	}
}

func TestFormGroupFieldTabCyclesKnownGroups(t *testing.T) {
	model := NewModel(Config{
		Entries:    []sshconfig.HostEntry{{Alias: "a", Group: "alpha"}, {Alias: "b", Group: "beta"}},
		ConfigPath: "/tmp/cfg",
	})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if model.mode != modeForm {
		t.Fatalf("mode = %d, want modeForm", model.mode)
	}
	// Move down once to land on Group field (Alias, Group, ...)
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyDown})
	if model.form.field != formFieldGroupIndex() {
		t.Fatalf("form.field = %d, want %d", model.form.field, formFieldGroupIndex())
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	if model.form.values.Group == "" {
		t.Fatalf("Group empty after Tab cycle")
	}
	if model.form.values.Group != "alpha" {
		t.Fatalf("first Tab landed on %q, want alpha (sorted)", model.form.values.Group)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	if model.form.values.Group != "beta" {
		t.Fatalf("second Tab landed on %q, want beta", model.form.values.Group)
	}
}

func TestBrowseViewFitsWindowHeight(t *testing.T) {
	entries := []sshconfig.HostEntry{}
	for i := 0; i < 20; i++ {
		entries = append(entries, sshconfig.HostEntry{
			Alias:        fmt.Sprintf("prod-api-node-with-a-very-long-name-%02d", i),
			HostName:     fmt.Sprintf("192.0.2.%d", i+1),
			User:         "deploy-user-with-a-long-name",
			IdentityFile: "~/.ssh/a-very-long-identity-file-name.pem",
			ProxyJump:    "jump-host-with-a-long-name",
			SourceFile:   "/Users/example/.ssh/config",
			SourceLine:   i + 1,
			RawBlock: strings.Join([]string{
				"Host prod-api-node-with-a-very-long-name",
				"    HostName 192.0.2.1",
				"    User deploy-user-with-a-long-name",
				"    IdentityFile ~/.ssh/a-very-long-identity-file-name.pem",
				"    ProxyJump jump-host-with-a-long-name",
			}, "\n"),
		})
	}
	model := NewModel(Config{ConfigPath: "/Users/example/.ssh/config", Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 72, Height: 18})

	view := model.View()
	if got, want := renderedLineCount(view), 18; got > want {
		t.Fatalf("view rendered %d lines, want at most %d:\n%s", got, want, view)
	}
	if !strings.Contains(view, "Enter connect") {
		t.Fatalf("footer missing from constrained view:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if got, want := lipgloss.Width(line), 72; got > want {
			t.Fatalf("line width = %d, want at most %d: %q", got, want, line)
		}
	}
}

func TestModelAddHostFlowWritesConfigAndReloads(t *testing.T) {
	path := writeAppTempConfig(t, "")
	load := func() ([]sshconfig.HostEntry, []sshconfig.Warning, error) {
		return sshconfig.ParseFile(path, sshconfig.Options{})
	}
	model := NewModel(Config{ConfigPath: path, Load: load})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if cmd != nil || model.mode != modeForm || model.form.operation != operationAdd {
		t.Fatalf("expected add form, model=%#v cmd=%v", model, cmd)
	}

	model.form.values = sshconfig.HostForm{Alias: "prod", HostName: "prod.example.com", User: "deploy"}
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || model.mode != modeConfirm {
		t.Fatalf("expected confirm mode, model=%#v cmd=%v", model, cmd)
	}

	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected write command")
	}
	msg := cmd()
	model, cmd = model.update(msg)
	if model.mode != modeBrowse {
		t.Fatalf("expected browse mode after write, got %v", model.mode)
	}
	if cmd == nil {
		t.Fatal("expected reload command after write")
	}
	reloadMsg := cmd()
	model, _ = model.update(reloadMsg)

	if len(model.entries) != 1 || model.entries[0].Alias != "prod" {
		t.Fatalf("entries = %#v, want prod", model.entries)
	}
	if !strings.Contains(readAppFile(t, path), "Host prod") {
		t.Fatalf("host not written:\n%s", readAppFile(t, path))
	}
}

func TestModelEditRejectsInvalidPortBeforeConfirm(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{{Alias: "prod", SourceFile: "config", SourceLine: 1}}})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd != nil || model.mode != modeForm || model.form.operation != operationEdit {
		t.Fatalf("expected edit form, model=%#v cmd=%v", model, cmd)
	}

	model.form.values.Port = "bad"
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		t.Fatal("invalid form should not create command")
	}
	if model.mode != modeForm || !strings.Contains(model.status, "port") {
		t.Fatalf("expected port validation in form mode, mode=%v status=%q", model.mode, model.status)
	}
}

func TestModelDeleteFlowWritesConfigAndReloads(t *testing.T) {
	path := writeAppTempConfig(t, `
Host prod
    HostName prod.example.com
`)
	load := func() ([]sshconfig.HostEntry, []sshconfig.Warning, error) {
		return sshconfig.ParseFile(path, sshconfig.Options{})
	}
	entries, warnings, err := load()
	if err != nil {
		t.Fatal(err)
	}
	model := NewModel(Config{ConfigPath: path, Entries: entries, Warnings: warnings, Load: load})

	var cmd tea.Cmd
	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd != nil || model.mode != modeConfirm || model.pending.operation != operationDelete {
		t.Fatalf("expected delete confirm, model=%#v cmd=%v", model, cmd)
	}

	model, cmd = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected delete command")
	}
	model, cmd = model.update(cmd())
	if model.mode != modeBrowse || cmd == nil {
		t.Fatalf("expected browse and reload command, mode=%v cmd=%v", model.mode, cmd)
	}
	model, _ = model.update(cmd())

	if len(model.entries) != 0 {
		t.Fatalf("entries = %#v, want empty", model.entries)
	}
	if strings.Contains(readAppFile(t, path), "Host prod") {
		t.Fatalf("host still present:\n%s", readAppFile(t, path))
	}
}

func TestSlashEntersSearchModeAndKeepsPreviousFilter(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api"},
		{Alias: "dev-db"},
	}})
	model = typeSearch(model, "prod")
	if model.Filter() != "prod" {
		t.Fatalf("filter = %q, want prod", model.Filter())
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !model.searchActive {
		t.Fatalf("slash should enter search mode")
	}
	if model.Filter() != "prod" {
		t.Fatalf("slash should keep filter for editing, got %q", model.Filter())
	}
	if got := len(model.FilteredEntries()); got != 1 {
		t.Fatalf("filtered count = %d, want 1", got)
	}
}

func TestCtrlUClearSearchFilter(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api"},
		{Alias: "dev-db"},
	}})
	model = typeSearch(model, "prod")

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.Filter() != "" {
		t.Fatalf("ctrl+u should clear filter, got %q", model.Filter())
	}
	if got := len(model.FilteredEntries()); got != 2 {
		t.Fatalf("filtered count = %d, want 2", got)
	}
}

func TestModelFuzzyFiltersHostAliases(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "prod-api"},
		{Alias: "dev-db"},
	}})

	model = typeSearch(model, "pa")
	filtered := model.FilteredEntries()
	if len(filtered) != 1 || filtered[0].Alias != "prod-api" {
		t.Fatalf("fuzzy filtered = %#v, want prod-api", filtered)
	}
}

func TestModelRanksAliasMatchesBeforeFieldAndFuzzyMatches(t *testing.T) {
	model := NewModel(Config{Entries: []sshconfig.HostEntry{
		{Alias: "xpa-host", Tags: []string{"api"}},
		{Alias: "api-prod"},
		{Alias: "prod-api"},
		{Alias: "web", Group: "api"},
		{Alias: "host-field", HostName: "api.example.com"},
		{Alias: "a-p-i-fuzzy"},
	}})

	model = typeSearch(model, "api")

	if got := aliases(model.FilteredEntries()); strings.Join(got, ",") != "api-prod,prod-api,web,xpa-host,host-field,a-p-i-fuzzy" {
		t.Fatalf("aliases = %v, want alias exact/prefix/contains before tag/group, host fields, and fuzzy", got)
	}
}

func TestBatchConnectUsesAllMarkedHostsAcrossFilters(t *testing.T) {
	runner := &countingRunner{}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{
			{Alias: "prod-a"},
			{Alias: "dev-b"},
		},
		Manager: terminal.Manager{
			Runner:     runner,
			Preference: "terminal",
		},
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeySpace})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeySpace})
	model = typeSearch(model, "prod")
	counts := model.dashboardCounts()
	if counts.Selected != 2 || counts.VisibleSelected != 1 {
		t.Fatalf("counts = %#v, want selected=2 visible=1", counts)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEnter})

	model, cmd := model.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected batch connect command")
	}
	model, _ = model.update(cmd())
	if runner.runCalls != 2 {
		t.Fatalf("run calls = %d, want both marked hosts", runner.runCalls)
	}
}

func TestWarningsPanelOpensAndCloses(t *testing.T) {
	model := NewModel(Config{
		Entries:  []sshconfig.HostEntry{{Alias: "prod"}},
		Warnings: []sshconfig.Warning{{Path: "config", Line: 3, Message: "bad include"}},
	})

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	if model.mode != modeWarnings {
		t.Fatalf("mode = %v, want warnings", model.mode)
	}
	if view := stripANSI(model.View()); !strings.Contains(view, "bad include") {
		t.Fatalf("warnings view missing warning:\n%s", view)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.mode != modeBrowse {
		t.Fatalf("mode after esc = %v, want browse", model.mode)
	}
}

func TestFormCursorEditsInsideFieldAndClears(t *testing.T) {
	model := NewModel(Config{})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	for _, r := range "prod" {
		model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyLeft})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	if got := model.form.values.Alias; got != "pro-d" {
		t.Fatalf("alias = %q, want pro-d", got)
	}
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := model.form.values.Alias; got != "" {
		t.Fatalf("alias after ctrl+u = %q, want empty", got)
	}
	if model.form.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", model.form.cursor)
	}
}

func TestManualMonitorRefreshOpensPanelAndProbes(t *testing.T) {
	cache := monitor.NewCache(time.Minute, time.Second)
	probeCalls := 0
	probe := func(ctx context.Context, target monitor.ProbeTarget) (*monitor.Snapshot, error) {
		probeCalls++
		return &monitor.Snapshot{Alias: target.Alias, Uptime: "up 1 day"}, nil
	}
	model := NewModel(Config{
		Entries: []sshconfig.HostEntry{{Alias: "prod"}},
		Monitor: cache,
		Probe:   probe,
	})

	model, cmd := model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Fatal("expected monitor probe command")
	}
	if !model.monitorVisible {
		t.Fatal("manual refresh should enable monitor panel")
	}
	model, _ = model.update(cmd())
	if probeCalls != 1 {
		t.Fatalf("probe calls = %d, want 1", probeCalls)
	}
	if _, ok := cache.Get("prod"); !ok {
		t.Fatal("expected snapshot in cache")
	}
}

func renderedLineCount(value string) int {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

type countingRunner struct {
	runCalls      int
	outputCalls   int
	runErr        error
	outputErr     error
	failRunCall   int
	lastRunJoined string
}

func writeAppTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readAppFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (r *countingRunner) Run(name string, args ...string) error {
	r.runCalls++
	r.lastRunJoined = strings.Join(append([]string{name}, args...), " ")
	if r.failRunCall > 0 && r.runCalls == r.failRunCall {
		return errors.New("run failed")
	}
	return r.runErr
}

func (r *countingRunner) Output(name string, args ...string) ([]byte, error) {
	r.outputCalls++
	return nil, r.outputErr
}

func aliases(entries []sshconfig.HostEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Alias)
	}
	return out
}
