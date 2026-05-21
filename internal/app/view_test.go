package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dong/ssht/internal/sshconfig"
	"github.com/dong/ssht/internal/state"
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
	if !strings.Contains(view, "›") {
		t.Fatalf("sidebar missing selection marker ›\n%s", view)
	}
}

func TestSidebarHidesOnNarrowTerminal(t *testing.T) {
	entries := []sshconfig.HostEntry{{Alias: "a", Group: "prod"}}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 60, Height: 20})
	view := model.View()
	if strings.Contains(view, "GROUPS") {
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
		plain := stripANSI(line)
		if strings.Contains(plain, group+" 0") {
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

func TestPreviewPaneCanBeCollapsedWithShortcut(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod", SourceFile: "/tmp/config", SourceLine: 1},
		{Alias: "b", Group: "dev", SourceFile: "/tmp/config", SourceLine: 2},
	}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 24})
	_, _, preview := model.splitThreeColumns()
	if preview == 0 {
		t.Fatalf("preview should be visible before toggle")
	}

	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	sidebar, list, preview := model.splitThreeColumns()
	if preview != 0 {
		t.Fatalf("preview width = %d, want 0", preview)
	}
	if sidebar == 0 || list < listMinWidth {
		t.Fatalf("collapsed layout should keep sidebar/list, got sidebar=%d list=%d preview=%d", sidebar, list, preview)
	}
	if !strings.Contains(model.View(), "preview off") {
		t.Fatalf("view should show preview off status:\n%s", model.View())
	}
}

func TestPreviewPaneCanBeCollapsedFromSidebarFocus(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "a", Group: "prod"},
		{Alias: "b", Group: "dev"},
	}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 24})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyTab})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})

	_, _, preview := model.splitThreeColumns()
	if preview != 0 {
		t.Fatalf("preview width = %d, want 0", preview)
	}
}

func TestBrowseViewRendersProfessionalStructureOnWideTerminal(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod", HostName: "192.0.2.12", User: "deploy"},
		{Alias: "dev-db", Group: "dev", HostName: "192.0.2.20", User: "root"},
	}
	model := newWideModel(t, entries, state.NewStore())

	view := model.View()
	for _, want := range []string{"GROUPS", "state HOST", "/ search shortcuts active", "▎", "›"} {
		if !strings.Contains(view, want) {
			t.Fatalf("wide view missing %q:\n%s", want, view)
		}
	}
}

func TestSearchViewShowsEditAndClearHints(t *testing.T) {
	model := newWideModel(t, []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod"},
	}, state.NewStore())
	model = typeSearch(model, "prod")

	view := stripANSI(model.View())
	for _, want := range []string{"SEARCH prod", "editing current filter", "Ctrl+U clear", "Enter/Esc done"} {
		if !strings.Contains(view, want) {
			t.Fatalf("search view missing %q:\n%s", want, view)
		}
	}
}

func TestBrowseViewShowsNoResultsQueryAndClearHint(t *testing.T) {
	model := newWideModel(t, []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod"},
	}, state.NewStore())
	model = typeSearch(model, "missing")

	view := stripANSI(model.View())
	for _, want := range []string{`No hosts match "missing"`, "Press Ctrl+U in search to clear"} {
		if !strings.Contains(view, want) {
			t.Fatalf("no results view missing %q:\n%s", want, view)
		}
	}
}

func TestHelpViewDocumentsSearchClearShortcut(t *testing.T) {
	model := NewModel(Config{})
	model, _ = model.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})

	view := stripANSI(model.View())
	for _, want := range []string{"/      Enter search mode and edit the current filter.", "Ctrl+U Clear the filter while search mode is active."} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q:\n%s", want, view)
		}
	}
}

func TestBrowseViewPinsFooterToBottom(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{Alias: "prod-api", Group: "prod", HostName: "192.0.2.12", User: "deploy"},
	}
	model := NewModel(Config{Entries: entries})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 24})

	lines := strings.Split(model.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("view height = %d, want 24\n%s", len(lines), model.View())
	}
	if !strings.Contains(stripANSI(lines[22]), "connect") {
		t.Fatalf("primary footer should be on second-to-last row, got %q\n%s", stripANSI(lines[22]), model.View())
	}
	if !strings.Contains(stripANSI(lines[23]), "Tab focus") {
		t.Fatalf("secondary footer should be on last row, got %q\n%s", stripANSI(lines[23]), model.View())
	}
}

func TestBrowseViewWidthIsRespectedAcrossBreakpoints(t *testing.T) {
	entries := []sshconfig.HostEntry{
		{
			Alias:        "prod-api-node-with-a-very-long-name",
			Group:        "production-east-with-long-name",
			HostName:     "192.0.2.12",
			User:         "deploy-user-with-a-long-name",
			IdentityFile: "~/.ssh/a-very-long-identity-file-name.pem",
			SourceFile:   "/Users/example/.ssh/config",
			SourceLine:   42,
		},
		{Alias: "dev-db", Group: "dev", HostName: "192.0.2.20", User: "root"},
	}
	for _, width := range []int{72, 80, 96, 120, 140} {
		model := NewModel(Config{Entries: entries})
		model, _ = model.update(tea.WindowSizeMsg{Width: width, Height: 18})
		view := model.View()
		if !strings.Contains(view, "↵ connect") {
			t.Fatalf("width=%d footer missing:\n%s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width=%d line width = %d: %q\n%s", width, got, line, view)
			}
		}
	}
}
