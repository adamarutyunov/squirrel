package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func AttachCommand(contextPath, command string) *exec.Cmd {
	sessionName := sessionNameFor(contextPath, command)
	sessionExists := SessionExists(contextPath, command)

	// If session already exists, just attach to it.
	if sessionExists {
		shellCommand := fmt.Sprintf(
			`tmux attach-session -t '%s' \; bind-key -n C-q detach-client`,
			sessionName,
		)
		return exec.Command("sh", "-c", shellCommand)
	}

	// No existing session — start a new one.
	agentCommand := SessionCommand(contextPath, command)

	// Bind Ctrl+Q to detach for easy exit (simpler than default Ctrl+B, D).
	escapedPath := strings.ReplaceAll(contextPath, "'", "'\\''")
	shellCommand := fmt.Sprintf(
		`tmux new-session -s '%s' -c '%s' -E 'exec sh -c "%s"' \; bind-key -n C-q detach-client`,
		sessionName, escapedPath, strings.ReplaceAll(agentCommand, `"`, `\"`),
	)
	return exec.Command("sh", "-c", shellCommand)
}

// SessionCommand returns the agent command to run for a context.
// When a saved session exists, it prefers resuming and falls back to a fresh launch.
func SessionCommand(contextPath, command string) string {
	commandBase := strings.Fields(command)
	if len(commandBase) == 0 {
		return command
	}

	sessionID, _ := ReadSessionID(contextPath)
	if sessionID == "" {
		return command
	}

	switch commandBase[0] {
	case "claude":
		return fmt.Sprintf("%s --resume %s || %s", command, sessionID, command)
	case "codex":
		return fmt.Sprintf("%s resume %s || %s", command, sessionID, command)
	default:
		return command
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

// LaunchBackground starts an agent tmux session in detached mode so it's
// ready when the user attaches later. No-op if the session already exists.
func LaunchBackground(contextPath, command string) error {
	sessionName := sessionNameFor(contextPath, command)
	// Check if session already exists.
	checkCommand := exec.Command("tmux", "has-session", "-t", sessionName)
	if checkCommand.Run() == nil {
		return nil // session already running
	}
	launchCommand := exec.Command(
		"tmux", "new-session", "-d",
		"-s", sessionName,
		"-c", contextPath,
		"exec "+command,
	)
	return launchCommand.Run()
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
