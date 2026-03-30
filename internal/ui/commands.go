package ui

import (
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)

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
		if strings.TrimSpace(command) == "" {
			return setupCommandResultMsg{}
		}

		cmd := exec.Command("sh", "-lc", command)
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
