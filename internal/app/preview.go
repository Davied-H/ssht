package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dong/ssh-config-tmux-tui/internal/monitor"
	"github.com/dong/ssh-config-tmux-tui/internal/sshconfig"
	"github.com/dong/ssh-config-tmux-tui/internal/state"
)

// Layout constants for the redesigned preview pane. Tied to the 50-column
// baseline in splitColumns(); wider terminals just get more headroom for the
// trailing values, never tighter columns.
const (
	previewBodyIndent     = 2  // body rows hang under their section header
	previewBarIndent      = 2  // "  ┃ Title" matches body indent
	previewSectionLabelW  = 8  // History/Source/Top CPU label column
	previewMetricLabelW   = 7  // uptime/mem/disk/conns label column
	previewProgressCells  = 10 // mem/disk bar width
	previewTopProcDefault = 1  // collapsed view
	previewTopProcMax     = 5  // expanded view (matches probe.go's NR<=5)
)

// monitorState classifies the freshness of a Snapshot so the preview header can
// render a single colored dot summarizing the host's reachability.
type monitorState int

const (
	monitorAbsent monitorState = iota
	monitorLoading
	monitorLive
	monitorStale
	monitorDead
)

func (m Model) previewLines(maxLines, width int) []string {
	if maxLines <= 0 || width <= 0 {
		return nil
	}
	entry, ok := m.Selected()
	if !ok {
		return nil
	}

	hostState := m.state.Hosts[entry.Alias]

	showMonitor := m.monitor != nil && m.monitorVisible

	var snap *monitor.Snapshot
	mstate := monitorAbsent
	var age time.Duration
	var ttl time.Duration
	if showMonitor {
		ttl = m.monitor.TTL()
		if s, present := m.monitor.Get(entry.Alias); !present {
			mstate = monitorLoading
		} else {
			snap = s
			age = m.monitor.Age(snap)
			switch {
			case snap.Err != nil:
				mstate = monitorDead
			case age >= ttl && ttl > 0:
				mstate = monitorStale
			default:
				mstate = monitorLive
			}
		}
	}

	lines := renderPreviewHeader(entry, mstate, width, showMonitor)

	if hist := renderHistorySection(entry, hostState, width); len(hist) > 0 {
		lines = append(lines, "")
		lines = append(lines, hist...)
	}

	if showMonitor {
		if health := renderHealthSection(snap, mstate, age, width); len(health) > 0 {
			lines = append(lines, "")
			lines = append(lines, health...)
		}
		if top := renderTopCPUSection(snap, mstate, width, m.showRawPreview); len(top) > 0 {
			lines = append(lines, "")
			lines = append(lines, top...)
		}
	}

	if m.showRawPreview {
		if raw := renderRawSection(entry, width); len(raw) > 0 {
			lines = append(lines, "")
			lines = append(lines, raw...)
		}
	}

	return fitLines(lines, maxLines)
}

// renderPreviewHeader produces the alias / state-dot / connection-identity rows
// at the very top of the preview pane.
func renderPreviewHeader(entry sshconfig.HostEntry, state monitorState, width int, hasMonitor bool) []string {
	bar := accentStyle.Render("▎ ")
	chip := monitorStateChip(state)
	chipW := lipgloss.Width(chip)
	titleRoom := width - lipgloss.Width(bar)
	if hasMonitor && chipW > 0 {
		titleRoom -= chipW + 1
	}
	if titleRoom < 1 {
		titleRoom = 1
	}
	title := titleStyle.Render(truncate(entry.Alias, titleRoom))
	headerLeft := bar + title

	var first string
	if hasMonitor && chipW > 0 {
		first = layoutLeftRight(headerLeft, chip, width)
	} else {
		first = headerLeft
	}

	lines := []string{first}

	conn := connectionString(entry)
	if conn != "" {
		lines = append(lines, indented(truncate(conn, max(width-previewBodyIndent, 1))))
	}

	secondParts := []string{}
	if entry.IdentityFile != "" {
		secondParts = append(secondParts, entry.IdentityFile)
	}
	if entry.ProxyJump != "" {
		secondParts = append(secondParts, mutedStyle.Render("proxy: ")+entry.ProxyJump)
	}
	if len(secondParts) > 0 {
		joined := strings.Join(secondParts, mutedStyle.Render("  ·  "))
		lines = append(lines, indented(truncate(joined, max(width-previewBodyIndent, 1))))
	}

	// Edge case: alias-only host (no HostName/User/Port/Identity/ProxyJump).
	// Fall back to the legacy per-field layout so the preview is never empty.
	if conn == "" && len(secondParts) == 0 {
		fallbackFields := []struct{ k, v string }{
			{"HostName", entry.HostName},
			{"Port", entry.Port},
			{"User", entry.User},
			{"Identity", entry.IdentityFile},
			{"ProxyJump", entry.ProxyJump},
		}
		for _, kv := range fallbackFields {
			if kv.v != "" {
				lines = append(lines, formatField(kv.k, kv.v, width))
			}
		}
	}

	if entry.ProxyCommand != "" {
		lines = append(lines, formatField("ProxyCmd", entry.ProxyCommand, width))
	}

	return lines
}

// renderHistorySection renders the History/Source rows. Returns nil when
// neither value applies (e.g. brand-new host with no source line).
func renderHistorySection(entry sshconfig.HostEntry, hostState state.HostState, width int) []string {
	historyValue := buildHistoryString(entry, hostState)
	source := ""
	if entry.SourceFile != "" {
		source = fmt.Sprintf("%s:%d", entry.SourceFile, entry.SourceLine)
	}
	if historyValue == "" && source == "" {
		return nil
	}
	out := []string{}
	if historyValue != "" {
		out = append(out, barInfoRow("History", historyValue, width))
	}
	if source != "" {
		out = append(out, barInfoRow("Source", source, width))
	}
	return out
}

// renderHealthSection renders the Health header plus uptime/load/mem/disk/conns
// rows. Returns just the header + status line for loading/dead states, nil only
// when the monitor was disabled (which the caller already filters out).
func renderHealthSection(snap *monitor.Snapshot, state monitorState, age time.Duration, width int) []string {
	suffix, suffixStyle := healthHeaderSuffix(state, age)
	header := barSectionHeader("Health", suffix, suffixStyle, width)

	switch state {
	case monitorLoading:
		return []string{header, indented(mutedStyle.Render("probing…"))}
	case monitorDead:
		body := "unreachable"
		if snap != nil && snap.Err != nil {
			body = snap.Err.Error()
		}
		return []string{header, indented(errorStyle.Render(truncate(body, max(width-previewBodyIndent, 1))))}
	}
	if snap == nil {
		return []string{header}
	}

	rows := []string{header}
	rows = append(rows, healthMetricRows(snap, width)...)
	return rows
}

func healthMetricRows(snap *monitor.Snapshot, width int) []string {
	var rows []string

	uptime := monitor.ParseUptime(snap.Uptime)
	one, _, _, loadOK := monitor.ParseLoad(snap.Load)
	loadVal := ""
	if loadOK {
		loadVal = one
	}
	if uptime != "" || loadVal != "" {
		rows = append(rows, uptimeLoadRow(uptime, loadVal, width))
	}

	if used, total, unit, pct, ok := monitor.ParseMemPct(snap.Mem); ok {
		usedShort := monitor.ShortMemBytes(used, unit)
		totalShort := monitor.ShortMemBytes(total, unit)
		rows = append(rows, healthBarRow("mem", pct, usedShort+"/"+totalShort, width))
	} else if snap.Mem != "" {
		rows = append(rows, metricRow("mem", snap.Mem, width))
	}

	if used, total, pct, ok := monitor.ParseDiskPct(snap.Disk); ok {
		rows = append(rows, healthBarRow("disk", pct, used+"/"+total, width))
	} else if snap.Disk != "" {
		rows = append(rows, metricRow("disk", snap.Disk, width))
	}

	if snap.Conns != "" && snap.Conns != "n/a" {
		rows = append(rows, metricRow("conns", snap.Conns+" established", width))
	}

	return rows
}

// renderTopCPUSection emits the Top CPU section. In collapsed mode the header
// + a single highest row (with "+N" trailing chip when there are more); in
// expanded mode all five rows are listed verbatim. Returns nil when the probe
// did not return any process data or the host is loading/dead.
func renderTopCPUSection(snap *monitor.Snapshot, state monitorState, width int, expanded bool) []string {
	if state == monitorLoading || state == monitorDead {
		return nil
	}
	if snap == nil {
		return nil
	}
	procs := monitor.ParseTopProcs(snap.TopProcs)
	if len(procs) == 0 {
		return nil
	}
	header := barSectionHeader("Top CPU", "", lipgloss.Style{}, width)
	rows := []string{header}
	limit := previewTopProcDefault
	if expanded {
		limit = previewTopProcMax
	}
	if limit > len(procs) {
		limit = len(procs)
	}
	for i := 0; i < limit; i++ {
		suffix := ""
		if !expanded && i == 0 && len(procs) > 1 {
			suffix = fmt.Sprintf("+%d", len(procs)-1)
		}
		rows = append(rows, topProcRow(procs[i], suffix, width))
	}
	return rows
}

// renderRawSection renders the original SSH config block when the user toggled
// the expanded preview with `v`. The block is shown verbatim with each line
// truncated to the preview width.
func renderRawSection(entry sshconfig.HostEntry, width int) []string {
	if entry.RawBlock == "" {
		return nil
	}
	header := barSectionHeader("Raw config", "", lipgloss.Style{}, width)
	rows := []string{header}
	for _, line := range strings.Split(entry.RawBlock, "\n") {
		rows = append(rows, indented(dimStyle.Render(truncate(line, max(width-previewBodyIndent, 1)))))
	}
	return rows
}

func buildHistoryString(entry sshconfig.HostEntry, hostState state.HostState) string {
	bits := []string{}
	if hostState.ConnectCount > 0 {
		bit := fmt.Sprintf("%d×", hostState.ConnectCount)
		if hostState.LastConnectedAt != "" {
			bit += " · " + shortTime(hostState.LastConnectedAt)
		}
		bits = append(bits, bit)
	}
	if hostState.Favorite {
		bits = append(bits, "★")
	}
	if len(entry.Tags) > 0 {
		bits = append(bits, "tags: "+strings.Join(entry.Tags, " "))
	}
	if entry.Group != "" {
		bits = append(bits, "group: "+entry.Group)
	}
	if len(bits) == 0 {
		// Surface "never connected" hint so the row isn't dropped silently.
		return ""
	}
	return strings.Join(bits, " · ")
}

func monitorStateChip(state monitorState) string {
	switch state {
	case monitorLive:
		return liveStyle.Render("●") + dimStyle.Render(" live")
	case monitorStale:
		return warnStyle.Render("●") + dimStyle.Render(" stale")
	case monitorDead:
		return errorStyle.Render("●") + dimStyle.Render(" dead")
	case monitorLoading:
		return mutedStyle.Render("● …")
	default:
		return ""
	}
}

func healthHeaderSuffix(state monitorState, age time.Duration) (string, lipgloss.Style) {
	switch state {
	case monitorLive:
		if age <= 0 {
			return "updated just now", mutedStyle
		}
		return "updated " + shortDuration(age) + " ago", mutedStyle
	case monitorStale:
		if age <= 0 {
			return "stale", warnStyle
		}
		return "updated " + shortDuration(age) + " ago · stale", warnStyle
	case monitorDead:
		return "unreachable", errorStyle
	case monitorLoading:
		// Header suffix stays empty; the body row shows "probing…" so we don't repeat ourselves.
		return "", lipgloss.Style{}
	}
	return "", lipgloss.Style{}
}

// barInfoRow:  "  ┃ Label    value"  (single-line section row)
func barInfoRow(label, value string, width int) string {
	bar := accentStyle.Render("┃ ")
	labelText := mutedStyle.Render(padRight(label, previewSectionLabelW))
	prefix := strings.Repeat(" ", previewBarIndent) + bar + labelText + " "
	available := width - lipgloss.Width(prefix)
	if available < 1 {
		return prefix
	}
	return prefix + truncate(value, available)
}

// barSectionHeader: "  ┃ Title                  suffix"  (header row that
// introduces a multi-line section like Health/Top CPU/Raw).
func barSectionHeader(title, suffix string, suffixStyle lipgloss.Style, width int) string {
	bar := accentStyle.Render("┃ ")
	left := strings.Repeat(" ", previewBarIndent) + bar + mutedStyle.Render(title)
	if suffix == "" {
		return left
	}
	suffixStyled := suffixStyle.Render(suffix)
	return layoutLeftRight(left, suffixStyled, width)
}

// metricRow: "  label   value"  (used under Health for fallback / conns).
func metricRow(label, value string, width int) string {
	prefix := strings.Repeat(" ", previewBodyIndent) + mutedStyle.Render(padRight(label, previewMetricLabelW)) + "  "
	available := width - lipgloss.Width(prefix)
	if available < 1 {
		return prefix
	}
	return prefix + truncate(value, available)
}

// healthBarRow: "  label   ▰▰▱▱▱  28%   used/total"
func healthBarRow(label string, pct int, valueText string, width int) string {
	level := monitor.HealthLevel(pct)
	style := styleForLevel(level)
	bar := style.Render(monitor.ProgressBar(pct, previewProgressCells))
	pctText := style.Render(fmt.Sprintf("%3d%%", pct))
	prefix := strings.Repeat(" ", previewBodyIndent) + mutedStyle.Render(padRight(label, previewMetricLabelW)) + "  "
	core := prefix + bar + "  " + pctText
	if valueText == "" {
		return core
	}
	coreW := lipgloss.Width(core)
	tail := "   " + dimStyle.Render(valueText)
	tailW := lipgloss.Width(tail)
	if coreW+tailW > width {
		// Fall back to value-only when there is no room.
		return truncate(core, width)
	}
	return core + tail
}

// uptimeLoadRow lays out uptime on the left and load 1m on the right.
func uptimeLoadRow(uptime, load string, width int) string {
	left := metricRow("uptime", uptime, width)
	if load == "" {
		return left
	}
	right := mutedStyle.Render("load ") + load
	return layoutLeftRight(left, right, width)
}

// topProcRow: "   13.8%  systemd            +4"
func topProcRow(proc monitor.ProcInfo, suffix string, width int) string {
	pctStyle := topPctStyle(proc.Pct)
	pctText := pctStyle.Render(fmt.Sprintf("%5.1f%%", proc.Pct))
	left := " " + pctText + "  " + proc.Name
	if suffix == "" {
		return truncate(left, width)
	}
	return layoutLeftRight(left, dimStyle.Render(suffix), width)
}

// topPctStyle softly colors the pct based on absolute load; multi-core hosts
// can legitimately exceed 100 so we use load-weighted thresholds rather than
// HealthLevel which assumes 0..100.
func topPctStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 80:
		return errorStyle
	case pct >= 40:
		return warnStyle
	default:
		return mutedStyle
	}
}

func styleForLevel(level monitor.Level) lipgloss.Style {
	switch level {
	case monitor.LevelCrit:
		return errorStyle
	case monitor.LevelWarn:
		return warnStyle
	default:
		return liveStyle
	}
}

// layoutLeftRight returns "<left><pad><right>" so right ends at column width.
// When the two parts together would overflow, the left is hard-truncated; if
// that still doesn't fit, the whole line is truncated to width.
func layoutLeftRight(left, right string, width int) string {
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	if leftW+rightW+1 > width {
		room := width - rightW - 1
		if room < 0 {
			return truncate(right, width)
		}
		return ansi.Truncate(left, room, "…") + " " + right
	}
	return left + strings.Repeat(" ", width-leftW-rightW) + right
}

func indented(s string) string {
	return strings.Repeat(" ", previewBodyIndent) + s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
