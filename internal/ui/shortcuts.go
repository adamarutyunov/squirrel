package ui

import "strings"

const (
	shortcutSeparator = " • "
	shortcutNBSP      = "\u00A0"
)

type shortcutItem struct {
	key    string
	action string
}

func joinShortcutWords(words ...string) string {
	return strings.Join(words, shortcutNBSP)
}

func buildShortcutHelp(items ...shortcutItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.key+":"+item.action)
	}
	return "  " + strings.Join(parts, shortcutSeparator)
}
