package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"squirrel/internal/agent"
	"squirrel/internal/git"
	"squirrel/internal/linear"
	"squirrel/internal/ui"
	"squirrel/internal/workspace"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "claude-hook" {
		if err := agent.HandleHookCommand(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--install-hooks" {
		installed, err := agent.InstallHooks()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		for _, name := range installed {
			fmt.Println("✓ " + name)
		}
		return
	}

	// Ensure we're running inside tmux for companion terminal pane.
	if os.Getenv("TMUX") == "" {
		launchInTmux()
		return
	}
	mainPaneID := os.Getenv("TMUX_PANE")

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Create companion shell pane on the right.
	companionPaneID := createCompanionPane(mainPaneID, dir)
	if companionPaneID != "" {
		exec.Command("tmux", "select-pane", "-t", mainPaneID, "-T", "Squirrel").Run()
		defer func() {
			exec.Command("tmux", "kill-pane", "-t", companionPaneID).Run()
			exec.Command("tmux", "unbind-key", "-n", "C-w").Run()
		}()
	}

	repoPaths, err := git.DiscoverRepos(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error discovering repos:", err)
		os.Exit(1)
	}
	if len(repoPaths) == 0 {
		fmt.Fprintln(os.Stderr, "no git repositories found in", dir)
		os.Exit(1)
	}

	// Load configs first so each repo can use its own Linear API key.
	repoContexts := make([][]workspace.Context, len(repoPaths))
	repoConfigs := make([]workspace.Config, len(repoPaths))
	repoLinearAPIKeys := make([]string, len(repoPaths))
	repoLinearIssues := make([]map[string]linear.Issue, len(repoPaths))

	envLinearAPIKey := os.Getenv("LINEAR_API_KEY")

	for i, repoPath := range repoPaths {
		cfg, err := workspace.LoadConfig(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: config: %v\n", filepath.Base(repoPath), err)
		}
		repoConfigs[i] = cfg
		repoLinearAPIKeys[i] = projectLinearAPIKey(cfg, envLinearAPIKey)
		repoLinearIssues[i] = map[string]linear.Issue{}
	}

	// Fetch Linear issues per repo, using that repo's configured API key.
	for i, repoPath := range repoPaths {
		worktrees, err := git.ListWorktrees(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", filepath.Base(repoPath), err)
			continue
		}

		var branchNames []string
		for _, wt := range worktrees {
			if wt.Branch != "" {
				branchNames = append(branchNames, wt.Branch)
			}
		}

		apiKey := repoLinearAPIKeys[i]
		if apiKey != "" {
			identifiers := git.ExtractLinearIdentifiersFromStrings(branchNames)
			if len(identifiers) > 0 {
				client := linear.NewClient(apiKey)
				fetched, err := client.FetchIssues(identifiers)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %s: linear: %v\n", filepath.Base(repoPath), err)
				} else {
					repoLinearIssues[i] = fetched
				}
			}
		}

		contexts, err := workspace.ListContexts(repoPath, repoLinearIssues[i], true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", filepath.Base(repoPath), err)
			continue
		}
		repoContexts[i] = contexts
	}

	userConfig, err := workspace.LoadUserConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: user config:", err)
	}

	model := ui.NewModel(repoPaths, repoContexts, repoConfigs, repoLinearIssues, repoLinearAPIKeys, userConfig.AgentCommand, userConfig.SortMode, mainPaneID, companionPaneID, Version)
	program := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := program.Run()
	if typed, ok := finalModel.(ui.Model); ok {
		typed.CleanupLaunchPanes()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func projectLinearAPIKey(cfg workspace.Config, fallback string) string {
	if strings.TrimSpace(cfg.LinearAPIKey) != "" {
		return strings.TrimSpace(cfg.LinearAPIKey)
	}
	return strings.TrimSpace(fallback)
}

func launchInTmux() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		`tmux new-session '%s' \; set mouse on \; set status off \; set pane-border-status top \; set pane-border-format '#{?pane_active,#[bold fg=#f59e0b],#[fg=#71717a]} #{pane_title} ' \; set pane-border-style 'fg=#71717a' \; set pane-active-border-style 'fg=#f59e0b'`,
		exePath,
	))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func createCompanionPane(mainPaneID, dir string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	output, err := exec.Command("tmux", "split-window", "-h", "-d", "-t", mainPaneID, "-l", "35%", "-c", dir, "-P", "-F", "#{pane_id}", shell).Output()
	if err != nil {
		return ""
	}
	paneID := strings.TrimSpace(string(output))
	_ = exec.Command("tmux", "select-pane", "-t", paneID, "-T", "Agent").Run()

	// Bind Ctrl+W to toggle between panes (works from either pane).
	exec.Command("tmux", "bind-key", "-n", "C-w", "select-pane", "-t", ":.+").Run()

	return paneID
}
