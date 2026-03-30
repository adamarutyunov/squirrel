package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Worktree represents a single git worktree.
type Worktree struct {
	Path     string
	Branch   string // empty if HEAD is detached
	HeadHash string
	IsMain   bool
	IsDirty  bool
	HeadTime time.Time
}

// ListWorktrees returns all worktrees for the repository at repoPath.
// The first entry is always the main worktree.
func ListWorktrees(repoPath string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git worktree list: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git worktree list failed")
	}

	var worktrees []Worktree
	var current Worktree
	index := 0

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if current.Path != "" {
				current.IsMain = index == 0
				worktrees = append(worktrees, current)
				current = Worktree{}
				index++
			}
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.HeadHash = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	// Handle last block (may not end with blank line in all git versions).
	if current.Path != "" {
		current.IsMain = index == 0
		worktrees = append(worktrees, current)
	}

	for i := range worktrees {
		worktrees[i].IsDirty, _ = IsWorktreeDirty(worktrees[i].Path)
		worktrees[i].HeadTime, _ = GetWorktreeHeadTime(worktrees[i].Path)
	}

	return worktrees, nil
}

// IsWorktreeDirty reports whether the worktree has uncommitted changes.
// Returns false if the directory doesn't exist (nothing to protect).
func IsWorktreeDirty(path string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return false, fmt.Errorf("git status: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return false, fmt.Errorf("git status failed")
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// GetWorktreeHeadTime returns the commit timestamp of HEAD in the given worktree.
func GetWorktreeHeadTime(path string) (time.Time, error) {
	cmd := exec.Command("git", "-C", path, "log", "-1", "--format=%ct")
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}
	ts := strings.TrimSpace(string(output))
	if ts == "" {
		return time.Time{}, nil
	}
	var unix int64
	if _, err := fmt.Sscanf(ts, "%d", &unix); err != nil {
		return time.Time{}, err
	}
	return time.Unix(unix, 0), nil
}

// AddWorktree creates a new worktree at worktreePath.
// If createBranch is true, creates and checks out a new branch named branch.
// If createBranch is false, checks out an existing branch.
func AddWorktree(repoPath, worktreePath, branch string, createBranch bool) error {
	var args []string
	if createBranch {
		args = []string{"-C", repoPath, "worktree", "add", "-b", branch, worktreePath}
	} else {
		args = []string{"-C", repoPath, "worktree", "add", worktreePath, branch}
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// RemoveWorktree removes the worktree at worktreePath.
// force=true passes --force to skip the dirty check.
func RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := []string{"-C", repoPath, "worktree", "remove"}
	if force {
		// Two --force flags: first bypasses dirty check, second bypasses
		// structural validation (e.g. .git is a directory instead of a file).
		args = append(args, "--force", "--force")
	}
	args = append(args, worktreePath)
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
