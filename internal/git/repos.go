package git

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverRepos finds git repos at or within dir.
// If dir itself contains a .git directory, returns [dir].
// Otherwise scans immediate subdirectories for .git directories.
func DiscoverRepos(dir string) ([]string, error) {
	if isMainWorktree(dir) {
		return []string{dir}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subdir := filepath.Join(dir, entry.Name())
		if isMainWorktree(subdir) {
			paths = append(paths, subdir)
		}
	}

	sort.Strings(paths)
	return paths, nil
}

// isMainWorktree returns true only when dir is a primary git repository
// (i.e., .git is a directory). Linked worktrees have a .git file, not a
// directory, so they are excluded — they are discovered via git worktree list.
func isMainWorktree(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
