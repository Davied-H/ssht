package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dong/ssh-config-tmux-tui/internal/monitor"
	"github.com/dong/ssh-config-tmux-tui/internal/sshconfig"
)

// previewModel sets up a Model with one host plus a monitor cache and returns
// the rendered preview lines for the given window width / height.
func previewModel(t *testing.T, snap *monitor.Snapshot, expanded bool, width, height int) []string {
	t.Helper()
	entry := sshconfig.HostEntry{
		Alias:        "estha-en-dify",
		HostName:     "4.194.92.132",
		Port:         "22",
		User:         "azureuser",
		IdentityFile: "~/.ssh/estha-en-dify.pem",
		SourceFile:   "/Users/dong/.ssh/config",
		SourceLine:   148,
	}
	cache := monitor.NewCache(30*time.Second, 5*time.Second)
	if snap != nil {
		snap.Alias = entry.Alias
		if snap.FetchedAt.IsZero() {
			snap.FetchedAt = time.Now()
		}
		cache.Put(snap)
	}
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}, Monitor: cache, MonitorVisible: true})
	model.showRawPreview = expanded
	model, _ = model.update(tea.WindowSizeMsg{Width: width, Height: height})
	_, right := model.splitColumns()
	return model.previewLines(height, right)
}

// stripANSI removes ANSI color escape sequences so we can assert on plain
// content without coupling the tests to specific color codes.
func stripANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func joinPreview(lines []string) string {
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = stripANSI(l)
	}
	return strings.Join(plain, "\n")
}

func TestPreviewLiveSnapshotCollapsed(t *testing.T) {
	snap := &monitor.Snapshot{
		Uptime:    "05:17:09 up 140 days, 3 users, load average: 0.18, 0.25, 0.21",
		Load:      "0.18, 0.25, 0.21",
		Mem:       "4451/15944 MB (28%)",
		Disk:      "17G used / 61G (28%)",
		TopProcs:  "systemd(13.8%) azure-proxy-agent(8.2%) containerd(5.1%) shim(3.0%) sshd(2.1%)",
		Conns:     "1",
		FetchedAt: time.Now(),
	}
	rendered := joinPreview(previewModel(t, snap, false, 140, 30))

	wants := []string{
		"▎ estha-en-dify",
		"● live",
		"azureuser@4.194.92.132:22",
		"~/.ssh/estha-en-dify.pem",
		"┃ Source",
		"┃ Health",
		"updated",
		"uptime",
		"140d",
		"load 0.18",
		"mem",
		"▰",
		"▱",
		" 28%",
		"4.3G/16G",
		"disk",
		"17G/61G",
		"conns",
		"1 established",
		"┃ Top CPU",
		"13.8%",
		"systemd",
		"+4",
	}
	for _, w := range wants {
		if !strings.Contains(rendered, w) {
			t.Errorf("expected substring %q in preview:\n%s", w, rendered)
		}
	}
	// Top1 + 1 line, no full top-5 unless expanded
	if strings.Contains(rendered, "azure-proxy-agent") {
		t.Errorf("collapsed preview should not list azure-proxy-agent, got:\n%s", rendered)
	}
	// No raw section unless expanded
	if strings.Contains(rendered, "Raw config") {
		t.Errorf("collapsed preview should not show Raw config:\n%s", rendered)
	}
}

func TestPreviewExpandedShowsAllProcsAndRaw(t *testing.T) {
	snap := &monitor.Snapshot{
		Uptime:    "05:17:09 up 140 days, 3 users",
		Load:      "0.18, 0.25, 0.21",
		Mem:       "4451/15944 MB (28%)",
		Disk:      "17G used / 61G (28%)",
		TopProcs:  "systemd(13.8%) azure-proxy-agent(8.2%) containerd(5.1%) shim(3.0%) sshd(2.1%)",
		Conns:     "1",
		FetchedAt: time.Now(),
	}
	t.Helper()
	entry := sshconfig.HostEntry{
		Alias:        "estha-en-dify",
		HostName:     "4.194.92.132",
		Port:         "22",
		User:         "azureuser",
		IdentityFile: "~/.ssh/estha-en-dify.pem",
		SourceFile:   "/Users/dong/.ssh/config",
		SourceLine:   148,
		RawBlock: strings.Join([]string{
			"Host estha-en-dify",
			"    HostName 4.194.92.132",
			"    User azureuser",
		}, "\n"),
	}
	cache := monitor.NewCache(30*time.Second, 5*time.Second)
	snap.Alias = entry.Alias
	cache.Put(snap)
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}, Monitor: cache, MonitorVisible: true})
	model.showRawPreview = true
	model, _ = model.update(tea.WindowSizeMsg{Width: 140, Height: 40})
	_, right := model.splitColumns()
	rendered := joinPreview(model.previewLines(40, right))

	for _, w := range []string{"systemd", "azure-proxy-agent", "containerd", "shim", "sshd"} {
		if !strings.Contains(rendered, w) {
			t.Errorf("expanded preview missing %q:\n%s", w, rendered)
		}
	}
	if strings.Contains(rendered, "+4") {
		t.Errorf("expanded preview should not show +4 chip:\n%s", rendered)
	}
	if !strings.Contains(rendered, "┃ Raw config") {
		t.Errorf("expanded preview should include Raw config section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Host estha-en-dify") {
		t.Errorf("Raw config body missing:\n%s", rendered)
	}
}

func TestPreviewStaleSnapshotShowsAmberHeader(t *testing.T) {
	cache := monitor.NewCache(30*time.Second, 5*time.Second)
	now := time.Now()
	cache.SetClock(func() time.Time { return now })
	cache.Put(&monitor.Snapshot{
		Alias:     "estha-en-dify",
		FetchedAt: now.Add(-2 * time.Minute),
		Uptime:    "up 1 day",
		Mem:       "8000/16000 MB (50%)",
		Disk:      "30G used / 60G (50%)",
	})
	entry := sshconfig.HostEntry{
		Alias:    "estha-en-dify",
		HostName: "1.2.3.4",
		User:     "ubuntu",
	}
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}, Monitor: cache, MonitorVisible: true})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, right := model.splitColumns()
	rendered := joinPreview(model.previewLines(30, right))

	if !strings.Contains(rendered, "● stale") {
		t.Errorf("expected ● stale chip:\n%s", rendered)
	}
	if !strings.Contains(rendered, "stale") {
		t.Errorf("expected 'stale' suffix in Health header:\n%s", rendered)
	}
}

func TestPreviewDeadSnapshotShowsErrorBody(t *testing.T) {
	cache := monitor.NewCache(30*time.Second, 5*time.Second)
	cache.Put(&monitor.Snapshot{
		Alias:     "estha-en-dify",
		FetchedAt: time.Now(),
		Err:       errors.New("ssh: connect: connection refused"),
	})
	entry := sshconfig.HostEntry{
		Alias:    "estha-en-dify",
		HostName: "1.2.3.4",
	}
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}, Monitor: cache, MonitorVisible: true})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, right := model.splitColumns()
	rendered := joinPreview(model.previewLines(30, right))

	if !strings.Contains(rendered, "● dead") {
		t.Errorf("expected ● dead chip:\n%s", rendered)
	}
	if !strings.Contains(rendered, "connection refused") {
		t.Errorf("expected error body:\n%s", rendered)
	}
	if strings.Contains(rendered, "uptime") {
		t.Errorf("dead host should not render uptime row:\n%s", rendered)
	}
}

func TestPreviewLoadingStateShowsProbing(t *testing.T) {
	cache := monitor.NewCache(30*time.Second, 5*time.Second)
	entry := sshconfig.HostEntry{
		Alias:    "estha-en-dify",
		HostName: "1.2.3.4",
	}
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}, Monitor: cache, MonitorVisible: true})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, right := model.splitColumns()
	rendered := joinPreview(model.previewLines(30, right))

	if !strings.Contains(rendered, "● …") {
		t.Errorf("expected ● … loading chip:\n%s", rendered)
	}
	if !strings.Contains(rendered, "probing") {
		t.Errorf("expected 'probing…' body:\n%s", rendered)
	}
}

func TestPreviewWithoutMonitorOmitsHealthSection(t *testing.T) {
	entry := sshconfig.HostEntry{
		Alias:    "estha-en-dify",
		HostName: "1.2.3.4",
		User:     "ubuntu",
	}
	model := NewModel(Config{Entries: []sshconfig.HostEntry{entry}})
	model, _ = model.update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, right := model.splitColumns()
	rendered := joinPreview(model.previewLines(30, right))

	if strings.Contains(rendered, "Health") {
		t.Errorf("--no-monitor preview should omit Health section:\n%s", rendered)
	}
	if strings.Contains(rendered, "Top CPU") {
		t.Errorf("--no-monitor preview should omit Top CPU section:\n%s", rendered)
	}
	if strings.Contains(rendered, "● live") || strings.Contains(rendered, "● dead") {
		t.Errorf("--no-monitor preview should not render state chip:\n%s", rendered)
	}
}

func TestPreviewWidthIsRespected(t *testing.T) {
	snap := &monitor.Snapshot{
		Uptime:   "05:17:09 up 140 days",
		Load:     "0.18, 0.25, 0.21",
		Mem:      "4451/15944 MB (28%)",
		Disk:     "17G used / 61G (28%)",
		TopProcs: "systemd(13.8%) azure-proxy-agent(8.2%)",
		Conns:    "1",
	}
	for _, width := range []int{80, 100, 140} {
		lines := previewModel(t, snap, false, width, 30)
		_, right := newWidthModel(width).splitColumns()
		for _, line := range lines {
			if got := visibleWidth(line); got > right {
				t.Errorf("width=%d preview row exceeds %d cells (got %d): %q", width, right, got, line)
			}
		}
	}
}

// newWidthModel builds a fresh Model just for splitColumns() math.
func newWidthModel(width int) Model {
	m := NewModel(Config{})
	m.width = width
	return m
}

// visibleWidth strips ANSI and counts terminal cells.
func visibleWidth(s string) int {
	return len([]rune(stripANSI(s)))
}

func TestPreviewProgressBarFillsByPercent(t *testing.T) {
	snap := &monitor.Snapshot{
		Uptime:   "up 1 day",
		Load:     "0.5",
		Mem:      "14000/15944 MB (88%)",
		Disk:     "55G used / 61G (90%)",
		TopProcs: "kworker(2%)",
		Conns:    "5",
	}
	rendered := joinPreview(previewModel(t, snap, false, 140, 30))

	// At 88% the 10-cell bar should be 8 filled / 2 empty (88*10/100 = 8).
	if !strings.Contains(rendered, "▰▰▰▰▰▰▰▰▱▱") {
		t.Errorf("expected 8/10 filled bar for 88%%:\n%s", rendered)
	}
	// At 90% the 10-cell bar should be 9 filled / 1 empty.
	if !strings.Contains(rendered, "▰▰▰▰▰▰▰▰▰▱") {
		t.Errorf("expected 9/10 filled bar for 90%%:\n%s", rendered)
	}
	if !strings.Contains(rendered, " 88%") || !strings.Contains(rendered, " 90%") {
		t.Errorf("expected percentage suffixes:\n%s", rendered)
	}
}
