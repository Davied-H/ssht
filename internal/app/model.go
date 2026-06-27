package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dong/ssht/internal/monitor"
	"github.com/dong/ssht/internal/sshconfig"
	"github.com/dong/ssht/internal/state"
	"github.com/dong/ssht/internal/terminal"
)

type Config struct {
	ConfigPath      string
	Entries         []sshconfig.HostEntry
	Warnings        []sshconfig.Warning
	Manager         terminal.Manager
	Load            func() ([]sshconfig.HostEntry, []sshconfig.Warning, error)
	StatePath       string
	State           state.Store
	Now             func() time.Time
	Monitor         *monitor.Cache
	Probe           monitor.ProbeFunc
	MonitorVisible  bool
	MonitorExplicit bool
}

type Model struct {
	configPath   string
	entries      []sshconfig.HostEntry
	filtered     []sshconfig.HostEntry
	warnings     []sshconfig.Warning
	filter       string
	searchActive bool
	cursor       int
	groupCursor  int
	status       string
	showHelp     bool
	mode         appMode
	form         formState
	pending      pendingOperation
	manager      terminal.Manager
	load         func() ([]sshconfig.HostEntry, []sshconfig.Warning, error)
	statePath    string
	state        state.Store
	selected     map[string]bool
	now          func() time.Time
	width        int
	height       int
	focus        focusKind
	groupInline  groupInlineState
	groupPicker  groupPickerState
	command      commandPaletteState
	settings     settingsState

	showRawPreview bool
	previewVisible bool
	density        string

	monitor        *monitor.Cache
	probe          monitor.ProbeFunc
	monitorVisible bool
}

type appMode int

const (
	modeBrowse appMode = iota
	modeForm
	modeConfirm
	modeGroupInline
	modeGroupPicker
	modeWarnings
	modeHistory
	modeCommandPalette
	modeSettings
)

type focusKind int

const (
	focusList focusKind = iota
	focusSidebar
)

type operationType int

const (
	operationAdd operationType = iota
	operationEdit
	operationDelete
	operationGroupRename
	operationGroupMerge
	operationGroupDelete
	operationGroupMoveHosts
	operationConnect
)

type formState struct {
	operation operationType
	entry     sshconfig.HostEntry
	values    sshconfig.HostForm
	field     int
	cursor    int
}

// groupInlineState holds the temporary input buffer for the lightweight
// modeGroupInline (used by sidebar group create / rename actions).
type groupInlineState struct {
	action groupInlineAction
	buffer string
	target string // the existing group name being renamed; empty when creating
}

type groupInlineAction int

const (
	groupInlineNone groupInlineAction = iota
	groupInlineCreate
	groupInlineRename
)

// groupPickerState holds the temporary target selection for list-level group
// moves. movingHosts is fixed when the picker opens so filter/cursor changes do
// not change the operation's source set.
type groupPickerState struct {
	buffer      string
	cursor      int
	movingHosts []sshconfig.HostEntry
}

type commandPaletteState struct {
	buffer string
	cursor int
}

type settingsState struct {
	field          int
	terminal       string
	openMode       string
	density        string
	previewVisible bool
	showRawPreview bool
	monitorVisible bool
}

type pendingOperation struct {
	operation   operationType
	entry       sshconfig.HostEntry
	values      sshconfig.HostForm
	target      string
	groupFrom   string
	groupTo     string
	movingHosts []sshconfig.HostEntry
}

type dashboardCounts struct {
	Hosts           int
	Matched         int
	Favorites       int
	Recent          int
	Selected        int
	VisibleSelected int
	Warnings        int
}

type groupItem struct {
	Name  string
	Count int
}

type commandEntry struct {
	ID          string
	Title       string
	Description string
}

type historyRow struct {
	Alias           string
	Favorite        bool
	ConnectCount    int
	LastConnectedAt string
}

type ReloadedMsg struct {
	Entries  []sshconfig.HostEntry
	Warnings []sshconfig.Warning
	Err      error
}

type ConnectMsg struct {
	Succeeded []string
	Failed    int
	Err       error
}

type stateSavedMsg struct {
	Err error
}

type monitorMsg struct {
	snap *monitor.Snapshot
}

func NewModel(cfg Config) Model {
	manager := cfg.Manager
	if manager.Runner == nil {
		manager = terminal.New()
	}
	store := cfg.State
	if store.Hosts == nil {
		store = state.NewStore()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	m := Model{
		configPath:     cfg.ConfigPath,
		entries:        cfg.Entries,
		warnings:       cfg.Warnings,
		manager:        manager,
		load:           cfg.Load,
		statePath:      cfg.StatePath,
		state:          store,
		selected:       map[string]bool{},
		now:            now,
		status:         statusFromWarnings(cfg.Warnings),
		previewVisible: true,
		density:        "comfortable",
		monitor:        cfg.Monitor,
		probe:          cfg.Probe,
		monitorVisible: cfg.MonitorVisible,
	}
	if cfg.State.Settings != nil {
		m.applySavedSettings(*cfg.State.Settings, cfg.MonitorExplicit)
	}
	m.applyFilter()
	return m
}

func (m Model) Init() tea.Cmd {
	if m.load == nil || len(m.warnings) > 0 {
		return nil
	}
	return m.reloadCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.update(msg)
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	nm, cmd := m.dispatch(msg)
	if probe := nm.maybeProbeCmd(); probe != nil {
		cmd = tea.Batch(cmd, probe)
	}
	return nm, cmd
}

func (m Model) dispatch(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case monitorMsg:
		if m.monitor != nil && msg.snap != nil {
			m.monitor.Put(msg.snap)
		}
		return m, nil
	case ReloadedMsg:
		if msg.Err != nil {
			m.status = msg.Err.Error()
			return m, nil
		}
		m.entries = msg.Entries
		m.warnings = msg.Warnings
		m.filter = ""
		m.cursor = 0
		m.status = statusFromWarnings(msg.Warnings)
		m.applyFilter()
		return m, nil
	case writeConfigMsg:
		if msg.Err != nil {
			m.status = msg.Err.Error()
			return m, nil
		}
		m.applyGroupStateAfterWrite(msg)
		m.mode = modeBrowse
		m.form = formState{}
		m.pending = pendingOperation{}
		m.groupPicker = groupPickerState{}
		m.selected = map[string]bool{}
		m.status = "config saved"
		return m, tea.Batch(m.reloadCmd(), m.saveStateCmd())
	case ConnectMsg:
		if msg.Err != nil {
			m.status = msg.Err.Error()
			return m, nil
		}
		for _, alias := range msg.Succeeded {
			m.state.RecordConnect(alias, m.now())
		}
		if len(msg.Succeeded) == 1 && msg.Failed == 0 {
			m.status = "opened ssh connection in " + m.manager.BackendName() + " " + m.manager.OpenModeName()
		} else if msg.Failed > 0 {
			m.status = fmt.Sprintf("opened %d connection; %d failed", len(msg.Succeeded), msg.Failed)
		} else {
			m.status = fmt.Sprintf("opened %d connections", len(msg.Succeeded))
		}
		m.selected = map[string]bool{}
		m.applyFilter()
		return m, m.saveStateCmd()
	case stateSavedMsg:
		if msg.Err != nil {
			m.status = msg.Err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode != modeBrowse {
			return m.handleModeKey(msg)
		}
		return m.handleKey(msg)
	case tea.MouseMsg:
		if m.mode == modeBrowse {
			return m.handleMouse(msg)
		}
	}
	return m, nil
}

func (m Model) Filter() string {
	return m.filter
}

func (m Model) FilteredEntries() []sshconfig.HostEntry {
	return append([]sshconfig.HostEntry(nil), m.filtered...)
}

func (m Model) Selected() (sshconfig.HostEntry, bool) {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return sshconfig.HostEntry{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) dashboardCounts() dashboardCounts {
	favorites := 0
	recent := 0
	visibleSelected := 0
	for _, entry := range m.entries {
		host := m.state.Hosts[entry.Alias]
		if host.Favorite {
			favorites++
		}
		if host.LastConnectedAt != "" {
			recent++
		}
	}
	for _, entry := range m.filtered {
		if m.selected[entry.Alias] {
			visibleSelected++
		}
	}
	return dashboardCounts{
		Hosts:           len(m.entries),
		Matched:         len(m.filtered),
		Favorites:       favorites,
		Recent:          recent,
		Selected:        len(m.selected),
		VisibleSelected: visibleSelected,
		Warnings:        len(m.warnings),
	}
}

func (m Model) handleKey(key tea.KeyMsg) (Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if m.searchActive {
		return m.handleSearchKey(key)
	}
	if m.focus == focusSidebar {
		return m.handleSidebarKey(key)
	}
	switch key.Type {
	case tea.KeyEsc:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, tea.Quit
	case tea.KeyTab:
		if len(m.groupItems()) > 0 {
			m.focus = focusSidebar
		}
		return m, nil
	case tea.KeyShiftTab:
		return m, nil
	case tea.KeyEnter:
		return m.prepareConnect()
	case tea.KeyCtrlK:
		return m.startCommandPalette(), nil
	case tea.KeyCtrlO:
		return m.startSettings(), nil
	case tea.KeyCtrlL:
		m.showHelp = !m.showHelp
	case tea.KeyCtrlY:
		m.mode = modeHistory
		m.status = ""
	case tea.KeyCtrlW:
		if len(m.warnings) == 0 {
			m.status = "no warnings"
			return m, nil
		}
		m.mode = modeWarnings
		m.status = ""
		return m, nil
	case tea.KeyCtrlA:
		m.mode = modeForm
		m.status = ""
		m.form = formState{
			operation: operationAdd,
			values:    sshconfig.HostForm{},
			cursor:    0,
		}
	case tea.KeyCtrlE:
		entry, ok := m.Selected()
		if !ok {
			m.status = "no host selected"
			return m, nil
		}
		values := formFromEntry(entry)
		m.mode = modeForm
		m.status = ""
		m.form = formState{
			operation: operationEdit,
			entry:     entry,
			values:    values,
			cursor:    formFieldLen(values, 0),
		}
	case tea.KeyCtrlG:
		return m.startGroupPicker()
	case tea.KeyCtrlD:
		entry, ok := m.Selected()
		if !ok {
			m.status = "no host selected"
			return m, nil
		}
		m.mode = modeConfirm
		m.status = ""
		m.pending = pendingOperation{
			operation: operationDelete,
			entry:     entry,
			target:    entry.SourceFile,
		}
	case tea.KeyCtrlR:
		return m, m.reloadCmd()
	case tea.KeyCtrlT:
		return m.refreshMonitor()
	case tea.KeyCtrlF:
		return m.toggleFavorite()
	case tea.KeyCtrlV:
		m.showRawPreview = !m.showRawPreview
		return m, nil
	case tea.KeyCtrlP:
		return m.togglePreview(), nil
	case tea.KeyCtrlN:
		if m.monitor != nil {
			m.monitorVisible = !m.monitorVisible
			if m.monitorVisible {
				m.status = "monitor on"
			} else {
				m.status = "monitor off"
			}
		}
		return m, nil
	case tea.KeySpace:
		return m.toggleSelected(), nil
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case tea.KeyPgUp:
		m.moveCursorBy(-m.listPageSize())
	case tea.KeyPgDown:
		m.moveCursorBy(m.listPageSize())
	case tea.KeyHome:
		m.cursor = 0
	case tea.KeyEnd:
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
	case tea.KeyLeft:
		m.moveGroup(-1)
	case tea.KeyRight:
		m.moveGroup(1)
	case tea.KeyRunes:
		if key.String() == "/" {
			return m.startSearch(), nil
		}
		if digit, ok := quickConnectDigit(key.String()); ok {
			return m.prepareQuickConnect(digit)
		}
		return m.startSearchWith(key.String()), nil
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.Action != tea.MouseActionPress {
		return m, nil
	}
	if event.Button == tea.MouseButtonWheelUp {
		if m.focus == focusSidebar {
			m.moveGroup(-1)
		} else {
			m.moveCursorBy(-1)
		}
		return m, nil
	}
	if event.Button == tea.MouseButtonWheelDown {
		if m.focus == focusSidebar {
			m.moveGroup(1)
		} else {
			m.moveCursorBy(1)
		}
		return m, nil
	}
	if event.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if event.Y == 1 {
		return m.startSearch(), nil
	}

	sidebarW, listW, previewW := m.splitThreeColumns()
	bodyY := event.Y - 2
	if bodyY < 1 {
		return m, nil
	}
	contentRow := bodyY - 1
	bodyHeight := m.mouseBodyHeight()
	if bodyHeight <= panelChromeHeight || contentRow < 0 || contentRow >= bodyHeight-panelChromeHeight {
		return m, nil
	}

	x := event.X
	switch {
	case sidebarW > 0 && x >= 0 && x < sidebarW:
		return m.clickSidebarRow(contentRow, bodyHeight), nil
	case sidebarW > 0 && x >= sidebarW+2 && x < sidebarW+2+listW:
		return m.clickListRow(contentRow, bodyHeight, listW), nil
	case sidebarW == 0 && x >= 0 && x < listW:
		return m.clickListRow(contentRow, bodyHeight, listW), nil
	case previewW > 0:
		return m, nil
	}
	return m, nil
}

func (m Model) mouseBodyHeight() int {
	footerLines := m.footerLines()
	if m.status != "" {
		footerLines = append(footerLines, statusLine(m.status))
	}
	contentHeight := maxViewHeight
	if m.height > 0 {
		contentHeight = m.height - len(footerLines)
	}
	return remainingLines(contentHeight, 2)
}

func (m Model) clickSidebarRow(contentRow, bodyHeight int) Model {
	items := m.groupItems()
	if len(items) == 0 {
		return m
	}
	rowBudget := bodyHeight - panelChromeHeight
	start, end := visibleGroupRange(items, m.selectedGroup(), rowBudget)
	index := start + contentRow
	if index < start || index >= end || index >= len(items) {
		return m
	}
	m.groupCursor = index
	m.focus = focusSidebar
	m.cursor = 0
	m.applyFilter()
	return m
}

func (m Model) clickListRow(contentRow, bodyHeight, panelWidth int) Model {
	if len(m.filtered) == 0 {
		return m
	}
	innerW := innerWidth(panelWidth)
	headerRows := 0
	if innerW >= 28 {
		headerRows = 1
	}
	rowBudget := bodyHeight - panelChromeHeight - headerRows
	start, visibleRows := visibleListRange(len(m.filtered), m.cursor, rowBudget)
	index := start + contentRow - headerRows
	if index < start || index >= start+visibleRows || index >= len(m.filtered) {
		m.focus = focusList
		return m
	}
	m.cursor = index
	m.focus = focusList
	m.searchActive = false
	return m
}

func (m Model) startSearch() Model {
	m.focus = focusList
	m.searchActive = true
	m.cursor = 0
	m.status = ""
	m.applyFilter()
	return m
}

func (m Model) startSearchWith(text string) Model {
	m = m.startSearch()
	m.filter += text
	m.cursor = 0
	m.applyFilter()
	return m
}

func (m Model) handleSearchKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyEnter:
		m.searchActive = false
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.filter) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.cursor = 0
			m.applyFilter()
		}
	case tea.KeyCtrlU:
		m.filter = ""
		m.cursor = 0
		m.applyFilter()
	case tea.KeyRunes:
		m.filter += key.String()
		m.cursor = 0
		m.applyFilter()
	}
	return m, nil
}

func (m Model) togglePreview() Model {
	m.previewVisible = !m.previewVisible
	if m.previewVisible {
		m.status = "preview on"
	} else {
		m.status = "preview off"
	}
	return m
}

// handleSidebarKey is invoked from handleKey when focus is on the sidebar
// (and we are still in modeBrowse). It scopes navigation and Ctrl-based group
// management shortcuts to the sidebar context.
func (m Model) handleSidebarKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyCtrlK:
		m.focus = focusList
		return m.startCommandPalette(), nil
	case tea.KeyCtrlO:
		m.focus = focusList
		return m.startSettings(), nil
	case tea.KeyCtrlL:
		m.showHelp = !m.showHelp
		return m, nil
	case tea.KeyCtrlY:
		m.mode = modeHistory
		m.status = ""
		return m, nil
	case tea.KeyCtrlW:
		if len(m.warnings) == 0 {
			m.status = "no warnings"
			return m, nil
		}
		m.mode = modeWarnings
		m.status = ""
		return m, nil
	case tea.KeyCtrlT:
		m.focus = focusList
		return m.refreshMonitor()
	case tea.KeyCtrlP:
		return m.togglePreview(), nil
	case tea.KeyCtrlA:
		m.mode = modeGroupInline
		m.groupInline = groupInlineState{action: groupInlineCreate}
		m.status = ""
		return m, nil
	case tea.KeyCtrlR:
		current := m.selectedGroup()
		if current == "all" || current == "ungrouped" {
			m.status = "cannot rename reserved group"
			return m, nil
		}
		m.mode = modeGroupInline
		m.groupInline = groupInlineState{action: groupInlineRename, target: current, buffer: current}
		m.status = ""
		return m, nil
	case tea.KeyCtrlB:
		current := m.selectedGroup()
		if current == "all" || current == "ungrouped" {
			m.status = "cannot merge reserved group"
			return m, nil
		}
		return m.startGroupMerge(current)
	case tea.KeyCtrlD:
		current := m.selectedGroup()
		if current == "all" || current == "ungrouped" {
			m.status = "cannot delete reserved group"
			return m, nil
		}
		return m.startGroupDelete(current)
	case tea.KeyCtrlG:
		current := m.selectedGroup()
		return m.startMoveMarkedHosts(current)
	case tea.KeyCtrlDown:
		return m.shiftGroupOrder(m.selectedGroup(), 1)
	case tea.KeyCtrlUp:
		return m.shiftGroupOrder(m.selectedGroup(), -1)
	case tea.KeyEsc:
		m.focus = focusList
		m.status = ""
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		m.focus = focusList
		return m, nil
	case tea.KeyEnter:
		m.focus = focusList
		return m, nil
	case tea.KeyUp:
		m.moveGroup(-1)
		return m, nil
	case tea.KeyDown:
		m.moveGroup(1)
		return m, nil
	case tea.KeyLeft:
		m.moveGroup(-1)
		return m, nil
	case tea.KeyRight:
		m.moveGroup(1)
		return m, nil
	case tea.KeyRunes:
		if key.String() == "/" {
			return m.startSearch(), nil
		}
		return m.startSearchWith(key.String()), nil
	}
	return m, nil
}

// startGroupMerge picks the next available group as a default merge target and
// queues a confirm panel.
func (m Model) startGroupMerge(from string) (Model, tea.Cmd) {
	target := ""
	for _, item := range m.groupItems() {
		if item.Name == "all" || item.Name == "ungrouped" {
			continue
		}
		if item.Name == from {
			continue
		}
		target = item.Name
		break
	}
	if target == "" {
		m.status = "no other group to merge into"
		return m, nil
	}
	m.mode = modeConfirm
	m.pending = pendingOperation{
		operation: operationGroupMerge,
		groupFrom: from,
		groupTo:   target,
	}
	m.status = ""
	return m, nil
}

func (m Model) startGroupDelete(name string) (Model, tea.Cmd) {
	m.mode = modeConfirm
	m.pending = pendingOperation{
		operation: operationGroupDelete,
		groupFrom: name,
	}
	m.status = ""
	return m, nil
}

func (m Model) startMoveMarkedHosts(target string) (Model, tea.Cmd) {
	if len(m.selected) == 0 {
		m.status = "no hosts marked (Space to mark)"
		return m, nil
	}
	if target == "all" {
		m.status = "pick a real group (not all)"
		return m, nil
	}
	m.mode = modeConfirm
	m.pending = pendingOperation{
		operation:   operationGroupMoveHosts,
		groupTo:     target,
		movingHosts: m.collectSelectedEntries(),
	}
	m.status = ""
	return m, nil
}

func (m Model) collectSelectedEntries() []sshconfig.HostEntry {
	out := make([]sshconfig.HostEntry, 0, len(m.selected))
	for _, entry := range m.entries {
		if m.selected[entry.Alias] {
			out = append(out, entry)
		}
	}
	return out
}

func (m Model) startGroupPicker() (Model, tea.Cmd) {
	hosts := m.collectSelectedEntries()
	if len(hosts) == 0 {
		entry, ok := m.Selected()
		if !ok {
			m.status = "no host selected"
			return m, nil
		}
		hosts = []sshconfig.HostEntry{entry}
	}
	m.mode = modeGroupPicker
	m.groupPicker = groupPickerState{movingHosts: hosts}
	m.status = ""
	return m, nil
}

// shiftGroupOrder moves `name` up (-1) or down (+1) in state.GroupOrder. If
// name (or any currently-displayed group) is not present yet, all displayed
// groups are first injected in their current order so the reordering is
// well-defined.
func (m Model) shiftGroupOrder(name string, delta int) (Model, tea.Cmd) {
	if name == "" || name == "all" || name == "ungrouped" {
		m.status = "cannot reorder reserved group"
		return m, nil
	}
	order := append([]string(nil), m.state.GroupOrder...)
	in := func(slice []string, target string) bool {
		for _, n := range slice {
			if n == target {
				return true
			}
		}
		return false
	}
	for _, item := range m.groupItems() {
		if item.Name == "all" || item.Name == "ungrouped" {
			continue
		}
		if !in(order, item.Name) {
			order = append(order, item.Name)
		}
	}
	idx := -1
	for i, n := range order {
		if n == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return m, nil
	}
	target := idx + delta
	if target < 0 || target >= len(order) {
		return m, nil
	}
	order[idx], order[target] = order[target], order[idx]
	m.state.GroupOrder = order
	m.applyFilter()
	return m, m.saveStateCmd()
}

func (m Model) handleModeKey(key tea.KeyMsg) (Model, tea.Cmd) {
	if m.mode == modeGroupInline {
		return m.handleGroupInlineKey(key)
	}
	if m.mode == modeGroupPicker {
		return m.handleGroupPickerKey(key)
	}
	if m.mode == modeWarnings {
		return m.handleWarningsKey(key)
	}
	if m.mode == modeHistory {
		return m.handleHistoryKey(key)
	}
	if m.mode == modeCommandPalette {
		return m.handleCommandPaletteKey(key)
	}
	if m.mode == modeSettings {
		return m.handleSettingsKey(key)
	}
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyCtrlA, tea.KeyHome:
		if m.mode == modeForm {
			m.form.cursor = 0
		}
	case tea.KeyCtrlE, tea.KeyEnd:
		if m.mode == modeForm {
			m.form.cursor = formFieldLen(m.form.values, m.form.field)
		}
	case tea.KeyCtrlU:
		if m.mode == modeForm {
			m.form.values = replaceFormField(m.form.values, m.form.field, "")
			m.form.cursor = 0
		}
	case tea.KeyCtrlS:
		if m.mode == modeForm {
			return m.prepareFormSave()
		}
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.form = formState{}
		m.pending = pendingOperation{}
		m.status = ""
		return m, nil
	case tea.KeyUp:
		if m.mode == modeForm && m.form.field > 0 {
			m.form.field--
			m.form.cursor = formFieldLen(m.form.values, m.form.field)
		}
	case tea.KeyDown:
		if m.mode == modeForm && m.form.field < len(formFields())-1 {
			m.form.field++
			m.form.cursor = formFieldLen(m.form.values, m.form.field)
		}
	case tea.KeyLeft:
		if m.mode == modeForm && m.form.cursor > 0 {
			m.form.cursor--
		}
	case tea.KeyRight:
		if m.mode == modeForm && m.form.cursor < formFieldLen(m.form.values, m.form.field) {
			m.form.cursor++
		}
	case tea.KeyTab:
		if m.mode == modeForm && m.form.field == formFieldGroupIndex() {
			m.form.values.Group = nextKnownGroup(m.form.values.Group, m.knownGroupSet())
			m.form.cursor = formFieldLen(m.form.values, m.form.field)
			return m, nil
		}
		if m.mode == modeForm && m.form.field < len(formFields())-1 {
			m.form.field++
			m.form.cursor = formFieldLen(m.form.values, m.form.field)
		}
	case tea.KeyBackspace, tea.KeyDelete:
		if m.mode == modeForm {
			if key.Type == tea.KeyBackspace {
				m.form.values, m.form.cursor = deleteBeforeFormCursor(m.form.values, m.form.field, m.form.cursor)
			} else {
				m.form.values = deleteAtFormCursor(m.form.values, m.form.field, m.form.cursor)
			}
		}
	case tea.KeyRunes:
		if key.String() == "s" && m.mode == modeConfirm {
			if m.pending.operation == operationConnect {
				m.mode = modeBrowse
				m.pending = pendingOperation{}
				m.status = ""
				return m, m.connectCmd()
			}
			return m, m.writePendingCmd()
		}
		if m.mode == modeForm {
			text := key.String()
			m.form.values, m.form.cursor = insertAtFormCursor(m.form.values, m.form.field, m.form.cursor, text)
		}
	}
	return m, nil
}

func (m Model) handleWarningsKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyEnter:
		m.mode = modeBrowse
		m.status = ""
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "W", "?":
			m.mode = modeBrowse
			m.status = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleHistoryKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyEnter:
		m.mode = modeBrowse
		m.status = ""
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "H", "?":
			m.mode = modeBrowse
			m.status = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleCommandPaletteKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.command = commandPaletteState{}
		m.status = ""
		return m, nil
	case tea.KeyEnter:
		return m.runCommandPaletteSelection()
	case tea.KeyUp:
		m.moveCommandCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCommandCursor(1)
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.command.buffer)
		if len(runes) > 0 {
			m.command.buffer = string(runes[:len(runes)-1])
			m.command.cursor = 0
		}
		return m, nil
	case tea.KeyCtrlU:
		m.command.buffer = ""
		m.command.cursor = 0
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "j":
			if m.command.buffer == "" {
				m.moveCommandCursor(1)
				return m, nil
			}
		case "k":
			if m.command.buffer == "" {
				m.moveCommandCursor(-1)
				return m, nil
			}
		}
		m.command.buffer += key.String()
		m.command.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m Model) handleSettingsKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.settings = settingsState{}
		m.status = ""
		return m, nil
	case tea.KeyEnter:
		return m.commitSettings()
	case tea.KeyUp:
		m.moveSettingsField(-1)
		return m, nil
	case tea.KeyDown:
		m.moveSettingsField(1)
		return m, nil
	case tea.KeyLeft:
		m.cycleSetting(-1)
		return m, nil
	case tea.KeyRight, tea.KeySpace:
		m.cycleSetting(1)
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "s":
			return m.commitSettings()
		case "j":
			m.moveSettingsField(1)
			return m, nil
		case "k":
			m.moveSettingsField(-1)
			return m, nil
		case "h":
			m.cycleSetting(-1)
			return m, nil
		case "l":
			m.cycleSetting(1)
			return m, nil
		}
	}
	return m, nil
}

func (m Model) startSettings() Model {
	m.mode = modeSettings
	m.status = ""
	m.settings = settingsStateFromModel(m)
	return m
}

func settingsStateFromModel(m Model) settingsState {
	terminalPreference := strings.TrimSpace(m.manager.Preference)
	if terminalPreference == "" {
		terminalPreference = "auto"
	}
	openMode := strings.TrimSpace(string(m.manager.OpenMode))
	if openMode == "" {
		openMode = string(terminal.OpenModeAuto)
	}
	return settingsState{
		terminal:       terminalPreference,
		openMode:       openMode,
		density:        m.density,
		previewVisible: m.previewVisible,
		showRawPreview: m.showRawPreview,
		monitorVisible: m.monitorVisible,
	}
}

func (m *Model) moveSettingsField(delta int) {
	m.settings.field += delta
	if m.settings.field < 0 {
		m.settings.field = len(settingFields()) - 1
	}
	if m.settings.field >= len(settingFields()) {
		m.settings.field = 0
	}
}

func (m *Model) cycleSetting(delta int) {
	switch settingFields()[m.settings.field].id {
	case "open-mode":
		m.settings.openMode = cycleStringOption(m.settings.openMode, settingOpenModes(), delta)
	case "terminal":
		m.settings.terminal = cycleStringOption(m.settings.terminal, settingTerminals(), delta)
	case "density":
		m.settings.density = cycleStringOption(m.settings.density, settingDensities(), delta)
	case "preview":
		m.settings.previewVisible = !m.settings.previewVisible
	case "raw-preview":
		m.settings.showRawPreview = !m.settings.showRawPreview
	case "monitor":
		m.settings.monitorVisible = !m.settings.monitorVisible
	}
}

func (m Model) commitSettings() (Model, tea.Cmd) {
	m.state.Settings = &state.Settings{
		OpenMode:       m.settings.openMode,
		Terminal:       m.settings.terminal,
		Density:        m.settings.density,
		PreviewVisible: m.settings.previewVisible,
		ShowRawPreview: m.settings.showRawPreview,
		MonitorVisible: m.settings.monitorVisible,
	}
	m.applySettingsState(m.settings)
	m.mode = modeBrowse
	m.settings = settingsState{}
	m.status = "settings saved"
	return m, m.saveStateCmd()
}

func (m *Model) applySavedSettings(settings state.Settings, monitorExplicit bool) {
	next := settingsStateFromModel(*m)
	if validSettingsValue(settings.OpenMode, settingOpenModes()) {
		next.openMode = settings.OpenMode
	}
	if validSettingsValue(settings.Terminal, settingTerminals()) {
		next.terminal = settings.Terminal
	}
	if validSettingsValue(settings.Density, settingDensities()) {
		next.density = settings.Density
	}
	next.previewVisible = settings.PreviewVisible
	next.showRawPreview = settings.ShowRawPreview
	if !monitorExplicit {
		next.monitorVisible = settings.MonitorVisible
	}
	m.applySettingsState(next)
}

func (m *Model) applySettingsState(settings settingsState) {
	m.manager = terminal.Manager{
		Runner:     m.manager.Runner,
		Preference: settings.terminal,
		OpenMode:   terminal.OpenMode(settings.openMode),
		Env:        m.manager.Env,
	}
	m.previewVisible = settings.previewVisible
	m.showRawPreview = settings.showRawPreview
	m.monitorVisible = settings.monitorVisible
	m.density = settings.density
	if m.density == "" {
		m.density = "comfortable"
	}
}

func validSettingsValue(value string, options []string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func cycleStringOption(current string, options []string, delta int) string {
	if len(options) == 0 {
		return current
	}
	idx := 0
	for i, option := range options {
		if option == current {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	return options[idx]
}

type settingField struct {
	id    string
	label string
}

func settingFields() []settingField {
	return []settingField{
		{id: "open-mode", label: "Open mode"},
		{id: "terminal", label: "Terminal"},
		{id: "density", label: "Density"},
		{id: "preview", label: "Preview"},
		{id: "raw-preview", label: "Raw preview"},
		{id: "monitor", label: "Monitor"},
	}
}

func settingOpenModes() []string {
	return []string{"auto", "split", "tab", "window"}
}

func settingTerminals() []string {
	return []string{"auto", "iterm", "terminal", "wezterm", "kitty", "alacritty", "ghostty"}
}

func settingDensities() []string {
	return []string{"comfortable", "compact"}
}

func (m Model) handleGroupPickerKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.groupPicker = groupPickerState{}
		m.status = ""
		return m, nil
	case tea.KeyEnter:
		return m.commitGroupPicker()
	case tea.KeyUp:
		m.moveGroupPickerCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveGroupPickerCursor(1)
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.groupPicker.buffer)
		if len(runes) > 0 {
			m.groupPicker.buffer = string(runes[:len(runes)-1])
			m.groupPicker.cursor = 0
			m.status = ""
		}
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "j":
			if m.groupPicker.buffer == "" {
				m.moveGroupPickerCursor(1)
				return m, nil
			}
		case "k":
			if m.groupPicker.buffer == "" {
				m.moveGroupPickerCursor(-1)
				return m, nil
			}
		}
		m.groupPicker.buffer += key.String()
		m.groupPicker.cursor = 0
		m.status = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) moveGroupPickerCursor(delta int) {
	candidates := m.groupPickerCandidates()
	if len(candidates) == 0 {
		m.groupPicker.cursor = 0
		return
	}
	m.groupPicker.cursor += delta
	if m.groupPicker.cursor < 0 {
		m.groupPicker.cursor = len(candidates) - 1
	}
	if m.groupPicker.cursor >= len(candidates) {
		m.groupPicker.cursor = 0
	}
}

func (m Model) commitGroupPicker() (Model, tea.Cmd) {
	target := m.groupPickerTarget()
	if target == "" {
		m.status = "type a group name"
		return m, nil
	}
	if err := sshconfig.ValidateGroupName(target); err != nil {
		m.status = err.Error()
		return m, nil
	}
	if len(m.groupPicker.movingHosts) == 0 {
		m.status = "no host selected"
		return m, nil
	}
	m.mode = modeConfirm
	m.pending = pendingOperation{
		operation:   operationGroupMoveHosts,
		groupTo:     target,
		movingHosts: append([]sshconfig.HostEntry(nil), m.groupPicker.movingHosts...),
	}
	m.groupPicker = groupPickerState{}
	m.status = ""
	return m, nil
}

func (m Model) groupPickerTarget() string {
	buffer := strings.TrimSpace(m.groupPicker.buffer)
	if buffer != "" && !m.isKnownGroup(buffer) {
		return buffer
	}
	candidates := m.groupPickerCandidates()
	if len(candidates) == 0 {
		return buffer
	}
	cursor := m.groupPicker.cursor
	if cursor < 0 || cursor >= len(candidates) {
		cursor = 0
	}
	return candidates[cursor]
}

func (m Model) groupPickerCandidates() []string {
	query := strings.ToLower(strings.TrimSpace(m.groupPicker.buffer))
	var names []string
	for _, item := range m.groupItems() {
		if item.Name == "all" || item.Name == "ungrouped" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name), query) {
			continue
		}
		names = append(names, item.Name)
	}
	return names
}

func (m Model) isKnownGroup(name string) bool {
	for _, item := range m.groupItems() {
		if item.Name == name && item.Name != "all" && item.Name != "ungrouped" {
			return true
		}
	}
	return false
}

func (m Model) handleGroupInlineKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.groupInline = groupInlineState{}
		m.status = ""
		return m, nil
	case tea.KeyEnter:
		return m.commitGroupInline()
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.groupInline.buffer)
		if len(runes) > 0 {
			m.groupInline.buffer = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyRunes:
		m.groupInline.buffer += key.String()
		return m, nil
	}
	return m, nil
}

func (m Model) commitGroupInline() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.groupInline.buffer)
	if err := sshconfig.ValidateGroupName(name); err != nil {
		m.status = err.Error()
		return m, nil
	}
	known := m.knownGroupSet()
	switch m.groupInline.action {
	case groupInlineCreate:
		if known[name] {
			m.status = fmt.Sprintf("group %q already exists", name)
			return m, nil
		}
		m.state.EmptyGroups = append(m.state.EmptyGroups, name)
		m.mode = modeBrowse
		m.groupInline = groupInlineState{}
		m.status = fmt.Sprintf("created group %q (empty)", name)
		m.applyFilter()
		return m, m.saveStateCmd()
	case groupInlineRename:
		old := m.groupInline.target
		if name == old {
			m.mode = modeBrowse
			m.groupInline = groupInlineState{}
			return m, nil
		}
		if known[name] {
			m.status = fmt.Sprintf("group %q already exists; use merge", name)
			return m, nil
		}
		// Empty-group rename = state-only rename, no file writes.
		if isEmptyGroup(m.state.EmptyGroups, old) {
			m.state.EmptyGroups = renameInSlice(m.state.EmptyGroups, old, name)
			m.state.GroupOrder = renameInSlice(m.state.GroupOrder, old, name)
			m.mode = modeBrowse
			m.groupInline = groupInlineState{}
			m.status = fmt.Sprintf("renamed empty group %q → %q", old, name)
			m.applyFilter()
			return m, m.saveStateCmd()
		}
		// Real rename: queue confirm for file writes.
		m.mode = modeConfirm
		m.pending = pendingOperation{
			operation: operationGroupRename,
			groupFrom: old,
			groupTo:   name,
		}
		m.groupInline = groupInlineState{}
		m.status = ""
		return m, nil
	}
	return m, nil
}

func (m Model) knownGroupSet() map[string]bool {
	known := map[string]bool{}
	for _, item := range m.groupItems() {
		known[item.Name] = true
	}
	delete(known, "all")
	delete(known, "ungrouped")
	return known
}

func formFieldGroupIndex() int {
	for i, name := range formFields() {
		if name == "Group" {
			return i
		}
	}
	return -1
}

func nextKnownGroup(current string, known map[string]bool) string {
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return current
	}
	for i, n := range names {
		if n == current {
			return names[(i+1)%len(names)]
		}
	}
	return names[0]
}

func isEmptyGroup(empties []string, name string) bool {
	for _, n := range empties {
		if n == name {
			return true
		}
	}
	return false
}

func renameInSlice(slice []string, old, new string) []string {
	out := make([]string, 0, len(slice))
	for _, n := range slice {
		if n == old {
			out = append(out, new)
		} else {
			out = append(out, n)
		}
	}
	return out
}

func (m Model) prepareFormSave() (Model, tea.Cmd) {
	if err := validateFormForApp(m.form.values); err != nil {
		m.status = err.Error()
		return m, nil
	}
	target := m.configPath
	if m.form.operation == operationEdit {
		target = m.form.entry.SourceFile
	}
	m.pending = pendingOperation{
		operation: m.form.operation,
		entry:     m.form.entry,
		values:    m.form.values,
		target:    target,
	}
	m.mode = modeConfirm
	m.status = ""
	return m, nil
}

func (m Model) writePendingCmd() tea.Cmd {
	pending := m.pending
	configPath := m.configPath
	sources := m.sourceFiles()
	return func() tea.Msg {
		var err error
		switch pending.operation {
		case operationAdd:
			err = sshconfig.AddHost(configPath, pending.values)
		case operationEdit:
			err = sshconfig.EditHost(pending.entry, pending.values)
		case operationDelete:
			err = sshconfig.DeleteHost(pending.entry)
		case operationGroupRename:
			_, err = sshconfig.RenameGroup(sources, pending.groupFrom, pending.groupTo)
		case operationGroupMerge:
			_, err = sshconfig.MergeGroup(sources, pending.groupFrom, pending.groupTo)
		case operationGroupDelete:
			_, err = sshconfig.DeleteGroup(sources, pending.groupFrom)
		case operationGroupMoveHosts:
			_, err = sshconfig.MoveHostsToGroup(pending.movingHosts, pending.groupTo)
		}
		return writeConfigMsg{Err: err, op: pending.operation, groupFrom: pending.groupFrom, groupTo: pending.groupTo}
	}
}

// applyGroupStateAfterWrite mutates state.GroupOrder and state.EmptyGroups so
// they stay coherent with the file changes that just succeeded.
func (m *Model) applyGroupStateAfterWrite(msg writeConfigMsg) {
	switch msg.op {
	case operationGroupRename, operationGroupMerge:
		m.state.GroupOrder = renameInSlice(m.state.GroupOrder, msg.groupFrom, msg.groupTo)
		m.state.EmptyGroups = removeFromSlice(m.state.EmptyGroups, msg.groupFrom)
	case operationGroupDelete:
		m.state.GroupOrder = removeFromSlice(m.state.GroupOrder, msg.groupFrom)
		m.state.EmptyGroups = removeFromSlice(m.state.EmptyGroups, msg.groupFrom)
	case operationGroupMoveHosts:
		// If hosts moved into a previously empty group, drop it from EmptyGroups.
		if msg.groupTo != "" {
			m.state.EmptyGroups = removeFromSlice(m.state.EmptyGroups, msg.groupTo)
		}
	}
}

func removeFromSlice(slice []string, name string) []string {
	out := make([]string, 0, len(slice))
	for _, n := range slice {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}

func (m Model) sourceFiles() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, entry := range m.entries {
		if entry.SourceFile == "" || seen[entry.SourceFile] {
			continue
		}
		seen[entry.SourceFile] = true
		out = append(out, entry.SourceFile)
	}
	return out
}

type writeConfigMsg struct {
	Err       error
	op        operationType
	groupFrom string
	groupTo   string
}

func (m Model) reloadCmd() tea.Cmd {
	if m.load == nil {
		return nil
	}
	return func() tea.Msg {
		entries, warnings, err := m.load()
		return ReloadedMsg{Entries: entries, Warnings: warnings, Err: err}
	}
}

func (m Model) maybeProbeCmd() tea.Cmd {
	if m.monitor == nil || m.probe == nil || !m.monitorVisible {
		return nil
	}
	if m.mode != modeBrowse {
		return nil
	}
	entry, ok := m.Selected()
	if !ok {
		return nil
	}
	if !m.monitor.NeedsRefresh(entry.Alias) {
		return nil
	}
	return m.probeCmd(entry)
}

func (m Model) refreshMonitor() (Model, tea.Cmd) {
	if m.monitor == nil || m.probe == nil {
		m.status = "monitor unavailable"
		return m, nil
	}
	if !m.monitorVisible {
		m.monitorVisible = true
	}
	entry, ok := m.Selected()
	if !ok {
		m.status = "no host selected"
		return m, nil
	}
	cmd := m.probeCmd(entry)
	if cmd == nil {
		m.status = "monitor already refreshing"
		return m, nil
	}
	m.status = "refreshing monitor"
	return m, cmd
}

func (m Model) probeCmd(entry sshconfig.HostEntry) tea.Cmd {
	if m.monitor == nil || m.probe == nil {
		return nil
	}
	if !m.monitor.MarkInflight(entry.Alias) {
		return nil
	}
	target := monitor.ProbeTarget{Alias: entry.Alias, Password: entry.SSHPassword}
	probe := m.probe
	timeout := m.monitor.Timeout()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		snap, err := probe(ctx, target)
		if err != nil {
			return monitorMsg{snap: &monitor.Snapshot{Alias: target.Alias, FetchedAt: time.Now(), Err: err}}
		}
		if snap == nil {
			return monitorMsg{snap: &monitor.Snapshot{Alias: target.Alias, FetchedAt: time.Now()}}
		}
		snap.Alias = target.Alias
		return monitorMsg{snap: snap}
	}
}

func (m Model) connectCmd() tea.Cmd {
	targets := m.connectTargets()
	return m.connectTargetsCmd(targets)
}

func (m Model) connectHostsCmd(hosts []sshconfig.HostEntry) tea.Cmd {
	targets := make([]terminal.Target, 0, len(hosts))
	for _, host := range hosts {
		targets = append(targets, targetFromEntry(host))
	}
	return m.connectTargetsCmd(targets)
}

func (m Model) connectTargetsCmd(targets []terminal.Target) tea.Cmd {
	if len(targets) == 0 {
		return func() tea.Msg { return ConnectMsg{Err: fmt.Errorf("no host selected")} }
	}
	return func() tea.Msg {
		var succeeded []string
		failed := 0
		for _, target := range targets {
			if err := m.manager.Connect(target); err != nil {
				failed++
				continue
			}
			succeeded = append(succeeded, target.Alias)
		}
		if len(succeeded) == 0 && failed > 0 {
			return ConnectMsg{Err: fmt.Errorf("failed to open %d connection", failed)}
		}
		return ConnectMsg{Succeeded: succeeded, Failed: failed}
	}
}

func (m Model) prepareConnect() (Model, tea.Cmd) {
	hosts := m.connectHostEntries()
	if len(hosts) == 0 {
		m.status = "no host selected"
		return m, nil
	}
	return m, m.connectCmd()
}

func (m Model) prepareQuickConnect(slot int) (Model, tea.Cmd) {
	hosts := m.quickConnectEntries()
	if slot < 1 || slot > len(hosts) {
		m.status = fmt.Sprintf("no quick host at %d", slot)
		return m, nil
	}
	host := hosts[slot-1]
	m.cursor = m.indexOfFilteredAlias(host.Alias)
	m.status = "quick connect " + host.Alias
	return m, m.connectHostsCmd([]sshconfig.HostEntry{host})
}

func (m Model) connectHostEntries() []sshconfig.HostEntry {
	if len(m.selected) == 0 {
		entry, ok := m.Selected()
		if !ok {
			return nil
		}
		return []sshconfig.HostEntry{entry}
	}
	hosts := make([]sshconfig.HostEntry, 0, len(m.selected))
	for _, entry := range m.entries {
		if m.selected[entry.Alias] {
			hosts = append(hosts, entry)
		}
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Alias < hosts[j].Alias })
	return hosts
}

func (m Model) quickConnectEntries() []sshconfig.HostEntry {
	source := m.filtered
	if strings.TrimSpace(m.filter) == "" && m.selectedGroup() == "all" {
		source = m.entries
	}
	hosts := append([]sshconfig.HostEntry(nil), source...)
	sort.SliceStable(hosts, func(i, j int) bool {
		return m.lessQuick(hosts[i], hosts[j])
	})
	if len(hosts) > 9 {
		hosts = hosts[:9]
	}
	return hosts
}

func (m Model) lessQuick(a, b sshconfig.HostEntry) bool {
	aState := m.state.Hosts[a.Alias]
	bState := m.state.Hosts[b.Alias]
	if aState.Favorite != bState.Favorite {
		return aState.Favorite
	}
	if aState.LastConnectedAt != bState.LastConnectedAt {
		return aState.LastConnectedAt > bState.LastConnectedAt
	}
	if aState.ConnectCount != bState.ConnectCount {
		return aState.ConnectCount > bState.ConnectCount
	}
	if strings.TrimSpace(a.Group) != strings.TrimSpace(b.Group) {
		return strings.TrimSpace(a.Group) < strings.TrimSpace(b.Group)
	}
	return a.Alias < b.Alias
}

func (m Model) quickSlot(alias string) int {
	for i, entry := range m.quickConnectEntries() {
		if entry.Alias == alias {
			return i + 1
		}
	}
	return 0
}

func (m Model) indexOfFilteredAlias(alias string) int {
	for i, entry := range m.filtered {
		if entry.Alias == alias {
			return i
		}
	}
	return m.cursor
}

func quickConnectDigit(value string) (int, bool) {
	if len(value) != 1 || value[0] < '1' || value[0] > '9' {
		return 0, false
	}
	return int(value[0] - '0'), true
}

func (m Model) startCommandPalette() Model {
	m.mode = modeCommandPalette
	m.command = commandPaletteState{}
	m.status = ""
	return m
}

func (m Model) commandEntries() []commandEntry {
	entries := []commandEntry{
		{ID: "connect", Title: "Connect", Description: "Open the selected or marked host(s)"},
		{ID: "edit", Title: "Edit host", Description: "Edit the selected Host block"},
		{ID: "move", Title: "Move to group", Description: "Move selected or marked host(s)"},
		{ID: "refresh-monitor", Title: "Refresh monitor", Description: "Probe the selected host now"},
		{ID: "source", Title: "Show source", Description: "Show source config path and line"},
		{ID: "warnings", Title: "Show warnings", Description: "Open parser warnings"},
		{ID: "history", Title: "Show history", Description: "Recent and frequent connections"},
		{ID: "settings", Title: "Settings", Description: "Open terminal and UI preferences"},
		{ID: "reload", Title: "Reload config", Description: "Reload SSH config files"},
		{ID: "favorite", Title: "Toggle favorite", Description: "Favorite or unfavorite selected host"},
	}
	query := strings.ToLower(strings.TrimSpace(m.command.buffer))
	if query == "" {
		return entries
	}
	var filtered []commandEntry
	for _, entry := range entries {
		haystack := strings.ToLower(entry.Title + " " + entry.Description + " " + entry.ID)
		if strings.Contains(haystack, query) || fuzzyContains(haystack, query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m Model) historyRows() []historyRow {
	rows := make([]historyRow, 0, len(m.state.Hosts))
	known := map[string]bool{}
	for _, entry := range m.entries {
		known[entry.Alias] = true
	}
	for alias, host := range m.state.Hosts {
		if host.ConnectCount == 0 && host.LastConnectedAt == "" && !host.Favorite {
			continue
		}
		if !known[alias] {
			continue
		}
		rows = append(rows, historyRow{
			Alias:           alias,
			Favorite:        host.Favorite,
			ConnectCount:    host.ConnectCount,
			LastConnectedAt: host.LastConnectedAt,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastConnectedAt != rows[j].LastConnectedAt {
			return rows[i].LastConnectedAt > rows[j].LastConnectedAt
		}
		if rows[i].ConnectCount != rows[j].ConnectCount {
			return rows[i].ConnectCount > rows[j].ConnectCount
		}
		return rows[i].Alias < rows[j].Alias
	})
	return rows
}

func (m *Model) moveCommandCursor(delta int) {
	entries := m.commandEntries()
	if len(entries) == 0 {
		m.command.cursor = 0
		return
	}
	m.command.cursor += delta
	if m.command.cursor < 0 {
		m.command.cursor = len(entries) - 1
	}
	if m.command.cursor >= len(entries) {
		m.command.cursor = 0
	}
}

func (m Model) runCommandPaletteSelection() (Model, tea.Cmd) {
	entries := m.commandEntries()
	if len(entries) == 0 {
		m.status = "no command matches"
		return m, nil
	}
	cursor := m.command.cursor
	if cursor < 0 || cursor >= len(entries) {
		cursor = 0
	}
	selected := entries[cursor]
	m.mode = modeBrowse
	m.command = commandPaletteState{}
	switch selected.ID {
	case "connect":
		return m.prepareConnect()
	case "edit":
		entry, ok := m.Selected()
		if !ok {
			m.status = "no host selected"
			return m, nil
		}
		values := formFromEntry(entry)
		m.mode = modeForm
		m.status = ""
		m.form = formState{operation: operationEdit, entry: entry, values: values, cursor: formFieldLen(values, 0)}
		return m, nil
	case "move":
		return m.startGroupPicker()
	case "refresh-monitor":
		return m.refreshMonitor()
	case "source":
		entry, ok := m.Selected()
		if !ok {
			m.status = "no host selected"
			return m, nil
		}
		if entry.SourceFile == "" {
			m.status = "source unavailable"
			return m, nil
		}
		m.status = fmt.Sprintf("source %s:%d", entry.SourceFile, entry.SourceLine)
		return m, nil
	case "warnings":
		if len(m.warnings) == 0 {
			m.status = "no warnings"
			return m, nil
		}
		m.mode = modeWarnings
		m.status = ""
		return m, nil
	case "history":
		m.mode = modeHistory
		m.status = ""
		return m, nil
	case "settings":
		return m.startSettings(), nil
	case "reload":
		return m, m.reloadCmd()
	case "favorite":
		return m.toggleFavorite()
	}
	return m, nil
}

func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.filter))
	selectedGroup := m.selectedGroup()
	m.filtered = m.filtered[:0]
	for _, entry := range m.entries {
		if matchesGroup(entry, selectedGroup) && (query == "" || m.matches(entry, query)) {
			m.filtered = append(m.filtered, entry)
		}
	}
	sort.SliceStable(m.filtered, func(i, j int) bool {
		return m.less(m.filtered[i], m.filtered[j])
	})
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) matches(entry sshconfig.HostEntry, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		entry.Alias,
		entry.Group,
		entry.HostName,
		entry.User,
		entry.Port,
		entry.IdentityFile,
		entry.ProxyJump,
		entry.ProxyCommand,
		entry.SourceFile,
		strings.Join(entry.Tags, " "),
	}, " "))
	parts := strings.Fields(query)
	for _, part := range parts {
		negated := false
		if strings.HasPrefix(part, "-") && len(part) > 1 {
			negated = true
			part = strings.TrimPrefix(part, "-")
		}
		matched := false
		if tag, ok := strings.CutPrefix(part, "tag:"); ok {
			matched = hasTag(entry, tag)
		} else if key, value, ok := strings.Cut(part, ":"); ok && isStructuredSearchKey(key) {
			matched = structuredFieldMatches(entry, key, value)
		} else if part == "fav:" {
			matched = m.state.Hosts[entry.Alias].Favorite
		} else if part == "recent:" {
			matched = m.state.Hosts[entry.Alias].LastConnectedAt != ""
		} else if negated {
			matched = strings.Contains(haystack, part)
		} else {
			matched = strings.Contains(haystack, part) || fuzzyContains(haystack, part)
		}
		if negated {
			matched = !matched
		}
		if !matched {
			return false
		}
	}
	return true
}

func (m Model) less(a, b sshconfig.HostEntry) bool {
	if strings.TrimSpace(m.filter) != "" {
		aRank := m.matchRank(a)
		bRank := m.matchRank(b)
		if aRank != bRank {
			return aRank < bRank
		}
		aContext := m.searchContext(a, m.filter)
		bContext := m.searchContext(b, m.filter)
		if aContext != bContext {
			return aContext < bContext
		}
	}
	aState := m.state.Hosts[a.Alias]
	bState := m.state.Hosts[b.Alias]
	if aState.Favorite != bState.Favorite {
		return aState.Favorite
	}
	if aState.LastConnectedAt != "" || bState.LastConnectedAt != "" {
		if aState.LastConnectedAt != bState.LastConnectedAt {
			return aState.LastConnectedAt > bState.LastConnectedAt
		}
	}
	if aState.ConnectCount != bState.ConnectCount {
		return aState.ConnectCount > bState.ConnectCount
	}
	return a.Alias < b.Alias
}

func (m Model) matchRank(entry sshconfig.HostEntry) int {
	terms := textSearchTerms(m.filter)
	if len(terms) == 0 {
		return 0
	}
	score := 0
	for _, term := range terms {
		score += searchTermRank(entry, term)
	}
	return score
}

func searchTermRank(entry sshconfig.HostEntry, term string) int {
	alias := strings.ToLower(entry.Alias)
	group := strings.ToLower(entry.Group)
	tags := strings.ToLower(strings.Join(entry.Tags, " "))
	hostFields := strings.ToLower(strings.Join([]string{
		entry.HostName,
		entry.User,
		entry.Port,
		entry.ProxyJump,
		entry.SourceFile,
	}, " "))
	haystack := strings.ToLower(strings.Join([]string{
		entry.Alias,
		entry.Group,
		entry.HostName,
		entry.User,
		entry.Port,
		entry.ProxyJump,
		entry.SourceFile,
		strings.Join(entry.Tags, " "),
	}, " "))

	switch {
	case alias == term:
		return 0
	case strings.HasPrefix(alias, term):
		return 1
	case strings.Contains(alias, term):
		return 2
	case strings.Contains(group, term) || strings.Contains(tags, term):
		return 3
	case strings.Contains(hostFields, term):
		return 4
	case fuzzyContains(alias, term):
		return 5
	case fuzzyContains(haystack, term):
		return 6
	default:
		return 7
	}
}

func (m Model) searchContext(entry sshconfig.HostEntry, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	for _, part := range strings.Fields(query) {
		part = strings.TrimPrefix(part, "-")
		switch {
		case part == "fav:" && m.state.Hosts[entry.Alias].Favorite:
			return "favorite"
		case part == "recent:" && m.state.Hosts[entry.Alias].LastConnectedAt != "":
			return "recent"
		case strings.HasPrefix(part, "tag:"):
			value := strings.TrimPrefix(part, "tag:")
			if tag := matchingListValue(entry.Tags, value); tag != "" {
				return "tag " + tag
			}
		default:
			if key, value, ok := strings.Cut(part, ":"); ok && isStructuredSearchKey(key) {
				if context := structuredFieldContext(entry, key, value); context != "" {
					return context
				}
			}
		}
	}
	for _, term := range textSearchTerms(query) {
		if context := textFieldContext(entry, term); context != "" {
			return context
		}
	}
	return ""
}

func textSearchTerms(query string) []string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		check := strings.TrimPrefix(part, "-")
		if strings.HasPrefix(check, "tag:") || check == "fav:" || check == "recent:" {
			continue
		}
		if key, _, ok := strings.Cut(check, ":"); ok && isStructuredSearchKey(key) {
			continue
		}
		terms = append(terms, check)
	}
	return terms
}

func isStructuredSearchKey(key string) bool {
	switch key {
	case "alias", "host", "hostname", "user", "port", "group", "jump", "proxy", "file", "source", "identity", "key":
		return true
	default:
		return false
	}
}

func structuredFieldMatches(entry sshconfig.HostEntry, key, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	contains := func(field string) bool {
		return strings.Contains(strings.ToLower(field), value)
	}
	switch key {
	case "alias":
		return contains(entry.Alias)
	case "host", "hostname":
		return contains(entry.HostName)
	case "user":
		return contains(entry.User)
	case "port":
		return contains(entry.Port)
	case "group":
		if value == "ungrouped" {
			return strings.TrimSpace(entry.Group) == ""
		}
		return contains(entry.Group)
	case "jump":
		return contains(entry.ProxyJump)
	case "proxy":
		return contains(entry.ProxyJump) || contains(entry.ProxyCommand)
	case "file", "source":
		return contains(entry.SourceFile)
	case "identity", "key":
		return contains(entry.IdentityFile)
	default:
		return false
	}
}

func structuredFieldContext(entry sshconfig.HostEntry, key, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	switch key {
	case "alias":
		return matchingFieldContext("alias", entry.Alias, value)
	case "host", "hostname":
		return matchingFieldContext("host", entry.HostName, value)
	case "user":
		return matchingFieldContext("user", entry.User, value)
	case "port":
		return matchingFieldContext("port", entry.Port, value)
	case "group":
		if value == "ungrouped" && strings.TrimSpace(entry.Group) == "" {
			return "ungrouped"
		}
		return matchingFieldContext("group", entry.Group, value)
	case "jump":
		return matchingFieldContext("jump", entry.ProxyJump, value)
	case "proxy":
		if context := matchingFieldContext("jump", entry.ProxyJump, value); context != "" {
			return context
		}
		return matchingFieldContext("proxy", entry.ProxyCommand, value)
	case "file", "source":
		return matchingFieldContext("file", entry.SourceFile, value)
	case "identity", "key":
		return matchingFieldContext("key", entry.IdentityFile, value)
	default:
		return ""
	}
}

func textFieldContext(entry sshconfig.HostEntry, term string) string {
	fields := []struct {
		label string
		value string
	}{
		{"alias", entry.Alias},
		{"group", entry.Group},
		{"tag", matchingListValue(entry.Tags, term)},
		{"host", entry.HostName},
		{"user", entry.User},
		{"port", entry.Port},
		{"jump", entry.ProxyJump},
		{"proxy", entry.ProxyCommand},
		{"file", entry.SourceFile},
		{"key", entry.IdentityFile},
	}
	for _, field := range fields {
		if context := matchingFieldContext(field.label, field.value, term); context != "" {
			return context
		}
	}
	for _, field := range fields {
		if field.value != "" && fuzzyContains(field.value, term) {
			return field.label + " " + field.value
		}
	}
	return ""
}

func matchingListValue(values []string, term string) string {
	term = strings.ToLower(strings.TrimSpace(term))
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), term) {
			return value
		}
	}
	return ""
}

func matchingFieldContext(label, value, term string) string {
	if value == "" || term == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(value), strings.ToLower(term)) {
		return label + " " + value
	}
	return ""
}

func fuzzyContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hr := []rune(strings.ToLower(haystack))
	nr := []rune(strings.ToLower(needle))
	j := 0
	for _, r := range hr {
		if r == nr[j] {
			j++
			if j == len(nr) {
				return true
			}
		}
	}
	return false
}

func (m Model) toggleFavorite() (Model, tea.Cmd) {
	entry, ok := m.Selected()
	if !ok {
		m.status = "no host selected"
		return m, nil
	}
	m.state.ToggleFavorite(entry.Alias)
	m.applyFilter()
	return m, m.saveStateCmd()
}

func (m Model) toggleSelected() Model {
	entry, ok := m.Selected()
	if !ok {
		m.status = "no host selected"
		return m
	}
	if m.selected[entry.Alias] {
		delete(m.selected, entry.Alias)
	} else {
		m.selected[entry.Alias] = true
	}
	return m
}

func (m Model) connectAliases() []string {
	targets := m.connectTargets()
	aliases := make([]string, 0, len(targets))
	for _, target := range targets {
		aliases = append(aliases, target.Alias)
	}
	return aliases
}

func (m Model) connectTargets() []terminal.Target {
	if len(m.selected) == 0 {
		entry, ok := m.Selected()
		if !ok {
			return nil
		}
		return []terminal.Target{targetFromEntry(entry)}
	}
	targets := make([]terminal.Target, 0, len(m.selected))
	for _, entry := range m.entries {
		if m.selected[entry.Alias] {
			targets = append(targets, targetFromEntry(entry))
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Alias < targets[j].Alias
	})
	return targets
}

func (m Model) saveStateCmd() tea.Cmd {
	if m.statePath == "" {
		return nil
	}
	path := m.statePath
	store := m.state
	return func() tea.Msg {
		return stateSavedMsg{Err: state.Save(path, store)}
	}
}

func matchesGroup(entry sshconfig.HostEntry, group string) bool {
	switch group {
	case "", "all":
		return true
	case "ungrouped":
		return strings.TrimSpace(entry.Group) == ""
	default:
		return entry.Group == group
	}
}

func hasTag(entry sshconfig.HostEntry, tag string) bool {
	for _, entryTag := range entry.Tags {
		if strings.EqualFold(entryTag, tag) {
			return true
		}
	}
	return false
}

func (m Model) groupItems() []groupItem {
	counts := map[string]int{}
	hasUngrouped := false
	known := map[string]bool{}
	for _, entry := range m.entries {
		group := strings.TrimSpace(entry.Group)
		if group == "" {
			hasUngrouped = true
			continue
		}
		counts[group]++
		known[group] = true
	}
	for _, name := range m.state.EmptyGroups {
		if name == "" || name == "all" || name == "ungrouped" {
			continue
		}
		if !known[name] {
			known[name] = true
		}
	}

	used := map[string]bool{}
	var ordered []string
	for _, name := range m.state.GroupOrder {
		if name == "" || name == "all" || name == "ungrouped" {
			continue
		}
		if known[name] && !used[name] {
			ordered = append(ordered, name)
			used[name] = true
		}
	}
	var unordered []string
	for name := range known {
		if !used[name] {
			unordered = append(unordered, name)
		}
	}
	sort.Strings(unordered)

	items := make([]groupItem, 0, len(known)+2)
	items = append(items, groupItem{Name: "all", Count: len(m.entries)})
	for _, name := range ordered {
		items = append(items, groupItem{Name: name, Count: counts[name]})
	}
	for _, name := range unordered {
		items = append(items, groupItem{Name: name, Count: counts[name]})
	}
	if hasUngrouped {
		ungroupedCount := 0
		for _, entry := range m.entries {
			if strings.TrimSpace(entry.Group) == "" {
				ungroupedCount++
			}
		}
		items = append(items, groupItem{Name: "ungrouped", Count: ungroupedCount})
	}
	return items
}

func (m Model) selectedGroup() string {
	items := m.groupItems()
	if len(items) == 0 {
		return "all"
	}
	if m.groupCursor < 0 || m.groupCursor >= len(items) {
		return "all"
	}
	return items[m.groupCursor].Name
}

func (m *Model) moveGroup(delta int) {
	items := m.groupItems()
	if len(items) == 0 {
		m.groupCursor = 0
		return
	}
	m.groupCursor += delta
	if m.groupCursor < 0 {
		m.groupCursor = len(items) - 1
	}
	if m.groupCursor >= len(items) {
		m.groupCursor = 0
	}
	m.cursor = 0
	m.applyFilter()
}

func (m *Model) moveCursorBy(delta int) {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m Model) listPageSize() int {
	if m.height <= 0 {
		return 10
	}
	footer := len(m.footerLines())
	if m.status != "" {
		footer++
	}
	page := m.height - footer - 3 // top status, search line, spacer
	if page < 1 {
		return 1
	}
	return page
}

func visibleGroupRange(items []groupItem, selected string, rowBudget int) (int, int) {
	if rowBudget <= 0 || len(items) == 0 {
		return 0, 0
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
	return start, end
}

func visibleListRange(total, cursor, rowBudget int) (start, visible int) {
	if rowBudget <= 0 || total <= 0 {
		return 0, 0
	}
	visible = rowBudget
	if total > visible && visible > 1 {
		visible--
	}
	start = 0
	if total > visible {
		start = cursor - visible/2
		if start < 0 {
			start = 0
		}
		if maxStart := total - visible; start > maxStart {
			start = maxStart
		}
	}
	if visible > total-start {
		visible = total - start
	}
	return start, visible
}

func statusFromWarnings(warnings []sshconfig.Warning) string {
	if len(warnings) == 0 {
		return ""
	}
	if len(warnings) == 1 {
		return warnings[0].Error()
	}
	return fmt.Sprintf("%s (+%d more)", warnings[0].Error(), len(warnings)-1)
}

func formFromEntry(entry sshconfig.HostEntry) sshconfig.HostForm {
	return sshconfig.HostForm{
		Alias:        entry.Alias,
		Group:        entry.Group,
		HostName:     entry.HostName,
		User:         entry.User,
		Port:         entry.Port,
		IdentityFile: entry.IdentityFile,
		ProxyJump:    entry.ProxyJump,
		ProxyCommand: entry.ProxyCommand,
		SSHPassword:  entry.SSHPassword,
		Tags:         entry.Tags,
	}
}

func targetFromEntry(entry sshconfig.HostEntry) terminal.Target {
	return terminal.Target{
		Alias:       entry.Alias,
		SSHPassword: entry.SSHPassword,
	}
}

func validateFormForApp(form sshconfig.HostForm) error {
	return sshconfig.ValidateHostForm(form)
}

func formFields() []string {
	return []string{"Alias", "Group", "HostName", "User", "Port", "IdentityFile", "ProxyJump", "ProxyCommand", "Tags"}
}

func formFieldValue(form sshconfig.HostForm, field int) string {
	switch field {
	case 0:
		return form.Alias
	case 1:
		return form.Group
	case 2:
		return form.HostName
	case 3:
		return form.User
	case 4:
		return form.Port
	case 5:
		return form.IdentityFile
	case 6:
		return form.ProxyJump
	case 7:
		return form.ProxyCommand
	case 8:
		return strings.Join(form.Tags, " ")
	default:
		return ""
	}
}

func formFieldLen(form sshconfig.HostForm, field int) int {
	return len([]rune(formFieldValue(form, field)))
}

func replaceFormField(form sshconfig.HostForm, field int, value string) sshconfig.HostForm {
	return updateFormField(form, field, func(string) string { return value })
}

func insertAtFormCursor(form sshconfig.HostForm, field, cursor int, text string) (sshconfig.HostForm, int) {
	value := formFieldValue(form, field)
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	insert := []rune(text)
	next := string(append(append(append([]rune{}, runes[:cursor]...), insert...), runes[cursor:]...))
	return replaceFormField(form, field, next), cursor + len(insert)
}

func deleteBeforeFormCursor(form sshconfig.HostForm, field, cursor int) (sshconfig.HostForm, int) {
	value := formFieldValue(form, field)
	runes := []rune(value)
	if cursor <= 0 || len(runes) == 0 {
		return form, max(0, cursor)
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	next := string(append(append([]rune{}, runes[:cursor-1]...), runes[cursor:]...))
	return replaceFormField(form, field, next), cursor - 1
}

func deleteAtFormCursor(form sshconfig.HostForm, field, cursor int) sshconfig.HostForm {
	value := formFieldValue(form, field)
	runes := []rune(value)
	if cursor < 0 || cursor >= len(runes) {
		return form
	}
	next := string(append(append([]rune{}, runes[:cursor]...), runes[cursor+1:]...))
	return replaceFormField(form, field, next)
}

func updateFormField(form sshconfig.HostForm, field int, update func(string) string) sshconfig.HostForm {
	switch field {
	case 0:
		form.Alias = update(form.Alias)
	case 1:
		form.Group = update(form.Group)
	case 2:
		form.HostName = update(form.HostName)
	case 3:
		form.User = update(form.User)
	case 4:
		form.Port = update(form.Port)
	case 5:
		form.IdentityFile = update(form.IdentityFile)
	case 6:
		form.ProxyJump = update(form.ProxyJump)
	case 7:
		form.ProxyCommand = update(form.ProxyCommand)
	case 8:
		form.Tags = strings.Fields(update(strings.Join(form.Tags, " ")))
	}
	return form
}
