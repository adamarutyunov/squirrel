package ui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"squirrel/internal/git"
	"squirrel/internal/linear"
)

const oldBranchThreshold = 7 * 24 * time.Hour
const termPanelLines = 3 // divider + term-header + term-input

var (
	colorGreen     = lipgloss.Color("#22c55e")
	colorBlue      = lipgloss.Color("#60a5fa")
	colorDim       = lipgloss.Color("#71717a")
	colorWhite     = lipgloss.Color("#f4f4f5")
	colorSelection = lipgloss.Color("#3f3f46")
	colorAmber     = lipgloss.Color("#f59e0b")
	colorRed       = lipgloss.Color("#ef4444")

	styleTitle      = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	styleDim        = lipgloss.NewStyle().Foreground(colorDim)
	styleCurrent    = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	styleLinearID   = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleLinearDim  = lipgloss.NewStyle().Foreground(colorDim)
	styleRepoHeader = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	styleToggle     = lipgloss.NewStyle().Foreground(colorDim) // older branches: grey
	styleStatus     = lipgloss.NewStyle().Foreground(colorGreen)
	styleRemote     = lipgloss.NewStyle().Foreground(colorBlue)
	styleDanger     = lipgloss.NewStyle().Foreground(colorRed)
	styleWarning    = lipgloss.NewStyle().Foreground(colorAmber)
)

// RowType classifies each rendered line in the flat list.
type RowType int

const (
	rowTypeHeader  RowType = iota // repo name separator — not navigable
	rowTypeBranch                 // a branch — navigable
	rowTypeToggle                 // expand/collapse old branches — navigable
	rowTypeSpacer                 // empty line between repos — not navigable
)

type row struct {
	kind        RowType
	repoIdx     int
	itemIdx     int  // for rowTypeBranch: index into filteredItems[repoIdx]
	toggleCount int  // for rowTypeToggle: number of hidden/shown branches
	isExpanded  bool // for rowTypeToggle: current expansion state
}

type branchItem struct {
	branch   git.Branch
	issue    *linear.Issue
	repoPath string
	repoName string
}

type confirmActionType int

const (
	confirmDeleteLocal confirmActionType = iota
	confirmDeleteLocalAndRemote
)

// Model is the BubbleTea model.
type Model struct {
	repoPaths    []string
	repoNames    []string
	repoItems    [][]branchItem
	repoExpanded []bool
	linearIssues map[string]linear.Issue

	// Rebuilt whenever filter or expansion state changes.
	filteredItems [][]branchItem
	rows          []row

	cursor       int
	scrollOffset int

	filter      textinput.Model
	filterValue string

	width  int
	height int

	// Delete confirmation state.
	confirmMode   bool
	confirmAction confirmActionType
	confirmItem   branchItem

	// Right panel: output log.
	outputLines  []string
	outputScroll int

	// Right panel: terminal input.
	termInput   textinput.Model
	termFocused bool
}

var linearIDRegex = regexp.MustCompile(`(?i)[a-z][a-z0-9]+-\d+`)

func NewModel(repoPaths []string, repoBranches [][]git.Branch, linearIssues map[string]linear.Issue) Model {
	repoNames := make([]string, len(repoPaths))
	for i, path := range repoPaths {
		repoNames[i] = filepath.Base(path)
	}

	repoItems := make([][]branchItem, len(repoPaths))
	for repoIdx, branches := range repoBranches {
		repoItems[repoIdx] = buildItems(branches, repoPaths[repoIdx], repoNames[repoIdx], linearIssues)
	}

	filterInput := textinput.New()
	filterInput.Placeholder = "Filter..."
	filterInput.Focus()
	filterInput.Prompt = ""

	termInput := textinput.New()
	termInput.Placeholder = "shell command..."
	termInput.Prompt = "$ "

	m := Model{
		repoPaths:    repoPaths,
		repoNames:    repoNames,
		repoItems:    repoItems,
		repoExpanded: make([]bool, len(repoPaths)),
		linearIssues: linearIssues,
		filter:       filterInput,
		termInput:    termInput,
	}
	m.rebuild()
	return m
}

func buildItems(branches []git.Branch, repoPath, repoName string, linearIssues map[string]linear.Issue) []branchItem {
	items := make([]branchItem, len(branches))
	for i, branch := range branches {
		item := branchItem{branch: branch, repoPath: repoPath, repoName: repoName}
		for _, match := range linearIDRegex.FindAllString(branch.Name, -1) {
			if issue, ok := linearIssues[strings.ToUpper(match)]; ok {
				issueCopy := issue
				item.issue = &issueCopy
				break
			}
		}
		items[i] = item
	}
	return items
}

// rebuild rebuilds filteredItems and the flat row list from current state.
func (m *Model) rebuild() {
	query := strings.ToLower(m.filterValue)
	filterActive := query != ""
	multiRepo := len(m.repoPaths) > 1

	m.filteredItems = make([][]branchItem, len(m.repoPaths))
	for repoIdx, items := range m.repoItems {
		if !filterActive {
			m.filteredItems[repoIdx] = items
			continue
		}
		var filtered []branchItem
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.branch.Name), query) {
				filtered = append(filtered, item)
				continue
			}
			if item.issue != nil && strings.Contains(strings.ToLower(item.issue.Title), query) {
				filtered = append(filtered, item)
			}
		}
		m.filteredItems[repoIdx] = filtered
	}

	m.rows = nil
	firstRepo := true
	for repoIdx := range m.repoPaths {
		items := m.filteredItems[repoIdx]
		if len(items) == 0 {
			continue
		}

		if multiRepo {
			if !firstRepo {
				m.rows = append(m.rows, row{kind: rowTypeSpacer, repoIdx: repoIdx})
			}
			m.rows = append(m.rows, row{kind: rowTypeHeader, repoIdx: repoIdx})
			firstRepo = false
		}

		if filterActive {
			for itemIdx := range items {
				m.rows = append(m.rows, row{kind: rowTypeBranch, repoIdx: repoIdx, itemIdx: itemIdx})
			}
		} else {
			var recentIdx, oldIdx []int
			for itemIdx, item := range items {
				if !item.branch.IsCurrent && time.Since(item.branch.LastCommitTime) > oldBranchThreshold {
					oldIdx = append(oldIdx, itemIdx)
				} else {
					recentIdx = append(recentIdx, itemIdx)
				}
			}

			for _, itemIdx := range recentIdx {
				m.rows = append(m.rows, row{kind: rowTypeBranch, repoIdx: repoIdx, itemIdx: itemIdx})
			}

			if len(oldIdx) > 0 {
				expanded := m.repoExpanded[repoIdx]
				m.rows = append(m.rows, row{
					kind:        rowTypeToggle,
					repoIdx:     repoIdx,
					toggleCount: len(oldIdx),
					isExpanded:  expanded,
				})
				if expanded {
					for _, itemIdx := range oldIdx {
						m.rows = append(m.rows, row{kind: rowTypeBranch, repoIdx: repoIdx, itemIdx: itemIdx})
					}
				}
			}
		}
	}

	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	for m.cursor < len(m.rows) && !isNavigable(m.rows[m.cursor].kind) {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
		for m.cursor > 0 && !isNavigable(m.rows[m.cursor].kind) {
			m.cursor--
		}
	}
}

func isNavigable(kind RowType) bool {
	return kind == rowTypeBranch || kind == rowTypeToggle
}

func (m *Model) moveCursor(dir int) {
	next := m.cursor + dir
	for next >= 0 && next < len(m.rows) && !isNavigable(m.rows[next].kind) {
		next += dir
	}
	if next >= 0 && next < len(m.rows) {
		m.cursor = next
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+h {
		m.scrollOffset = m.cursor - h + 1
	}
}

func (m Model) listHeight() int {
	const fixedRows = 6 // header + blank + filter + blank + div + footer
	h := m.height - fixedRows
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) leftPanelWidth() int {
	return m.width / 2
}

func (m Model) rightPanelWidth() int {
	return m.width - m.leftPanelWidth() - 1 // 1 for the │ separator
}

// outputAreaHeight is the number of scrollable output lines shown (right panel top).
// Right panel: 1 (output header) + outputAreaHeight + termPanelLines = m.height
func (m Model) outputAreaHeight() int {
	h := m.height - 1 - termPanelLines
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) appendOutput(line string) {
	m.outputLines = append(m.outputLines, line)
	// Auto-scroll to bottom.
	if excess := len(m.outputLines) - m.outputAreaHeight(); excess > 0 {
		m.outputScroll = excess
	}
}

func (m *Model) activeRepoPath() string {
	if m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.repoIdx < len(m.repoPaths) {
			return m.repoPaths[r.repoIdx]
		}
	}
	if len(m.repoPaths) > 0 {
		return m.repoPaths[0]
	}
	return "."
}

// --- Tea messages ---

type tickMsg time.Time

type refreshMsg struct {
	repoIdx  int
	branches []git.Branch
}

type checkoutResultMsg struct {
	repoIdx int
	branch  string
	output  string
	err     error
}

type deleteResultMsg struct {
	repoIdx int
	output  string
	err     error
}

type termCmdResultMsg struct {
	output string
	err    error
}

type unpushedCheckMsg struct {
	commits []string
	err     error
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func refreshRepoCmd(repoIdx int, repoPath string) tea.Cmd {
	return func() tea.Msg {
		branches, err := git.GetBranches(repoPath)
		if err != nil {
			return nil
		}
		return refreshMsg{repoIdx: repoIdx, branches: branches}
	}
}

func checkoutCmd(repoIdx int, repoPath, branch string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "-C", repoPath, "checkout", branch)
		outputBytes, err := cmd.CombinedOutput()
		return checkoutResultMsg{
			repoIdx: repoIdx,
			branch:  branch,
			output:  strings.TrimSpace(string(outputBytes)),
			err:     err,
		}
	}
}

func deleteLocalCmd(repoIdx int, repoPath, branchName string, force bool) tea.Cmd {
	return func() tea.Msg {
		output, err := git.DeleteLocalBranch(repoPath, branchName, force)
		return deleteResultMsg{repoIdx: repoIdx, output: output, err: err}
	}
}

func deleteLocalAndRemoteCmd(repoIdx int, repoPath, branchName, remoteName string) tea.Cmd {
	return func() tea.Msg {
		localOut, localErr := git.DeleteLocalBranch(repoPath, branchName, true)
		if localErr != nil {
			return deleteResultMsg{repoIdx: repoIdx, output: localOut, err: localErr}
		}
		remoteOut, remoteErr := git.DeleteRemoteBranch(repoPath, remoteName)
		combined := strings.TrimSpace(localOut + "\n" + remoteOut)
		return deleteResultMsg{repoIdx: repoIdx, output: combined, err: remoteErr}
	}
}

func runTermCmd(repoPath, input string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Fields(input)
		if len(parts) == 0 {
			return termCmdResultMsg{}
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = repoPath
		outputBytes, err := cmd.CombinedOutput()
		return termCmdResultMsg{output: strings.TrimSpace(string(outputBytes)), err: err}
	}
}

func unpushedCheckCmd(repoPath, branchName, remoteName string) tea.Cmd {
	return func() tea.Msg {
		commits, err := git.GetUnpushedCommits(repoPath, branchName, remoteName)
		return unpushedCheckMsg{commits: commits, err: err}
	}
}

// --- BubbleTea interface ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		cmds := make([]tea.Cmd, len(m.repoPaths)+1)
		for i, path := range m.repoPaths {
			cmds[i] = refreshRepoCmd(i, path)
		}
		cmds[len(m.repoPaths)] = tickCmd()
		return m, tea.Batch(cmds...)

	case refreshMsg:
		m.applyRefresh(msg)
		return m, nil

	case checkoutResultMsg:
		if msg.err != nil {
			m.appendOutput("✗ checkout failed")
			if msg.output != "" {
				for _, line := range strings.Split(msg.output, "\n") {
					m.appendOutput("  " + line)
				}
			}
		} else {
			m.appendOutput("✓ switched to " + msg.branch)
			if msg.output != "" {
				for _, line := range strings.Split(msg.output, "\n") {
					m.appendOutput("  " + line)
				}
			}
			return m, refreshRepoCmd(msg.repoIdx, m.repoPaths[msg.repoIdx])
		}
		return m, nil

	case deleteResultMsg:
		if msg.err != nil {
			m.appendOutput("✗ delete failed")
		} else {
			m.appendOutput("✓ deleted")
		}
		if msg.output != "" {
			for _, line := range strings.Split(msg.output, "\n") {
				if line != "" {
					m.appendOutput("  " + line)
				}
			}
		}
		return m, refreshRepoCmd(msg.repoIdx, m.repoPaths[msg.repoIdx])

	case termCmdResultMsg:
		if msg.output != "" {
			for _, line := range strings.Split(msg.output, "\n") {
				m.appendOutput(line)
			}
		}
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("exit: " + msg.err.Error()))
		}
		return m, nil

	case unpushedCheckMsg:
		if msg.err == nil && len(msg.commits) > 0 {
			m.appendOutput(styleWarning.Render(fmt.Sprintf("  ⚠ %d unpushed commit(s):", len(msg.commits))))
			for _, c := range msg.commits {
				m.appendOutput("    " + c)
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if msg.Type == tea.KeyTab {
		m.termFocused = !m.termFocused
		if m.termFocused {
			m.termInput.Focus()
			m.filter.Blur()
		} else {
			m.filter.Focus()
			m.termInput.Blur()
		}
		return m, nil
	}

	if m.termFocused {
		return m.handleTermKey(msg)
	}
	return m.handleListKey(msg)
}

func (m Model) handleTermKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.termFocused = false
		m.filter.Focus()
		m.termInput.Blur()
		return m, nil
	case tea.KeyEnter:
		input := strings.TrimSpace(m.termInput.Value())
		if input == "" {
			return m, nil
		}
		repoPath := m.activeRepoPath()
		m.appendOutput(styleDim.Render("$ "+input) + " " + styleDim.Render("["+filepath.Base(repoPath)+"]"))
		m.termInput.SetValue("")
		return m, runTermCmd(repoPath, input)
	case tea.KeyUp:
		if m.outputScroll > 0 {
			m.outputScroll--
		}
		return m, nil
	case tea.KeyDown:
		maxScroll := len(m.outputLines) - m.outputAreaHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.outputScroll < maxScroll {
			m.outputScroll++
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.termInput, cmd = m.termInput.Update(msg)
		return m, cmd
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.confirmMode {
			m.confirmMode = false
			return m, nil
		}
		if m.filterValue != "" {
			m.filter.SetValue("")
			m.filterValue = ""
			m.rebuild()
		} else {
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil

	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil

	case tea.KeyEnter:
		if m.cursor < len(m.rows) {
			r := m.rows[m.cursor]
			switch r.kind {
			case rowTypeBranch:
				item := m.filteredItems[r.repoIdx][r.itemIdx]
				if !item.branch.IsCurrent {
					return m, checkoutCmd(r.repoIdx, item.repoPath, item.branch.Name)
				}
			case rowTypeToggle:
				m.repoExpanded[r.repoIdx] = !m.repoExpanded[r.repoIdx]
				m.rebuild()
			}
		}
		return m, nil

	default:
		key := msg.String()

		if key == "d" && m.filterValue == "" {
			return m.handleDeleteKey(false)
		}
		if key == "D" && m.filterValue == "" {
			return m.handleDeleteKey(true)
		}
		if key == "q" && m.filterValue == "" {
			return m, tea.Quit
		}

		prevValue := m.filterValue
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.filterValue = m.filter.Value()
		if m.filterValue != prevValue {
			m.confirmMode = false
			m.rebuild()
		}
		return m, cmd
	}
}

func (m Model) handleDeleteKey(includeRemote bool) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) {
		return m, nil
	}
	r := m.rows[m.cursor]
	if r.kind != rowTypeBranch {
		return m, nil
	}
	item := m.filteredItems[r.repoIdx][r.itemIdx]

	if item.branch.IsCurrent {
		m.appendOutput(styleDanger.Render("✗ cannot delete current branch"))
		return m, nil
	}

	actionType := confirmDeleteLocal
	if includeRemote {
		actionType = confirmDeleteLocalAndRemote
	}

	// Second press on the same branch+action → execute.
	if m.confirmMode &&
		m.confirmAction == actionType &&
		m.confirmItem.branch.Name == item.branch.Name &&
		m.confirmItem.repoPath == item.repoPath {
		m.confirmMode = false
		if includeRemote {
			if !item.branch.HasRemote() {
				m.appendOutput(styleWarning.Render("  no remote tracking branch, deleting local only"))
				return m, deleteLocalCmd(r.repoIdx, item.repoPath, item.branch.Name, true)
			}
			return m, deleteLocalAndRemoteCmd(r.repoIdx, item.repoPath, item.branch.Name, item.branch.RemoteName)
		}
		return m, deleteLocalCmd(r.repoIdx, item.repoPath, item.branch.Name, false)
	}

	// First press → enter confirm mode and show info.
	m.confirmMode = true
	m.confirmAction = actionType
	m.confirmItem = item

	if includeRemote {
		if item.branch.HasRemote() {
			m.appendOutput(styleDanger.Render(fmt.Sprintf("delete '%s' + remote '%s'?", item.branch.Name, item.branch.RemoteName)))
		} else {
			m.appendOutput(styleDanger.Render(fmt.Sprintf("delete '%s'?", item.branch.Name)) +
				styleWarning.Render(" (no remote)"))
		}
	} else {
		m.appendOutput(styleDanger.Render(fmt.Sprintf("delete '%s'?", item.branch.Name)))
		if !item.branch.HasRemote() {
			m.appendOutput(styleWarning.Render("  ⚠ no remote tracking branch"))
		}
	}

	// Fire unpushed check if has remote.
	if item.branch.HasRemote() {
		return m, unpushedCheckCmd(item.repoPath, item.branch.Name, item.branch.RemoteName)
	}
	return m, nil
}

func (m *Model) applyRefresh(msg refreshMsg) {
	var cursorBranchName string
	if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowTypeBranch {
		r := m.rows[m.cursor]
		cursorBranchName = m.filteredItems[r.repoIdx][r.itemIdx].branch.Name
	}

	m.repoItems[msg.repoIdx] = buildItems(msg.branches, m.repoPaths[msg.repoIdx], m.repoNames[msg.repoIdx], m.linearIssues)
	m.rebuild()

	if cursorBranchName != "" {
		for rowIdx, r := range m.rows {
			if r.kind == rowTypeBranch {
				if m.filteredItems[r.repoIdx][r.itemIdx].branch.Name == cursorBranchName {
					m.cursor = rowIdx
					m.ensureVisible()
					return
				}
			}
		}
	}
}

// --- Rendering ---

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	leftW := m.leftPanelWidth()
	rightW := m.rightPanelWidth()

	leftLines := strings.Split(m.renderLeft(leftW), "\n")
	rightLines := strings.Split(m.renderRight(rightW), "\n")

	// Ensure both panels have exactly m.height lines.
	for len(leftLines) < m.height {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < m.height {
		rightLines = append(rightLines, "")
	}

	sep := styleDim.Render("│")
	var result []string
	for i := 0; i < m.height; i++ {
		left := padToWidth(leftLines[i], leftW)
		right := padToWidth(rightLines[i], rightW)
		result = append(result, left+sep+right)
	}
	return strings.Join(result, "\n")
}

func padToWidth(line string, width int) string {
	visible := lipgloss.Width(line)
	if visible < width {
		return line + strings.Repeat(" ", width-visible)
	}
	return line
}

func (m Model) renderLeft(w int) string {
	divider := styleDim.Render(strings.Repeat("─", w))

	title := styleTitle.Render(" Squirrel 0.9.9 ")
	var subtitle string
	if len(m.repoPaths) > 1 {
		subtitle = fmt.Sprintf("%d repos", len(m.repoPaths))
	} else {
		subtitle = m.repoNames[0]
	}
	subtitleStr := styleDim.Render(subtitle)
	spacer := strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(subtitleStr)))
	header := title + spacer + subtitleStr

	filterRow := styleDim.Render("  filter: ") + m.filter.View()

	listH := m.listHeight()
	end := min(m.scrollOffset+listH, len(m.rows))
	rendered := make([]string, 0, listH)
	for i := m.scrollOffset; i < end; i++ {
		rendered = append(rendered, m.renderRow(m.rows[i], i == m.cursor, w))
	}
	for len(rendered) < listH {
		rendered = append(rendered, "")
	}

	var footer string
	if m.confirmMode {
		if m.confirmAction == confirmDeleteLocal {
			footer = " " + styleDanger.Render(fmt.Sprintf("delete '%s'? [d] confirm  [esc] cancel", m.confirmItem.branch.Name))
		} else {
			footer = " " + styleDanger.Render(fmt.Sprintf("delete local+remote '%s'? [D] confirm  [esc] cancel", m.confirmItem.branch.Name))
		}
	} else {
		footer = styleDim.Render(" ↑↓ nav  enter checkout  d del  D del+remote  tab terminal  q quit")
	}

	return strings.Join([]string{
		header,
		"",
		filterRow,
		"",
		strings.Join(rendered, "\n"),
		divider,
		footer,
	}, "\n")
}

func (m Model) renderRight(w int) string {
	divider := styleDim.Render(strings.Repeat("─", w))
	outH := m.outputAreaHeight()

	// Output header.
	outputHeader := styleDim.Render(" output")

	// Output lines (scroll window).
	outputRendered := make([]string, 0, outH)
	for i := 0; i < outH; i++ {
		lineIdx := m.outputScroll + i
		if lineIdx < len(m.outputLines) {
			outputRendered = append(outputRendered, " "+m.outputLines[lineIdx])
		} else {
			outputRendered = append(outputRendered, "")
		}
	}

	// Terminal header with focus indicator.
	var focusIndicator string
	if m.termFocused {
		focusIndicator = styleStatus.Render("● ")
	} else {
		focusIndicator = styleDim.Render("○ ")
	}
	termHeader := " " + focusIndicator + styleDim.Render("terminal  (tab focus, esc blur, ↑↓ scroll output)")
	termLine := " " + m.termInput.View()

	return strings.Join([]string{
		outputHeader,
		strings.Join(outputRendered, "\n"),
		divider,
		termHeader,
		termLine,
	}, "\n")
}

func (m Model) renderRow(r row, selected bool, w int) string {
	switch r.kind {
	case rowTypeHeader:
		return m.renderHeader(r, w)
	case rowTypeBranch:
		return m.renderBranch(r, selected, w)
	case rowTypeToggle:
		return m.renderToggle(r, selected, w)
	case rowTypeSpacer:
		return ""
	}
	return ""
}

func (m Model) renderHeader(r row, w int) string {
	name := m.repoNames[r.repoIdx]
	prefix := " ── " + name + " "
	line := prefix + strings.Repeat("─", max(0, w-len(prefix)))
	return styleRepoHeader.Render(line)
}

func (m Model) renderToggle(r row, selected bool, w int) string {
	var label string
	if r.isExpanded {
		label = fmt.Sprintf("  ▾ hide %d older branches", r.toggleCount)
	} else {
		label = fmt.Sprintf("  ▸ %d older branches", r.toggleCount)
	}
	if selected {
		return lipgloss.NewStyle().Background(colorSelection).Foreground(colorDim).Width(w).Render(label)
	}
	return styleToggle.Render(label)
}

func (m Model) renderBranch(r row, selected bool, w int) string {
	item := m.filteredItems[r.repoIdx][r.itemIdx]

	const timeWidth = 8
	const remoteWidth = 2  // "↑ " or "  "
	const prefixWidth = 2  // "* " or "  "
	const rightSectionWidth = remoteWidth + timeWidth // 10

	// middleWidth is the space between prefix and the right section.
	// remote+time are always anchored at w - rightSectionWidth, so ↑ is always aligned.
	middleWidth := w - prefixWidth - rightSectionWidth
	hasLinear := item.issue != nil

	var branchColW, linearColW int
	if hasLinear {
		branchColW = 35
		if branchColW > middleWidth*2/5 {
			branchColW = middleWidth * 2 / 5
		}
		linearColW = middleWidth - branchColW - 2 // 2 for sep between branch and linear
		if linearColW < 10 {
			linearColW = 10
			branchColW = middleWidth - 2 - linearColW
		}
	} else {
		branchColW = middleWidth
	}

	prefix := "  "
	if item.branch.IsCurrent {
		prefix = "* "
	}
	branchRaw := prefix + item.branch.Name
	branchRunes := []rune(branchRaw)
	if len(branchRunes) > branchColW {
		branchRunes = append(branchRunes[:branchColW-1], '…')
	}
	branchPadded := string(branchRunes) + strings.Repeat(" ", max(0, branchColW-len(branchRunes)))

	timeStr := fmt.Sprintf("%-*s", timeWidth, relativeTime(item.branch.LastCommitTime))

	var remoteStr string
	if item.branch.HasRemote() {
		remoteStr = "↑ "
	} else {
		remoteStr = "  "
	}

	if selected {
		return m.renderBranchSelected(item, branchPadded, remoteStr, timeStr, hasLinear, linearColW, w)
	}
	return m.renderBranchNormal(item, branchPadded, remoteStr, timeStr, hasLinear, linearColW)
}

func (m Model) renderBranchNormal(item branchItem, branchPadded, remoteStr, timeStr string, hasLinear bool, linearColW int) string {
	var branchStyled string
	if item.branch.IsCurrent {
		branchStyled = styleCurrent.Render(branchPadded)
	} else {
		branchStyled = branchPadded
	}

	var remoteStyled string
	if item.branch.HasRemote() {
		remoteStyled = styleRemote.Render(remoteStr)
	} else {
		remoteStyled = remoteStr
	}

	timeStyled := styleDim.Render(timeStr)

	if !hasLinear {
		// branch fills middleWidth, remote+time follow immediately — ↑ stays aligned
		return "  " + branchStyled + remoteStyled + timeStyled
	}

	linearRaw := item.issue.Identifier + " " + item.issue.Title
	linearRunes := []rune(linearRaw)
	if len(linearRunes) > linearColW {
		linearRunes = append(linearRunes[:linearColW-1], '…')
	}
	linearPadded := string(linearRunes) + strings.Repeat(" ", max(0, linearColW-len(linearRunes)))

	spaceIdx := strings.Index(linearPadded, " ")
	var linearStyled string
	if spaceIdx > 0 {
		linearStyled = styleLinearID.Render(linearPadded[:spaceIdx]) + styleLinearDim.Render(linearPadded[spaceIdx:])
	} else {
		linearStyled = styleLinearID.Render(linearPadded)
	}

	// branch + sep(2) + linear fills middleWidth, remote+time follow immediately
	return "  " + branchStyled + "  " + linearStyled + remoteStyled + timeStyled
}

func (m Model) renderBranchSelected(item branchItem, branchPadded, remoteStr, timeStr string, hasLinear bool, linearColW int, w int) string {
	bg := lipgloss.NewStyle().Background(colorSelection)

	// Branch name: amber if current (git), white otherwise.
	var branchStyled string
	if item.branch.IsCurrent {
		branchStyled = bg.Foreground(colorAmber).Bold(true).Render(branchPadded)
	} else {
		branchStyled = bg.Foreground(colorWhite).Render(branchPadded)
	}

	remoteStyled := bg.Foreground(colorBlue).Render(remoteStr)
	timeStyled := bg.Foreground(colorDim).Render(timeStr)
	sepStyled := bg.Render("  ")

	var line string
	if hasLinear {
		linearRaw := item.issue.Identifier + " " + item.issue.Title
		linearRunes := []rune(linearRaw)
		if len(linearRunes) > linearColW {
			linearRunes = append(linearRunes[:linearColW-1], '…')
		}
		linearPadded := string(linearRunes) + strings.Repeat(" ", max(0, linearColW-len(linearRunes)))
		spaceIdx := strings.Index(linearPadded, " ")
		var linearStyled string
		if spaceIdx > 0 {
			linearStyled = bg.Foreground(colorBlue).Bold(true).Render(linearPadded[:spaceIdx]) +
				bg.Foreground(colorDim).Render(linearPadded[spaceIdx:])
		} else {
			linearStyled = bg.Foreground(colorBlue).Bold(true).Render(linearPadded)
		}
		line = bg.Render("  ") + branchStyled + sepStyled + linearStyled + remoteStyled + timeStyled
	} else {
		line = bg.Render("  ") + branchStyled + remoteStyled + timeStyled
	}

	// Pad the remainder with the selection background color.
	visibleWidth := lipgloss.Width(line)
	if visibleWidth < w {
		line += bg.Render(strings.Repeat(" ", w-visibleWidth))
	}
	return line
}

func relativeTime(t time.Time) string {
	since := time.Since(t)
	switch {
	case since < time.Minute:
		return "now"
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh", int(since.Hours()))
	case since < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(since.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
