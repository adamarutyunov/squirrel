package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type HookInput struct {
	HookEventName    string `json:"hook_event_name"`
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
