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
	"squirrel/internal/workspace"
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
	return m.openLaunchWithForce(false)
}

func (m Model) openLaunchWithForce(force bool) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	repoIdx := r.repoIdx
	ctx := m.filteredItems[repoIdx][r.itemIdx].context
	contextPath := ctx.Path

	if !force && ctx.SetupStatus == workspace.SetupStatusRunning {
		m.prompt = &promptState{
			title:       "Setup Still Running",
			message:     "Project setup is still running for " + ctx.Name + ". Force start launch before install completes?",
			confirmText: "enter/y: Force run",
			cancelText:  "esc/n: Abort",
			action:      promptActionOpenLaunch,
		}
		return m, nil
	}

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
	return m.toggleAgentWithForce(false)
}

func (m Model) toggleAgentWithForce(force bool) (tea.Model, tea.Cmd) {
	if m.companionPaneID == "" {
		return m, nil
	}
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}

	r := m.rows[m.cursor]
	ctx := m.filteredItems[r.repoIdx][r.itemIdx].context
	if !force && ctx.SetupStatus == workspace.SetupStatusRunning {
		m.prompt = &promptState{
			title:       "Setup Still Running",
			message:     "Project setup is still running for " + ctx.Name + ". Force start the agent before install completes?",
			confirmText: "enter/y: Force run",
			cancelText:  "esc/n: Abort",
			action:      promptActionToggleAgent,
		}
		return m, nil
	}

	contextPath := ctx.Path
	command := agent.PreferredCommand(m.agentCommand)
	launchCommand := agent.CommandForIssue(command, ctx.LinearIssue)
	agent.MarkAttached(contextPath)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	respawnCommand := fmt.Sprintf("%s; exec %s", agent.AttachShellCommand(contextPath, command, launchCommand, true), shell)
	exec.Command("tmux", "respawn-pane", "-k", "-t", m.companionPaneID, "-c", contextPath, respawnCommand).Run()

	m.appendOutput(styleDim.Render("Agent: " + filepath.Base(contextPath) + " (" + command + ")"))
	exec.Command("tmux", "select-pane", "-t", m.companionPaneID).Run()
	return m, nil
}

func (m Model) attachAgentFullscreen() (tea.Model, tea.Cmd) {
	return m.attachAgentFullscreenWithForce(false)
}

func (m Model) attachAgentFullscreenWithForce(force bool) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}

	r := m.rows[m.cursor]
	ctx := m.filteredItems[r.repoIdx][r.itemIdx].context
	if !force && ctx.SetupStatus == workspace.SetupStatusRunning {
		m.prompt = &promptState{
			title:       "Setup Still Running",
			message:     "Project setup is still running for " + ctx.Name + ". Force start the fullscreen agent before install completes?",
			confirmText: "enter/y: Force run",
			cancelText:  "esc/n: Abort",
			action:      promptActionAttachAgentFullscreen,
		}
		return m, nil
	}

	contextPath := ctx.Path
	command := agent.PreferredCommand(m.agentCommand)
	launchCommand := agent.CommandForIssue(command, ctx.LinearIssue)
	agent.MarkAttached(contextPath)

	m.appendOutput(styleDim.Render("Attaching agent (fullscreen): " + filepath.Base(contextPath) + "  (ctrl+q to detach)"))
	return m, tea.ExecProcess(agent.AttachCommand(contextPath, command, launchCommand), func(err error) tea.Msg {
		return agentAttachFinishedMsg{err: err}
	})
}
