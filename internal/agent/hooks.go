package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type HookInput struct {
	HookEventName    string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	NotificationType string `json:"notification_type"`
}

func HandleHook(reader io.Reader) error {
	var hookInput HookInput
	if err := json.NewDecoder(reader).Decode(&hookInput); err != nil {
		return err
	}
	if hookInput.Cwd == "" {
		return fmt.Errorf("hook payload missing cwd")
	}

	switch hookInput.HookEventName {
	case "SessionStart":
		if hookInput.SessionID != "" {
			if err := WriteSessionID(hookInput.Cwd, hookInput.SessionID); err != nil {
				return err
			}
		}
		return WriteStatus(hookInput.Cwd, StatusIdle)
	case "UserPromptSubmit":
		return WriteStatus(hookInput.Cwd, StatusThinking)
	case "Stop", "StopFailure":
		return WriteStatus(hookInput.Cwd, StatusDone)
	case "Notification":
		if hookInput.NotificationType == "idle_prompt" {
			return WriteStatus(hookInput.Cwd, StatusIdle)
		}
		return nil
	case "SessionEnd":
		return RemoveStatus(hookInput.Cwd)
	default:
		return nil
	}
}

func HookCommand(executablePath string) string {
	return executablePath + " claude-hook"
}

func HandleHookCommand() error {
	return HandleHook(os.Stdin)
}

func InstallHooks() ([]string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not determine executable path: %w", err)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine home directory: %w", err)
	}

	var installed []string
	if err := installClaudeHooks(exePath, homeDirectory); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	installed = append(installed, "Claude (~/.claude/settings.json)")

	if err := installCodexHooks(exePath, homeDirectory); err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}
	installed = append(installed, "Codex (~/.codex/hooks.json)")

	return installed, nil
}

func installClaudeHooks(exePath, homeDirectory string) error {
	settingsPath := filepath.Join(homeDirectory, ".claude", "settings.json")

	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("could not parse %s: %w", settingsPath, err)
		}
	} else if os.IsNotExist(err) {
		settings = map[string]any{}
	} else {
		return fmt.Errorf("could not read %s: %w", settingsPath, err)
	}

	hookCommand := HookCommand(exePath)

	hookEntry := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": hookCommand},
			},
		},
	}

	notificationEntry := []any{
		map[string]any{
			"matcher": "idle_prompt",
			"hooks": []any{
				map[string]any{"type": "command", "command": hookCommand},
			},
		},
	}

	settings["hooks"] = map[string]any{
		"SessionStart":     hookEntry,
		"UserPromptSubmit": hookEntry,
		"Stop":             hookEntry,
		"StopFailure":      hookEntry,
		"Notification":     notificationEntry,
		"SessionEnd":       hookEntry,
	}

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(settingsPath, output, 0o644)
}

func installCodexHooks(exePath, homeDirectory string) error {
	// Enable hooks feature flag in config.toml.
	configPath := filepath.Join(homeDirectory, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	configData, err := os.ReadFile(configPath)
	configContent := string(configData)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read %s: %w", configPath, err)
	}
	if !strings.Contains(configContent, "codex_hooks") {
		if !strings.Contains(configContent, "[features]") {
			configContent += "\n[features]\ncodex_hooks = true\n"
		} else {
			configContent = strings.Replace(configContent, "[features]", "[features]\ncodex_hooks = true", 1)
		}
		if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
			return fmt.Errorf("could not write %s: %w", configPath, err)
		}
	}

	hooksPath := filepath.Join(homeDirectory, ".codex", "hooks.json")

	var settings map[string]any
	data, err := os.ReadFile(hooksPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("could not parse %s: %w", hooksPath, err)
		}
	} else if os.IsNotExist(err) {
		settings = map[string]any{}
	} else {
		return fmt.Errorf("could not read %s: %w", hooksPath, err)
	}

	hookCommand := HookCommand(exePath)

	hookEntry := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": hookCommand},
			},
		},
	}

	settings["hooks"] = map[string]any{
		"SessionStart":     hookEntry,
		"UserPromptSubmit": hookEntry,
		"Stop":             hookEntry,
	}

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(hooksPath, output, 0o644)
}
