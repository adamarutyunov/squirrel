package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"squirrel/internal/agent"
	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)

func (m Model) renderPrompt(ctx workspace.Context) string {
	line := styleStatus.Render(m.renderPromptPath(ctx.Path))
	if ctx.Branch != "" {
		line += styleDim.Render(" (") + styleDim.Render(ctx.Branch) + styleDim.Render(")")
	}
	line += styleStatus.Render(" >")
	return line
}

func (m Model) hasActiveLaunch() bool { return len(m.launchPaneIDs) > 0 }

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	return m.renderLeft(m.width)
}

func (m Model) renderLeft(w int) string {
	divider := styleDim.Render(strings.Repeat("─", w))

	title := styleTitle.Render("  Contexts")
	var subtitle string
	switch len(m.repoPaths) {
	case 0:
		subtitle = ""
	case 1:
		subtitle = m.repoNames[0]
	default:
		subtitle = fmt.Sprintf("%d projects", len(m.repoPaths))
	}
	subtitleStr := styleDim.Render(subtitle)
	spacer := strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(subtitleStr)))
	header := title + spacer + subtitleStr

	filterRow := styleDim.Render("  Filter: ") + m.filter.View()

	listH := m.listHeight()
	end := min(m.scrollOffset+listH, len(m.rows))
	rendered := make([]string, 0, listH)
	for i := m.scrollOffset; i < end; i++ {
		rendered = append(rendered, m.renderRow(m.rows[i], i == m.cursor, w))
	}
	for len(rendered) < listH {
		rendered = append(rendered, "")
	}

	footer := m.renderFooter(w)
	statusCount := m.statusLineCount()
	var statusLines []string
	for i := len(m.outputLines) - statusCount; i < len(m.outputLines); i++ {
		line := "  " + m.outputLines[i]
		statusLines = append(statusLines, lipgloss.NewStyle().MaxWidth(w).Render(line))
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, "")
	lines = append(lines, header)
	lines = append(lines, "")
	lines = append(lines, filterRow)
	lines = append(lines, "")
	lines = append(lines, rendered...)

	footerLines := strings.Split(footer, "\n")
	bottomNeeded := 1 + len(footerLines)
	remaining := m.height - len(lines) - bottomNeeded
	if remaining > 0 && len(statusLines) > 0 {
		shown := min(remaining, len(statusLines))
		lines = append(lines, statusLines[len(statusLines)-shown:]...)
	}

	for len(lines) < m.height-bottomNeeded {
		lines = append(lines, "")
	}

	lines = append(lines, divider)
	lines = append(lines, footerLines...)

	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFooter(w int) string {
	if m.prompt != nil {
		lines := []string{
			"  " + styleWarning.Bold(true).Render(m.prompt.title),
			"  " + styleDim.Render(m.prompt.message),
			"  " + styleDim.Render(m.prompt.confirmText+"  "+m.prompt.cancelText),
		}
		return strings.Join(lines, "\n")
	}

	switch m.mode {
	case modeCreating:
		inputLine := "  " + styleDim.Render("New Context: ") + m.createInput.View() +
			styleDim.Render("  enter: Create  ↑↓: Pick  esc: Cancel")

		if m.pickerLoading {
			return inputLine + "\n" + styleDim.Render("  Loading issues...")
		}

		filtered := m.filteredPickerIssues()
		pickerIDWidth := maxIssueIdentifierWidth(filtered)
		total := len(filtered)
		if total == 0 {
			return inputLine
		}

		shown := min(10, total-m.pickerScroll)
		lines := []string{inputLine}
		for i := 0; i < shown; i++ {
			idx := m.pickerScroll + i
			issue := filtered[idx]
			idLabel := padIssueIdentifier(issue.Identifier, pickerIDWidth)
			if idx == m.pickerCursor {
				idStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorBlue).Bold(true).Render("  " + idLabel)
				titleStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorWhite).Width(w - 2 - lipgloss.Width("  "+idLabel) - 2).Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			} else {
				idStr := styleLinearID.Render("  " + idLabel)
				titleStr := styleDim.Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			}
		}
		return strings.Join(lines, "\n")

	default:
		help := buildShortcutHelp(
			shortcutItem{key: "↑↓/jk", action: "Nav"},
			shortcutItem{key: "enter", action: "Select"},
			shortcutItem{key: "n", action: "New"},
			shortcutItem{key: "d", action: "Del"},
			shortcutItem{key: "c", action: "Copy"},
			shortcutItem{key: "a", action: "Agent"},
			shortcutItem{key: "l", action: "Launch"},
			shortcutItem{key: "L", action: "Kill"},
			shortcutItem{key: "s", action: "Sort(" + m.sortModeLabel() + ")"},
			shortcutItem{key: "ctrl+r", action: "Pane"},
			shortcutItem{key: "ctrl+u", action: joinShortcutWords("User", "cfg")},
			shortcutItem{key: "ctrl+p", action: joinShortcutWords("Project", "cfg")},
			shortcutItem{key: "q", action: "Quit"},
		)
		return styleDim.Width(max(1, w)).Render(help)
	}
}

func (m Model) renderRow(r row, selected bool, w int) string {
	switch r.kind {
	case rowTypeHeader:
		name := m.repoNames[r.repoIdx]
		prefix := " ── " + name + " "
		line := prefix + strings.Repeat("─", max(0, w-len(prefix)))
		return styleHeader.Render(line)
	case rowTypeContext:
		return m.renderContext(r, selected, w)
	case rowTypeSpacer:
		return ""
	}
	return ""
}

func (m Model) renderContext(r row, selected bool, w int) string {
	item := m.filteredItems[r.repoIdx][r.itemIdx]
	ctx := item.context

	const prefixWidth = 4
	const installWidth = 3
	const dirtyWidth = 2
	const agentWidth = 3
	const launchWidth = 2
	const timeWidth = 8
	rightWidth := installWidth + dirtyWidth + agentWidth + launchWidth + timeWidth

	middleWidth := w - prefixWidth - rightWidth
	if middleWidth < 10 {
		middleWidth = 10
	}

	hasLinear := ctx.LinearIssue != nil
	hasBranch := ctx.Branch != "" && ctx.Branch != ctx.Name

	var nameColW, branchColW, linearColW int
	if !hasBranch && !hasLinear {
		nameColW = middleWidth
	} else {
		nameColW = min(20, middleWidth*2/5)
		if nameColW < 8 {
			nameColW = 8
		}
		remaining := middleWidth - nameColW - 2
		if remaining < 0 {
			remaining = 0
		}
		if hasBranch && hasLinear {
			branchColW = remaining / 2
			linearColW = remaining - branchColW - 2
			if linearColW < 0 {
				linearColW = 0
			}
		} else if hasBranch {
			branchColW = remaining
		} else {
			linearColW = remaining
		}
	}

	nameRunes := []rune(ctx.Name)
	if len(nameRunes) > nameColW {
		nameRunes = append(nameRunes[:nameColW-1], '…')
	}
	namePadded := string(nameRunes) + strings.Repeat(" ", max(0, nameColW-len(nameRunes)))

	branchPadded := ""
	if hasBranch && branchColW > 0 {
		branchRunes := []rune(ctx.Branch)
		if len(branchRunes) > branchColW {
			branchRunes = append(branchRunes[:branchColW-1], '…')
		}
		branchPadded = string(branchRunes) + strings.Repeat(" ", max(0, branchColW-len(branchRunes)))
	}

	timeStr := fmt.Sprintf("%-*s", timeWidth, relativeTime(ctx.HeadTime))
	dirtyStr := "  "
	if ctx.IsDirty {
		dirtyStr = "● "
	}
	agentStr := "   "
	if m.companionAgentContextPath == ctx.Path {
		agentStr = "🤖 "
	}
	installStr := "    "
	if ctx.SetupStatus == workspace.SetupStatusRunning {
		spinnerFrames := []string{"◐", "◓", "◑", "◒"}
		installStr = "🛠" + spinnerFrames[m.spinnerFrame%len(spinnerFrames)] + " "
	}
	launchStr := "  "
	if m.launchContextPath[r.repoIdx] == ctx.Path {
		launchStr = "▶ "
	}

	isActive := m.selectedContextPath != "" && ctx.Path == m.selectedContextPath
	return m.renderContextRow(ctx, r.repoIdx, namePadded, branchPadded, installStr, dirtyStr, agentStr, launchStr, timeStr, hasLinear, linearColW, w, selected, isActive)
}

func (m Model) renderContextRow(ctx workspace.Context, repoIdx int, namePadded, branchPadded, installStr, dirtyStr, agentStr, launchStr, timeStr string, hasLinear bool, linearColW, w int, isCursor, isActive bool) string {
	base := lipgloss.NewStyle()
	if isCursor {
		base = base.Background(colorSelectionActive)
	}

	activePrefix := "  "
	if isActive {
		activePrefix = "* "
	}

	statusPrefix := "  "
	statusStyle := base.Foreground(colorDim)
	switch {
	case ctx.AgentStatus == agent.StatusThinking:
		spinnerFrames := []string{"◐ ", "◓ ", "◑ ", "◒ "}
		statusPrefix = spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		statusStyle = base.Foreground(colorAmber)
	case ctx.AgentStatus == agent.StatusDone:
		statusPrefix = "● "
		statusStyle = base.Foreground(colorBlue)
	case ctx.AgentStatus == agent.StatusIdle:
		statusPrefix = "○ "
		statusStyle = base.Foreground(colorDim)
	}

	prefixStyled := base.Render(activePrefix) + statusStyle.Render(statusPrefix)

	var nameStyled string
	if isActive {
		nameStyled = base.Foreground(colorAmber).Bold(true).Render(namePadded)
	} else if ctx.IsMain {
		nameStyled = base.Bold(true).Render(namePadded)
	} else if isCursor {
		nameStyled = base.Foreground(colorWhite).Render(namePadded)
	} else {
		nameStyled = namePadded
	}

	var dirtyStyled string
	if ctx.IsDirty {
		dirtyStyled = base.Foreground(colorAmber).Render(dirtyStr)
	} else {
		dirtyStyled = base.Foreground(colorDim).Render(dirtyStr)
	}
	var agentStyled string
	if agentStr == "🤖 " {
		agentStyled = base.Foreground(colorBlue).Render(agentStr)
	} else {
		agentStyled = base.Foreground(colorDim).Render(agentStr)
	}
	installStyled := base.Foreground(colorAmber).Render(installStr)

	var launchStyled string
	if launchStr == "▶ " {
		launchStyled = base.Foreground(colorGreen).Render(launchStr)
	} else {
		launchStyled = base.Foreground(colorDim).Render(launchStr)
	}
	timeStyled := base.Foreground(colorDim).Render(timeStr)

	var linearStyled string
	if hasLinear {
		linearStyled = renderLinearIssue(base, *ctx.LinearIssue, m.maxRepoIssueIdentifierWidth(repoIdx), linearColW)
	}

	var branchStyled string
	if branchPadded != "" {
		branchStyled = base.Foreground(colorDim).Render("  " + branchPadded)
	}

	var line string
	switch {
	case branchPadded != "" && hasLinear:
		line = prefixStyled + nameStyled + branchStyled + base.Render("  ") + linearStyled + installStyled + dirtyStyled + agentStyled + launchStyled + timeStyled
	case branchPadded != "":
		line = prefixStyled + nameStyled + branchStyled + installStyled + dirtyStyled + agentStyled + launchStyled + timeStyled
	case hasLinear:
		line = prefixStyled + nameStyled + base.Render("  ") + linearStyled + installStyled + dirtyStyled + agentStyled + launchStyled + timeStyled
	default:
		line = prefixStyled + nameStyled + installStyled + dirtyStyled + agentStyled + launchStyled + timeStyled
	}

	if isCursor {
		visibleWidth := lipgloss.Width(line)
		if visibleWidth < w {
			line += base.Render(strings.Repeat(" ", w-visibleWidth))
		}
	}
	return line
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	since := time.Since(t)
	switch {
	case since < time.Minute:
		return "now"
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh", int(since.Hours()))
	case since < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(since.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func renderLinearIssue(base lipgloss.Style, issue linear.Issue, idWidth, width int) string {
	if width <= 0 {
		return ""
	}

	if idWidth <= 0 {
		idWidth = len([]rune(issue.Identifier))
	}

	if width <= idWidth {
		return linearIDStyle(base, issue).Render(truncateRunes(issue.Identifier, width))
	}

	idPart := padIssueIdentifier(issue.Identifier, idWidth)
	textWidth := max(0, width-idWidth-1)
	titlePart := " " + issue.Title
	titlePart = truncateRunes(titlePart, textWidth)
	padding := strings.Repeat(" ", max(0, textWidth-len([]rune(titlePart))))

	return linearIDStyle(base, issue).Render(idPart) +
		linearTextStyle(base, issue).Render(titlePart) +
		base.Render(padding)
}

func linearIDStyle(base lipgloss.Style, issue linear.Issue) lipgloss.Style {
	style := base.Bold(true)

	switch issue.State.Type {
	case "backlog", "canceled":
		return style.Foreground(colorDim).Strikethrough(issue.State.Type == "canceled")
	case "unstarted":
		return style.Foreground(linearStatusColor(issue.State.Color, colorBlue))
	case "started":
		return style.Foreground(linearStatusColor(issue.State.Color, colorBlue))
	case "completed":
		return style.Foreground(linearStatusColor(issue.State.Color, colorBlue))
	case "triage":
		return style.Foreground(colorAmber)
	default:
		return style.Foreground(colorBlue)
	}
}

func linearTextStyle(base lipgloss.Style, issue linear.Issue) lipgloss.Style {
	style := base

	switch issue.State.Type {
	case "backlog":
		return style.Foreground(colorDim)
	case "unstarted":
		return style.Foreground(colorWhite)
	case "started":
		return style.Foreground(colorWhite)
	case "completed":
		return style.Foreground(colorDim).Strikethrough(true)
	case "canceled":
		return style.Foreground(colorDim).Strikethrough(true)
	case "triage":
		return style.Foreground(colorWhite)
	default:
		return style.Foreground(colorWhite)
	}
}

func linearStatusColor(value string, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return lipgloss.Color(value)
}

func (m Model) maxRepoIssueIdentifierWidth(repoIdx int) int {
	if repoIdx < 0 || repoIdx >= len(m.filteredItems) {
		return 0
	}

	width := 0
	for _, item := range m.filteredItems[repoIdx] {
		if item.context.LinearIssue == nil {
			continue
		}
		issueWidth := len([]rune(item.context.LinearIssue.Identifier))
		if issueWidth > width {
			width = issueWidth
		}
	}
	return width
}

func maxIssueIdentifierWidth(issues []linear.Issue) int {
	width := 0
	for _, issue := range issues {
		issueWidth := len([]rune(issue.Identifier))
		if issueWidth > width {
			width = issueWidth
		}
	}
	return width
}

func padIssueIdentifier(value string, width int) string {
	runes := []rune(value)
	if width <= len(runes) {
		return value
	}
	return value + strings.Repeat(" ", width-len(runes))
}

func truncateRunes(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(append(runes[:width-1], '…'))
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
