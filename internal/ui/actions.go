package ui

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
	"squirrel/internal/layout"
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

func (m Model) openUserConfig() (tea.Model, tea.Cmd) {
	configPath, err := workspace.EnsureUserConfigFile()
	if err != nil {
		m.appendOutput(styleDanger.Render("✗ User config: " + err.Error()))
		return m, nil
	}
	return m.openConfigInCompanion(configPath)
}

func (m Model) openProjectConfig() (tea.Model, tea.Cmd) {
	repoIdx := 0
	if m.cursor < len(m.rows) && m.rows[m.cursor].repoIdx < len(m.repoPaths) {
		repoIdx = m.rows[m.cursor].repoIdx
	}
	configPath, err := workspace.EnsureProjectConfigFile(m.repoPaths[repoIdx])
	if err != nil {
		m.appendOutput(styleDanger.Render("✗ Project config: " + err.Error()))
		return m, nil
	}
	return m.openConfigInCompanion(configPath)
}

func (m Model) openConfigInCompanion(configPath string) (tea.Model, tea.Cmd) {
	if m.companionPaneID == "" {
		m.appendOutput(styleDanger.Render("✗ Config editor: companion pane unavailable"))
		return m, nil
	}
	command := editorShellCommand(configPath)
	exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-c", "").Run()
	exec.Command("tmux", "send-keys", "-t", m.companionPaneID, command, "Enter").Run()
	m.companionAgentContextPath = ""
	_ = stmux.SelectPane(m.companionPaneID)
	m.appendOutput(styleStatus.Render("✓ Opening config"))
	m.appendOutput(styleDim.Render("  " + configPath))
	return m, nil
}

func (m Model) selectContext() Model {
	r := m.rows[m.cursor]
	ctx := m.filteredItems[r.repoIdx][r.itemIdx].context
	m.selectedContextPath = ctx.Path
	m.appendOutput(m.renderPrompt(ctx))
	if m.companionPaneID != "" {
		_ = setCompanionPendingCwd(m.companionPaneID, ctx.Path)
		if companionAgentRunning(m.companionPaneID) {
			return m
		}
		escapedPath := strings.ReplaceAll(ctx.Path, "'", "'\\''")
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-c", "").Run()
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, fmt.Sprintf("cd '%s'", escapedPath), "Enter").Run()
		exec.Command("tmux", "send-keys", "-t", m.companionPaneID, "C-l", "").Run()
		m.companionAgentContextPath = ""
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
			_ = layout.MainPaneSoloWidth.Resize(m.mainPaneID)
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

	title := formatPaneTitle("Launch "+filepath.Base(m.repoPaths[repoIdx]), ctx.Name)
	command := shellCommand("launch --force-autostart --embed " + stmux.ShellQuote(contextPath))
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
			_ = layout.ApplyLaunchLayout(m.mainPaneID, paneID)
		}
	} else {
		paneID, err = stmux.SplitPaneVertical(m.firstLaunchPaneID(), contextPath, title, command)
		if err == nil {
			_ = layout.ApplyLaunchLayout(m.mainPaneID, m.firstLaunchPaneID())
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
		_ = layout.MainPaneSoloWidth.Resize(m.mainPaneID)
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
	_ = setCompanionPendingCwd(m.companionPaneID, m.activeContextPath())
	respawnCommand := companionShellCommand(agent.AttachShellCommand(contextPath, command, launchCommand, true), contextPath, m.companionPaneID)
	if err := stmux.RespawnPane(m.companionPaneID, contextPath, formatPaneTitle("Agent", ctx.Name), respawnCommand); err != nil {
		m.appendOutput(styleDanger.Render("✗ Agent: " + err.Error()))
		return m, nil
	}

	m.companionAgentContextPath = contextPath
	m.appendOutput(styleDim.Render("Agent: " + filepath.Base(contextPath) + " (" + command + ")"))
	_ = stmux.SelectPane(m.companionPaneID)
	return m, nil
}

func formatPaneTitle(base, contextName string) string {
	contextName = strings.TrimSpace(contextName)
	if contextName == "" {
		return base
	}
	return base + " (" + truncateRunes(contextName, 18) + ")"
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

func companionShellCommand(command, fallbackPath, paneID string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cwdPath, err := companionPendingCwdPath(paneID)
	if err != nil {
		return shellCommand(command)
	}
	agentFlagPath, err := companionAgentFlagPath(paneID)
	if err != nil {
		return shellCommand(command)
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Sprintf("cd %s 2>/dev/null || true; exec %s", stmux.ShellQuote(fallbackPath), shell)
	}
	return fmt.Sprintf(
		"mkdir -p %s; : > %s; %s; rm -f %s; cd \"$(cat %s 2>/dev/null)\" 2>/dev/null || cd %s; exec %s",
		stmux.ShellQuote(filepath.Dir(agentFlagPath)),
		stmux.ShellQuote(agentFlagPath),
		command,
		stmux.ShellQuote(agentFlagPath),
		stmux.ShellQuote(cwdPath),
		stmux.ShellQuote(fallbackPath),
		shell,
	)
}

func setCompanionPendingCwd(paneID, cwd string) error {
	path, err := companionPendingCwdPath(paneID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(cwd), 0o644)
}

func companionPendingCwdPath(paneID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	hash := sha1.Sum([]byte(paneID))
	fileName := hex.EncodeToString(hash[:8]) + ".cwd"
	return filepath.Join(home, ".config", "squirrel", "companion", fileName), nil
}

func companionAgentFlagPath(paneID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	hash := sha1.Sum([]byte("agent:" + paneID))
	fileName := hex.EncodeToString(hash[:8]) + ".active"
	return filepath.Join(home, ".config", "squirrel", "companion", fileName), nil
}

func companionAgentRunning(paneID string) bool {
	path, err := companionAgentFlagPath(paneID)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
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
