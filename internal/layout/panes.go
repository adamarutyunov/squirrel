package layout

import (
	"fmt"
	"strings"

	"squirrel/internal/tmux"
)

type PaneWidth struct {
	Percent  int
	MinWidth int
	MaxWidth int
}

var (
	CompanionPaneWidth  = PaneWidth{Percent: 35}
	MainPaneSoloWidth   = PaneWidth{Percent: 45, MinWidth: 50, MaxWidth: 90}
	MainPaneLaunchWidth = PaneWidth{Percent: 40, MinWidth: 50, MaxWidth: 90}
	LaunchPaneWidth     = PaneWidth{Percent: 15, MinWidth: 25}
)

func (p PaneWidth) SplitArg() string {
	if p.Percent <= 0 {
		return ""
	}
	return fmt.Sprintf("%d%%", p.Percent)
}

func (p PaneWidth) Resize(target string) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	return tmux.ResizePaneWidth(target, p.Percent, p.MinWidth, p.MaxWidth)
}

func ApplyLaunchLayout(mainPaneID, launchPaneID string) error {
	if strings.TrimSpace(launchPaneID) == "" {
		return MainPaneSoloWidth.Resize(mainPaneID)
	}
	if err := MainPaneLaunchWidth.Resize(mainPaneID); err != nil {
		return err
	}
	return LaunchPaneWidth.Resize(launchPaneID)
}
