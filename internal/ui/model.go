package ui

import (
	"path/filepath"

	"github.com/adamarutyunov/launch/embed"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"squirrel/internal/linear"
	"squirrel/internal/workspace"
)

var (
	colorGreen     = lipgloss.Color("#22c55e")
	colorBlue      = lipgloss.Color("#60a5fa")
	colorDim       = lipgloss.Color("#71717a")
	colorWhite     = lipgloss.Color("#f4f4f5")
	colorSelection = lipgloss.Color("#3f3f46")
	colorAmber     = lipgloss.Color("#f59e0b")
	colorRed       = lipgloss.Color("#ef4444")

	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	styleDim       = lipgloss.NewStyle().Foreground(colorDim)
	styleMain      = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	styleLinearID  = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleLinearDim = lipgloss.NewStyle().Foreground(colorDim)
	styleHeader    = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	styleStatus    = lipgloss.NewStyle().Foreground(colorGreen)
	styleDanger    = lipgloss.NewStyle().Foreground(colorRed)
	styleWarning   = lipgloss.NewStyle().Foreground(colorAmber)
)

type rowType int

const (
	rowTypeHeader rowType = iota
	rowTypeContext
	rowTypeSpacer
)

type row struct {
	kind    rowType
	repoIdx int
	itemIdx int
}

type contextItem struct {
	context workspace.Context
}

type uiMode int

const (
	modeBrowsing uiMode = iota
	modeCreating
)

type sortMode int

const (
	sortModeAgent sortMode = iota
	sortModeAlphabetical
	sortModeLinear
	sortModeUpdated
)

// Model is the BubbleTea model.
type Model struct {
	version      string
	repoPaths    []string
	repoNames    []string
	repoConfigs  []workspace.Config
	repoItems    [][]contextItem
	linearIssues map[string]linear.Issue

	filteredItems [][]contextItem
	rows          []row

	cursor       int
	scrollOffset int

	filter       textinput.Model
	filterValue  string
	filterActive bool
	sortMode     sortMode

	width  int
	height int

	mode          uiMode
	createInput   textinput.Model
	createRepoIdx int

	// Linear issue picker (shown while in modeCreating when API key is set).
	linearAPIKey  string
	pickerIssues  []linear.Issue
	pickerCursor  int // -1 = no selection; ≥0 = index into filteredPickerIssues()
	pickerScroll  int // top visible index in filtered picker list
	pickerLoading bool

	// Active context — set by Enter; shown with amber * in the list.
	selectedContextPath string

	// Pending cursor target — after refresh, move cursor to this path.
	pendingCursorPath string

	// Status messages shown above the footer.
	outputLines []string

	// Companion tmux pane (real terminal on the right).
	companionPaneID string

	// Right panel: launch panels, one per project (keyed by repoIdx).
	launchPanels      map[int]*embed.Model
	launchContextPath map[int]string // repoIdx → context path the panel was opened from
	launchFocusIndex  int            // -1 = main window, 0+ = index into sorted active repo indices

	// Spinner animation frame counter (incremented every tick).
	spinnerFrame int

	// Agent command from user config.
	agentCommand string
}

func NewModel(
	repoPaths []string,
	repoContexts [][]workspace.Context,
	repoConfigs []workspace.Config,
	linearIssues map[string]linear.Issue,
	linearAPIKey string,
	agentCommand string,
	companionPaneID string,
	version string,
) Model {
	repoNames := make([]string, len(repoPaths))
	for i, path := range repoPaths {
		repoNames[i] = filepath.Base(path)
	}

	repoItems := make([][]contextItem, len(repoPaths))
	for repoIdx, contexts := range repoContexts {
		items := make([]contextItem, len(contexts))
		for i, ctx := range contexts {
			items[i] = contextItem{context: ctx}
		}
		repoItems[repoIdx] = items
	}

	filterInput := textinput.New()
	filterInput.Placeholder = "ctrl+f to search..."
	filterInput.Blur()
	filterInput.Prompt = ""

	createInput := textinput.New()
	createInput.Placeholder = "context name or filter issues..."
	createInput.Prompt = ""

	m := Model{
		version:           version,
		repoPaths:         repoPaths,
		repoNames:         repoNames,
		repoConfigs:       repoConfigs,
		repoItems:         repoItems,
		linearIssues:      linearIssues,
		linearAPIKey:      linearAPIKey,
		agentCommand:      agentCommand,
		companionPaneID:   companionPaneID,
		launchPanels:      make(map[int]*embed.Model),
		launchContextPath: make(map[int]string),
		launchFocusIndex:  -1,
		pickerCursor:      -1,
		filter:            filterInput,
		createInput:       createInput,
		sortMode:          sortModeAgent,
	}
	m.rebuild()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd())
}
