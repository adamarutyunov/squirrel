package ui

import (
	"fmt"
	"strings"

	"github.com/adamarutyunov/launch/embed"
	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
	"squirrel/internal/workspace"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.hasActiveLaunch() {
			panelHeight := m.launchPanelHeight()
			var cmds []tea.Cmd
			for _, panel := range m.launchPanels {
				newPanel, cmd := panel.Update(tea.WindowSizeMsg{
					Width:  m.launchPanelWidth(),
					Height: panelHeight,
				})
				*panel = newPanel
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case tickMsg:
		m.spinnerFrame++
		cmds := []tea.Cmd{tickCmd()}
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
		if panel, ok := m.launchPanels[msg.Tag]; ok {
			newPanel, cmd := panel.Update(msg)
			*panel = newPanel
			return m, cmd
		}
		return m, nil

	case createContextResultMsg:
		return m.handleCreateContextResult(msg)

	case setupCommandResultMsg:
		return m.handleSetupCommandResult(msg)

	case deleteContextResultMsg:
		return m.handleDeleteContextResult(msg)

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

func (m Model) handleCreateContextResult(msg createContextResultMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) handleSetupCommandResult(msg setupCommandResultMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) handleDeleteContextResult(msg deleteContextResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.appendOutput(styleDanger.Render("✗ Delete failed: " + msg.err.Error()))
	} else {
		m.appendOutput(styleStatus.Render("✓ Deleted"))
	}
	m.applyRefresh(refreshMsg{repoIdx: msg.repoIdx, contexts: msg.newContexts})
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	if msg.Type == tea.KeyTab {
		if m.hasActiveLaunch() {
			sorted := m.sortedLaunchIndices()
			m.launchFocusIndex++
			if m.launchFocusIndex >= len(sorted) {
				m.launchFocusIndex = -1
			}
		}
		return m, nil
	}

	if m.isLaunchFocused() {
		sorted := m.sortedLaunchIndices()
		if m.launchFocusIndex < len(sorted) {
			repoIdx := sorted[m.launchFocusIndex]
			if panel, ok := m.launchPanels[repoIdx]; ok {
				newPanel, cmd := panel.Update(msg)
				*panel = newPanel
				return m, cmd
			}
		}
	}

	return m.handleListKey(msg)
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeCreating {
		return m.handleCreateKey(msg)
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.filterActive {
			m.filterActive = false
			m.filter.Blur()
			if m.filterValue != "" {
				m.filter.SetValue("")
				m.filterValue = ""
				m.rebuild()
			}
			return m, nil
		}
		return m, nil

	case tea.KeyUp:
		m.filterActive = false
		m.filter.Blur()
		m.moveCursor(-1)
		return m, nil

	case tea.KeyDown:
		m.filterActive = false
		m.filter.Blur()
		m.moveCursor(1)
		return m, nil

	case tea.KeyEnter:
		if m.filterActive {
			m.filterActive = false
			m.filter.Blur()
			return m, nil
		}
		if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowTypeContext {
			return m.selectContext(), nil
		}
		return m, nil
	}

	if msg.String() == "ctrl+f" {
		m.filterActive = !m.filterActive
		if m.filterActive {
			m.filter.Focus()
		} else {
			m.filter.Blur()
		}
		return m, nil
	}

	if m.filterActive {
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

	switch msg.String() {
	case "d":
		return m.handleDeleteKey()
	case "n":
		return m.startCreateContext()
	case "c":
		return m.copyContextPath()
	case "l":
		return m.openLaunch()
	case "L":
		return m.closeLaunch()
	case "a":
		return m.toggleAgent()
	case "A":
		return m.attachAgentFullscreen()
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
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.resetCreateState()
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
		return m.submitCreateContext()
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

func (m Model) submitCreateContext() (tea.Model, tea.Cmd) {
	var name string
	filtered := m.filteredPickerIssues()
	if m.pickerCursor >= 0 && m.pickerCursor < len(filtered) {
		selectedIssue := filtered[m.pickerCursor]
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

	repoPath := m.repoPaths[m.createRepoIdx]
	cfg := m.repoConfigs[m.createRepoIdx]
	m.resetCreateState()
	m.appendOutput(styleDim.Render("Creating context '" + name + "'..."))
	return m, createContextCmd(m.createRepoIdx, repoPath, name, cfg)
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

	if item.context.IsDirty {
		m.appendOutput(styleDanger.Render(fmt.Sprintf(
			"✗ Cannot delete '%s': context has uncommitted changes", item.context.Name)))
		m.appendOutput(styleWarning.Render("  Commit, stash, or discard changes first"))
		return m, nil
	}

	m.cleanupContext(r.repoIdx, item.context.Path)
	m.appendOutput(styleDim.Render("Deleting '" + item.context.Name + "'..."))
	return m, deleteContextCmd(r.repoIdx, m.repoPaths[r.repoIdx], item.context, false, m.linearIssues)
}
