package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dong/ssh-config-tmux-tui/internal/sshconfig"
	"github.com/dong/ssh-config-tmux-tui/internal/state"
)

func newWideModel(t *testing.T, entries []sshconfig.HostEntry, store state.Store) Model {
	t.Helper()
	model := NewModel(Config{Entries: entries, State: store})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 30})
	return model
}

func TestSidebarRenderListsAllGroupsAndCurrentSelection(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod"},
		{Alias: "b", Group: "dev"},
	}
	model := newWideModel(t, entries, state.NewStore())
	view := model.View()
	for _, name := range []string{"prod", "dev", "all"} {
		if !strings.Contains(view, name) {
			t.Fatalf("sidebar missing %q\n%s", name, view)
		}
	}
	if !strings.Contains(view, "▸") {
		t.Fatalf("sidebar missing selection marker ▸\n%s", view)
	}
}

func TestSidebarHidesOnNarrowTerminal(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "prod"}}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 60, Height: 20})
	view := model.View()
	if strings.Contains(view, "▸") {
		t.Fatalf("sidebar should be hidden on narrow terminal\n%s", view)
	}
}

func TestSidebarShowsEmptyGroupsWithZeroCount(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "prod"}}
	store := state.NewStore()
	store.EmptyGroups = []string{"future-east"}
	model := newWideModel(t, entries, store)

	view := model.View()
	if !strings.Contains(view, "future-east") {
		t.Fatalf("sidebar missing empty group name:\n%s", view)
	}
	if !strings.Contains(view, "future-east") || !containsZeroCount(view, "future-east") {
		t.Fatalf("empty group should show count 0:\n%s", view)
	}
}

func containsZeroCount(view, group string) bool {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, group) && strings.HasSuffix(strings.TrimRight(line, " "), "0") {
			return true
		}
	}
	return false
}

func TestTopStatusLineHasNoGroupTabs(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod"},
		{Alias: "b", Group: "dev"},
	}
	model := newWideModel(t, entries, state.NewStore())
	statusLine := model.topStatusLine()
	if strings.Contains(statusLine, "[prod]") || strings.Contains(statusLine, "[dev]") {
		t.Fatalf("top status line should not contain group tabs anymore: %q", statusLine)
	}
	if !strings.Contains(statusLine, "Hosts") {
		t.Fatalf("top status line should contain Hosts count: %q", statusLine)
	}
}

func TestSplitThreeColumnsFallsBackToTwoColumnsWhenNarrow(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "prod"}}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	sidebar, list, _ := model.splitThreeColumns()
	if sidebar != 0 {
		t.Fatalf("sidebar = %d, want 0 on width=80", sidebar)
	}
	if list <= 0 {
		t.Fatalf("list width = %d, want >0", list)
	}
}

func TestSplitThreeColumnsAllocatesSidebarOnWideTerminal(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod"},
		{Alias: "b", Group: "dev"},
	}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 30})
	sidebar, list, preview := model.splitThreeColumns()
	if sidebar < sidebarMinWidth || sidebar > sidebarMaxWidth {
		t.Fatalf("sidebar width = %d, want between %d and %d", sidebar, sidebarMinWidth, sidebarMaxWidth)
	}
	if list < listMinWidth || preview < previewMinWidth {
		t.Fatalf("widths = sidebar %d list %d preview %d", sidebar, list, preview)
	}
}
