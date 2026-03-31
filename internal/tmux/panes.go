package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func WindowWidth(target string) (int, error) {
	output, err := exec.Command("tmux", "display-message", "-p", "-t", target, "#{window_width}").Output()
	if err != nil {
		return 0, err
	}

	width, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}
	return width, nil
}

func PaneWidth(target string) (int, error) {
	output, err := exec.Command("tmux", "display-message", "-p", "-t", target, "#{pane_width}").Output()
	if err != nil {
		return 0, err
	}

	width, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}
	return width, nil
}

func WidthForPercent(windowWidth, percent, minWidth int) int {
	if windowWidth <= 0 {
		return 0
	}

	targetWidth := windowWidth * percent / 100
	if targetWidth < minWidth {
		targetWidth = minWidth
	}
	if targetWidth >= windowWidth {
		targetWidth = windowWidth - 1
	}
	if targetWidth < 1 {
		return 0
	}
	return targetWidth
}

func ResizePaneWidth(target string, percent, minWidth int) error {
	windowWidth, err := WindowWidth(target)
	if err != nil {
		return err
	}

	targetWidth := WidthForPercent(windowWidth, percent, minWidth)
	if targetWidth == 0 {
		return nil
	}

	currentWidth, err := PaneWidth(target)
	if err == nil && absInt(currentWidth-targetWidth) <= 1 {
		return nil
	}

	return exec.Command("tmux", "resize-pane", "-t", target, "-x", fmt.Sprintf("%d", targetWidth)).Run()
}

func PaneExists(target string) bool {
	return exec.Command("tmux", "display-message", "-p", "-t", target, "#{pane_id}").Run() == nil
}

func SelectPane(target string) error {
	return exec.Command("tmux", "select-pane", "-t", target).Run()
}

func SetPaneTitle(target, title string) error {
	return exec.Command("tmux", "select-pane", "-t", target, "-T", title).Run()
}

func KillPane(target string) error {
	return exec.Command("tmux", "kill-pane", "-t", target).Run()
}

func SplitPaneHorizontal(target, dir, title, shellCommand string) (string, error) {
	tmuxCommand := "exec sh -lc " + ShellQuote(shellCommand)
	output, err := exec.Command(
		"tmux", "split-window", "-h", "-d",
		"-t", target,
		"-c", dir,
		"-P", "-F", "#{pane_id}",
		tmuxCommand,
	).Output()
	if err != nil {
		return "", err
	}

	paneID := strings.TrimSpace(string(output))
	if paneID != "" && title != "" {
		_ = SetPaneTitle(paneID, title)
	}
	return paneID, nil
}

func SplitPaneVertical(target, dir, title, shellCommand string) (string, error) {
	tmuxCommand := "exec sh -lc " + ShellQuote(shellCommand)
	output, err := exec.Command(
		"tmux", "split-window", "-v", "-d",
		"-t", target,
		"-c", dir,
		"-P", "-F", "#{pane_id}",
		tmuxCommand,
	).Output()
	if err != nil {
		return "", err
	}

	paneID := strings.TrimSpace(string(output))
	if paneID != "" && title != "" {
		_ = SetPaneTitle(paneID, title)
	}
	return paneID, nil
}

func RespawnPane(target, dir, title, shellCommand string) error {
	tmuxCommand := "exec sh -lc " + ShellQuote(shellCommand)
	if err := exec.Command("tmux", "respawn-pane", "-k", "-t", target, "-c", dir, tmuxCommand).Run(); err != nil {
		return err
	}
	if title != "" {
		return SetPaneTitle(target, title)
	}
	return nil
}

func ApplyMainVerticalLayout(mainPaneID string, mainPercent, minMainWidth int) error {
	if strings.TrimSpace(mainPaneID) == "" {
		return nil
	}
	if err := exec.Command("tmux", "select-layout", "-t", mainPaneID, "main-vertical").Run(); err != nil {
		return err
	}
	return ResizePaneWidth(mainPaneID, mainPercent, minMainWidth)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
