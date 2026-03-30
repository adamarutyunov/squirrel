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

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Create companion shell pane on the right.
	companionPaneID := createCompanionPane(dir)
	if companionPaneID != "" {
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

	// Collect branch names from all worktrees for Linear ID extraction.
	var allBranchNames []string
	for _, repoPath := range repoPaths {
		worktrees, err := git.ListWorktrees(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", filepath.Base(repoPath), err)
			continue
		}
		for _, wt := range worktrees {
			if wt.Branch != "" {
				allBranchNames = append(allBranchNames, wt.Branch)
			}
		}
	}

	// Batch-fetch Linear issues for all branch identifiers.
	linearIssues := map[string]linear.Issue{}
	if apiKey := os.Getenv("LINEAR_API_KEY"); apiKey != "" {
		identifiers := git.ExtractLinearIdentifiersFromStrings(allBranchNames)
		if len(identifiers) > 0 {
			client := linear.NewClient(apiKey)
			fetched, err := client.FetchIssues(identifiers)
			if err != nil {
				fmt.Fprintln(os.Stderr, "warning: linear:", err)
			} else {
				linearIssues = fetched
			}
		}
	}

	// Load configs and list contexts for each repo.
	repoContexts := make([][]workspace.Context, len(repoPaths))
	repoConfigs := make([]workspace.Config, len(repoPaths))

	for i, repoPath := range repoPaths {
		cfg, err := workspace.LoadConfig(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: config: %v\n", filepath.Base(repoPath), err)
		}
		repoConfigs[i] = cfg

		contexts, err := workspace.ListContexts(repoPath, linearIssues)
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

	model := ui.NewModel(repoPaths, repoContexts, repoConfigs, linearIssues, os.Getenv("LINEAR_API_KEY"), userConfig.AgentCommand, companionPaneID, Version)
	program := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func launchInTmux() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		`tmux new-session '%s' \; set mouse on \; set status off \; set pane-border-style 'fg=#71717a' \; set pane-active-border-style 'fg=#71717a'`,
		exePath,
	))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func createCompanionPane(dir string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	output, err := exec.Command("tmux", "split-window", "-h", "-d", "-l", "35%", "-c", dir, "-P", "-F", "#{pane_id}", shell).Output()
	if err != nil {
		return ""
	}
	paneID := strings.TrimSpace(string(output))

	// Bind Ctrl+W to toggle between panes (works from either pane).
	exec.Command("tmux", "bind-key", "-n", "C-w", "select-pane", "-t", ":.+").Run()

	return paneID
}
