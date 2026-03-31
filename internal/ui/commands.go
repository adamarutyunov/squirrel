package ui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"squirrel/internal/agent"
	"squirrel/internal/git"
	"squirrel/internal/linear"
	"squirrel/internal/tmux"
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

func setupCommandCmd(repoIdx int, worktreePath, command string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(command) == "" {
			return setupCommandResultMsg{}
		}

		cmd := exec.Command("sh", "-lc", command)
		cmd.Dir = worktreePath
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		if err := cmd.Start(); err != nil {
			_ = workspace.ClearSetupStatus(worktreePath)
			return setupCommandResultMsg{
				repoIdx:      repoIdx,
				worktreePath: worktreePath,
				err:          err,
			}
		}
		if err := workspace.WriteSetupStatus(worktreePath, workspace.SetupStatusRunning, cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = workspace.ClearSetupStatus(worktreePath)
			return setupCommandResultMsg{
				repoIdx:      repoIdx,
				worktreePath: worktreePath,
				err:          err,
			}
		}

		err := cmd.Wait()
		clearErr := workspace.ClearSetupStatus(worktreePath)
		if err == nil && clearErr != nil {
			err = clearErr
		}

		return setupCommandResultMsg{
			repoIdx:      repoIdx,
			worktreePath: worktreePath,
			output:       strings.TrimSpace(output.String()),
			err:          err,
		}
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

func fetchLinearIssuesCmd(repoIdx int, apiKey, query string) tea.Cmd {
	return func() tea.Msg {
		client := linear.NewClient(apiKey)
		issues, err := client.FetchPickerIssues(query)
		return linearIssuesLoadedMsg{repoIdx: repoIdx, query: query, issues: issues, err: err}
	}
}

func fetchRepoLinearIssuesCmd(repoIdx int, repoPath, apiKey string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(apiKey) == "" {
			return repoLinearIssuesLoadedMsg{repoIdx: repoIdx, issues: map[string]linear.Issue{}}
		}

		worktrees, err := git.ListWorktrees(repoPath)
		if err != nil {
			return repoLinearIssuesLoadedMsg{repoIdx: repoIdx, err: err}
		}

		var branchNames []string
		for _, wt := range worktrees {
			if wt.Branch != "" {
				branchNames = append(branchNames, wt.Branch)
			}
		}

		identifiers := git.ExtractLinearIdentifiersFromStrings(branchNames)
		if len(identifiers) == 0 {
			return repoLinearIssuesLoadedMsg{repoIdx: repoIdx, issues: map[string]linear.Issue{}}
		}

		client := linear.NewClient(apiKey)
		issues, err := client.FetchIssues(identifiers)
		return repoLinearIssuesLoadedMsg{repoIdx: repoIdx, issues: issues, err: err}
	}
}

func launchAgentBackgroundCmd(contextPath, sessionCommand, launchCommand string) tea.Cmd {
	return func() tea.Msg {
		err := agent.LaunchBackground(contextPath, sessionCommand, launchCommand)
		return agentLaunchBackgroundMsg{err: err}
	}
}

func editorShellCommand(path string) string {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	return fmt.Sprintf("%s %s", editor, tmux.ShellQuote(path))
}

func applyManagedLayoutCmd(mainPaneID, launchPaneID string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(mainPaneID) == "" {
			return nil
		}
		if strings.TrimSpace(launchPaneID) == "" {
			if err := tmux.ResizePaneWidth(mainPaneID, 65, 40); err != nil {
				return nil
			}
			return nil
		}
		if err := tmux.ResizePaneWidth(mainPaneID, 50, 40); err != nil {
			return nil
		}
		if err := tmux.ResizePaneWidth(launchPaneID, 15, 25); err != nil {
			return nil
		}
		return nil
	}
}
