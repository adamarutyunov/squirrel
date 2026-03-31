package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"squirrel/internal/agent"
	"squirrel/internal/git"
	"squirrel/internal/linear"
)

var linearIDRegex = regexp.MustCompile(`(?i)[a-z][a-z0-9]+-\d+`)

// Context represents a single workspace context (a git worktree) within a project.
type Context struct {
	Name        string
	RepoPath    string // path to the main worktree root
	RepoName    string // basename of RepoPath
	Path        string // path to this worktree (for main = RepoPath)
	Branch      string
	IsMain      bool
	IsDirty     bool
	HeadTime    time.Time
	LinearIssue *linear.Issue
	AgentStatus string
	SetupStatus string
}

// ListContexts returns all contexts for the repository at repoPath.
// Linear issues are attached to contexts whose branch names contain matching identifiers.
func ListContexts(repoPath string, linearIssues map[string]linear.Issue, resetThinking ...bool) ([]Context, error) {
	worktrees, err := git.ListWorktrees(repoPath)
	if err != nil {
		return nil, err
	}

	repoName := filepath.Base(repoPath)
	contexts := make([]Context, 0, len(worktrees))

	for _, wt := range worktrees {
		ctx := Context{
			Name:     deriveContextName(wt.Path, repoPath, repoName, wt.IsMain),
			RepoPath: repoPath,
			RepoName: repoName,
			Path:     wt.Path,
			Branch:   wt.Branch,
			IsMain:   wt.IsMain,
			IsDirty:  wt.IsDirty,
			HeadTime: wt.HeadTime,
		}
		status, err := agent.ReadStatus(wt.Path)
		if err == nil {
			ctx.AgentStatus = status.State
		}
		setupStatus, err := ReadSetupStatus(wt.Path)
		if err == nil {
			ctx.SetupStatus = setupStatus.State
		}
		// On first load, reset stale thinking states to idle.
		// Running agents will update back to thinking via hooks.
		if len(resetThinking) > 0 && resetThinking[0] && ctx.AgentStatus == agent.StatusThinking {
			agent.WriteStatus(wt.Path, agent.StatusIdle)
			ctx.AgentStatus = agent.StatusIdle
		}
		for _, match := range linearIDRegex.FindAllString(wt.Branch, -1) {
			if issue, ok := linearIssues[strings.ToUpper(match)]; ok {
				issueCopy := issue
				ctx.LinearIssue = &issueCopy
				break
			}
		}
		contexts = append(contexts, ctx)
	}
	return contexts, nil
}

func deriveContextName(worktreePath, repoPath, repoName string, isMain bool) string {
	if isMain {
		return "main"
	}
	dirName := filepath.Base(worktreePath)
	prefix := repoName + "-"
	if strings.HasPrefix(dirName, prefix) {
		return strings.TrimPrefix(dirName, prefix)
	}
	return dirName
}

// WorktreePath returns the path where a new worktree for contextName would be placed.
// Convention: <parent_of_repo>/<repo_name>-<sanitized_context_name>
func WorktreePath(repoPath, contextName string) string {
	parentDir := filepath.Dir(repoPath)
	repoName := filepath.Base(repoPath)
	return filepath.Join(parentDir, repoName+"-"+SanitizeName(contextName))
}

// SanitizeName converts a context name to a safe directory/branch name
// by replacing any character that isn't alphanumeric, hyphen, or underscore with a hyphen.
func SanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(name))
}

// CreateContext creates a git worktree for a new context, then creates any configured symlinks.
// Returns the worktree path so the caller can run the setup command there.
func CreateContext(repoPath, contextName, branchName string, cfg Config) (string, error) {
	worktreePath := WorktreePath(repoPath, contextName)

	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("path already exists: %s", worktreePath)
	}

	createBranch, err := shouldCreateBranch(repoPath, branchName)
	if err != nil {
		return "", err
	}

	if err := git.AddWorktree(repoPath, worktreePath, branchName, createBranch); err != nil {
		return "", err
	}

	for _, linkPath := range cfg.Symlinks {
		src := filepath.Join(repoPath, linkPath)
		dst := filepath.Join(worktreePath, linkPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return worktreePath, fmt.Errorf("create symlink parent dir: %w", err)
		}
		os.Remove(dst) // remove if git already created it
		if err := os.Symlink(src, dst); err != nil {
			return worktreePath, fmt.Errorf("create symlink %s: %w", linkPath, err)
		}
	}

	return worktreePath, nil
}

func shouldCreateBranch(repoPath, branchName string) (bool, error) {
	branches, err := git.GetBranches(repoPath)
	if err != nil {
		return false, fmt.Errorf("list branches: %w", err)
	}

	for _, branch := range branches {
		if branch.Name == branchName {
			return false, nil
		}
	}

	return true, nil
}

// DeleteContext removes the worktree for ctx.
// If force is true, skips the dirty check and forces worktree removal.
func DeleteContext(ctx Context, force bool) error {
	if !force {
		dirty, err := git.IsWorktreeDirty(ctx.Path)
		if err != nil {
			return fmt.Errorf("checking dirty state: %w", err)
		}
		if dirty {
			return fmt.Errorf("context has uncommitted changes")
		}
	}
	if err := agent.CleanupContext(ctx.Path); err != nil {
		return err
	}
	if err := agent.RemoveStatus(ctx.Path); err != nil {
		return err
	}
	if err := ClearSetupStatus(ctx.Path); err != nil {
		return err
	}
	// Remove saved session ID so --resume doesn't try to resume a deleted context.
	sessionIDPath, err := agent.SessionIDPath(ctx.Path)
	if err == nil {
		os.Remove(sessionIDPath)
	}
	return git.RemoveWorktree(ctx.RepoPath, ctx.Path, force)
}
