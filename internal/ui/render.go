package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"squirrel/internal/agent"
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

func (m Model) launchPanelWidth() int {
	w := m.width * 15 / 100
	if w < 30 {
		w = 30
	}
	return w
}

func (m Model) hasActiveLaunch() bool { return len(m.launchPanels) > 0 }

func (m Model) launchPanelHeight() int {
	n := len(m.launchPanels)
	if n == 0 {
		return 0
	}
	return (m.height - (n - 1)) / n
}

func (m Model) isLaunchFocused() bool { return m.launchFocusIndex >= 0 }

func (m Model) sortedLaunchIndices() []int {
	indices := make([]int, 0, len(m.launchPanels))
	for idx := range m.launchPanels {
		indices = append(indices, idx)
	}
	sortInts(indices)
	return indices
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	if !m.hasActiveLaunch() {
		return m.renderLeft(m.width)
	}

	launchW := m.launchPanelWidth()
	leftW := m.width - launchW - 1

	leftContent := lipgloss.NewStyle().Width(leftW).MaxWidth(leftW).Render(m.renderLeft(leftW))
	leftLines := strings.Split(leftContent, "\n")

	sorted := m.sortedLaunchIndices()
	panelH := m.launchPanelHeight()
	rightW := launchW
	var rightLines []string
	for panelIndex, repoIdx := range sorted {
		if panelIndex > 0 {
			rightLines = append(rightLines, styleDim.Render(strings.Repeat("─", rightW)))
		}
		panel := m.launchPanels[repoIdx]
		panelLines := strings.Split(panel.View(), "\n")
		for len(panelLines) < panelH {
			panelLines = append(panelLines, "")
		}
		rightLines = append(rightLines, panelLines[:panelH]...)
	}

	for len(leftLines) < m.height {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < m.height {
		rightLines = append(rightLines, "")
	}

	sep := styleDim.Render("│")
	var result []string
	for i := 0; i < m.height; i++ {
		left := padToWidth(leftLines[i], leftW)
		right := padToWidth(rightLines[i], rightW)
		result = append(result, left+sep+right)
	}
	return strings.Join(result, "\n")
}

func padToWidth(line string, width int) string {
	visible := lipgloss.Width(line)
	if visible < width {
		return line + strings.Repeat(" ", width-visible)
	}
	return line
}

func (m Model) renderLeft(w int) string {
	divider := styleDim.Render(strings.Repeat("─", w))

	titleText := "Squirrel " + m.version
	title := styleTitle.Render("  " + titleText)
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
	switch m.mode {
	case modeCreating:
		inputLine := "  " + styleDim.Render("New Context: ") + m.createInput.View() +
			styleDim.Render("  enter: Create  ↑↓: Pick  esc: Cancel")

		if m.pickerLoading {
			return inputLine + "\n" + styleDim.Render("  Loading issues...")
		}

		filtered := m.filteredPickerIssues()
		total := len(filtered)
		if total == 0 {
			return inputLine
		}

		shown := min(10, total-m.pickerScroll)
		lines := []string{inputLine}
		for i := 0; i < shown; i++ {
			idx := m.pickerScroll + i
			issue := filtered[idx]
			if idx == m.pickerCursor {
				idStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorBlue).Bold(true).Render("  " + issue.Identifier)
				titleStr := lipgloss.NewStyle().Background(colorSelection).Foreground(colorWhite).Width(w - 2 - lipgloss.Width("  "+issue.Identifier) - 2).Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			} else {
				idStr := styleLinearID.Render("  " + issue.Identifier)
				titleStr := styleDim.Render("  " + issue.Title)
				lines = append(lines, idStr+titleStr)
			}
		}
		return strings.Join(lines, "\n")

	default:
		launchHint := ""
		if m.isLaunchFocused() {
			launchHint = "  " + styleStatus.Render("● Launch") + styleDim.Render("  tab: Next")
		}
		return styleDim.Render("  ↑↓/jk: Nav  enter: Select  n: New  d: Del  c: Copy  a: Agent  l: Launch  L: Kill  s: Sort("+m.sortModeLabel()+")  tab: Cycle  ctrl+w: Terminal  q: Quit") + launchHint
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
	const dirtyWidth = 2
	const launchWidth = 2
	const timeWidth = 8
	rightWidth := dirtyWidth + launchWidth + timeWidth

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
	launchStr := "  "
	if m.launchContextPath[r.repoIdx] == ctx.Path {
		launchStr = "▶ "
	}

	isActive := m.selectedContextPath != "" && ctx.Path == m.selectedContextPath
	return m.renderContextRow(ctx, namePadded, branchPadded, dirtyStr, launchStr, timeStr, hasLinear, linearColW, w, selected, isActive)
}

func (m Model) renderContextRow(ctx workspace.Context, namePadded, branchPadded, dirtyStr, launchStr, timeStr string, hasLinear bool, linearColW, w int, isCursor, isActive bool) string {
	base := lipgloss.NewStyle()
	if isCursor {
		base = base.Background(colorSelection)
	}

	activePrefix := "  "
	if isActive {
		activePrefix = "* "
	}

	statusPrefix := "  "
	statusStyle := base.Foreground(colorDim)
	switch ctx.AgentStatus {
	case agent.StatusThinking:
		spinnerFrames := []string{"◐ ", "◓ ", "◑ ", "◒ "}
		statusPrefix = spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		statusStyle = base.Foreground(colorAmber)
	case agent.StatusDone:
		statusPrefix = "● "
		statusStyle = base.Foreground(colorBlue)
	case agent.StatusIdle:
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

	var launchStyled string
	if launchStr == "▶ " {
		launchStyled = base.Foreground(colorGreen).Render(launchStr)
	} else {
		launchStyled = base.Foreground(colorDim).Render(launchStr)
	}
	timeStyled := base.Foreground(colorDim).Render(timeStr)

	var linearStyled string
	if hasLinear {
		linearRaw := ctx.LinearIssue.Identifier + " " + ctx.LinearIssue.Title
		linearRunes := []rune(linearRaw)
		if len(linearRunes) > linearColW {
			linearRunes = append(linearRunes[:linearColW-1], '…')
		}
		linearPadded := string(linearRunes) + strings.Repeat(" ", max(0, linearColW-len(linearRunes)))
		if spaceIdx := strings.Index(linearPadded, " "); spaceIdx > 0 {
			linearStyled = base.Foreground(colorBlue).Bold(true).Render(linearPadded[:spaceIdx]) +
				base.Foreground(colorWhite).Render(linearPadded[spaceIdx:])
		} else {
			linearStyled = base.Foreground(colorBlue).Bold(true).Render(linearPadded)
		}
	}

	var branchStyled string
	if branchPadded != "" {
		branchStyled = base.Foreground(colorDim).Render("  " + branchPadded)
	}

	var line string
	switch {
	case branchPadded != "" && hasLinear:
		line = prefixStyled + nameStyled + branchStyled + base.Render("  ") + linearStyled + dirtyStyled + launchStyled + timeStyled
	case branchPadded != "":
		line = prefixStyled + nameStyled + branchStyled + dirtyStyled + launchStyled + timeStyled
	case hasLinear:
		line = prefixStyled + nameStyled + base.Render("  ") + linearStyled + dirtyStyled + launchStyled + timeStyled
	default:
		line = prefixStyled + nameStyled + dirtyStyled + launchStyled + timeStyled
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
