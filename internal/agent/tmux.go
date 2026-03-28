package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func PreferredCommand() string {
	if configuredCommand := strings.TrimSpace(os.Getenv("SQUIRREL_AGENT_COMMAND")); configuredCommand != "" {
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
	return exec.Command(
		"tmux",
		"new-session",
		"-A",
		"-s", sessionName,
		"-c", contextPath,
		"exec "+command,
	)
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
