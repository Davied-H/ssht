package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dong/ssht/internal/sshconfig"
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
	rowFocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

const (
	footerPrimary       = "↵ connect · / search · g move · ←/→ group · Space mark · e edit · A add"
	footerSecondary     = "Tab focus · ←/→ group · P preview · v expand · M monitor · r reload · ? help"
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
	if m.mode == modeGroupPicker {
		return m.fitToWindow(m.groupPickerView())
	}
	if m.mode == modeWarnings {
		return m.fitToWindow(m.warningsView())
	}

	return m.browseView()
}

func panelTitle(title string) string {
	return accentStyle.Render("▎ ") + titleStyle.Render(title)
}

func sectionLabel(label string) string {
	return dimStyle.Render(strings.ToUpper(label))
}

func statusChip(label string, value int, style lipgloss.Style) string {
	return dimStyle.Render(label+" ") + style.Render(fmt.Sprintf("%d", value))
}

func keyHints(hints ...string) string {
	styled := make([]string, 0, len(hints))
	for _, hint := range hints {
		styled = append(styled, mutedStyle.Render(hint))
	}
	return strings.Join(styled, dimStyle.Render(" · "))
}

func focusRow(row string) string {
	return rowFocusStyle.Render(row)
}

func infoRow(label, value string) string {
	return "  " + mutedStyle.Render(padRight(label, 10)) + "  " + value
}

func inputRow(label, value string) string {
	return "  " + mutedStyle.Render(padRight(label, 10)) + "  " + value
}

func renderCursorValue(value string, cursor int) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	return string(runes[:cursor]) + selectedStyle.Render("▏") + string(runes[cursor:])
}

func statusLine(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return ""
	}
	style := errorStyle
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "saved"), strings.Contains(lower, "opened"), strings.Contains(lower, "refreshing"), lower == "monitor on", lower == "monitor off", lower == "preview on", lower == "preview off":
		style = liveStyle
	case strings.Contains(lower, "warning"), strings.Contains(lower, "stale"):
		style = warnStyle
	}
	return style.Render("status ") + mutedStyle.Render(status)
}

func (m Model) footerLines() []string {
	primary := footerPrimary
	secondary := footerSecondary
	if m.searchActive {
		primary = "type search · Enter done · Esc done · Ctrl+U clear"
		secondary = ""
	} else if m.focus == focusSidebar {
		primary = "j/k move · Enter list · a create · r rename · m merge · d delete"
		secondary = "/ search · P preview · M move marked here · J/K reorder · Esc list · ? help"
	} else if len(m.selected) > 0 {
		primary = "↵ connect marked · Space unmark · g move marked · / search · W warnings"
	} else if m.filter != "" {
		primary = "↵ connect · / search · g group · Space mark · e edit"
	} else if m.monitor != nil && m.monitorVisible {
		secondary = "R refresh monitor · P preview · v expand · Tab focus · r reload · W warnings · ? help"
	}
	lines := []string{keyHints(strings.Split(primary, " · ")...)}
	if secondary != "" && (m.width <= 0 || m.width >= 96) {
		lines = append(lines, keyHints(strings.Split(secondary, " · ")...))
	}
	return lines
}

func (m Model) groupInlineView() string {
	var b strings.Builder
	switch m.groupInline.action {
	case groupInlineCreate:
		b.WriteString(panelTitle("Create group"))
	case groupInlineRename:
		b.WriteString(panelTitle("Rename group"))
		b.WriteString("\n")
		b.WriteString(infoRow("Current", m.groupInline.target))
	default:
		b.WriteString(panelTitle("Group"))
	}
	b.WriteString("\n\n")
	b.WriteString(inputRow("Name", m.groupInline.buffer))
	b.WriteString(selectedStyle.Render("▏"))
	b.WriteString("\n\n")
	b.WriteString(keyHints("Enter confirm", "Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusLine(m.status))
	}
	return b.String()
}

func (m Model) groupPickerView() string {
	var b strings.Builder
	candidates := m.groupPickerCandidates()
	buffer := strings.TrimSpace(m.groupPicker.buffer)

	b.WriteString(panelTitle("Move to group"))
	b.WriteString("\n\n")
	moving := fmt.Sprintf("%d host(s)", len(m.groupPicker.movingHosts))
	if len(m.groupPicker.movingHosts) == 1 {
		moving += " · " + m.groupPicker.movingHosts[0].Alias
	}
	b.WriteString(infoRow("Moving", moving))
	b.WriteString("\n")
	b.WriteString(inputRow("Group", m.groupPicker.buffer))
	b.WriteString(selectedStyle.Render("▏"))
	b.WriteString("\n\n")

	if len(candidates) > 0 {
		b.WriteString(sectionLabel("Groups"))
		b.WriteByte('\n')
		for i, name := range candidates {
			if i >= 8 {
				b.WriteString(mutedStyle.Render(fmt.Sprintf("  … %d more", len(candidates)-i)))
				b.WriteByte('\n')
				break
			}
			row := "  " + padRight(name, 20)
			if i == m.groupPicker.cursor {
				row = focusRow("› " + padRight(name, 20))
			}
			b.WriteString(row)
			b.WriteByte('\n')
		}
	} else {
		b.WriteString(mutedStyle.Render("No existing groups match."))
		b.WriteByte('\n')
	}
	if buffer != "" && !m.isKnownGroup(buffer) {
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("New group: " + buffer))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(keyHints("Enter review", "↑/↓ choose", "type new/filter", "Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusLine(m.status))
	}
	return b.String()
}

func (m Model) warningsView() string {
	var b strings.Builder
	b.WriteString(panelTitle("Warnings"))
	b.WriteString("\n\n")
	if len(m.warnings) == 0 {
		b.WriteString(mutedStyle.Render("No parser warnings."))
	} else {
		for i, warning := range m.warnings {
			label := fmt.Sprintf("%d.", i+1)
			b.WriteString(warnStyle.Render(padRight(label, 3)))
			b.WriteString(" ")
			b.WriteString(warning.Error())
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n")
	b.WriteString(keyHints("Esc close", "Enter close", "W close", "q quit"))
	return b.String()
}

func (m Model) browseView() string {
	footerLines := m.footerLines()
	if m.status != "" {
		footerLines = append(footerLines, statusLine(m.status))
	}

	contentHeight := maxViewHeight
	if m.height > 0 {
		contentHeight = m.height - len(footerLines)
	}
	lines := m.browseLines(contentHeight)
	lines = fitLines(lines, contentHeight)
	if m.height > 0 && len(lines) < contentHeight {
		lines = append(lines, make([]string, contentHeight-len(lines))...)
	}
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
	b.WriteString(panelTitle(title))
	b.WriteString("\n\n")
	values := formFieldValues(m.form.values)
	for i, label := range formFields() {
		value := values[i]
		if i == m.form.field {
			value = renderCursorValue(value, m.form.cursor)
		}
		line := inputRow(label, value)
		if i == m.form.field {
			line = focusRow("› " + strings.TrimSpace(line))
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(keyHints("↑/↓ move", "←/→ cursor", "Ctrl+U clear", "Ctrl+S review", "Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusLine(m.status))
	}
	return b.String()
}

func (m Model) confirmView() string {
	var b strings.Builder
	b.WriteString(panelTitle("Confirm"))
	b.WriteString("\n\n")
	b.WriteString(infoRow("Operation", operationLabel(m.pending.operation)))
	b.WriteByte('\n')

	switch m.pending.operation {
	case operationGroupRename, operationGroupMerge:
		b.WriteString(infoRow("From", m.pending.groupFrom))
		b.WriteByte('\n')
		b.WriteString(infoRow("To", m.pending.groupTo))
		b.WriteByte('\n')
	case operationGroupDelete:
		b.WriteString(infoRow("Group", warnStyle.Render(m.pending.groupFrom)))
		b.WriteByte('\n')
		b.WriteString(mutedStyle.Render("(member hosts will move to ungrouped)"))
		b.WriteByte('\n')
	case operationGroupMoveHosts:
		b.WriteString(infoRow("Target", m.pending.groupTo))
		b.WriteByte('\n')
		b.WriteString(sectionLabel(fmt.Sprintf("Moving %d host(s)", len(m.pending.movingHosts))))
		b.WriteByte('\n')
		for _, host := range m.pending.movingHosts {
			b.WriteString("  - ")
			b.WriteString(host.Alias)
			b.WriteByte('\n')
		}
	default:
		b.WriteString(infoRow("Target", m.pending.target))
		b.WriteByte('\n')
		if m.pending.entry.Alias != "" {
			b.WriteString(infoRow("Current", m.pending.entry.Alias))
			b.WriteByte('\n')
		}
		if m.pending.operation != operationDelete {
			b.WriteString(infoRow("Alias", m.pending.values.Alias))
			b.WriteByte('\n')
		}
	}

	b.WriteString("\n")
	b.WriteString(keyHints("s confirm", "Esc cancel"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusLine(m.status))
	}
	return b.String()
}

func (m Model) topStatusLine() string {
	counts := m.dashboardCounts()
	parts := []string{
		titleStyle.Render("ssht"),
		dimStyle.Render("group ") + accentStyle.Render(m.selectedGroup()),
		statusChip("Hosts", counts.Hosts, mutedStyle),
	}
	if counts.Matched != counts.Hosts {
		parts = append(parts, statusChip("Matched", counts.Matched, accentStyle))
	}
	if counts.Favorites > 0 {
		parts = append(parts, statusChip("Fav", counts.Favorites, mutedStyle))
	}
	if counts.Recent > 0 {
		parts = append(parts, statusChip("Recent", counts.Recent, mutedStyle))
	}
	if counts.Selected > 0 {
		value := fmt.Sprintf("%d", counts.Selected)
		if counts.VisibleSelected != counts.Selected {
			value = fmt.Sprintf("%d/%d visible", counts.VisibleSelected, counts.Selected)
		}
		parts = append(parts, dimStyle.Render("Marked ")+selectedStyle.Render(value))
	}
	if counts.Warnings > 0 {
		parts = append(parts, statusChip("⚠", counts.Warnings, errorStyle))
	}
	return strings.Join(parts, dimStyle.Render("  "))
}

func (m Model) filterLine() string {
	if m.searchActive {
		prompt := selectedStyle.Render("SEARCH ")
		value := m.filter
		if value == "" {
			value = mutedStyle.Render("tag:<name> · fav: · recent:")
		}
		return accentStyle.Render("▎ ") + prompt + value + selectedStyle.Render("▏") + dimStyle.Render("  Enter/Esc done")
	}
	prompt := accentStyle.Render("/ search ")
	if m.filter == "" {
		return prompt + mutedStyle.Render("shortcuts active · tag:<name> · fav: · recent:")
	}
	return prompt + m.filter + dimStyle.Render("  matched")
}

func (m Model) splitColumns() (left, right int) {
	if m.width <= 0 {
		return 0, 0
	}
	if !m.previewVisible {
		return m.width - 2, 0
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
	if !m.previewVisible {
		if sidebarW > 0 && m.width >= threeColumnMinWidth {
			listW = m.width - sidebarW - 2
			if listW >= listMinWidth {
				return sidebarW, listW, 0
			}
		}
		return 0, m.width - 2, 0
	}
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
	longest := lipgloss.Width("groups")
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
	lines := make([]string, 0, len(items)+1)
	title := sectionLabel("groups")
	if m.focus == focusSidebar {
		title = accentStyle.Render("groups") + dimStyle.Render(" · focus")
	}
	lines = append(lines, truncate(title, width))

	rowBudget := maxLines - len(lines)
	if rowBudget <= 0 {
		return fitLines(lines, maxLines)
	}
	start := 0
	if len(items) > rowBudget {
		for i, item := range items {
			if item.Name == selected {
				start = i - rowBudget/2
				break
			}
		}
		if start < 0 {
			start = 0
		}
		if maxStart := len(items) - rowBudget; start > maxStart {
			start = maxStart
		}
	}
	end := start + rowBudget
	if end > len(items) {
		end = len(items)
	}
	for _, item := range items[start:end] {
		marker := "  "
		if item.Name == selected {
			marker = "› "
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
	lines := []string{}
	if width >= 28 {
		lines = append(lines, m.listHeader(width))
	}
	rowBudget := maxLines - len(lines)
	if rowBudget <= 0 {
		return fitLines(lines, maxLines)
	}
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

	for i := start; i < end; i++ {
		lines = append(lines, m.formatListRow(i, width))
	}
	hidden := len(m.filtered) - (end - start)
	if hidden > 0 && len(lines) < maxLines {
		lines = append(lines, "  "+mutedStyle.Render(fmt.Sprintf("… %d more", hidden)))
	}
	return lines
}

func (m Model) listHeader(width int) string {
	if width <= 0 {
		return ""
	}
	header := "  " + dimStyle.Render("state") + " " + sectionLabel("host")
	if len(m.filtered) > 0 {
		pos := fmt.Sprintf("%d/%d", m.cursor+1, len(m.filtered))
		header = layoutLeftRight(header, dimStyle.Render(pos), width)
	}
	if lipgloss.Width(header) > width {
		return truncate(header, width)
	}
	return header
}

func (m Model) formatListRow(i, width int) string {
	entry := m.filtered[i]
	markers := m.entryMarkerCell(entry)
	connection := connectionString(entry)

	prefix := "  "
	if i == m.cursor {
		prefix = "› "
	}

	aliasReserve := 24
	connectionReserve := 0
	if width > 0 {
		// total = prefix + status cell + space + alias + space + connection.
		base := 2 + 5 + 1 + 1
		fluid := width - base
		if fluid < 8 {
			aliasReserve = max(fluid, 1)
			connectionReserve = 0
		} else {
			aliasReserve = fluid * 5 / 9
			if aliasReserve < 16 {
				aliasReserve = 16
			}
			connectionReserve = fluid - aliasReserve
		}
	}

	aliasText := truncate(entry.Alias, aliasReserve)
	alias := padRight(aliasText, aliasReserve)
	connStr := ""
	if connectionReserve > 0 && connection != "" {
		connStr = " " + mutedStyle.Render(truncate(connection, connectionReserve))
	}

	renderedAlias := titleStyle.Render(alias)
	if i != m.cursor {
		renderedAlias = titleStyle.Render(highlightSearchTerm(alias, m.filter))
	}
	row := prefix + markers + " " + renderedAlias + connStr
	if i == m.cursor {
		row = focusRow(prefix+markers+" "+highlightSearchTerm(alias, m.filter)) + connStr
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

func highlightSearchTerm(value, query string) string {
	terms := textSearchTerms(query)
	if len(terms) == 0 {
		return value
	}
	lower := strings.ToLower(value)
	for _, term := range terms {
		idx := strings.Index(lower, term)
		if idx < 0 {
			continue
		}
		runes := []rune(value)
		termRunes := []rune(term)
		runeIdx := len([]rune(lower[:idx]))
		end := runeIdx + len(termRunes)
		if end > len(runes) {
			end = len(runes)
		}
		return string(runes[:runeIdx]) + accentStyle.Render(string(runes[runeIdx:end])) + string(runes[end:])
	}
	if fuzzyHighlight := highlightFuzzyRunes(value, terms[0]); fuzzyHighlight != "" {
		return fuzzyHighlight
	}
	return value
}

func highlightFuzzyRunes(value, term string) string {
	if term == "" || !fuzzyContains(value, term) {
		return ""
	}
	termRunes := []rune(strings.ToLower(term))
	j := 0
	var b strings.Builder
	for _, r := range value {
		if j < len(termRunes) && []rune(strings.ToLower(string(r)))[0] == termRunes[j] {
			b.WriteString(accentStyle.Render(string(r)))
			j++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
		"/      Enter search mode. Typing filters only while search mode is active.",
		"Enter  Open the selected host using the configured terminal mode.",
		"Space  Mark or unmark a host for batch connect across filters/groups.",
		"f      Toggle favorite for the selected host.",
		"g      Move current/marked host(s) to a group (recommended).",
		"P      Toggle the right preview pane.",
		"v      Toggle expanded preview (full top processes, raw config, all tags).",
		"M      Toggle the SSH monitoring panel (Health + Top CPU).",
		"R      Refresh the selected host's monitor snapshot now.",
		"W      Show parser warnings.",
		"PgUp/PgDn/Home/End  Move quickly through the host list.",
		"Tab    Toggle focus between host list and group sidebar.",
		"←/→    Move between groups (also [ and ]).",
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
		"M      Move marked hosts (Space) into the current group (advanced; g is faster from the host list).",
		"J/K    Shift the current group down/up in the saved order.",
		"Esc    Return focus to host list.",
		"",
		titleStyle.Render("Form Group field"),
		"Tab    Cycle through known group names while in the Group field.",
		"Ctrl+U Clear the current field; Ctrl+A/Ctrl+E move to field start/end.",
		"",
		mutedStyle.Render("Use tag:<name>, fav:, and recent: in search mode. Connection command: ssh <alias>; OpenSSH resolves the full config."),
	}, "\n")
}

func (m Model) entryMarkerCell(entry sshconfig.HostEntry) string {
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
	rendered := make([]string, 0, len(markers))
	for i, marker := range markers {
		cell := string(marker)
		switch {
		case marker == ' ':
			rendered = append(rendered, dimStyle.Render("·"))
		case i == 0:
			rendered = append(rendered, selectedStyle.Render(cell))
		case i == 1:
			rendered = append(rendered, warnStyle.Render(cell))
		default:
			rendered = append(rendered, liveStyle.Render(cell))
		}
	}
	return strings.Join(rendered, "")
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
