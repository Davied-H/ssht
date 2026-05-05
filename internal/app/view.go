package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dong/ssh-config-tmux-tui/internal/sshconfig"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	liveStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
)

const (
	footerHint          = "/ filter · Tab focus · [/] group · ↵ connect · v expand · M monitor · e edit · A add · ? help"
	previewMinWidth     = 26
	previewBaseWidth    = 50
	previewMaxWidth     = 80
	listMinWidth        = 36
	twoColumnMinWidth   = 70
	previewLabelWidth   = 9
	sidebarMinWidth     = 14
	sidebarMaxWidth     = 18
	threeColumnMinWidth = 96
)

func (m Model) View() string {
	if m.showHelp {
		return m.fitToWindow(m.helpView())
	}
	if m.mode == modeForm {
		return m.fitToWindow(m.formView())
	}
	if m.mode == modeConfirm {
		return m.fitToWindow(m.confirmView())
	}
	if m.mode == modeGroupInline {
		return m.fitToWindow(m.groupInlineView())
	}

	return m.browseView()
}

func (m Model) groupInlineView() string {
	var b strings.Builder
	switch m.groupInline.action {
	case groupInlineCreate:
		b.WriteString(titleStyle.Render("Create group"))
	case groupInlineRename:
		b.WriteString(titleStyle.Render("Rename group " + m.groupInline.target))
	default:
		b.WriteString(titleStyle.Render("Group"))
	}
	b.WriteString("\n\nName: ")
	b.WriteString(m.groupInline.buffer)
	b.WriteString(selectedStyle.Render("▏"))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Enter confirm | Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.status))
	}
	return b.String()
}

func (m Model) browseView() string {
	footerLines := []string{mutedStyle.Render(footerHint)}
	if m.status != "" {
		footerLines = append(footerLines, errorStyle.Render(m.status))
	}

	contentHeight := maxViewHeight
	if m.height > 0 {
		contentHeight = m.height - len(footerLines)
	}
	lines := m.browseLines(contentHeight)
	lines = fitLines(lines, contentHeight)
	lines = append(lines, footerLines...)
	return strings.Join(truncateLines(lines, m.width), "\n")
}

func (m Model) browseLines(maxHeight int) []string {
	lines := []string{
		m.topStatusLine(),
		m.filterLine(),
		"",
	}

	bodyHeight := remainingLines(maxHeight, len(lines))
	if bodyHeight <= 0 {
		return lines
	}

	if len(m.entries) == 0 {
		return append(lines, "  "+mutedStyle.Render("No SSH Host entries found. Check "+m.configPath))
	}
	if len(m.filtered) == 0 {
		return append(lines, "  "+mutedStyle.Render("No hosts match the current filter."))
	}

	sidebarWidth, listWidth, previewWidth := m.splitThreeColumns()
	listRows := m.listLines(bodyHeight, listWidth)

	switch {
	case sidebarWidth == 0 && previewWidth == 0:
		return append(lines, listRows...)
	case sidebarWidth == 0:
		previewRows := m.previewLines(bodyHeight, previewWidth)
		return append(lines, joinColumns(listRows, previewRows, listWidth)...)
	case previewWidth == 0:
		sidebarRows := m.sidebarLines(bodyHeight, sidebarWidth)
		return append(lines, joinColumns(sidebarRows, listRows, sidebarWidth)...)
	default:
		sidebarRows := m.sidebarLines(bodyHeight, sidebarWidth)
		previewRows := m.previewLines(bodyHeight, previewWidth)
		return append(lines, joinThreeColumns(sidebarRows, listRows, previewRows, sidebarWidth, listWidth)...)
	}
}

func (m Model) formView() string {
	var b strings.Builder
	title := "Add Host"
	if m.form.operation == operationEdit {
		title = "Edit Host"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	values := formFieldValues(m.form.values)
	for i, label := range formFields() {
		line := fmt.Sprintf("  %-12s %s", label+":", values[i])
		if i == m.form.field {
			line = selectedStyle.Render("> " + strings.TrimSpace(line))
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Up/Down move | type edit | Backspace delete | Ctrl+S review save | Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.status))
	}
	return b.String()
}

func (m Model) confirmView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Confirm"))
	b.WriteString("\n\n")
	b.WriteString("Operation: ")
	b.WriteString(operationLabel(m.pending.operation))
	b.WriteByte('\n')

	switch m.pending.operation {
	case operationGroupRename, operationGroupMerge:
		b.WriteString("From: ")
		b.WriteString(m.pending.groupFrom)
		b.WriteByte('\n')
		b.WriteString("To:   ")
		b.WriteString(m.pending.groupTo)
		b.WriteByte('\n')
	case operationGroupDelete:
		b.WriteString("Group: ")
		b.WriteString(m.pending.groupFrom)
		b.WriteByte('\n')
		b.WriteString(mutedStyle.Render("(member hosts will move to ungrouped)"))
		b.WriteByte('\n')
	case operationGroupMoveHosts:
		b.WriteString("Target group: ")
		b.WriteString(m.pending.groupTo)
		b.WriteByte('\n')
		b.WriteString(fmt.Sprintf("Moving %d host(s):", len(m.pending.movingHosts)))
		b.WriteByte('\n')
		for _, host := range m.pending.movingHosts {
			b.WriteString("  - ")
			b.WriteString(host.Alias)
			b.WriteByte('\n')
		}
	default:
		b.WriteString("Target: ")
		b.WriteString(m.pending.target)
		b.WriteByte('\n')
		if m.pending.entry.Alias != "" {
			b.WriteString("Current: ")
			b.WriteString(m.pending.entry.Alias)
			b.WriteByte('\n')
		}
		if m.pending.operation != operationDelete {
			b.WriteString("Alias: ")
			b.WriteString(m.pending.values.Alias)
			b.WriteByte('\n')
		}
	}

	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("s confirm | Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.status))
	}
	return b.String()
}

func (m Model) topStatusLine() string {
	counts := m.dashboardCounts()
	var b strings.Builder
	b.WriteString(titleStyle.Render("ssht"))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Hosts %d", counts.Hosts)))
	if counts.Matched != counts.Hosts {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  Matched %d", counts.Matched)))
	}
	if counts.Favorites > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ★ %d", counts.Favorites)))
	}
	if counts.Recent > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ⏱ %d", counts.Recent)))
	}
	if counts.Selected > 0 {
		b.WriteString("  ")
		b.WriteString(selectedStyle.Render(fmt.Sprintf("✓ %d", counts.Selected)))
	}
	if counts.Warnings > 0 {
		b.WriteString("  ")
		b.WriteString(errorStyle.Render(fmt.Sprintf("⚠ %d", counts.Warnings)))
	}
	return b.String()
}

func (m Model) filterLine() string {
	prompt := mutedStyle.Render("/ ")
	if m.filter == "" {
		return prompt + mutedStyle.Render("filter…")
	}
	return prompt + m.filter + selectedStyle.Render("▏")
}

func (m Model) splitColumns() (left, right int) {
	if m.width <= 0 {
		return 0, 0
	}
	available := m.width - 4 // 2 indent + 2 gap
	if available < listMinWidth+previewMinWidth || m.width < twoColumnMinWidth {
		return m.width - 2, 0
	}
	rightCap := previewBaseWidth
	if m.width > 100 {
		// On wide terminals, let the preview grow up to ~45% of usable width,
		// capped at previewMaxWidth so the list keeps room to breathe.
		grown := available * 9 / 20
		if grown > rightCap {
			rightCap = grown
		}
		if rightCap > previewMaxWidth {
			rightCap = previewMaxWidth
		}
	}
	leftW := available * 11 / 20 // ~55%
	if leftW < listMinWidth {
		leftW = listMinWidth
	}
	rightW := available - leftW
	if rightW > rightCap {
		rightW = rightCap
		leftW = available - rightW
	}
	if rightW < previewMinWidth {
		rightW = previewMinWidth
		leftW = available - rightW
	}
	return leftW, rightW
}

func (m Model) splitThreeColumns() (sidebarW, listW, previewW int) {
	if m.width <= 0 {
		return 0, 0, 0
	}
	sidebarW = m.computeSidebarWidth()
	if sidebarW > 0 && m.width >= threeColumnMinWidth {
		available := m.width - 6 // 2 indent + 2 gap (sidebar|list) + 2 gap (list|preview)
		remaining := available - sidebarW
		if remaining >= listMinWidth+previewMinWidth {
			rightCap := previewBaseWidth
			if m.width > 100 {
				grown := remaining * 9 / 20
				if grown > rightCap {
					rightCap = grown
				}
				if rightCap > previewMaxWidth {
					rightCap = previewMaxWidth
				}
			}
			listW = remaining * 11 / 20
			if listW < listMinWidth {
				listW = listMinWidth
			}
			previewW = remaining - listW
			if previewW > rightCap {
				previewW = rightCap
				listW = remaining - previewW
			}
			if previewW < previewMinWidth {
				previewW = previewMinWidth
				listW = remaining - previewW
			}
			return sidebarW, listW, previewW
		}
	}
	left, right := m.splitColumns()
	return 0, left, right
}

func (m Model) computeSidebarWidth() int {
	items := m.groupItems()
	if len(items) == 0 {
		return 0
	}
	longest := 0
	for _, item := range items {
		candidate := lipgloss.Width(item.Name) + 1 + countDigits(item.Count) + 2 // "▸ name 12"
		if candidate > longest {
			longest = candidate
		}
	}
	if longest < sidebarMinWidth {
		longest = sidebarMinWidth
	}
	if longest > sidebarMaxWidth {
		longest = sidebarMaxWidth
	}
	return longest
}

func countDigits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

func (m Model) sidebarLines(maxLines, width int) []string {
	items := m.groupItems()
	if maxLines <= 0 || width <= 0 {
		return nil
	}
	selected := m.selectedGroup()
	lines := make([]string, 0, len(items))
	for _, item := range items {
		marker := "  "
		if item.Name == selected {
			marker = "▸ "
		}
		countStr := fmt.Sprintf("%d", item.Count)
		nameWidth := width - 2 - 1 - lipgloss.Width(countStr)
		if nameWidth < 1 {
			nameWidth = 1
		}
		name := truncate(item.Name, nameWidth)
		row := marker + padRight(name, nameWidth) + " " + countStr
		switch {
		case m.focus == focusSidebar && item.Name == selected:
			row = selectedStyle.Render(row)
		case item.Name == selected:
			row = accentStyle.Render(row)
		default:
			row = mutedStyle.Render(row)
		}
		lines = append(lines, row)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lines
}

func joinThreeColumns(left, mid, right []string, leftWidth, midWidth int) []string {
	rows := len(left)
	if len(mid) > rows {
		rows = len(mid)
	}
	if len(right) > rows {
		rows = len(right)
	}
	out := make([]string, 0, rows)
	leftPad := strings.Repeat(" ", leftWidth)
	midPad := strings.Repeat(" ", midWidth)
	for i := 0; i < rows; i++ {
		l, mi, r := "", "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(mid) {
			mi = mid[i]
		}
		if i < len(right) {
			r = right[i]
		}
		l = fitColumnWidth(l, leftWidth, leftPad)
		mi = fitColumnWidth(mi, midWidth, midPad)
		out = append(out, l+"  "+mi+"  "+r)
	}
	return out
}

func fitColumnWidth(s string, width int, pad string) string {
	visible := lipgloss.Width(s)
	if visible > width {
		return ansi.Truncate(s, width, "…")
	}
	if visible < width {
		return s + pad[:width-visible]
	}
	return s
}

func (m Model) listLines(maxLines, width int) []string {
	if maxLines <= 0 {
		return nil
	}
	rowBudget := maxLines
	hasHidden := len(m.filtered) > rowBudget
	if hasHidden && rowBudget > 1 {
		rowBudget--
	}
	start := 0
	if len(m.filtered) > rowBudget {
		start = m.cursor - rowBudget/2
		if start < 0 {
			start = 0
		}
		if maxStart := len(m.filtered) - rowBudget; start > maxStart {
			start = maxStart
		}
	}
	end := start + rowBudget
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	lines := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		lines = append(lines, m.formatListRow(i, width))
	}
	hidden := len(m.filtered) - (end - start)
	if hidden > 0 && len(lines) < maxLines {
		lines = append(lines, "  "+mutedStyle.Render(fmt.Sprintf("… %d more", hidden)))
	}
	return lines
}

func (m Model) formatListRow(i, width int) string {
	entry := m.filtered[i]
	markers := m.entryMarkers(entry)
	connection := connectionString(entry)

	prefix := "  "
	if i == m.cursor {
		prefix = "> "
	}

	aliasReserve := 24
	connectionReserve := 0
	if width > 0 {
		// total = len(prefix=2) + len(markers=3) + 1 + alias + 1 + connection
		base := 2 + 3 + 1 + 1 // prefix + markers + 2 spaces
		fluid := width - base
		if fluid < 8 {
			aliasReserve = fluid
			connectionReserve = 0
		} else {
			aliasReserve = fluid * 5 / 9
			if aliasReserve < 16 {
				aliasReserve = 16
			}
			connectionReserve = fluid - aliasReserve
		}
	}

	alias := padRight(truncate(entry.Alias, aliasReserve), aliasReserve)
	connStr := ""
	if connectionReserve > 0 && connection != "" {
		connStr = " " + mutedStyle.Render(truncate(connection, connectionReserve))
	}

	row := prefix + markers + " " + alias + connStr
	if i == m.cursor {
		// re-apply highlight only to the alias (markers and conn stay subtle)
		row = selectedStyle.Render("> "+markers+" "+alias) + connStr
	}
	return row
}

func formatField(label, value string, width int) string {
	labelText := padRight(label, previewLabelWidth)
	valueWidth := width - previewLabelWidth - 2
	if valueWidth <= 0 {
		return mutedStyle.Render(labelText)
	}
	return mutedStyle.Render(labelText) + "  " + truncate(value, valueWidth)
}

func joinColumns(left, right []string, leftWidth int) []string {
	rows := len(left)
	if len(right) > rows {
		rows = len(right)
	}
	out := make([]string, 0, rows)
	pad := strings.Repeat(" ", leftWidth)
	for i := 0; i < rows; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		visible := lipgloss.Width(l)
		if visible > leftWidth {
			l = ansi.Truncate(l, leftWidth, "…")
		} else if visible < leftWidth {
			l = l + pad[:leftWidth-visible]
		}
		out = append(out, l+"  "+r)
	}
	return out
}

func remainingLines(maxHeight, used int) int {
	if maxHeight == maxViewHeight {
		return maxViewHeight
	}
	return maxHeight - used
}

func fitLines(lines []string, maxLines int) []string {
	if maxLines == maxViewHeight || len(lines) <= maxLines {
		return lines
	}
	if maxLines <= 0 {
		return nil
	}
	if maxLines == 1 {
		return []string{mutedStyle.Render(fmt.Sprintf("… %d more", len(lines)))}
	}
	fitted := append([]string(nil), lines[:maxLines-1]...)
	fitted = append(fitted, mutedStyle.Render(fmt.Sprintf("… %d more", len(lines)-len(fitted))))
	return fitted
}

func truncateLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	truncated := make([]string, 0, len(lines))
	for _, line := range lines {
		truncated = append(truncated, ansi.Truncate(line, width, "…"))
	}
	return truncated
}

func (m Model) fitToWindow(view string) string {
	lines := strings.Split(view, "\n")
	if m.height > 0 {
		lines = fitLines(lines, m.height)
	}
	return strings.Join(truncateLines(lines, m.width), "\n")
}

const maxViewHeight = int(^uint(0) >> 1)

func (m Model) helpView() string {
	return strings.Join([]string{
		titleStyle.Render("ssht help"),
		"",
		"Type to filter hosts.",
		"Enter  Open the selected host using the configured terminal mode.",
		"Space  Mark or unmark a host for batch connect.",
		"f      Toggle favorite for the selected host.",
		"v      Toggle expanded preview (full top processes, raw config, all tags).",
		"M      Toggle the SSH monitoring panel (Health + Top CPU).",
		"Tab    Toggle focus between host list and group sidebar.",
		"[/]    Move between groups (works in either focus).",
		"e      Edit the selected host.",
		"A      Add a new host.",
		"d      Delete the selected host.",
		"r      Reload SSH config.",
		"?      Toggle this help.",
		"q/Esc  Quit.",
		"",
		titleStyle.Render("Sidebar (Tab to focus)"),
		"j/k    Move group cursor.",
		"Enter  Confirm and switch focus back to host list.",
		"a      Create an empty group placeholder.",
		"r      Rename the current group (rewrites SSH config comments).",
		"m      Merge the current group into another group.",
		"d      Delete the current group (its hosts move to ungrouped).",
		"M      Move marked hosts (Space) into the current group.",
		"J/K    Shift the current group down/up in the saved order.",
		"Esc    Return focus to host list.",
		"",
		titleStyle.Render("Form Group field"),
		"Tab    Cycle through known group names while in the Group field.",
		"",
		mutedStyle.Render("Use tag:<name>, fav:, and recent: in search. Connection command: ssh <alias>; OpenSSH resolves the full config."),
	}, "\n")
}

func (m Model) entryMarkers(entry sshconfig.HostEntry) string {
	markers := []rune{' ', ' ', ' '}
	if m.selected[entry.Alias] {
		markers[0] = '✓'
	}
	hostState := m.state.Hosts[entry.Alias]
	if hostState.Favorite {
		markers[1] = '★'
	}
	if hostState.LastConnectedAt != "" {
		markers[2] = '●'
	}
	return string(markers)
}

func formFieldValues(form sshconfig.HostForm) []string {
	return []string{
		form.Alias,
		form.Group,
		form.HostName,
		form.User,
		form.Port,
		form.IdentityFile,
		form.ProxyJump,
		form.ProxyCommand,
		strings.Join(form.Tags, " "),
	}
}

func operationLabel(operation operationType) string {
	switch operation {
	case operationAdd:
		return "add"
	case operationEdit:
		return "edit"
	case operationDelete:
		return "delete"
	case operationGroupRename:
		return "rename group"
	case operationGroupMerge:
		return "merge group"
	case operationGroupDelete:
		return "delete group"
	case operationGroupMoveHosts:
		return "move hosts to group"
	default:
		return "unknown"
	}
}

func connectionString(entry sshconfig.HostEntry) string {
	parts := []string{}
	if entry.User != "" {
		parts = append(parts, entry.User+"@")
	}
	if entry.HostName != "" {
		parts = append(parts, entry.HostName)
	}
	if entry.Port != "" {
		parts = append(parts, ":"+entry.Port)
	}
	return strings.Join(parts, "")
}

func padRight(value string, width int) string {
	visible := lipgloss.Width(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func shortTime(value string) string {
	// Preserve a date-like prefix; full ISO-8601 would dominate the line.
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}
