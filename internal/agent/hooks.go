package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func InstallHooks() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDirectory, ".claude", "settings.json")

	// Read existing settings or start fresh.
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
				map[string]any{
					"type":    "command",
					"command": hookCommand,
				},
			},
		},
	}

	notificationEntry := []any{
		map[string]any{
			"matcher": "idle_prompt",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": hookCommand,
				},
			},
		},
	}

	hooks := map[string]any{
		"SessionStart":     hookEntry,
		"UserPromptSubmit": hookEntry,
		"Stop":             hookEntry,
		"StopFailure":      hookEntry,
		"Notification":     notificationEntry,
		"SessionEnd":       hookEntry,
	}

	settings["hooks"] = hooks

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal settings: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(settingsPath, output, 0o644)
}
