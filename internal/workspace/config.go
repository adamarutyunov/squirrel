package workspace

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// UserConfig holds global per-user squirrel configuration at ~/.config/squirrel/config.yaml.
type UserConfig struct {
	AgentCommand string `yaml:"agent_command"`
	SortMode     string `yaml:"sort_mode"`
}

func UserConfigTemplate() string {
	return strings.TrimSpace(`
# Global squirrel settings

# Command used to launch agents.
# agent_command: claude --dangerously-skip-permissions

# Initial sort mode.
# sort_mode: linear_status
`) + "\n"
}

func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "squirrel", "config.yaml"), nil
}

func LoadUserConfig() (UserConfig, error) {
	configPath, err := UserConfigPath()
	if err != nil {
		return UserConfig{}, err
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return UserConfig{}, nil
	}
	if err != nil {
		return UserConfig{}, err
	}
	var cfg UserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, err
	}
	return cfg, nil
}

func SaveUserConfig(cfg UserConfig) error {
	configPath, err := UserConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

func EnsureUserConfigFile() (string, error) {
	configPath, err := UserConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(UserConfigTemplate()), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return configPath, nil
}

// Config holds per-project squirrel configuration, stored in ~/.config/squirrel/
// rather than the project repo so it stays local to each developer's machine.
type Config struct {
	SetupCommand string   `yaml:"setup_command"`
	Symlinks     []string `yaml:"symlinks"`
	LinearAPIKey string   `yaml:"linear_api_key"`
}

func ProjectConfigTemplate() string {
	return strings.TrimSpace(`
# Project-local squirrel settings

# Command to run after creating a new context.
# setup_command: pnpm install

# Paths to symlink from the main worktree into new contexts.
# symlinks:
#   - node_modules
#   - .env

# Linear API key for this project. Falls back to LINEAR_API_KEY if omitted.
# linear_api_key: lin_api_...
`) + "\n"
}

// ConfigPath returns the path where config for the given repo is stored.
// Format: ~/.config/squirrel/projects/<basename>-<6hex>/config.yaml
// The hex suffix is derived from the full canonical path to avoid collisions
// between projects with the same directory name.
func ConfigPath(repoPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(repoPath)
	hash := sha256.Sum256([]byte(clean))
	dirName := fmt.Sprintf("%s-%x", filepath.Base(clean), hash[:3])
	return filepath.Join(home, ".config", "squirrel", "projects", dirName, "config.yaml"), nil
}

// LoadConfig reads the squirrel config for the given repo path.
// Returns an empty Config (no error) if no config file exists yet.
func LoadConfig(repoPath string) (Config, error) {
	configPath, err := ConfigPath(repoPath)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SaveConfig writes the squirrel config for the given repo path,
// creating the directory if it does not exist.
func SaveConfig(repoPath string, cfg Config) error {
	configPath, err := ConfigPath(repoPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

func EnsureProjectConfigFile(repoPath string) (string, error) {
	configPath, err := ConfigPath(repoPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(ProjectConfigTemplate()), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return configPath, nil
}
