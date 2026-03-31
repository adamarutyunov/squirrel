package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
	stmux "squirrel/internal/tmux"
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
		if paneID, ok := m.launchPaneIDs[repoIdx]; ok {
			_ = stmux.KillPane(paneID)
			delete(m.launchPaneIDs, repoIdx)
		}
		delete(m.launchContextPath, repoIdx)
		if m.mainPaneID != "" {
			_ = stmux.ResizePaneWidth(m.mainPaneID, 65, 40)
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
		if paneID, ok := m.launchPaneIDs[repoIdx]; ok {
			_ = stmux.SelectPane(paneID)
		}
		return m, nil
	}
	if _, err := exec.LookPath("launch"); err != nil {
		m.appendOutput(styleDanger.Render("✗ Launch: executable not found in PATH"))
		return m, nil
	}

	title := "Launch " + filepath.Base(m.repoPaths[repoIdx])
	command := shellCommand("launch --force-autostart " + stmux.ShellQuote(contextPath))
	if paneID, ok := m.launchPaneIDs[repoIdx]; ok {
		if err := stmux.RespawnPane(paneID, contextPath, title, command); err != nil {
			m.appendOutput(styleDanger.Render("✗ Launch: " + err.Error()))
			return m, nil
		}
		m.launchContextPath[repoIdx] = contextPath
		_ = stmux.SelectPane(paneID)
		return m, nil
	}

	var paneID string
	var err error
	if len(m.launchPaneIDs) == 0 {
		paneID, err = stmux.SplitPaneHorizontal(m.mainPaneID, contextPath, title, command)
		if err == nil {
			_ = stmux.ResizePaneWidth(m.mainPaneID, 50, 40)
			_ = stmux.ResizePaneWidth(paneID, 25, 25)
		}
	} else {
		paneID, err = stmux.SplitPaneVertical(m.firstLaunchPaneID(), contextPath, title, command)
		if err == nil {
			_ = stmux.ResizePaneWidth(m.mainPaneID, 50, 40)
			_ = stmux.ResizePaneWidth(m.firstLaunchPaneID(), 25, 25)
		}
	}
	if err != nil {
		m.appendOutput(styleDanger.Render("✗ Launch: " + err.Error()))
		return m, nil
	}
	m.launchPaneIDs[repoIdx] = paneID
	m.launchContextPath[repoIdx] = contextPath
	_ = stmux.SelectPane(paneID)
	return m, nil
}

func (m Model) closeLaunch() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].kind != rowTypeContext {
		return m, nil
	}
	r := m.rows[m.cursor]
	repoIdx := r.repoIdx

	paneID, ok := m.launchPaneIDs[repoIdx]
	if m.launchContextPath[repoIdx] == "" || !ok {
		return m, nil
	}
	_ = stmux.KillPane(paneID)
	delete(m.launchPaneIDs, repoIdx)
	delete(m.launchContextPath, repoIdx)
	if len(m.launchPaneIDs) == 0 {
		_ = stmux.ResizePaneWidth(m.mainPaneID, 65, 40)
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
	if err := agent.LaunchBackground(contextPath, command, launchCommand); err != nil {
		m.appendOutput(styleDanger.Render("✗ Agent: " + err.Error()))
		return m, nil
	}
	respawnCommand := shellCommand(agent.AttachShellCommand(contextPath, command, launchCommand, true))
	if err := stmux.RespawnPane(m.companionPaneID, contextPath, "Agent", respawnCommand); err != nil {
		m.appendOutput(styleDanger.Render("✗ Agent: " + err.Error()))
		return m, nil
	}

	m.appendOutput(styleDim.Render("Agent: " + filepath.Base(contextPath) + " (" + command + ")"))
	_ = stmux.SelectPane(m.companionPaneID)
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
	if err := agent.LaunchBackground(contextPath, command, launchCommand); err != nil {
		m.appendOutput(styleDanger.Render("✗ Agent: " + err.Error()))
		return m, nil
	}

	m.appendOutput(styleDim.Render("Attaching agent (fullscreen): " + filepath.Base(contextPath) + "  (ctrl+q to detach)"))
	return m, tea.ExecProcess(agent.AttachCommand(contextPath, command, launchCommand), func(err error) tea.Msg {
		return agentAttachFinishedMsg{err: err}
	})
}

func shellCommand(command string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if strings.TrimSpace(command) == "" {
		return "exec " + shell
	}
	return command + "; exec " + shell
}

func (m Model) firstLaunchPaneID() string {
	firstRepoIdx := -1
	for repoIdx := range m.launchPaneIDs {
		if firstRepoIdx == -1 || repoIdx < firstRepoIdx {
			firstRepoIdx = repoIdx
		}
	}
	if firstRepoIdx == -1 {
		return ""
	}
	return m.launchPaneIDs[firstRepoIdx]
}
