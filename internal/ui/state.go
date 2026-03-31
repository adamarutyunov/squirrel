package ui

import (
	"os"
	"strings"

	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)

func (m *Model) rebuild() {
	query := strings.ToLower(m.filterValue)
	filterActive := query != ""
	multiRepo := len(m.repoPaths) > 1

	m.filteredItems = make([][]contextItem, len(m.repoPaths))
	for repoIdx, items := range m.repoItems {
		var sourceItems []contextItem
		if !filterActive {
			sourceItems = append([]contextItem(nil), items...)
		} else {
			for _, item := range items {
				ctx := item.context
				if strings.Contains(strings.ToLower(ctx.Name), query) ||
					strings.Contains(strings.ToLower(ctx.Branch), query) {
					sourceItems = append(sourceItems, item)
					continue
				}
				if ctx.LinearIssue != nil && strings.Contains(strings.ToLower(ctx.LinearIssue.Title), query) {
					sourceItems = append(sourceItems, item)
				}
			}
		}
		m.sortItems(sourceItems)
		m.filteredItems[repoIdx] = sourceItems
	}

	m.rows = nil
	firstRepo := true
	for repoIdx := range m.repoPaths {
		items := m.filteredItems[repoIdx]
		if len(items) == 0 {
			continue
		}
		if multiRepo {
			if !firstRepo {
				m.rows = append(m.rows, row{kind: rowTypeSpacer, repoIdx: repoIdx})
			}
			m.rows = append(m.rows, row{kind: rowTypeHeader, repoIdx: repoIdx})
			firstRepo = false
		}
		for itemIdx := range items {
			m.rows = append(m.rows, row{kind: rowTypeContext, repoIdx: repoIdx, itemIdx: itemIdx})
		}
	}

	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	for m.cursor < len(m.rows) && !isNavigable(m.rows[m.cursor].kind) {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
		for m.cursor > 0 && !isNavigable(m.rows[m.cursor].kind) {
			m.cursor--
		}
	}
}

func isNavigable(kind rowType) bool { return kind == rowTypeContext }

func (m *Model) moveCursor(dir int) {
	if len(m.rows) == 0 {
		return
	}

	var navigableRows []int
	for rowIndex, currentRow := range m.rows {
		if isNavigable(currentRow.kind) {
			navigableRows = append(navigableRows, rowIndex)
		}
	}
	if len(navigableRows) == 0 {
		return
	}

	currentIndex := 0
	for index, rowIndex := range navigableRows {
		if rowIndex == m.cursor {
			currentIndex = index
			break
		}
	}

	nextIndex := currentIndex + dir
	if nextIndex < 0 {
		nextIndex = len(navigableRows) - 1
	} else if nextIndex >= len(navigableRows) {
		nextIndex = 0
	}
	m.cursor = navigableRows[nextIndex]
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+h {
		m.scrollOffset = m.cursor - h + 1
	}
}

func (m Model) footerLineCount() int {
	if m.prompt != nil {
		return 3
	}
	if m.mode == modeCreating {
		if m.pickerLoading {
			return 2
		}
		if n := len(m.filteredPickerIssues()); n > 0 {
			return 1 + min(10, n)
		}
	}
	return 1
}

func (m Model) statusLineCount() int {
	n := len(m.outputLines)
	if n > 3 {
		return 3
	}
	return n
}

func (m Model) listHeight() int {
	fixed := 6 + m.statusLineCount() + m.footerLineCount()
	h := m.height - fixed
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) appendOutput(line string) {
	m.outputLines = append(m.outputLines, line)
}

func (m *Model) activeContextPath() string {
	if m.selectedContextPath != "" {
		return m.selectedContextPath
	}
	if m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.kind == rowTypeContext && r.repoIdx < len(m.filteredItems) && r.itemIdx < len(m.filteredItems[r.repoIdx]) {
			return m.filteredItems[r.repoIdx][r.itemIdx].context.Path
		}
	}
	if len(m.repoPaths) > 0 {
		return m.repoPaths[0]
	}
	return "."
}

func (m *Model) filteredPickerIssues() []linear.Issue {
	if len(m.pickerIssues) == 0 {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(m.createInput.Value()))
	if query == "" {
		return m.pickerIssues
	}
	var filtered []linear.Issue
	for _, issue := range m.pickerIssues {
		if strings.Contains(strings.ToLower(issue.Identifier), query) ||
			strings.Contains(strings.ToLower(issue.Title), query) ||
			strings.Contains(strings.ToLower(issue.BranchName), query) {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func (m *Model) cycleSortMode() {
	switch m.sortMode {
	case sortModeAgent:
		m.sortMode = sortModeAlphabetical
	case sortModeAlphabetical:
		m.sortMode = sortModeLinearID
	case sortModeLinearID:
		m.sortMode = sortModeLinearStatus
	case sortModeLinearStatus:
		m.sortMode = sortModeUpdated
	default:
		m.sortMode = sortModeAgent
	}
	m.rebuild()
	if err := saveSortMode(m.sortMode); err != nil {
		m.appendOutput(styleWarning.Render("⚠ Failed to save sort mode: " + err.Error()))
	}
}

func (m *Model) applyRefresh(msg refreshMsg) {
	targetPath := m.pendingCursorPath
	if targetPath == "" && m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowTypeContext {
		r := m.rows[m.cursor]
		targetPath = m.filteredItems[r.repoIdx][r.itemIdx].context.Path
	}

	items := make([]contextItem, len(msg.contexts))
	for i, ctx := range msg.contexts {
		items[i] = contextItem{context: ctx}
	}
	m.repoItems[msg.repoIdx] = items
	m.rebuild()

	if targetPath != "" {
		for rowIdx, r := range m.rows {
			if r.kind == rowTypeContext && m.filteredItems[r.repoIdx][r.itemIdx].context.Path == targetPath {
				m.cursor = rowIdx
				m.pendingCursorPath = ""
				m.ensureVisible()
				return
			}
		}
	}
}

func (m *Model) resetCreateState() {
	m.mode = modeBrowsing
	m.createInput.SetValue("")
	m.createInput.Blur()
	m.filter.Focus()
	m.pickerIssues = nil
	m.pickerRepoIdx = -1
	m.pickerCursor = -1
	m.pickerScroll = 0
	m.pickerLoading = false
}

func (m Model) renderPromptPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" {
		path = strings.Replace(path, home, "~", 1)
	}

	parts := strings.Split(path, "/")
	for i := 1; i < len(parts)-1; i++ {
		if len(parts[i]) > 1 {
			parts[i] = string([]rune(parts[i])[:1])
		}
	}
	return strings.Join(parts, "/")
}

func saveSortMode(mode sortMode) error {
	cfg, err := workspace.LoadUserConfig()
	if err != nil {
		return err
	}
	cfg.SortMode = mode.configValue()
	return workspace.SaveUserConfig(cfg)
}
