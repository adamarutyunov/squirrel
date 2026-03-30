package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adamarutyunov/launch/embed"
	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
)

func (m Model) copyContextPath() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	item := m.filteredItems[r.repoIdx][r.itemIdx]
	return m, copyToClipboardCmd(item.context.Path)
}

func (m Model) selectContext() Model {
	r := m.rows[m.cursor]
	ctx := m.filteredItems[r.repoIdx][r.itemIdx].context
	m.selectedContextPath = ctx.Path
	m.appendOutput(m.renderPrompt(ctx))
	if m.companionPaneID != "" {
		escapedPath := strings.ReplaceAll(ctx.Path, "'", "'\\''")
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-c", "").Run()
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, fmt.Sprintf("cd '%s'", escapedPath), "Enter").Run()
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-l", "").Run()
	}
	return m
}

func (m *Model) cleanupContext(repoIdx int, contextPath string) {
	if m.launchContextPath[repoIdx] == contextPath {
		if panel, ok := m.launchPanels[repoIdx]; ok {
			panel.StopAll()
			delete(m.launchPanels, repoIdx)
			delete(m.launchContextPath, repoIdx)
			if !m.hasActiveLaunch() {
				m.launchFocusIndex = -1
			}
		}
	}
}

func (m Model) openLaunch() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	repoIdx := r.repoIdx
	contextPath := m.filteredItems[repoIdx][r.itemIdx].context.Path

	if m.launchContextPath[repoIdx] == contextPath {
		return m, nil
	}

	if oldPanel, ok := m.launchPanels[repoIdx]; ok {
		oldPanel.StopAll()
		delete(m.launchPanels, repoIdx)
		delete(m.launchContextPath, repoIdx)
	}

	panel, err := embed.New(contextPath)
	if err != nil {
		m.appendOutput(styleDanger.Render("✗ Launch: " + err.Error()))
		return m, nil
	}
	if !panel.HasProcesses() {
		m.appendOutput(styleWarning.Render("⚠ No launch.yml found in " + filepath.Base(contextPath)))
		return m, nil
	}

	panel.Tag = repoIdx
	m.launchPanels[repoIdx] = &panel
	m.launchContextPath[repoIdx] = contextPath

	panelHeight := m.launchPanelHeight()
	sizedPanel, sizeCmd := m.launchPanels[repoIdx].Update(tea.WindowSizeMsg{
		Width:  m.launchPanelWidth(),
		Height: panelHeight,
	})
	*m.launchPanels[repoIdx] = sizedPanel

	initCmd := m.launchPanels[repoIdx].Init()
	m.launchPanels[repoIdx].ForceStartAutoStart()

	return m, tea.Batch(initCmd, sizeCmd)
}

func (m Model) closeLaunch() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	repoIdx := r.repoIdx

	panel, ok := m.launchPanels[repoIdx]
	if !ok {
		return m, nil
	}
	panel.StopAll()
	delete(m.launchPanels, repoIdx)
	delete(m.launchContextPath, repoIdx)
	if !m.hasActiveLaunch() || m.launchFocusIndex >= len(m.launchPanels) {
		m.launchFocusIndex = -1
	}
	m.appendOutput(styleStatus.Render("✓ Processes stopped"))
	return m, nil
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
	agent.MarkAttached(contextPath)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	respawnCommand := fmt.Sprintf("%s; exec %s", agent.SessionCommand(contextPath, command), shell)
	exec.Command("tmux", "respawn-pane", "-k", "-t", m.companionPaneID, "-c", contextPath, respawnCommand).Run()

	m.appendOutput(styleDim.Render("Agent: " + filepath.Base(contextPath) + " (" + command + ")"))
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
	agent.MarkAttached(contextPath)

	m.appendOutput(styleDim.Render("Attaching agent (fullscreen): " + filepath.Base(contextPath) + "  (ctrl+q to detach)"))
	return m, tea.ExecProcess(agent.AttachCommand(contextPath, command), func(err error) tea.Msg {
		return agentAttachFinishedMsg{err: err}
	})
}
