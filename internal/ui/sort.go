package ui

import (
	"sort"
	"strconv"
	"strings"

	"squirrel/internal/agent"
	"squirrel/internal/workspace"
)

func (m Model) sortModeLabel() string {
	switch m.sortMode {
	case sortModeAlphabetical:
		return "Alpha"
	case sortModeLinearID:
		return "Linear (ID)"
	case sortModeLinearStatus:
		return "Linear (Status)"
	case sortModeUpdated:
		return "Updated"
	default:
		return "Agent"
	}
}

func (m sortMode) configValue() string {
	switch m {
	case sortModeAlphabetical:
		return "alpha"
	case sortModeLinearID:
		return "linear_id"
	case sortModeLinearStatus:
		return "linear_status"
	case sortModeUpdated:
		return "updated"
	default:
		return "agent"
	}
}

func parseSortMode(value string) sortMode {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "alpha", "alphabetical":
		return sortModeAlphabetical
	case "linear_id", "linear-id", "linear":
		return sortModeLinearID
	case "linear_status", "linear-status":
		return sortModeLinearStatus
	case "updated":
		return sortModeUpdated
	default:
		return sortModeAgent
	}
}

func (m Model) sortItems(items []contextItem) {
	sort.SliceStable(items, func(leftIndex, rightIndex int) bool {
		leftContext := items[leftIndex].context
		rightContext := items[rightIndex].context

		switch m.sortMode {
		case sortModeAlphabetical:
			return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
		case sortModeLinearID:
			return compareLinearIDContexts(leftContext, rightContext)
		case sortModeLinearStatus:
			return compareLinearStatusContexts(leftContext, rightContext)
		case sortModeUpdated:
			return compareUpdatedContexts(leftContext, rightContext)
		default:
			return compareAgentContexts(leftContext, rightContext)
		}
	})
}

func compareAgentContexts(leftContext, rightContext workspace.Context) bool {
	leftRank := agentStatusRank(leftContext.AgentStatus)
	rightRank := agentStatusRank(rightContext.AgentStatus)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return compareUpdatedContexts(leftContext, rightContext)
}

func compareUpdatedContexts(leftContext, rightContext workspace.Context) bool {
	if !leftContext.HeadTime.Equal(rightContext.HeadTime) {
		return leftContext.HeadTime.After(rightContext.HeadTime)
	}
	return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
}

func compareLinearIDContexts(leftContext, rightContext workspace.Context) bool {
	leftTeam, leftNumber, leftHasIssue := linearSortKey(leftContext)
	rightTeam, rightNumber, rightHasIssue := linearSortKey(rightContext)
	if leftHasIssue != rightHasIssue {
		return leftHasIssue
	}
	if leftTeam != rightTeam {
		return leftTeam < rightTeam
	}
	if leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
}

func compareLinearStatusContexts(leftContext, rightContext workspace.Context) bool {
	leftBucket, leftPos, leftHasIssue := linearStatusSortKey(leftContext)
	rightBucket, rightPos, rightHasIssue := linearStatusSortKey(rightContext)
	if leftHasIssue != rightHasIssue {
		return leftHasIssue
	}
	if leftBucket != rightBucket {
		return leftBucket < rightBucket
	}
	if leftPos != rightPos {
		return leftPos < rightPos
	}
	return compareLinearIDContexts(leftContext, rightContext)
}

func linearSortKey(context workspace.Context) (string, int, bool) {
	if context.LinearIssue == nil {
		return "", 0, false
	}
	parts := strings.SplitN(context.LinearIssue.Identifier, "-", 2)
	if len(parts) != 2 {
		return strings.ToLower(context.LinearIssue.Identifier), 0, true
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil {
		number = 0
	}
	return strings.ToLower(parts[0]), number, true
}

func linearStatusSortKey(context workspace.Context) (int, float64, bool) {
	if context.LinearIssue == nil {
		return 0, 0, false
	}
	return linearStatusBucket(context.LinearIssue.State.Type), context.LinearIssue.State.Position, true
}

func linearStatusBucket(stateType string) int {
	switch stateType {
	case "triage", "backlog":
		return 0
	case "unstarted":
		return 1
	case "started":
		return 2
	case "completed":
		return 3
	case "canceled":
		return 4
	default:
		return 5
	}
}

func agentStatusRank(status string) int {
	switch status {
	case agent.StatusDone:
		return 0
	case agent.StatusThinking:
		return 1
	case agent.StatusIdle:
		return 2
	default:
		return 3
	}
}
