package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"squirrel/internal/git"
	"squirrel/internal/linear"
	"squirrel/internal/ui"
)

func main() {
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

	// Collect branches from all repos.
	repoBranches := make([][]git.Branch, len(repoPaths))
	var allBranches []git.Branch
	for i, path := range repoPaths {
		branches, err := git.GetBranches(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", filepath.Base(path), err)
			continue
		}
		repoBranches[i] = branches
		allBranches = append(allBranches, branches...)
	}

	// Batch-fetch Linear issues for all branch identifiers.
	linearIssues := map[string]linear.Issue{}
	if apiKey := os.Getenv("LINEAR_API_KEY"); apiKey != "" {
		identifiers := git.ExtractLinearIdentifiers(allBranches)
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

	model := ui.NewModel(repoPaths, repoBranches, linearIssues)
	program := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
