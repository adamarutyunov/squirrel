package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"squirrel/internal/linear"
)

func PreferredCommand(userConfigCommand string) string {
	if configuredCommand := strings.TrimSpace(userConfigCommand); configuredCommand != "" {
		return configuredCommand
	}
	if _, err := exec.LookPath("claude"); err == nil {
		return "claude"
	}
	if _, err := exec.LookPath("codex"); err == nil {
		return "codex"
	}
	return "claude"
}

func CommandForIssue(command string, issue *linear.Issue) string {
	if issue == nil {
		return command
	}

	if !strings.Contains(strings.ToLower(command), "claude") {
		return command
	}

	prompt := fmt.Sprintf(
		"Current Linear task: %s - %s. If the user asks you to examine the task, begin by using this issue context while inspecting the codebase.",
		issue.Identifier,
		issue.Title,
	)
	return fmt.Sprintf("%s --append-system-prompt %s", command, shellQuote(prompt))
}

func AttachCommand(contextPath, sessionCommand, launchCommand string) *exec.Cmd {
	return exec.Command("sh", "-c", AttachShellCommand(contextPath, sessionCommand, launchCommand, true))
}

// SessionCommand returns the agent command to run for a context.
// When a saved session exists, it prefers resuming and falls back to a fresh launch.
func SessionCommand(contextPath, sessionCommand, launchCommand string) string {
	commandBase := strings.Fields(sessionCommand)
	if len(commandBase) == 0 {
		return launchCommand
	}

	sessionID, _ := ReadSessionID(contextPath)
	if sessionID == "" {
		return launchCommand
	}

	switch commandBase[0] {
	case "claude":
		return fmt.Sprintf("%s --resume %s || %s", launchCommand, sessionID, launchCommand)
	case "codex":
		return fmt.Sprintf("%s resume %s || %s", launchCommand, sessionID, launchCommand)
	default:
		return launchCommand
	}
}

// MarkAttached resets a completed agent state back to idle before reopening it.
func MarkAttached(contextPath string) {
	status, err := ReadStatus(contextPath)
	if err != nil || status.State != StatusDone {
		return
	}
	_ = WriteStatus(contextPath, StatusIdle)
}

// AttachShellCommand returns a shell command that attaches to the per-context
// tmux-backed agent session. The session must already exist.
// When nested is true, the command clears TMUX so the attach happens as a
// nested client inside the current pane instead of switching the outer client.
func AttachShellCommand(contextPath, sessionCommand, launchCommand string, nested bool) string {
	sessionName := sessionNameFor(contextPath, sessionCommand)
	prefix := ""
	if nested {
		prefix = "export TMUX=''; "
	}

	return fmt.Sprintf(
		`%stmux bind-key -n C-q detach-client >/dev/null 2>&1 || true; tmux attach-session -t '%s'`,
		prefix,
		sessionName,
	)
}

// LaunchBackground starts an agent tmux session in detached mode so it's
// ready when the user attaches later. No-op if the session already exists.
func LaunchBackground(contextPath, sessionCommand, launchCommand string) error {
	sessionName := sessionNameFor(contextPath, sessionCommand)
	// Check if session already exists.
	checkCommand := exec.Command("tmux", "has-session", "-t", sessionName)
	if checkCommand.Run() == nil {
		return nil // session already running
	}
	cmd := exec.Command(
		"tmux", "new-session", "-d",
		"-s", sessionName,
		"-c", contextPath,
		"exec "+launchCommand,
	)
	if err := cmd.Run(); err != nil {
		return err
	}
	return exec.Command("tmux", "set-option", "-t", sessionName, "status", "off").Run()
}

// SessionExists returns true if the tmux session for this context+command exists.
func SessionExists(contextPath, command string) bool {
	sessionName := sessionNameFor(contextPath, command)
	checkCommand := exec.Command("tmux", "has-session", "-t", sessionName)
	return checkCommand.Run() == nil
}

func CleanupContext(contextPath string) error {
	var firstError error
	for _, command := range knownCommands() {
		sessionName := sessionNameFor(contextPath, command)
		killCommand := exec.Command("tmux", "kill-session", "-t", sessionName)
		output, err := killCommand.CombinedOutput()
		if err == nil {
			continue
		}

		outputText := strings.TrimSpace(string(output))
		if outputText == "" || strings.Contains(outputText, "can't find session") {
			continue
		}
		if firstError == nil {
			firstError = fmt.Errorf("tmux kill-session: %s", outputText)
		}
	}
	return firstError
}

func knownCommands() []string {
	commands := []string{"claude", "codex"}
	if configuredCommand := strings.TrimSpace(os.Getenv("SQUIRREL_AGENT_COMMAND")); configuredCommand != "" {
		commands = append([]string{configuredCommand}, commands...)
	}

	seenCommands := make(map[string]bool, len(commands))
	var dedupedCommands []string
	for _, command := range commands {
		if command == "" || seenCommands[command] {
			continue
		}
		seenCommands[command] = true
		dedupedCommands = append(dedupedCommands, command)
	}
	return dedupedCommands
}

func sessionNameFor(contextPath, command string) string {
	hash := sha1.Sum([]byte(contextPath + "\x00" + command))
	return "squirrel-agent-" + hex.EncodeToString(hash[:6])
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
