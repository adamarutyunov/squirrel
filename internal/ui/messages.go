package ui

import (
	"time"

	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)

type tickMsg time.Time

type refreshMsg struct {
	repoIdx  int
	contexts []workspace.Context
}

type createContextResultMsg struct {
	repoIdx      int
	contextName  string
	worktreePath string
	err          error
}

type setupCommandResultMsg struct {
	repoIdx      int
	worktreePath string
	output       string
	err          error
}

type deleteContextResultMsg struct {
	repoIdx     int
	err         error
	newContexts []workspace.Context
}

type clipboardMsg struct {
	path string
	err  error
}

type linearIssuesLoadedMsg struct {
	repoIdx int
	issues  []linear.Issue
	err     error
}

type agentAttachFinishedMsg struct {
	err error
}

type agentLaunchBackgroundMsg struct {
	err error
}
