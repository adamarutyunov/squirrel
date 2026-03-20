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
	if isGitRoot(dir) {
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
		if isGitRoot(subdir) {
			paths = append(paths, subdir)
		}
	}

	sort.Strings(paths)
	return paths, nil
}

func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
