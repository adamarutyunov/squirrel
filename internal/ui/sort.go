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
	case sortModeLinear:
		return "Linear"
	case sortModeUpdated:
		return "Updated"
	default:
		return "Agent"
	}
}

func (m Model) sortItems(items []contextItem) {
	sort.SliceStable(items, func(leftIndex, rightIndex int) bool {
		leftContext := items[leftIndex].context
		rightContext := items[rightIndex].context

		switch m.sortMode {
		case sortModeAlphabetical:
			return strings.ToLower(leftContext.Name) < strings.ToLower(rightContext.Name)
		case sortModeLinear:
			return compareLinearContexts(leftContext, rightContext)
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

func compareLinearContexts(leftContext, rightContext workspace.Context) bool {
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
