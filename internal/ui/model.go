package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adamarutyunov/launch/embed"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"squirrel/internal/agent"
	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)


var (
	colorGreen     = lipgloss.Color("#22c55e")
	colorBlue      = lipgloss.Color("#60a5fa")
	colorDim       = lipgloss.Color("#71717a")
	colorWhite     = lipgloss.Color("#f4f4f5")
	colorSelection = lipgloss.Color("#3f3f46")
	colorAmber     = lipgloss.Color("#f59e0b")
	colorRed       = lipgloss.Color("#ef4444")

	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	styleDim       = lipgloss.NewStyle().Foreground(colorDim)
	styleMain      = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	styleLinearID  = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleLinearDim = lipgloss.NewStyle().Foreground(colorDim)
	styleHeader    = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	styleStatus    = lipgloss.NewStyle().Foreground(colorGreen)
	styleDanger    = lipgloss.NewStyle().Foreground(colorRed)
	styleWarning   = lipgloss.NewStyle().Foreground(colorAmber)
)

type rowType int

const (
	rowTypeHeader rowType = iota
	rowTypeContext
	rowTypeSpacer
)

type row struct {
	kind    rowType
	repoIdx int
	itemIdx int
}

type contextItem struct {
	context workspace.Context
}

type uiMode int

const (
	modeBrowsing uiMode = iota
	modeCreating
	modeDeleteConfirm
)

type sortMode int

const (
	sortModeAgent sortMode = iota
	sortModeAlphabetical
	sortModeLinear
	sortModeUpdated
)

// Model is the BubbleTea model.
type Model struct {
	version      string
	repoPaths    []string
	repoNames    []string
	repoConfigs  []workspace.Config
	repoItems    [][]contextItem
	linearIssues map[string]linear.Issue

	filteredItems [][]contextItem
	rows          []row

	cursor       int
	scrollOffset int

	filter      textinput.Model
	filterValue string
	sortMode    sortMode

	width  int
	height int

	mode          uiMode
	createInput   textinput.Model
	createRepoIdx int

	// Linear issue picker (shown while in modeCreating when API key is set).
	linearAPIKey  string
	pickerIssues  []linear.Issue
	pickerCursor  int // -1 = no selection; ≥0 = index into filteredPickerIssues()
	pickerScroll  int // top visible index in filtered picker list
	pickerLoading bool

	// Delete confirmation state.
	deleteItem    contextItem
	deleteRepoIdx int

	// Active context — set by Enter; shown with amber * in the list.
	selectedContextPath string

	// Pending cursor target — after refresh, move cursor to this path.
	pendingCursorPath string

	// Status messages shown above the footer.
	outputLines []string

	// Companion tmux pane (real terminal on the right).
	companionPaneID string

	// Right panel: launch integration (when active, replaces terminal panel).
	launchPanel   *embed.Model
	launchFocused bool

	// Spinner animation frame counter (incremented every tick).
	spinnerFrame int

	// Agent command from user config.
	agentCommand string
}

func NewModel(
	repoPaths []string,
	repoContexts [][]workspace.Context,
	repoConfigs []workspace.Config,
	linearIssues map[string]linear.Issue,
	linearAPIKey string,
	agentCommand string,
	companionPaneID string,
	version string,
) Model {
	repoNames := make([]string, len(repoPaths))
	for i, path := range repoPaths {
		repoNames[i] = filepath.Base(path)
	}

	repoItems := make([][]contextItem, len(repoPaths))
	for repoIdx, contexts := range repoContexts {
		items := make([]contextItem, len(contexts))
		for i, ctx := range contexts {
			items[i] = contextItem{context: ctx}
		}
		repoItems[repoIdx] = items
	}

	filterInput := textinput.New()
	filterInput.Placeholder = "type..."
	filterInput.Focus()
	filterInput.Prompt = ""

	createInput := textinput.New()
	createInput.Placeholder = "context name or filter issues..."
	createInput.Prompt = ""

	m := Model{
		version:         version,
		repoPaths:       repoPaths,
		repoNames:       repoNames,
		repoConfigs:     repoConfigs,
		repoItems:       repoItems,
		linearIssues:    linearIssues,
		linearAPIKey:    linearAPIKey,
		agentCommand:    agentCommand,
		companionPaneID: companionPaneID,
		pickerCursor:    -1,
		filter:          filterInput,
		createInput:     createInput,
		sortMode:        sortModeAgent,
	}
	m.rebuild()
	return m
}

func (m *Model) rebuild() {
	query := strings.ToLower(m.filterValue)
	filterActive := query != ""
	multiRepo := len(m.repoPaths) > 1

	m.filteredItems = make([][]contextItem, len(m.repoPaths))
	for repoIdx, items := range m.repoItems {
		var sourceItems []contextItem
		if !filterActive {
			sourceItems = append([]contextItem(nil), items...)
		} else {
			for _, item := range items {
				ctx := item.context
				if strings.Contains(strings.ToLower(ctx.Name), query) ||
					strings.Contains(strings.ToLower(ctx.Branch), query) {
					sourceItems = append(sourceItems, item)
					continue
				}
				if ctx.LinearIssue != nil && strings.Contains(strings.ToLower(ctx.LinearIssue.Title), query) {
					sourceItems = append(sourceItems, item)
				}
			}
		}
		m.sortItems(sourceItems)
		m.filteredItems[repoIdx] = sourceItems
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
		for itemIdx := range items {
			m.rows = append(m.rows, row{kind: rowTypeContext, repoIdx: repoIdx, itemIdx: itemIdx})
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

func isNavigable(kind rowType) bool { return kind == rowTypeContext }

func (m *Model) moveCursor(dir int) {
	if len(m.rows) == 0 {
		return
	}

	var navigableRows []int
	for rowIndex, currentRow := range m.rows {
		if isNavigable(currentRow.kind) {
			navigableRows = append(navigableRows, rowIndex)
		}
	}
	if len(navigableRows) == 0 {
		return
	}

	currentIndex := 0
	for index, rowIndex := range navigableRows {
		if rowIndex == m.cursor {
			currentIndex = index
			break
		}
	}

	nextIndex := currentIndex + dir
	if nextIndex < 0 {
		nextIndex = len(navigableRows) - 1
	} else if nextIndex >= len(navigableRows) {
		nextIndex = 0
	}
	m.cursor = navigableRows[nextIndex]
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

func (m Model) footerLineCount() int {
	if m.mode == modeCreating {
		if m.pickerLoading {
			return 2 // input line + "Loading..." line
		}
		if n := len(m.filteredPickerIssues()); n > 0 {
			return 1 + min(10, n) // input line + up to 10 issue lines
		}
	}
	return 1
}

func (m Model) statusLineCount() int {
	n := len(m.outputLines)
	if n > 3 {
		return 3
	}
	return n
}

func (m Model) listHeight() int {
	// top-pad(1) + header(1) + blank(1) + filter(1) + blank(1) + status(N) + divider(1) + footer(N)
	fixed := 6 + m.statusLineCount() + m.footerLineCount()
	h := m.height - fixed
	if h < 1 {
		return 1
	}
	return h
}


func (m Model) launchPanelHeight() int { return m.height / 2 }


func (m Model) renderPrompt(ctx workspace.Context) string {
	home, _ := os.UserHomeDir()
	path := ctx.Path
	if home != "" {
		path = strings.Replace(path, home, "~", 1)
	}
	// Abbreviate intermediate path segments: ~/t/rcm-eng-9119
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts)-1; i++ {
		if len(parts[i]) > 1 {
			parts[i] = string([]rune(parts[i])[:1])
		}
	}
	shortPath := strings.Join(parts, "/")

	line := styleStatus.Render(shortPath)
	if ctx.Branch != "" {
		line += styleDim.Render(" (") + styleDim.Render(ctx.Branch) + styleDim.Render(")")
	}
	line += styleStatus.Render(" >")
	return line
}

func (m *Model) appendOutput(line string) {
	m.outputLines = append(m.outputLines, line)
}

func (m *Model) activeContextPath() string {
	if m.selectedContextPath != "" {
		return m.selectedContextPath
	}
	if m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.kind == rowTypeContext && r.repoIdx < len(m.filteredItems) && r.itemIdx < len(m.filteredItems[r.repoIdx]) {
			return m.filteredItems[r.repoIdx][r.itemIdx].context.Path
		}
	}
	if len(m.repoPaths) > 0 {
		return m.repoPaths[0]
	}
	return "."
}

func (m *Model) filteredPickerIssues() []linear.Issue {
	if len(m.pickerIssues) == 0 {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(m.createInput.Value()))
	if query == "" {
		return m.pickerIssues
	}
	var filtered []linear.Issue
	for _, issue := range m.pickerIssues {
		if strings.Contains(strings.ToLower(issue.Identifier), query) ||
			strings.Contains(strings.ToLower(issue.Title), query) ||
			strings.Contains(strings.ToLower(issue.BranchName), query) {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func (m *Model) cycleSortMode() {
	switch m.sortMode {
	case sortModeAgent:
		m.sortMode = sortModeAlphabetical
	case sortModeAlphabetical:
		m.sortMode = sortModeLinear
	case sortModeLinear:
		m.sortMode = sortModeUpdated
	default:
		m.sortMode = sortModeAgent
	}
	m.rebuild()
}

func (m Model) sortModeLabel() string {
	switch m.sortMode {
	case sortModeAlphabetical:
		return "Alpha"
	case sortModeLinear:
		return "Linear"
	case sortModeUpdated:
		return "Updated"
	default:
		return "Agent"
	}
}

func (m Model) sortItems(items []contextItem) {
	sort.SliceStable(items, func(leftIndex, rightIndex int) bool {
		leftContext := items[leftIndex].context
		rightContext := items[rightIndex].context

		switch m.sortMode {
		case sortModeAlphabetical:
			return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
		case sortModeLinear:
			return compareLinearContexts(leftContext, rightContext)
		case sortModeUpdated:
			return compareUpdatedContexts(leftContext, rightContext)
		default:
			return compareAgentContexts(leftContext, rightContext)
		}
	})
}

func compareAgentContexts(leftContext, rightContext workspace.Context) bool {
	leftRank := agentStatusRank(leftContext.AgentStatus)
	rightRank := agentStatusRank(rightContext.AgentStatus)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return compareUpdatedContexts(leftContext, rightContext)
}

func compareUpdatedContexts(leftContext, rightContext workspace.Context) bool {
	if !leftContext.HeadTime.Equal(rightContext.HeadTime) {
		return leftContext.HeadTime.After(rightContext.HeadTime)
	}
	return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
}

func compareLinearContexts(leftContext, rightContext workspace.Context) bool {
	leftTeam, leftNumber, leftHasIssue := linearSortKey(leftContext)
	rightTeam, rightNumber, rightHasIssue := linearSortKey(rightContext)
	if leftHasIssue != rightHasIssue {
		return leftHasIssue
	}
	if leftTeam != rightTeam {
		return leftTeam < rightTeam
	}
	if leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
}

func linearSortKey(context workspace.Context) (string, int, bool) {
	if context.LinearIssue == nil {
		return "", 0, false
	}
	parts := strings.SplitN(context.LinearIssue.Identifier, "-", 2)
	if len(parts) != 2 {
		return strings.ToLower(context.LinearIssue.Identifier), 0, true
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil {
		number = 0
	}
	return strings.ToLower(parts[0]), number, true
}

func agentStatusRank(status string) int {
	switch status {
	case agent.StatusDone:
		return 0
	case agent.StatusThinking:
		return 1
	case agent.StatusIdle:
		return 2
	default:
		return 3
	}
}

// --- Tea messages ---

type tickMsg time.Time

type refreshMsg struct {
	repoIdx  int
	contexts []workspace.Context
}

type createContextResultMsg struct {
	repoIdx      int
	contextName  string
	worktreePath string
	err          error
}

type setupCommandResultMsg struct {
	output string
	err    error
}

type deleteContextResultMsg struct {
	repoIdx     int
	err         error
	newContexts []workspace.Context // fresh list fetched immediately after the operation
}

type clipboardMsg struct {
	path string
	err  error
}

type linearIssuesLoadedMsg struct {
	issues []linear.Issue
	err    error
}

type agentAttachFinishedMsg struct {
	err error
}

type agentLaunchBackgroundMsg struct {
	err error
}

// --- Tea commands ---

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func refreshRepoCmd(repoIdx int, repoPath string, linearIssues map[string]linear.Issue) tea.Cmd {
	return func() tea.Msg {
		contexts, err := workspace.ListContexts(repoPath, linearIssues)
		if err != nil {
			return nil
		}
		return refreshMsg{repoIdx: repoIdx, contexts: contexts}
	}
}

func createContextCmd(repoIdx int, repoPath, contextName string, cfg workspace.Config) tea.Cmd {
	return func() tea.Msg {
		worktreePath, err := workspace.CreateContext(repoPath, contextName, contextName, cfg)
		return createContextResultMsg{repoIdx: repoIdx, contextName: contextName, worktreePath: worktreePath, err: err}
	}
}

func setupCommandCmd(worktreePath, command string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return setupCommandResultMsg{}
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = worktreePath
		output, err := cmd.CombinedOutput()
		return setupCommandResultMsg{output: strings.TrimSpace(string(output)), err: err}
	}
}

func deleteContextCmd(repoIdx int, repoPath string, ctx workspace.Context, force bool, linearIssues map[string]linear.Issue) tea.Cmd {
	return func() tea.Msg {
		err := workspace.DeleteContext(ctx, force)
		newContexts, _ := workspace.ListContexts(repoPath, linearIssues)
		return deleteContextResultMsg{repoIdx: repoIdx, err: err, newContexts: newContexts}
	}
}

func copyToClipboardCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(path)
		return clipboardMsg{path: path, err: err}
	}
}

func fetchLinearIssuesCmd(apiKey string) tea.Cmd {
	return func() tea.Msg {
		client := linear.NewClient(apiKey)
		issues, err := client.FetchAssignedIssues()
		return linearIssuesLoadedMsg{issues: issues, err: err}
	}
}

func launchAgentBackgroundCmd(contextPath, command string) tea.Cmd {
	return func() tea.Msg {
		err := agent.LaunchBackground(contextPath, command)
		return agentLaunchBackgroundMsg{err: err}
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
		if m.launchPanel != nil {
			newPanel, cmd := m.launchPanel.Update(tea.WindowSizeMsg{
				Width:  m.width,
				Height: m.launchPanelHeight(),
			})
			*m.launchPanel = newPanel
			return m, cmd
		}
		return m, nil

	case tickMsg:
		m.spinnerFrame++
		cmds := []tea.Cmd{tickCmd()}
		// Refresh repos every 4th tick (~2 seconds).
		if m.spinnerFrame%4 == 0 {
			for i, path := range m.repoPaths {
				cmds = append(cmds, refreshRepoCmd(i, path, m.linearIssues))
			}
		}
		return m, tea.Batch(cmds...)

	case refreshMsg:
		m.applyRefresh(msg)
		return m, nil

	case embed.EventMsg:
		if m.launchPanel != nil {
			newPanel, cmd := m.launchPanel.Update(msg)
			*m.launchPanel = newPanel
			return m, cmd
		}
		return m, nil

	case createContextResultMsg:
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Create failed: " + msg.err.Error()))
			return m, nil
		}
		m.appendOutput(styleStatus.Render("✓ Created '" + msg.contextName + "'"))
		m.appendOutput(styleDim.Render("  " + msg.worktreePath))
		m.pendingCursorPath = msg.worktreePath

		cfg := m.repoConfigs[msg.repoIdx]
		refreshCmd := refreshRepoCmd(msg.repoIdx, m.repoPaths[msg.repoIdx], m.linearIssues)
		agentCmd := launchAgentBackgroundCmd(msg.worktreePath, agent.PreferredCommand(m.agentCommand))
		if cfg.SetupCommand != "" {
			m.appendOutput(styleDim.Render("  Running: " + cfg.SetupCommand))
			return m, tea.Batch(refreshCmd, setupCommandCmd(msg.worktreePath, cfg.SetupCommand), agentCmd)
		}
		return m, tea.Batch(refreshCmd, agentCmd)

	case setupCommandResultMsg:
		if msg.output != "" {
			for _, line := range strings.Split(msg.output, "\n") {
				m.appendOutput("  " + line)
			}
		}
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Setup failed: " + msg.err.Error()))
		} else {
			m.appendOutput(styleStatus.Render("✓ Setup complete"))
		}
		return m, nil

	case deleteContextResultMsg:
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Delete failed: " + msg.err.Error()))
		} else {
			m.appendOutput(styleStatus.Render("✓ Deleted"))
		}
		m.applyRefresh(refreshMsg{repoIdx: msg.repoIdx, contexts: msg.newContexts})
		return m, nil

	case clipboardMsg:
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Clipboard: " + msg.err.Error()))
		} else {
			m.appendOutput(styleStatus.Render("Copied: " + msg.path))
		}
		return m, nil

	case linearIssuesLoadedMsg:
		m.pickerLoading = false
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Linear: " + msg.err.Error()))
		} else {
			m.pickerIssues = msg.issues
			if len(msg.issues) > 0 {
				m.pickerCursor = 0
			}
		}
		return m, nil

	case agentAttachFinishedMsg:
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Agent: " + msg.err.Error()))
		}
		return m, nil

	case agentLaunchBackgroundMsg:
		if msg.err != nil {
			m.appendOutput(styleDanger.Render("✗ Agent launch: " + msg.err.Error()))
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		if m.launchFocused && m.launchPanel != nil {
			// ctrl+c in launch pane = kill processes and close panel
			m.launchPanel.StopAll()
			m.launchPanel = nil
			m.launchFocused = false
			m.appendOutput(styleStatus.Render("✓ Processes stopped"))
			return m, nil
		}
		return m, tea.Quit
	}

	if msg.Type == tea.KeyTab {
		if m.launchPanel != nil {
			// Toggle focus between context list and launch panel.
			m.launchFocused = !m.launchFocused
		}
		return m, nil
	}

	// Forward all keys to launch when it has focus (except q = detach).
	if m.launchFocused && m.launchPanel != nil {
		if msg.String() == "q" {
			if err := m.launchPanel.SaveState(); err != nil {
				m.appendOutput(styleWarning.Render("⚠ Save state: " + err.Error()))
			}
			m.launchPanel = nil
			m.launchFocused = false
			m.appendOutput(styleDim.Render("Detached — processes still running"))
			return m, nil
		}
		newPanel, cmd := m.launchPanel.Update(msg)
		*m.launchPanel = newPanel
		return m, cmd
	}

	return m.handleListKey(msg)
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeCreating {
		return m.handleCreateKey(msg)
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.mode == modeDeleteConfirm {
			m.mode = modeBrowsing
			return m, nil
		}
		if m.filterValue != "" {
			m.filter.SetValue("")
			m.filterValue = ""
			m.rebuild()
		}
		return m, nil

	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil

	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil

	case tea.KeyEnter:
		if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowTypeContext {
			r := m.rows[m.cursor]
			ctx := m.filteredItems[r.repoIdx][r.itemIdx].context
			m.selectedContextPath = ctx.Path
			m.appendOutput(m.renderPrompt(ctx))
			// Send cd to companion terminal pane.
			if m.companionPaneID != "" {
				escapedPath := strings.ReplaceAll(ctx.Path, "'", "'\\''")
				exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-c", "").Run()
				exec.Command("tmux", "send-keys", "-t", m.companionPaneID,
					fmt.Sprintf("cd '%s'", escapedPath), "Enter").Run()
				exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-l", "").Run()
			}
		}
		return m, nil
	}

	key := msg.String()
	switch key {
	case "d":
		if m.filterValue == "" {
			return m.handleDeleteKey()
		}
	case "n":
		if m.filterValue == "" {
			return m.startCreateContext()
		}
	case "c":
		if m.filterValue == "" {
			return m.copyContextPath()
		}
	case "l":
		if m.filterValue == "" {
			return m.toggleLaunch()
		}
	case "a":
		if m.filterValue == "" {
			return m.toggleAgent()
		}
	case "A":
		if m.filterValue == "" {
			return m.attachAgentFullscreen()
		}
	case "s":
		m.cycleSortMode()
		return m, nil
	case "j":
		m.moveCursor(1)
		return m, nil
	case "k":
		m.moveCursor(-1)
		return m, nil
	case "q":
		if m.filterValue == "" {
			if m.launchPanel != nil {
				if err := m.launchPanel.SaveState(); err != nil {
					m.appendOutput(styleWarning.Render("⚠ Save state: " + err.Error()))
				}
				m.launchPanel = nil
				m.launchFocused = false
				m.appendOutput(styleDim.Render("Detached — processes still running"))
				return m, nil
			}
			return m, tea.Quit
		}
	}

	prevValue := m.filterValue
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.filterValue = m.filter.Value()
	if m.filterValue != prevValue {
		m.mode = modeBrowsing
		m.rebuild()
	}
	return m, cmd
}

func (m Model) handleCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeBrowsing
		m.createInput.SetValue("")
		m.createInput.Blur()
		m.filter.Focus()
		m.pickerIssues = nil
		m.pickerCursor = -1
		m.pickerScroll = 0
		m.pickerLoading = false
		return m, nil

	case tea.KeyUp:
		filtered := m.filteredPickerIssues()
		if len(filtered) == 0 {
			return m, nil
		}
		if m.pickerCursor <= 0 {
			m.pickerCursor = len(filtered) - 1
			if len(filtered) > 10 {
				m.pickerScroll = len(filtered) - 10
			} else {
				m.pickerScroll = 0
			}
		} else {
			m.pickerCursor--
			if m.pickerCursor < m.pickerScroll {
				m.pickerScroll = m.pickerCursor
			}
		}
		return m, nil

	case tea.KeyDown:
		filtered := m.filteredPickerIssues()
		if len(filtered) == 0 {
			return m, nil
		}
		if m.pickerCursor < len(filtered)-1 {
			m.pickerCursor++
			if m.pickerCursor >= m.pickerScroll+10 {
				m.pickerScroll = m.pickerCursor - 9
			}
		} else {
			m.pickerCursor = 0
			m.pickerScroll = 0
		}
		return m, nil

	case tea.KeyEnter:
		var name string
		filtered := m.filteredPickerIssues()
		if m.pickerCursor >= 0 && m.pickerCursor < len(filtered) {
			selectedIssue := filtered[m.pickerCursor]
			// Pre-register the issue so refresh immediately links the new context.
			m.linearIssues[selectedIssue.Identifier] = selectedIssue
			branchName := selectedIssue.BranchName
			if branchName == "" {
				branchName = selectedIssue.Identifier
			}
			name = workspace.SanitizeName(branchName)
		} else {
			name = workspace.SanitizeName(m.createInput.Value())
		}
		if name == "" {
			return m, nil
		}
		m.mode = modeBrowsing
		m.createInput.SetValue("")
		m.createInput.Blur()
		m.filter.Focus()
		m.pickerIssues = nil
		m.pickerCursor = -1
		m.pickerScroll = 0
		m.pickerLoading = false

		repoPath := m.repoPaths[m.createRepoIdx]
		cfg := m.repoConfigs[m.createRepoIdx]
		m.appendOutput(styleDim.Render("Creating context '" + name + "'..."))
		return m, createContextCmd(m.createRepoIdx, repoPath, name, cfg)
	}

	prevValue := m.createInput.Value()
	var cmd tea.Cmd
	m.createInput, cmd = m.createInput.Update(msg)
	if m.createInput.Value() != prevValue {
		m.pickerCursor = 0
		m.pickerScroll = 0
	}
	return m, cmd
}

func (m Model) startCreateContext() (tea.Model, tea.Cmd) {
	repoIdx := 0
	if m.cursor < len(m.rows) && m.rows[m.cursor].repoIdx < len(m.repoPaths) {
		repoIdx = m.rows[m.cursor].repoIdx
	}
	m.createRepoIdx = repoIdx
	m.mode = modeCreating
	m.createInput.Focus()
	m.filter.Blur()

	if m.linearAPIKey != "" && len(m.pickerIssues) == 0 {
		m.pickerCursor = 0
		m.pickerLoading = true
		return m, fetchLinearIssuesCmd(m.linearAPIKey)
	}
	if len(m.pickerIssues) > 0 {
		m.pickerCursor = 0
	} else {
		m.pickerCursor = -1
	}
	return m, nil
}

func (m Model) copyContextPath() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	item := m.filteredItems[r.repoIdx][r.itemIdx]
	return m, copyToClipboardCmd(item.context.Path)
}

func (m Model) handleDeleteKey() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	item := m.filteredItems[r.repoIdx][r.itemIdx]

	if item.context.IsMain {
		m.appendOutput(styleDanger.Render("✗ Cannot delete main context"))
		return m, nil
	}

	if !item.context.IsDirty {
		m.appendOutput(styleDim.Render("Deleting '" + item.context.Name + "'..."))
		return m, deleteContextCmd(r.repoIdx, m.repoPaths[r.repoIdx], item.context, true, m.linearIssues)
	}

	// Dirty — require double-press confirmation.
	if m.mode == modeDeleteConfirm && m.deleteItem.context.Path == item.context.Path {
		m.mode = modeBrowsing
		m.appendOutput(styleDim.Render("Force deleting '" + item.context.Name + "'..."))
		return m, deleteContextCmd(r.repoIdx, m.repoPaths[r.repoIdx], item.context, true, m.linearIssues)
	}

	m.mode = modeDeleteConfirm
	m.deleteItem = item
	m.deleteRepoIdx = r.repoIdx
	m.appendOutput(styleDanger.Render(fmt.Sprintf(
		"Delete '%s'? Press d again to confirm, Esc to cancel", item.context.Name)))
	m.appendOutput(styleWarning.Render("  ⚠ Context has uncommitted changes"))
	return m, nil
}

func (m Model) toggleLaunch() (tea.Model, tea.Cmd) {
	// Close if already open.
	if m.launchPanel != nil {
		if err := m.launchPanel.SaveState(); err != nil {
			m.appendOutput(styleWarning.Render("⚠ Save state: " + err.Error()))
		}
		m.launchPanel.StopAll()
		m.launchPanel = nil
		m.launchFocused = false
		return m, nil
	}

	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	contextPath := m.filteredItems[r.repoIdx][r.itemIdx].context.Path

	panel, err := embed.New(contextPath)
	if err != nil {
		m.appendOutput(styleDanger.Render("✗ Launch: " + err.Error()))
		return m, nil
	}
	if !panel.HasProcesses() {
		m.appendOutput(styleWarning.Render("⚠ No launch.yml found in " + filepath.Base(contextPath)))
		return m, nil
	}

	m.launchPanel = &panel
	m.launchFocused = true

	// Size the launch panel to the top half of the right panel.
	sizedPanel, sizeCmd := m.launchPanel.Update(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.launchPanelHeight(),
	})
	*m.launchPanel = sizedPanel

	initCmd := m.launchPanel.Init()
	return m, tea.Batch(initCmd, sizeCmd)
}

func (m Model) toggleAgent() (tea.Model, tea.Cmd) {
	if m.companionPaneID == "" {
		return m, nil
	}
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}

	r := m.rows[m.cursor]
	contextPath := m.filteredItems[r.repoIdx][r.itemIdx].context.Path
	command := agent.PreferredCommand(m.agentCommand)

	// Build the command to run in the companion pane: attach to existing tmux
	// agent session, or start a new one (with --resume if we have a session ID).
	agentCommand := command
	if !agent.SessionExists(contextPath, command) {
		if strings.Fields(command)[0] == "claude" {
			if sessionID, _ := agent.ReadSessionID(contextPath); sessionID != "" {
				agentCommand = command + " --resume " + sessionID
			}
		}
	}

	// Respawn the companion pane with the agent command in the context directory.
	escapedPath := strings.ReplaceAll(contextPath, "'", "'\\''")
	exec.Command("tmux", "respawn-pane", "-k", "-t", m.companionPaneID,
		"-c", contextPath, fmt.Sprintf("exec %s", agentCommand)).Run()

	m.appendOutput(styleDim.Render("Agent: " + filepath.Base(escapedPath) + " (" + command + ")"))

	// Focus the companion pane.
	exec.Command("tmux", "select-pane", "-t", m.companionPaneID).Run()
	return m, nil
}

func (m Model) attachAgentFullscreen() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}

	r := m.rows[m.cursor]
	contextPath := m.filteredItems[r.repoIdx][r.itemIdx].context.Path
	command := agent.PreferredCommand(m.agentCommand)
	m.appendOutput(styleDim.Render("Attaching agent (fullscreen): " + filepath.Base(contextPath) + "  (ctrl+q to detach)"))
	return m, tea.ExecProcess(agent.AttachCommand(contextPath, command), func(err error) tea.Msg {
		return agentAttachFinishedMsg{err: err}
	})
}

func (m *Model) applyRefresh(msg refreshMsg) {
	// Pending cursor (e.g. newly created context) takes priority.
	targetPath := m.pendingCursorPath
	if targetPath == "" && m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowTypeContext {
		r := m.rows[m.cursor]
		targetPath = m.filteredItems[r.repoIdx][r.itemIdx].context.Path
	}

	items := make([]contextItem, len(msg.contexts))
	for i, ctx := range msg.contexts {
		items[i] = contextItem{context: ctx}
	}
	m.repoItems[msg.repoIdx] = items
	m.rebuild()

	if targetPath != "" {
		for rowIdx, r := range m.rows {
			if r.kind == rowTypeContext {
				if m.filteredItems[r.repoIdx][r.itemIdx].context.Path == targetPath {
					m.cursor = rowIdx
					m.pendingCursorPath = ""
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

	return m.renderLeft(m.width)
}

func (m Model) renderLeft(w int) string {
	divider := styleDim.Render(strings.Repeat("─", w))

	titleText := "Squirrel " + m.version
	title := styleTitle.Render("  " + titleText)
	var subtitle string
	switch len(m.repoPaths) {
	case 0:
		subtitle = ""
	case 1:
		subtitle = m.repoNames[0]
	default:
		subtitle = fmt.Sprintf("%d projects", len(m.repoPaths))
	}
	subtitleStr := styleDim.Render(subtitle)
	spacer := strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(subtitleStr)))
	header := title + spacer + subtitleStr

	filterRow := styleDim.Render("  Filter: ") + m.filter.View()

	listH := m.listHeight()
	end := min(m.scrollOffset+listH, len(m.rows))
	rendered := make([]string, 0, listH)
	for i := m.scrollOffset; i < end; i++ {
		rendered = append(rendered, m.renderRow(m.rows[i], i == m.cursor, w))
	}
	for len(rendered) < listH {
		rendered = append(rendered, "")
	}

	footer := m.renderFooter(w)

	// Show last few status messages above the divider.
	statusCount := m.statusLineCount()
	var statusLines []string
	for i := len(m.outputLines) - statusCount; i < len(m.outputLines); i++ {
		statusLines = append(statusLines, "  "+m.outputLines[i])
	}

	parts := []string{
		"",
		header,
		"",
		filterRow,
		"",
		strings.Join(rendered, "\n"),
	}
	if len(statusLines) > 0 {
		parts = append(parts, strings.Join(statusLines, "\n"))
	}
	parts = append(parts, divider, footer)
	return strings.Join(parts, "\n")
}

func (m Model) renderFooter(w int) string {
	switch m.mode {
	case modeCreating:
		inputLine := "  " + styleDim.Render("New Context: ") + m.createInput.View() +
			styleDim.Render("  enter: Create  ↑↓: Pick  esc: Cancel")

		if m.pickerLoading {
			return inputLine + "\n" + styleDim.Render("  Loading issues...")
		}

		filtered := m.filteredPickerIssues()
		total := len(filtered)
		if total == 0 {
			return inputLine
		}

		shown := min(10, total-m.pickerScroll)
		lines := []string{inputLine}
		for i := 0; i < shown; i++ {
			idx := m.pickerScroll + i
			issue := filtered[idx]
			if idx == m.pickerCursor {
				idStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorBlue).Bold(true).Render("  " + issue.Identifier)
				titleStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorWhite).Width(w - 2 - lipgloss.Width("  "+issue.Identifier) - 2).Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			} else {
				idStr := styleLinearID.Render("  " + issue.Identifier)
				titleStr := styleDim.Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			}
		}
		return strings.Join(lines, "\n")

	case modeDeleteConfirm:
		return "  " + styleDanger.Render(fmt.Sprintf(
			"Delete '%s'?  d: Confirm  esc: Cancel", m.deleteItem.context.Name))

	default:
		launchHint := ""
		if m.launchPanel != nil {
			if m.launchFocused {
				launchHint = "  " + styleStatus.Render("● Launch") + styleDim.Render("  tab: Context  q: Detach  ctrl+c: Kill")
				return launchHint
			}
			launchHint = "  l: Close Launch  "
		}
		return styleDim.Render("  ↑↓/jk: Nav  enter: Select  n: New  d: Del  c: Copy  a: Agent  l: Launch  s: Sort("+m.sortModeLabel()+")  ctrl+w: Terminal  q: Quit") + launchHint
	}
}


func (m Model) renderRow(r row, selected bool, w int) string {
	switch r.kind {
	case rowTypeHeader:
		name := m.repoNames[r.repoIdx]
		prefix := " ── " + name + " "
		line := prefix + strings.Repeat("─", max(0, w-len(prefix)))
		return styleHeader.Render(line)
	case rowTypeContext:
		return m.renderContext(r, selected, w)
	case rowTypeSpacer:
		return ""
	}
	return ""
}

func (m Model) renderContext(r row, selected bool, w int) string {
	item := m.filteredItems[r.repoIdx][r.itemIdx]
	ctx := item.context

	const prefixWidth = 4
	const dirtyWidth = 2
	const timeWidth = 8
	rightWidth := dirtyWidth + timeWidth

	middleWidth := w - prefixWidth - rightWidth
	if middleWidth < 10 {
		middleWidth = 10
	}

	hasLinear := ctx.LinearIssue != nil
	hasBranch := ctx.Branch != "" && ctx.Branch != ctx.Name

	var nameColW, branchColW, linearColW int
	if !hasBranch && !hasLinear {
		nameColW = middleWidth
	} else {
		nameColW = min(20, middleWidth*2/5)
		if nameColW < 8 {
			nameColW = 8
		}
		remaining := middleWidth - nameColW - 2
		if remaining < 0 {
			remaining = 0
		}
		if hasBranch && hasLinear {
			branchColW = remaining / 2
			linearColW = remaining - branchColW - 2
			if linearColW < 0 {
				linearColW = 0
			}
		} else if hasBranch {
			branchColW = remaining
		} else {
			linearColW = remaining
		}
	}

	nameRunes := []rune(ctx.Name)
	if len(nameRunes) > nameColW {
		nameRunes = append(nameRunes[:nameColW-1], '…')
	}
	namePadded := string(nameRunes) + strings.Repeat(" ", max(0, nameColW-len(nameRunes)))

	branchPadded := ""
	if hasBranch && branchColW > 0 {
		branchRunes := []rune(ctx.Branch)
		if len(branchRunes) > branchColW {
			branchRunes = append(branchRunes[:branchColW-1], '…')
		}
		branchPadded = string(branchRunes) + strings.Repeat(" ", max(0, branchColW-len(branchRunes)))
	}

	timeStr := fmt.Sprintf("%-*s", timeWidth, relativeTime(ctx.HeadTime))
	dirtyStr := "  "
	if ctx.IsDirty {
		dirtyStr = "● "
	}

	isActive := m.selectedContextPath != "" && ctx.Path == m.selectedContextPath
	return m.renderContextRow(ctx, namePadded, branchPadded, dirtyStr, timeStr, hasLinear, linearColW, w, selected, isActive)
}

// renderContextRow renders a context row with independent cursor and active-selection layers.
// isCursor = dark background highlight (navigation position).
// isActive = amber bold * (the context selected with Enter for the terminal).
func (m Model) renderContextRow(ctx workspace.Context, namePadded, branchPadded, dirtyStr, timeStr string, hasLinear bool, linearColW, w int, isCursor, isActive bool) string {
	base := lipgloss.NewStyle()
	if isCursor {
		base = base.Background(colorSelection)
	}

	activePrefix := "  "
	if isActive {
		activePrefix = "* "
	}

	statusPrefix := "  "
	statusStyle := base.Foreground(colorDim)
	switch ctx.AgentStatus {
	case agent.StatusThinking:
		spinnerFrames := []string{"◐ ", "◓ ", "◑ ", "◒ "}
		statusPrefix = spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		statusStyle = base.Foreground(colorAmber)
	case agent.StatusDone:
		statusPrefix = "● "
		statusStyle = base.Foreground(colorBlue)
	case agent.StatusIdle:
		statusPrefix = "○ "
		statusStyle = base.Foreground(colorDim)
	}

	prefixStyled := base.Render(activePrefix) + statusStyle.Render(statusPrefix)

	var nameStyled string
	if isActive {
		nameStyled = base.Foreground(colorAmber).Bold(true).Render(namePadded)
	} else if ctx.IsMain {
		nameStyled = base.Bold(true).Render(namePadded)
	} else {
		if isCursor {
			nameStyled = base.Foreground(colorWhite).Render(namePadded)
		} else {
			nameStyled = namePadded
		}
	}

	var dirtyStyled string
	if ctx.IsDirty {
		dirtyStyled = base.Foreground(colorAmber).Render(dirtyStr)
	} else {
		dirtyStyled = base.Foreground(colorDim).Render(dirtyStr)
	}
	timeStyled := base.Foreground(colorDim).Render(timeStr)

	var linearStyled string
	if hasLinear {
		linearRaw := ctx.LinearIssue.Identifier + " " + ctx.LinearIssue.Title
		linearRunes := []rune(linearRaw)
		if len(linearRunes) > linearColW {
			linearRunes = append(linearRunes[:linearColW-1], '…')
		}
		linearPadded := string(linearRunes) + strings.Repeat(" ", max(0, linearColW-len(linearRunes)))
		if spaceIdx := strings.Index(linearPadded, " "); spaceIdx > 0 {
			linearStyled = base.Foreground(colorBlue).Bold(true).Render(linearPadded[:spaceIdx]) +
				base.Foreground(colorWhite).Render(linearPadded[spaceIdx:])
		} else {
			linearStyled = base.Foreground(colorBlue).Bold(true).Render(linearPadded)
		}
	}

	var branchStyled string
	if branchPadded != "" {
		branchStyled = base.Foreground(colorDim).Render("  " + branchPadded)
	}

	var line string
	switch {
	case branchPadded != "" && hasLinear:
		line = prefixStyled + nameStyled + branchStyled + base.Render("  ") + linearStyled + dirtyStyled + timeStyled
	case branchPadded != "":
		line = prefixStyled + nameStyled + branchStyled + dirtyStyled + timeStyled
	case hasLinear:
		line = prefixStyled + nameStyled + base.Render("  ") + linearStyled + dirtyStyled + timeStyled
	default:
		line = prefixStyled + nameStyled + dirtyStyled + timeStyled
	}

	if isCursor {
		visibleWidth := lipgloss.Width(line)
		if visibleWidth < w {
			line += base.Render(strings.Repeat(" ", w-visibleWidth))
		}
	}
	return line
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
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
