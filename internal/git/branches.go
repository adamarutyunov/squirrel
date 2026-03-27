package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Branch struct {
	Name           string
	LastCommitTime time.Time
	IsCurrent      bool
	RemoteName     string // upstream tracking branch e.g. "origin/main", empty if none
}

func (b Branch) HasRemote() bool {
	return b.RemoteName != ""
}

// GetBranches returns local branches for the given repo path, sorted by most recent commit.
func GetBranches(repoPath string) ([]Branch, error) {
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref",
		"--sort=-committerdate",
		"refs/heads/",
		"--format=%(refname:short)\t%(committerdate:unix)\t%(HEAD)\t%(upstream:short)",
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("not a git repository or git not found")
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	branches := make([]Branch, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		unixTimestamp, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			continue
		}
		isCurrent := strings.TrimSpace(parts[2]) == "*"
		remoteName := ""
		if len(parts) >= 4 {
			remoteName = strings.TrimSpace(parts[3])
		}

		branches = append(branches, Branch{
			Name:           name,
			LastCommitTime: time.Unix(unixTimestamp, 0),
			IsCurrent:      isCurrent,
			RemoteName:     remoteName,
		})
	}

	return branches, nil
}

var linearIDPattern = regexp.MustCompile(`(?i)[a-z][a-z0-9]+-\d+`)

// ExtractLinearIdentifiersFromStrings finds all Linear issue IDs from a slice of strings.
func ExtractLinearIdentifiersFromStrings(names []string) []string {
	seen := map[string]bool{}
	var identifiers []string
	for _, name := range names {
		for _, match := range linearIDPattern.FindAllString(name, -1) {
			upper := strings.ToUpper(match)
			if !seen[upper] {
				seen[upper] = true
				identifiers = append(identifiers, upper)
			}
		}
	}
	return identifiers
}

// ExtractLinearIdentifiers finds all Linear issue IDs from branch names.
// Identifiers are normalized to uppercase (e.g. "eng-123" → "ENG-123").
func ExtractLinearIdentifiers(branches []Branch) []string {
	seen := map[string]bool{}
	var identifiers []string
	for _, branch := range branches {
		for _, match := range linearIDPattern.FindAllString(branch.Name, -1) {
			upper := strings.ToUpper(match)
			if !seen[upper] {
				seen[upper] = true
				identifiers = append(identifiers, upper)
			}
		}
	}
	return identifiers
}

// DeleteLocalBranch deletes a local branch.
// force=false uses git branch -d (safe, checks merge status).
// force=true uses git branch -D.
// Returns combined stdout+stderr.
func DeleteLocalBranch(repoPath, branchName string, force bool) (string, error) {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.Command("git", "-C", repoPath, "branch", flag, branchName)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// DeleteRemoteBranch deletes a remote tracking branch.
// remoteName is the full upstream name e.g. "origin/feat/foo".
// Returns combined stdout+stderr.
func DeleteRemoteBranch(repoPath, remoteName string) (string, error) {
	idx := strings.Index(remoteName, "/")
	if idx < 0 {
		return "", fmt.Errorf("invalid remote name: %q", remoteName)
	}
	remote := remoteName[:idx]
	branch := remoteName[idx+1:]
	cmd := exec.Command("git", "-C", repoPath, "push", remote, "--delete", branch)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// GetUnpushedCommits returns one-line summaries of commits in branchName not present in remoteName.
func GetUnpushedCommits(repoPath, branchName, remoteName string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log",
		remoteName+".."+branchName, "--oneline", "--no-merges")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}
