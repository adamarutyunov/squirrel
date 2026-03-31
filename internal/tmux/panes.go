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

	return exec.Command("tmux", "resize-pane", "-t", target, "-x", fmt.Sprintf("%d", targetWidth)).Run()
}
