package main

import (
	"fmt"
	"os"
	"path/filepath"

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

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
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

	model := ui.NewModel(repoPaths, repoContexts, repoConfigs, linearIssues, os.Getenv("LINEAR_API_KEY"), Version)
	program := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
