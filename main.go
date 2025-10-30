package main

import (
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tmux "github.com/jubnzv/go-tmux"
)

const escDelay = 150 * time.Millisecond
const historyMax = 20

type kaleidoscopeDefaults struct {
	Provider string                    `json:"provider"`
	Models   map[string][]string       `json:"models"`
	Choices  map[string]map[string]int `json:"choices"`
}

func loadDefaults() *kaleidoscopeDefaults {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var defaults kaleidoscopeDefaults
	if err := json.Unmarshal(data, &defaults); err != nil {
		return nil
	}

	return &defaults
}

func incrementChoice(provider string, model string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")

	defaults := loadDefaults()
	if defaults == nil {
		defaults = &kaleidoscopeDefaults{
			Provider: provider,
			Models:   make(map[string][]string),
			Choices:  make(map[string]map[string]int),
		}
	}

	if defaults.Choices == nil {
		defaults.Choices = make(map[string]map[string]int)
	}

	if defaults.Choices[provider] == nil {
		defaults.Choices[provider] = make(map[string]int)
	}

	defaults.Choices[provider][model]++

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func saveDefaults(provider string, selected map[string]map[string]int) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")

	existing := loadDefaults()
	var choices map[string]map[string]int
	if existing != nil && existing.Choices != nil {
		choices = existing.Choices
	} else {
		choices = make(map[string]map[string]int)
	}

	models := make(map[string][]string)
	for prov, sel := range selected {
		var selectedModels []string
		for model, count := range sel {
			if count > 0 {
				for i := 0; i < count; i++ {
					selectedModels = append(selectedModels, model)
				}
			}
		}
		if len(selectedModels) > 0 {
			models[prov] = selectedModels
		}
	}

	defaults := kaleidoscopeDefaults{
		Provider: provider,
		Models:   models,
		Choices:  choices,
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// History helpers - persist per-repo history in tmp directory with migration
func repoHistoryFilePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	hash := sha1.Sum([]byte(abs))
	dir := filepath.Join(os.TempDir(), "kaleidoscope-history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	file := filepath.Join(dir, fmt.Sprintf("%x.json", hash))
	return file, nil
}

func loadHistoryForRepo() []string {
	path, err := repoHistoryFilePath()
	if err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var h []string
			if jsonErr := json.Unmarshal(data, &h); jsonErr == nil {
				return h
			}
		}
	}

	// Migrate from old per-repo file if present
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	oldPath := filepath.Join(cwd, ".kaleidoscope_history.json")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil
	}
	var h []string
	if jsonErr := json.Unmarshal(data, &h); jsonErr != nil {
		return nil
	}
	if newPath, e := repoHistoryFilePath(); e == nil {
		_ = os.WriteFile(newPath, data, 0644)
		_ = os.Remove(oldPath)
	}
	return h
}

func saveHistoryForRepo(h []string) error {
	path, err := repoHistoryFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// pushHistorySlice prepends a new entry (most-recent-first), dedupes immediate duplicate,
// and trims the slice to historyMax.
func pushHistorySlice(h []string, entry string) []string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return h
	}
	if len(h) > 0 && h[0] == entry {
		return h
	}
	newH := append([]string{entry}, h...)
	if len(newH) > historyMax {
		newH = newH[:historyMax]
	}
	return newH
}

// identifier composes the current folder (repo) + branch + task + first selected model
func (m model) identifier() string {
	cwd, err := os.Getwd()
	repo := ""
	if err == nil {
		repo = filepath.Base(cwd)
	}
	branch := strings.TrimSpace(m.branch)
	task := strings.TrimSpace(m.task)
	// pick first selected model for current provider
	modelName := ""
	p := m.currentProvider()
	if sel := m.selected[p]; sel != nil {
		for _, name := range m.models[p] {
			if sel[name] > 0 {
				modelName = name
				break
			}
		}
	}
	parts := []string{}
	if repo != "" {
		parts = append(parts, repo)
	}
	if branch != "" {
		parts = append(parts, branch)
	}
	if task != "" {
		parts = append(parts, task)
	}
	if modelName != "" {
		parts = append(parts, modelName)
	}
	return strings.Join(parts, "-")
}

// identifierFor composes repo + branch + task + provided model name
func (m model) identifierFor(modelName string) string {
	cwd, err := os.Getwd()
	repo := ""
	if err == nil {
		repo = filepath.Base(cwd)
	}
	branch := strings.TrimSpace(m.branch)
	task := strings.TrimSpace(m.task)
	modelName = strings.TrimSpace(modelName)
	parts := []string{}
	if repo != "" {
		parts = append(parts, repo)
	}
	if branch != "" {
		parts = append(parts, branch)
	}
	if task != "" {
		parts = append(parts, task)
	}
	if modelName != "" {
		parts = append(parts, modelName)
	}
	return strings.Join(parts, "_")
}

// focusType indicates which input is focused
type focusType int

const (
	focusBranch focusType = iota
	focusTask
	focusPrompt
	focusProvider
	focusModels
)

// screenType indicates which screen is displayed
type screenType int

const (
	screenSetup screenType = iota
	screenIteration
	screenProgress
	screenNewTask
)

// model holds state for the TUI
// - multi-line prompt with cursor
// - single-line branch name and task name
// // - provider dropdown
// - models multi-select dropdown (depends on provider) and selections
// - sizes and focus control
type model struct {
	width  int
	height int

	// Prompt (multi-line)
	input  []string
	cursor struct {
		row int
		col int
	}

	// Branch name (single line)
	branch       string
	branchCursor int

	// Task name (single line)
	task       string
	taskCursor int

	// Provider dropdown
	providers     []string
	providerIndex int
	providerOpen  bool
	providerHover int

	// Models per provider and current multi-select state
	models      map[string][]string
	selected    map[string]map[string]int // provider -> model -> count selected (>=0)
	modelsOpen  bool
	modelsHover int

	// Focus
	focus focusType

	// Screen
	screen screenType

	// Iteration screen command prompt
	iterationInput  []string
	iterationCursor struct {
		row int
		col int
	}

	// Autocomplete state
	autocompleteOptions []string
	autocompleteIndex   int
	autocompleteActive  bool

	// Run command to execute after opencode
	runCmd string

	// Track created pane IDs and worktrees
	createdPanes     []string
	createdWorktrees []string
	modelToPaneID    map[string]string
	modelToWorktree  map[string]string
	modelPrompts     map[string][]string

	// Instance metadata
	instanceProvider  map[string]string // instance label -> provider at open time
	instanceBaseModel map[string]string // instance label -> base model name

	// New task screen state
	newTaskName       string
	newTaskNameCursor int
	newTaskPrompt     []string
	newTaskCursor     struct {
		row int
		col int
	}
	newTaskFocus focusType

	// Flag to save defaults
	setDefault bool

	// Cursor blinking state
	cursorVisible bool

	// Progress screen state
	progressMsg   string
	spinnerIndex  int
	spinnerFrames []string

	// Pending ESC to detect Alt sequences
	pendingEsc bool

	// Message history (per-repo). `history` holds most-recent-first order.
	history []string
	// historyIndex is -1 when not navigating; otherwise index into history (0 = most recent)
	historyIndex int
	// iterationHistoryIndex is for the iteration prompt navigation
	iterationHistoryIndex int
	// Drafts saved when the user begins history navigation so pressing Down restores
	// their in-progress input.
	draftInput          []string
	draftIterationInput []string
}

func initialModel(runCmd string, setDefault bool) model {
	mods := map[string][]string{
		"github-copilot": {"claude-sonnet-4.5", "claude-haiku-4.5", "gpt-5-mini", "gpt-5", "gemini-2.0-flash-001", "claude-opus-4", "grok-code-fast-1", "claude-3.5-sonnet", "o3-mini", "gpt-5-codex", "gpt-4o", "gpt-4.1", "o4-mini", "claude-opus-41", "claude-3.7-sonnet", "gemini-2.5-pro", "o3", "claude-sonnet-4", "claude-3.7-sonnet-thought"},
		"OpenAI":         {"gpt-5", "gpt-5-codex", "gpt-5-mini"},
	}
	sel := map[string]map[string]int{
		"github-copilot": {},
		"OpenAI":         {},
	}

	providerIndex := 0

	defaults := loadDefaults()
	if defaults != nil {
		for i, provider := range []string{"github-copilot", "OpenAI"} {
			if provider == defaults.Provider {
				providerIndex = i
				break
			}
		}

		if models, ok := defaults.Models[defaults.Provider]; ok {
			for _, model := range models {
				if sel[defaults.Provider] == nil {
					sel[defaults.Provider] = make(map[string]int)
				}
				sel[defaults.Provider][model]++
			}
		}
	}

	initialBranch := ""
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := cmd.Output(); err == nil {
		currentBranch := strings.TrimSpace(string(out))
		standardBranches := map[string]bool{
			"main":        true,
			"master":      true,
			"dev":         true,
			"develop":     true,
			"development": true,
		}
		if !standardBranches[currentBranch] {
			initialBranch = currentBranch
		}
	}

	m := model{
		input:            []string{""},
		branch:           initialBranch,
		branchCursor:     len(initialBranch),
		task:             "",
		providers:        []string{"github-copilot", "OpenAI"},
		providerIndex:    providerIndex,
		providerOpen:     false,
		providerHover:    0,
		models:           mods,
		selected:         sel,
		modelsOpen:       false,
		modelsHover:      0,
		focus:            focusPrompt,
		screen:           screenSetup,
		iterationInput:   []string{""},
		runCmd:           runCmd,
		createdPanes:     []string{},
		createdWorktrees: []string{},
		modelToPaneID:    map[string]string{},
		modelToWorktree:  map[string]string{},
		modelPrompts:     map[string][]string{},
		newTaskPrompt:    []string{""},
		newTaskFocus:     focusTask,
		setDefault:       setDefault,
		cursorVisible:    true,
		spinnerIndex:     0,
		spinnerFrames:    []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		progressMsg:      "",
		pendingEsc:       false,
	}
	// Load per-repo history and initialize indices/drafts
	m.history = loadHistoryForRepo()
	if m.history == nil {
		m.history = []string{}
	}
	m.historyIndex = -1
	m.iterationHistoryIndex = -1
	m.draftInput = nil
	m.draftIterationInput = nil
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg { return cursorBlinkMsg{} }),
		tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return spinnerTickMsg{} }),
	)
}

func (m model) currentProvider() string {
	if len(m.providers) == 0 {
		return ""
	}
	return m.providers[m.providerIndex]
}

func (m model) providerModels() []string {
	p := m.currentProvider()
	if p == "" {
		return nil
	}
	return m.models[p]
}

// Simple ASCII word helpers
func isWordByte(b byte) bool {
	// Treat any non-whitespace byte as a word character so Option/Alt
	// word movements and Option+Delete include punctuation like ',' and '.'.
	return b != ' ' && b != '\t' && b != '\n'
}

func wordLeft(line string, col int) int {
	if col <= 0 {
		return 0
	}
	i := col
	// Move left over spaces
	for i > 0 {
		c := line[i-1]
		if c == ' ' || c == '\t' || c == '\n' {
			i--
		} else {
			break
		}
	}
	// Move left over word chars
	for i > 0 && isWordByte(line[i-1]) {
		i--
	}
	return i
}

func wordRight(line string, col int) int {
	n := len(line)
	if col >= n {
		return n
	}
	i := col
	// If currently on a space, skip spaces
	for i < n {
		c := line[i]
		if c == ' ' || c == '\t' || c == '\n' {
			i++
		} else {
			break
		}
	}
	// If currently at a word, skip the word
	for i < n && isWordByte(line[i]) {
		i++
	}
	return i
}

func moveWordLeftLines(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	if col > 0 {
		return row, wordLeft(lines[row], col)
	}
	if row > 0 {
		row--
		return row, wordLeft(lines[row], len(lines[row]))
	}
	return row, col
}

func moveWordRightLines(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	line := lines[row]
	if col < len(line) {
		return row, wordRight(line, col)
	}
	if row < len(lines)-1 {
		row++
		return row, wordRight(lines[row], 0)
	}
	return row, col
}

// Line navigation helpers: jump to start/end of line,
// and traverse to previous/next line when already at boundary.
func lineLeft(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	if col > 0 {
		return row, 0
	}
	if row > 0 {
		return row - 1, 0
	}
	return row, col
}

func lineRight(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	lineLen := len(lines[row])
	if col < lineLen {
		return row, lineLen
	}
	if row < len(lines)-1 {
		row++
		return row, len(lines[row])
	}
	return row, col
}

func deleteWordBackward(line string, col int) (newLine string, newCol int) {
	if col <= 0 {
		return line, col
	}
	newCol = wordLeft(line, col)
	newLine = line[:newCol] + line[col:]
	return newLine, newCol
}

func deleteLineBackward(line string, col int) (newLine string, newCol int) {
	if col <= 0 {
		return line, col
	}
	newLine = line[col:]
	return newLine, 0
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cursorBlinkMsg:
		m.cursorVisible = !m.cursorVisible
		return m, tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
			return cursorBlinkMsg{}
		})
	case spinnerTickMsg:
		if len(m.spinnerFrames) > 0 {
			m.spinnerIndex = (m.spinnerIndex + 1) % len(m.spinnerFrames)
		}
		return m, tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return spinnerTickMsg{} })
	case bailCompleteMsg:
		return m, tea.Quit
	case nextCompleteMsg:
		// Clear iteration prompt and related state so it's empty next view
		m.iterationInput = []string{""}
		m.iterationCursor.row = 0
		m.iterationCursor.col = 0
		m.iterationHistoryIndex = -1
		m.draftIterationInput = nil
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		m.screen = screenNewTask
		m.newTaskFocus = focusTask
		return m, nil
	case wrapCompleteMsg:
		return m, tea.Quit
	case cleanupCompleteMsg:
		return m, tea.Quit
	case panesOpenedMsg:
		if msg.err == nil && msg.count > 0 {
			m.screen = screenIteration
			m.createdPanes = append(m.createdPanes, msg.paneIDs...)
			m.createdWorktrees = append(m.createdWorktrees, msg.worktrees...)
			initialPrompt := strings.TrimSpace(strings.Join(m.input, "\n"))
			// Push to history and persist
			m.history = pushHistorySlice(m.history, initialPrompt)
			_ = saveHistoryForRepo(m.history)
			for i, instanceLabel := range msg.modelNames {
				m.modelToPaneID[instanceLabel] = msg.paneIDs[i]
				m.modelToWorktree[instanceLabel] = msg.worktrees[i]
				m.modelPrompts[instanceLabel] = []string{initialPrompt}
				if m.instanceProvider == nil {
					m.instanceProvider = make(map[string]string)
				}
				if m.instanceBaseModel == nil {
					m.instanceBaseModel = make(map[string]string)
				}
				if i < len(msg.providers) {
					m.instanceProvider[instanceLabel] = msg.providers[i]
				}
				if i < len(msg.baseModels) {
					m.instanceBaseModel[instanceLabel] = msg.baseModels[i]
				}
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case escTimeoutMsg:
		if m.pendingEsc {
			m.pendingEsc = false
			return m, cleanupCmd(m)
		}
		return m, nil
	case tea.KeyMsg:
		// If we're in iteration or new-task screens, delegate
		if m.screen == screenIteration {
			return m.updateIteration(msg)
		}
		if m.screen == screenNewTask {
			return m.updateNewTask(msg)
		}

		// Handle Alt-b / Alt-f or ESC+b / ESC+f before anything else
		if (msg.Alt && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) || (m.pendingEsc && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) {
			m.pendingEsc = false
			if m.focus == focusBranch {
				if msg.Runes[0] == 'b' {
					m.branchCursor = wordLeft(m.branch, m.branchCursor)
				} else {
					m.branchCursor = wordRight(m.branch, m.branchCursor)
				}
				return m, nil
			}
			if m.focus == focusTask {
				if msg.Runes[0] == 'b' {
					m.taskCursor = wordLeft(m.task, m.taskCursor)
				} else {
					m.taskCursor = wordRight(m.task, m.taskCursor)
				}
				return m, nil
			}
			if m.focus == focusPrompt {
				if msg.Runes[0] == 'b' {
					m.cursor.row, m.cursor.col = moveWordLeftLines(m.input, m.cursor.row, m.cursor.col)
				} else {
					m.cursor.row, m.cursor.col = moveWordRightLines(m.input, m.cursor.row, m.cursor.col)
				}
				return m, nil
			}
			// If on provider/models, ignore
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, cleanupCmd(m)
		case tea.KeyEsc:
			// Start ESC timer to detect meta sequences
			m.pendingEsc = true
			return m, tea.Tick(escDelay, func(t time.Time) tea.Msg { return escTimeoutMsg{} })
		case tea.KeyCtrlA, tea.KeyHome:
			// Cmd-like: jump to start of line; if already at start, go to previous line start
			if m.focus == focusBranch {
				m.branchCursor = 0
				return m, nil
			}
			if m.focus == focusTask {
				m.taskCursor = 0
				return m, nil
			}
			if m.focus == focusPrompt {
				m.cursor.row, m.cursor.col = lineLeft(m.input, m.cursor.row, m.cursor.col)
				return m, nil
			}
			return m, nil
		case tea.KeyCtrlE, tea.KeyEnd:
			// Cmd-like: jump to end of line; if already at end, go to next line end
			if m.focus == focusBranch {
				m.branchCursor = len(m.branch)
				return m, nil
			}
			if m.focus == focusTask {
				m.taskCursor = len(m.task)
				return m, nil
			}
			if m.focus == focusPrompt {
				m.cursor.row, m.cursor.col = lineRight(m.input, m.cursor.row, m.cursor.col)
				return m, nil
			}
			return m, nil
		case tea.KeyTab, tea.KeyShiftTab:
			// Cycle focus among branch -> task -> prompt -> provider -> models -> branch
			switch m.focus {
			case focusBranch:
				m.focus = focusTask
			case focusTask:
				m.focus = focusPrompt
			case focusPrompt:
				m.focus = focusProvider
				m.providerHover = m.providerIndex
			case focusProvider:
				m.providerOpen = false
				m.focus = focusModels
				m.modelsHover = 0
			case focusModels:
				m.modelsOpen = false
				m.focus = focusBranch
			}
			return m, nil
		case tea.KeyEnter:
			if m.focus == focusBranch || m.focus == focusTask {
				m.focus = focusPrompt
				return m, nil
			}
			if m.focus == focusProvider {
				if m.providerOpen {
					m.providerIndex = m.providerHover
					m.providerOpen = false
					// Reset models hover to valid range for new provider
					m.modelsHover = 0
				} else {
					m.providerOpen = true
					m.providerHover = m.providerIndex
				}
				return m, nil
			}
			if m.focus == focusModels {
				// Enter toggles open/close (selection via Space)
				m.modelsOpen = !m.modelsOpen
				if m.modelsOpen {
					m.modelsHover = 0
				}
				return m, nil
			}
			// Insert newline in prompt
			before := m.input[m.cursor.row][:m.cursor.col]
			after := m.input[m.cursor.row][m.cursor.col:]
			m.input[m.cursor.row] = before
			m.input = append(m.input[:m.cursor.row+1], append([]string{after}, m.input[m.cursor.row+1:]...)...)
			m.cursor.row++
			m.cursor.col = 0

			// Also, spawn a tmux pane per selected model
			if m.focus == focusPrompt {
				models := m.selectedModels()
				if len(models) > 0 {
					return m, openPanesCmd(models, m)
				}
			}

		case tea.KeySpace:
			// Space increments selection count when in models multiselect and open.
			if m.focus == focusModels && m.modelsOpen {
				opts := m.providerModels()
				if len(opts) == 0 {
					return m, nil
				}
				if m.modelsHover < 0 {
					m.modelsHover = 0
				}
				if m.modelsHover >= len(opts) {
					m.modelsHover = len(opts) - 1
				}
				p := m.currentProvider()
				if m.selected[p] == nil {
					m.selected[p] = map[string]int{}
				}
				name := opts[m.modelsHover]
				m.selected[p][name] = m.selected[p][name] + 1
				return m, nil
			}
			// Otherwise, treat space as text input in focused text fields.
			if m.focus == focusBranch {
				m.branch = m.branch[:m.branchCursor] + " " + m.branch[m.branchCursor:]
				m.branchCursor++
				return m, nil
			}
			if m.focus == focusTask {
				m.task = m.task[:m.taskCursor] + " " + m.task[m.taskCursor:]
				m.taskCursor++
				return m, nil
			}
			if m.focus == focusPrompt {
				line := m.input[m.cursor.row]
				m.input[m.cursor.row] = line[:m.cursor.col] + " " + line[m.cursor.col:]
				m.cursor.col++
				return m, nil
			}
		case tea.KeyBackspace:
			if msg.Alt {
				// OPTION+delete: delete word backward
				if m.focus == focusBranch {
					m.branch, m.branchCursor = deleteWordBackward(m.branch, m.branchCursor)
					return m, nil
				}
				if m.focus == focusTask {
					m.task, m.taskCursor = deleteWordBackward(m.task, m.taskCursor)
					return m, nil
				}
				if m.focus == focusPrompt {
					line := m.input[m.cursor.row]
					m.input[m.cursor.row], m.cursor.col = deleteWordBackward(line, m.cursor.col)
					return m, nil
				}
				return m, nil
			}
			// CMD+delete on macOS is handled via KeyCtrlU (Ctrl-U typically deletes line backward)
			if m.focus == focusBranch {
				if m.branchCursor > 0 && len(m.branch) > 0 {
					m.branch = m.branch[:m.branchCursor-1] + m.branch[m.branchCursor:]
					m.branchCursor--
				}
				return m, nil
			}
			if m.focus == focusTask {
				if m.taskCursor > 0 && len(m.task) > 0 {
					m.task = m.task[:m.taskCursor-1] + m.task[m.taskCursor:]
					m.taskCursor--
				}
				return m, nil
			}
			if m.focus == focusProvider {
				if m.providerOpen {
					m.providerOpen = false
				}
				return m, nil
			}
			if m.focus == focusModels {
				// When the models dropdown is open, Backspace decrements the hovered model count.
				if m.modelsOpen {
					opts := m.providerModels()
					if len(opts) == 0 {
						return m, nil
					}
					if m.modelsHover < 0 {
						m.modelsHover = 0
					}
					if m.modelsHover >= len(opts) {
						m.modelsHover = len(opts) - 1
					}
					p := m.currentProvider()
					if m.selected[p] == nil {
						m.selected[p] = map[string]int{}
					}
					name := opts[m.modelsHover]
					if m.selected[p][name] > 0 {
						m.selected[p][name] = m.selected[p][name] - 1
					}
					return m, nil
				}
				return m, nil
			}
			// Prompt backspace
			if m.cursor.col > 0 {
				line := m.input[m.cursor.row]
				m.input[m.cursor.row] = line[:m.cursor.col-1] + line[m.cursor.col:]
				m.cursor.col--
			} else if m.cursor.row > 0 {
				prev := m.input[m.cursor.row-1]
				cur := m.input[m.cursor.row]
				m.input[m.cursor.row-1] = prev + cur
				m.input = append(m.input[:m.cursor.row], m.input[m.cursor.row+1:]...)
				m.cursor.row--
				m.cursor.col = len(prev)
			}
		case tea.KeyCtrlU:
			// CMD+delete: delete line backward (Ctrl-U is standard terminal binding)
			if m.focus == focusBranch {
				m.branch, m.branchCursor = deleteLineBackward(m.branch, m.branchCursor)
				return m, nil
			}
			if m.focus == focusTask {
				m.task, m.taskCursor = deleteLineBackward(m.task, m.taskCursor)
				return m, nil
			}
			if m.focus == focusPrompt {
				line := m.input[m.cursor.row]
				m.input[m.cursor.row], m.cursor.col = deleteLineBackward(line, m.cursor.col)
				return m, nil
			}
			return m, nil
		case tea.KeyLeft:
			if m.focus == focusBranch {
				if m.branchCursor > 0 {
					m.branchCursor--
				}
				return m, nil
			}
			if m.focus == focusTask {
				if m.taskCursor > 0 {
					m.taskCursor--
				}
				return m, nil
			}
			// no left/right in provider/models lists; fall through to prompt
			if m.cursor.col > 0 {
				m.cursor.col--
			} else if m.cursor.row > 0 {
				m.cursor.row--
				m.cursor.col = len(m.input[m.cursor.row])
			}
		case tea.KeyRight:
			if m.focus == focusBranch {
				if m.branchCursor < len(m.branch) {
					m.branchCursor++
				}
				return m, nil
			}
			if m.focus == focusTask {
				if m.taskCursor < len(m.task) {
					m.taskCursor++
				}
				return m, nil
			}
			line := m.input[m.cursor.row]
			if m.cursor.col < len(line) {
				m.cursor.col++
			} else if m.cursor.row < len(m.input)-1 {
				m.cursor.row++
				m.cursor.col = 0
			}
		case tea.KeyUp:
			if m.focus == focusPrompt {
				// History navigation: on first Up, save draft and load most recent
				if len(m.history) > 0 {
					if m.historyIndex == -1 {
						m.draftInput = append([]string{}, m.input...)
						m.historyIndex = 0
						entry := m.history[m.historyIndex]
						m.input = strings.Split(entry, "\n")
						m.cursor.row = len(m.input) - 1
						m.cursor.col = len(m.input[m.cursor.row])
					} else if m.historyIndex < len(m.history)-1 {
						m.historyIndex++
						entry := m.history[m.historyIndex]
						m.input = strings.Split(entry, "\n")
						m.cursor.row = len(m.input) - 1
						m.cursor.col = len(m.input[m.cursor.row])
					}
				} else if m.cursor.row > 0 {
					m.cursor.row--
					if m.cursor.col > len(m.input[m.cursor.row]) {
						m.cursor.col = len(m.input[m.cursor.row])
					}
				}
			} else if m.focus == focusProvider {
				if !m.providerOpen {
					m.providerOpen = true
					m.providerHover = m.providerIndex
				} else if m.providerHover > 0 {
					m.providerHover--
				}
			} else if m.focus == focusModels {
				if !m.modelsOpen {
					m.modelsOpen = true
					m.modelsHover = 0
				} else if m.modelsHover > 0 {
					m.modelsHover--
				}
			}
		case tea.KeyDown:
			if m.focus == focusPrompt {
				// If navigating history, move younger; when exiting, restore draft
				if m.historyIndex != -1 {
					if m.historyIndex > 0 {
						m.historyIndex--
						entry := m.history[m.historyIndex]
						m.input = strings.Split(entry, "\n")
						m.cursor.row = len(m.input) - 1
						m.cursor.col = len(m.input[m.cursor.row])
					} else {
						// historyIndex == 0 -> restore draft
						m.historyIndex = -1
						if m.draftInput != nil {
							m.input = append([]string{}, m.draftInput...)
						} else {
							m.input = []string{""}
						}
						m.cursor.row = len(m.input) - 1
						m.cursor.col = len(m.input[m.cursor.row])
					}
				} else if m.cursor.row < len(m.input)-1 {
					m.cursor.row++
					if m.cursor.col > len(m.input[m.cursor.row]) {
						m.cursor.col = len(m.input[m.cursor.row])
					}
				}
			} else if m.focus == focusProvider {
				if !m.providerOpen {
					m.providerOpen = true
					m.providerHover = m.providerIndex
				} else if m.providerHover < len(m.providers)-1 {
					m.providerHover++
				}
			} else if m.focus == focusModels {
				opts := m.providerModels()
				if !m.modelsOpen {
					m.modelsOpen = true
					m.modelsHover = 0
				} else if len(opts) > 0 && m.modelsHover < len(opts)-1 {
					m.modelsHover++
				}
			}
		default:
			if len(msg.Runes) > 0 {
				// Any other key cancels a pending ESC (we treat it as just ESC prefix)
				if m.pendingEsc {
					m.pendingEsc = false
				}
				r := string(msg.Runes)
				if m.focus == focusBranch {
					m.branch = m.branch[:m.branchCursor] + r + m.branch[m.branchCursor:]
					m.branchCursor += len(r)
					return m, nil
				}
				if m.focus == focusTask {
					m.task = m.task[:m.taskCursor] + r + m.task[m.taskCursor:]
					m.taskCursor += len(r)
					return m, nil
				}
				if m.focus == focusProvider || m.focus == focusModels {
					// ignore text input for dropdowns
					return m, nil
				}
				line := m.input[m.cursor.row]
				m.input[m.cursor.row] = line[:m.cursor.col] + r + line[m.cursor.col:]
				m.cursor.col += len(r)
			}
		}
	}
	return m, nil
}

func (m model) updateIteration(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, cleanupCmd(m)
	case tea.KeyEsc:
		m.pendingEsc = true
		return m, tea.Tick(escDelay, func(t time.Time) tea.Msg { return escTimeoutMsg{} })
	case tea.KeyCtrlA, tea.KeyHome:
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		m.iterationCursor.row, m.iterationCursor.col = lineLeft(m.iterationInput, m.iterationCursor.row, m.iterationCursor.col)
		return m, nil
	case tea.KeyCtrlE, tea.KeyEnd:
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		m.iterationCursor.row, m.iterationCursor.col = lineRight(m.iterationInput, m.iterationCursor.row, m.iterationCursor.col)
		return m, nil
	case tea.KeyTab:
		if m.autocompleteActive && len(m.autocompleteOptions) > 0 {
			m.autocompleteIndex = (m.autocompleteIndex + 1) % len(m.autocompleteOptions)
		} else {
			line := m.iterationInput[m.iterationCursor.row]
			prefix, _ := m.getAutocompletePrefix(line, m.iterationCursor.col)
			if prefix != "" {
				m.autocompleteOptions = m.getAutocompleteOptions(prefix)
				if len(m.autocompleteOptions) > 0 {
					m.autocompleteActive = true
					m.autocompleteIndex = 0
				}
			}
		}
	case tea.KeyEnter:
		if m.autocompleteActive && len(m.autocompleteOptions) > 0 {
			line := m.iterationInput[m.iterationCursor.row]
			prefix, start := m.getAutocompletePrefix(line, m.iterationCursor.col)
			if prefix != "" {
				completion := m.autocompleteOptions[m.autocompleteIndex]
				newLine := line[:start] + completion + line[m.iterationCursor.col:]
				m.iterationInput[m.iterationCursor.row] = newLine
				m.iterationCursor.col = start + len(completion)
			}
			m.autocompleteActive = false
			m.autocompleteOptions = nil
		} else {
			currentLine := strings.TrimSpace(strings.Join(m.iterationInput, "\n"))
			if currentLine == "/bail" {
				m.screen = screenProgress
				m.progressMsg = "Cleaning up panes, worktrees, and branches..."
				return m, bailCmd(m)
			}

			if strings.HasPrefix(currentLine, "/next ") {
				modelName := strings.TrimSpace(strings.TrimPrefix(currentLine, "/next "))
				if modelName != "" {
					m.screen = screenProgress
					m.progressMsg = fmt.Sprintf("Merging and pushing changes from %s...", modelName)
					return m, nextCmd(m, modelName)
				}
			}

			if strings.HasPrefix(currentLine, "/wrap ") {
				modelName := strings.TrimSpace(strings.TrimPrefix(currentLine, "/wrap "))
				if modelName != "" {
					m.screen = screenProgress
					m.progressMsg = fmt.Sprintf("Merging and pushing changes from %s...", modelName)
					return m, wrapCmd(m, modelName)
				}
			}

			if strings.HasPrefix(currentLine, "@") {
				parts := strings.SplitN(currentLine, " ", 2)
				if len(parts) == 2 {
					modelName := strings.TrimPrefix(parts[0], "@")
					prompt := parts[1]
					if paneID, ok := m.modelToPaneID[modelName]; ok {
						m.modelPrompts[modelName] = append(m.modelPrompts[modelName], prompt)
						// Push to per-repo history and persist
						m.history = pushHistorySlice(m.history, prompt)
						_ = saveHistoryForRepo(m.history)
						m.iterationInput = []string{""}
						m.iterationCursor.row = 0
						m.iterationCursor.col = 0
						return m, sendToModelPaneCmd(paneID, modelName, prompt, m)
					}
				}
			}

			before := m.iterationInput[m.iterationCursor.row][:m.iterationCursor.col]
			after := m.iterationInput[m.iterationCursor.row][m.iterationCursor.col:]
			m.iterationInput[m.iterationCursor.row] = before
			m.iterationInput = append(m.iterationInput[:m.iterationCursor.row+1], append([]string{after}, m.iterationInput[m.iterationCursor.row+1:]...)...)
			m.iterationCursor.row++
			m.iterationCursor.col = 0
		}
	case tea.KeyBackspace:
		if msg.Alt {
			// OPTION+delete: delete word backward
			m.autocompleteActive = false
			m.autocompleteOptions = nil
			line := m.iterationInput[m.iterationCursor.row]
			m.iterationInput[m.iterationCursor.row], m.iterationCursor.col = deleteWordBackward(line, m.iterationCursor.col)
			return m, nil
		}
		if m.iterationCursor.col > 0 {
			line := m.iterationInput[m.iterationCursor.row]
			m.iterationInput[m.iterationCursor.row] = line[:m.iterationCursor.col-1] + line[m.iterationCursor.col:]
			m.iterationCursor.col--

			line = m.iterationInput[m.iterationCursor.row]
			prefix, _ := m.getAutocompletePrefix(line, m.iterationCursor.col)
			if prefix != "" && (prefix[0] == '/' || prefix[0] == '@') {
				m.autocompleteOptions = m.getAutocompleteOptions(prefix)
				if len(m.autocompleteOptions) > 0 {
					if len(m.autocompleteOptions) == 1 && m.autocompleteOptions[0] == prefix {
						m.autocompleteActive = false
						m.autocompleteOptions = nil
					} else {
						m.autocompleteActive = true
						m.autocompleteIndex = 0
					}
				} else {
					m.autocompleteActive = false
				}
			} else {
				m.autocompleteActive = false
				m.autocompleteOptions = nil
			}
		} else if m.iterationCursor.row > 0 {
			m.autocompleteActive = false
			m.autocompleteOptions = nil
			prev := m.iterationInput[m.iterationCursor.row-1]
			cur := m.iterationInput[m.iterationCursor.row]
			m.iterationInput[m.iterationCursor.row-1] = prev + cur
			m.iterationInput = append(m.iterationInput[:m.iterationCursor.row], m.iterationInput[m.iterationCursor.row+1:]...)
			m.iterationCursor.row--
			m.iterationCursor.col = len(prev)
		}
	case tea.KeyCtrlU:
		// CMD+delete: delete line backward
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		line := m.iterationInput[m.iterationCursor.row]
		m.iterationInput[m.iterationCursor.row], m.iterationCursor.col = deleteLineBackward(line, m.iterationCursor.col)
		return m, nil
	case tea.KeyLeft:
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		if m.iterationCursor.col > 0 {
			m.iterationCursor.col--
		} else if m.iterationCursor.row > 0 {
			m.iterationCursor.row--
			m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
		}
	case tea.KeyRight:
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		line := m.iterationInput[m.iterationCursor.row]
		if m.iterationCursor.col < len(line) {
			m.iterationCursor.col++
		} else if m.iterationCursor.row < len(m.iterationInput)-1 {
			m.iterationCursor.row++
			m.iterationCursor.col = 0
		}
	case tea.KeyUp:
		if m.autocompleteActive && len(m.autocompleteOptions) > 0 {
			m.autocompleteIndex--
			if m.autocompleteIndex < 0 {
				m.autocompleteIndex = len(m.autocompleteOptions) - 1
			}
		} else {
			// Iteration prompt history navigation: on first Up, save draft and load most recent
			if len(m.history) > 0 {
				if m.iterationHistoryIndex == -1 {
					m.draftIterationInput = append([]string{}, m.iterationInput...)
					m.iterationHistoryIndex = 0
					entry := m.history[m.iterationHistoryIndex]
					m.iterationInput = strings.Split(entry, "\n")
					m.iterationCursor.row = len(m.iterationInput) - 1
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				} else if m.iterationHistoryIndex < len(m.history)-1 {
					m.iterationHistoryIndex++
					entry := m.history[m.iterationHistoryIndex]
					m.iterationInput = strings.Split(entry, "\n")
					m.iterationCursor.row = len(m.iterationInput) - 1
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				} else if m.iterationCursor.row > 0 {
					m.iterationCursor.row--
					if m.iterationCursor.col > len(m.iterationInput[m.iterationCursor.row]) {
						m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
					}
				}
			} else if m.iterationCursor.row > 0 {
				m.iterationCursor.row--
				if m.iterationCursor.col > len(m.iterationInput[m.iterationCursor.row]) {
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				}
			}
		}
	case tea.KeyDown:
		if m.autocompleteActive && len(m.autocompleteOptions) > 0 {
			m.autocompleteIndex = (m.autocompleteIndex + 1) % len(m.autocompleteOptions)
		} else {
			// Iteration prompt history down: move toward newer entries; restore draft when exiting
			if m.iterationHistoryIndex != -1 {
				if m.iterationHistoryIndex > 0 {
					m.iterationHistoryIndex--
					entry := m.history[m.iterationHistoryIndex]
					m.iterationInput = strings.Split(entry, "\n")
					m.iterationCursor.row = len(m.iterationInput) - 1
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				} else {
					m.iterationHistoryIndex = -1
					if m.draftIterationInput != nil {
						m.iterationInput = append([]string{}, m.draftIterationInput...)
					} else {
						m.iterationInput = []string{""}
					}
					m.iterationCursor.row = len(m.iterationInput) - 1
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				}
			} else if m.iterationCursor.row < len(m.iterationInput)-1 {
				m.iterationCursor.row++
				if m.iterationCursor.col > len(m.iterationInput[m.iterationCursor.row]) {
					m.iterationCursor.col = len(m.iterationInput[m.iterationCursor.row])
				}
			}
		}
	case tea.KeySpace:
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		line := m.iterationInput[m.iterationCursor.row]
		m.iterationInput[m.iterationCursor.row] = line[:m.iterationCursor.col] + " " + line[m.iterationCursor.col:]
		m.iterationCursor.col++
	default:
		// Handle Alt-b / Alt-f or ESC+b / ESC+f for iteration input
		if (msg.Alt && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) || (m.pendingEsc && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) {
			m.pendingEsc = false
			m.autocompleteActive = false
			m.autocompleteOptions = nil
			if msg.Runes[0] == 'b' {
				m.iterationCursor.row, m.iterationCursor.col = moveWordLeftLines(m.iterationInput, m.iterationCursor.row, m.iterationCursor.col)
			} else {
				m.iterationCursor.row, m.iterationCursor.col = moveWordRightLines(m.iterationInput, m.iterationCursor.row, m.iterationCursor.col)
			}
			return m, nil
		}

		if len(msg.Runes) > 0 {
			r := string(msg.Runes)
			line := m.iterationInput[m.iterationCursor.row]
			m.iterationInput[m.iterationCursor.row] = line[:m.iterationCursor.col] + r + line[m.iterationCursor.col:]
			m.iterationCursor.col += len(r)

			if r == "/" || r == "@" {
				m.autocompleteOptions = m.getAutocompleteOptions(r)
				if len(m.autocompleteOptions) > 0 {
					m.autocompleteActive = true
					m.autocompleteIndex = 0
				}
			} else {
				line = m.iterationInput[m.iterationCursor.row]
				prefix, _ := m.getAutocompletePrefix(line, m.iterationCursor.col)
				if prefix != "" && (prefix[0] == '/' || prefix[0] == '@') {
					m.autocompleteOptions = m.getAutocompleteOptions(prefix)
					if len(m.autocompleteOptions) > 0 {
						if len(m.autocompleteOptions) == 1 && m.autocompleteOptions[0] == prefix {
							m.autocompleteActive = false
							m.autocompleteOptions = nil
						} else {
							m.autocompleteActive = true
							m.autocompleteIndex = 0
						}
					} else {
						m.autocompleteActive = false
					}
				} else {
					m.autocompleteActive = false
					m.autocompleteOptions = nil
				}
			}
		}
	}
	return m, nil
}

func (m model) updateNewTask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, cleanupCmd(m)
	case tea.KeyEsc:
		m.pendingEsc = true
		return m, tea.Tick(escDelay, func(t time.Time) tea.Msg { return escTimeoutMsg{} })
	case tea.KeyCtrlA, tea.KeyHome:
		if m.newTaskFocus == focusTask {
			m.newTaskNameCursor = 0
			return m, nil
		}
		m.newTaskCursor.row, m.newTaskCursor.col = lineLeft(m.newTaskPrompt, m.newTaskCursor.row, m.newTaskCursor.col)
		return m, nil
	case tea.KeyCtrlE, tea.KeyEnd:
		if m.newTaskFocus == focusTask {
			m.newTaskNameCursor = len(m.newTaskName)
			return m, nil
		}
		m.newTaskCursor.row, m.newTaskCursor.col = lineRight(m.newTaskPrompt, m.newTaskCursor.row, m.newTaskCursor.col)
		return m, nil
	case tea.KeyTab:
		if m.newTaskFocus == focusTask {
			m.newTaskFocus = focusPrompt
		} else {
			m.newTaskFocus = focusTask
		}
		return m, nil
	case tea.KeyEnter:
		if m.newTaskFocus == focusTask {
			m.newTaskFocus = focusPrompt
			return m, nil
		}

		currentPrompt := strings.TrimSpace(strings.Join(m.newTaskPrompt, "\n"))
		if currentPrompt != "" {
			models := m.selectedModels()
			if len(models) > 0 {
				m.task = m.newTaskName
				m.input = m.newTaskPrompt
				m.newTaskName = ""
				m.newTaskNameCursor = 0
				m.newTaskPrompt = []string{""}
				m.newTaskCursor.row = 0
				m.newTaskCursor.col = 0
				return m, openPanesCmd(models, m)
			}
		}

		before := m.newTaskPrompt[m.newTaskCursor.row][:m.newTaskCursor.col]
		after := m.newTaskPrompt[m.newTaskCursor.row][m.newTaskCursor.col:]
		m.newTaskPrompt[m.newTaskCursor.row] = before
		m.newTaskPrompt = append(m.newTaskPrompt[:m.newTaskCursor.row+1], append([]string{after}, m.newTaskPrompt[m.newTaskCursor.row+1:]...)...)
		m.newTaskCursor.row++
		m.newTaskCursor.col = 0
		return m, nil
	case tea.KeyBackspace:
		if msg.Alt {
			// OPTION+delete: delete word backward
			if m.newTaskFocus == focusTask {
				m.newTaskName, m.newTaskNameCursor = deleteWordBackward(m.newTaskName, m.newTaskNameCursor)
				return m, nil
			}
			line := m.newTaskPrompt[m.newTaskCursor.row]
			m.newTaskPrompt[m.newTaskCursor.row], m.newTaskCursor.col = deleteWordBackward(line, m.newTaskCursor.col)
			return m, nil
		}
		if m.newTaskFocus == focusTask {
			if m.newTaskNameCursor > 0 && len(m.newTaskName) > 0 {
				m.newTaskName = m.newTaskName[:m.newTaskNameCursor-1] + m.newTaskName[m.newTaskNameCursor:]
				m.newTaskNameCursor--
			}
			return m, nil
		}
		if m.newTaskCursor.col > 0 {
			line := m.newTaskPrompt[m.newTaskCursor.row]
			m.newTaskPrompt[m.newTaskCursor.row] = line[:m.newTaskCursor.col-1] + line[m.newTaskCursor.col:]
			m.newTaskCursor.col--
		} else if m.newTaskCursor.row > 0 {
			prev := m.newTaskPrompt[m.newTaskCursor.row-1]
			cur := m.newTaskPrompt[m.newTaskCursor.row]
			m.newTaskPrompt[m.newTaskCursor.row-1] = prev + cur
			m.newTaskPrompt = append(m.newTaskPrompt[:m.newTaskCursor.row], m.newTaskPrompt[m.newTaskCursor.row+1:]...)
			m.newTaskCursor.row--
			m.newTaskCursor.col = len(prev)
		}
		return m, nil
	case tea.KeyCtrlU:
		// CMD+delete: delete line backward
		if m.newTaskFocus == focusTask {
			m.newTaskName, m.newTaskNameCursor = deleteLineBackward(m.newTaskName, m.newTaskNameCursor)
			return m, nil
		}
		line := m.newTaskPrompt[m.newTaskCursor.row]
		m.newTaskPrompt[m.newTaskCursor.row], m.newTaskCursor.col = deleteLineBackward(line, m.newTaskCursor.col)
		return m, nil
	case tea.KeyLeft:
		if m.newTaskFocus == focusTask {
			if m.newTaskNameCursor > 0 {
				m.newTaskNameCursor--
			}
			return m, nil
		}
		if m.newTaskCursor.col > 0 {
			m.newTaskCursor.col--
		} else if m.newTaskCursor.row > 0 {
			m.newTaskCursor.row--
			m.newTaskCursor.col = len(m.newTaskPrompt[m.newTaskCursor.row])
		}
		return m, nil
	case tea.KeyRight:
		if m.newTaskFocus == focusTask {
			if m.newTaskNameCursor < len(m.newTaskName) {
				m.newTaskNameCursor++
			}
			return m, nil
		}
		line := m.newTaskPrompt[m.newTaskCursor.row]
		if m.newTaskCursor.col < len(line) {
			m.newTaskCursor.col++
		} else if m.newTaskCursor.row < len(m.newTaskPrompt)-1 {
			m.newTaskCursor.row++
			m.newTaskCursor.col = 0
		}
		return m, nil
	case tea.KeyUp:
		if m.newTaskFocus == focusPrompt && m.newTaskCursor.row > 0 {
			m.newTaskCursor.row--
			if m.newTaskCursor.col > len(m.newTaskPrompt[m.newTaskCursor.row]) {
				m.newTaskCursor.col = len(m.newTaskPrompt[m.newTaskCursor.row])
			}
		}
		return m, nil
	case tea.KeyDown:
		if m.newTaskFocus == focusPrompt && m.newTaskCursor.row < len(m.newTaskPrompt)-1 {
			m.newTaskCursor.row++
			if m.newTaskCursor.col > len(m.newTaskPrompt[m.newTaskCursor.row]) {
				m.newTaskCursor.col = len(m.newTaskPrompt[m.newTaskCursor.row])
			}
		}
		return m, nil
	case tea.KeySpace:
		if m.newTaskFocus == focusTask {
			m.newTaskName = m.newTaskName[:m.newTaskNameCursor] + " " + m.newTaskName[m.newTaskNameCursor:]
			m.newTaskNameCursor++
			return m, nil
		}
		line := m.newTaskPrompt[m.newTaskCursor.row]
		m.newTaskPrompt[m.newTaskCursor.row] = line[:m.newTaskCursor.col] + " " + line[m.newTaskCursor.col:]
		m.newTaskCursor.col++
		return m, nil
	default:
		// Handle Alt-b / Alt-f or ESC+b / ESC+f in new task inputs
		if (msg.Alt && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) || (m.pendingEsc && len(msg.Runes) == 1 && (msg.Runes[0] == 'b' || msg.Runes[0] == 'f')) {
			m.pendingEsc = false
			if m.newTaskFocus == focusTask {
				if msg.Runes[0] == 'b' {
					m.newTaskNameCursor = wordLeft(m.newTaskName, m.newTaskNameCursor)
				} else {
					m.newTaskNameCursor = wordRight(m.newTaskName, m.newTaskNameCursor)
				}
				return m, nil
			}
			if m.newTaskFocus == focusPrompt {
				if msg.Runes[0] == 'b' {
					m.newTaskCursor.row, m.newTaskCursor.col = moveWordLeftLines(m.newTaskPrompt, m.newTaskCursor.row, m.newTaskCursor.col)
				} else {
					m.newTaskCursor.row, m.newTaskCursor.col = moveWordRightLines(m.newTaskPrompt, m.newTaskCursor.row, m.newTaskCursor.col)
				}
				return m, nil
			}
			return m, nil
		}

		if len(msg.Runes) > 0 {
			r := string(msg.Runes)
			if m.newTaskFocus == focusTask {
				m.newTaskName = m.newTaskName[:m.newTaskNameCursor] + r + m.newTaskName[m.newTaskNameCursor:]
				m.newTaskNameCursor += len(r)
				return m, nil
			}
			line := m.newTaskPrompt[m.newTaskCursor.row]
			m.newTaskPrompt[m.newTaskCursor.row] = line[:m.newTaskCursor.col] + r + line[m.newTaskCursor.col:]
			m.newTaskCursor.col += len(r)
		}
		return m, nil
	}
}

type escTimeoutMsg struct{}

type panesOpenedMsg struct {
	count      int
	err        error
	paneIDs    []string
	worktrees  []string
	modelNames []string // instance labels used as keys
	providers  []string // provider used to open each instance
	baseModels []string // base model name for each instance
}

type bailCompleteMsg struct{}

type nextCompleteMsg struct{}

type wrapCompleteMsg struct{}

type cleanupCompleteMsg struct{}

type cursorBlinkMsg struct{}

type spinnerTickMsg struct{}

func openPanesCmd(models []string, m model) tea.Cmd {
	return func() tea.Msg {
		if m.setDefault {
			if err := saveDefaults(m.currentProvider(), m.selected); err != nil {
				tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: failed to save defaults: %s", err)})
			} else {
				tmux.RunCmd([]string{"display-message", "Saved provider and model defaults to .kaleidoscope"})
			}
		}

		if !tmux.IsInsideTmux() {
			_, _, _ = tmux.RunCmd([]string{"display-message", "Not inside tmux; cannot open panes"})
			return panesOpenedMsg{count: 0, err: fmt.Errorf("not inside tmux")}
		}

		// Create feature branch first
		branchName := strings.TrimSpace(m.branch)
		if branchName == "" {
			return panesOpenedMsg{count: 0, err: fmt.Errorf("branch name is required")}
		}

		// Try to create the branch; if it already exists, just check it out
		cmd := exec.Command("git", "checkout", "-b", branchName)
		cmd.Run()
		// Ignore errors - branch may already exist, in which case we'll checkout to it
		cmd = exec.Command("git", "checkout", branchName)
		cmd.Run()

		// Capture the current pane id to restore focus later
		paneOut, _, err := tmux.RunCmd([]string{"display-message", "-p", "#{pane_id}"})
		if err != nil {
			return panesOpenedMsg{count: 0, err: err}
		}
		origPaneID := strings.TrimSpace(paneOut)

		opened := 0
		var lastErr error
		var paneIDs []string
		var worktrees []string
		var modelNames []string            // instance labels used as keys
		var providers []string             // provider used to open each instance
		var baseModels []string            // base model for each instance
		baseCounts := make(map[string]int) // base model -> count so far

		for _, baseName := range models {
			// Generate a unique instance label per base model: base, base-2, base-3, ...
			baseCounts[baseName] = baseCounts[baseName] + 1
			seq := baseCounts[baseName]
			instanceLabel := baseName
			if seq > 1 {
				instanceLabel = fmt.Sprintf("%s-%d", baseName, seq)
			}

			id := m.identifierFor(instanceLabel)

			// Build command for the pane: add worktree, cd, then run opencode bound to provider/base
			shellQuote := func(s string) string {
				return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
			}
			provider := m.currentProvider() // capture provider at open time
			prompt := strings.Join(m.input, "\n")
			modelFull := provider + "/" + baseName
			bashCmd := fmt.Sprintf("git worktree add -b %s ../%s %s || true; cd ../%s; opencode run -m %s %s; %s; exec $SHELL",
				shellQuote(id), shellQuote(id), shellQuote(branchName), shellQuote(id), shellQuote(modelFull), shellQuote(prompt), m.runCmd)

			out, _, err := tmux.RunCmd([]string{"split-window", "-v", "-P", "-F", "#{pane_id}", "bash", "-lc", bashCmd})
			if err != nil {
				lastErr = err
				continue
			}
			newPaneID := strings.TrimSpace(out)
			paneIDs = append(paneIDs, newPaneID)
			worktrees = append(worktrees, id)
			modelNames = append(modelNames, instanceLabel)
			providers = append(providers, provider)
			baseModels = append(baseModels, baseName)
			opened++
		}

		// Arrange panes nicely
		_, _, _ = tmux.RunCmd([]string{"select-layout", "tiled"})

		// Restore focus to the original pane
		_, _, _ = tmux.RunCmd([]string{"select-pane", "-t", origPaneID})

		// Inform in tmux status line
		_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Opened %d pane(s)", opened)})

		return panesOpenedMsg{count: opened, err: lastErr, paneIDs: paneIDs, worktrees: worktrees, modelNames: modelNames, providers: providers, baseModels: baseModels}
	}
}

func bailCmd(m model) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			return bailCompleteMsg{}
		}

		for _, paneID := range m.createdPanes {
			tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		cwd, err := os.Getwd()
		if err != nil {
			return bailCompleteMsg{}
		}
		parentDir := filepath.Dir(cwd)

		for _, worktree := range m.createdWorktrees {
			worktreePath := filepath.Join(parentDir, worktree)

			cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
			cmd.Run()

			cmd = exec.Command("git", "branch", "-D", worktree)
			cmd.Run()
		}

		tmux.RunCmd([]string{"display-message", "Bail complete: cleaned up panes, worktrees, and branches"})

		return bailCompleteMsg{}
	}
}

func nextCmd(m model, modelName string) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			return bailCompleteMsg{}
		}

		worktree, ok := m.modelToWorktree[modelName]
		if !ok {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error: model %s not found", modelName)})
			return bailCompleteMsg{}
		}

		// Increment choice for the bound provider/base model
		prov := m.instanceProvider[modelName]
		base := m.instanceBaseModel[modelName]
		if prov == "" || base == "" {
			prov = m.currentProvider()
			base = modelName
		}
		if err := incrementChoice(prov, base); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: failed to update choice count: %s", err)})
		}

		cwd, err := os.Getwd()
		if err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error: %s", err)})
			return bailCompleteMsg{}
		}
		parentDir := filepath.Dir(cwd)
		worktreePath := filepath.Join(parentDir, worktree)

		prompts := m.modelPrompts[modelName]
		commitMessage := "Changes from " + modelName
		if len(prompts) > 0 {
			commitMessage += "\n\n"
			for i, prompt := range prompts {
				commitMessage += fmt.Sprintf("%d. %s\n", i+1, prompt)
			}
		}

		cmd := exec.Command("git", "-C", worktreePath, "add", ".")
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error adding files: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "-C", worktreePath, "commit", "-m", commitMessage)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error committing: %s", err)})
		}

		featureBranch := strings.TrimSpace(m.branch)
		cmd = exec.Command("git", "checkout", featureBranch)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error checking out feature branch: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "merge", "--no-ff", worktree, "-m", fmt.Sprintf("Merge changes from %s", modelName))
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error merging: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "push", "origin", featureBranch)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error pushing: %s", err)})
		}

		for _, paneID := range m.createdPanes {
			tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		for _, wt := range m.createdWorktrees {
			wtPath := filepath.Join(parentDir, wt)
			cmd = exec.Command("git", "worktree", "remove", wtPath, "--force")
			cmd.Run()

			cmd = exec.Command("git", "branch", "-D", wt)
			cmd.Run()
		}

		tmux.RunCmd([]string{"display-message", fmt.Sprintf("Next complete: merged %s and cleaned up", modelName)})

		return nextCompleteMsg{}
	}
}

func wrapCmd(m model, modelName string) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			return bailCompleteMsg{}
		}

		worktree, ok := m.modelToWorktree[modelName]
		if !ok {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error: model %s not found", modelName)})
			return bailCompleteMsg{}
		}

		// Increment choice for the bound provider/base model
		prov := m.instanceProvider[modelName]
		base := m.instanceBaseModel[modelName]
		if prov == "" || base == "" {
			prov = m.currentProvider()
			base = modelName
		}
		if err := incrementChoice(prov, base); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: failed to update choice count: %s", err)})
		}

		cwd, err := os.Getwd()
		if err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error: %s", err)})
			return bailCompleteMsg{}
		}
		parentDir := filepath.Dir(cwd)
		worktreePath := filepath.Join(parentDir, worktree)

		prompts := m.modelPrompts[modelName]
		commitMessage := "Changes from " + modelName
		if len(prompts) > 0 {
			commitMessage += "\n\n"
			for i, prompt := range prompts {
				commitMessage += fmt.Sprintf("%d. %s\n", i+1, prompt)
			}
		}

		cmd := exec.Command("git", "-C", worktreePath, "add", ".")
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error adding files: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "-C", worktreePath, "commit", "-m", commitMessage)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error committing: %s", err)})
		}

		featureBranch := strings.TrimSpace(m.branch)
		cmd = exec.Command("git", "checkout", featureBranch)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error checking out feature branch: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "merge", "--no-ff", worktree, "-m", fmt.Sprintf("Merge changes from %s", modelName))
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error merging: %s", err)})
			return bailCompleteMsg{}
		}

		cmd = exec.Command("git", "push", "origin", featureBranch)
		if err := cmd.Run(); err != nil {
			tmux.RunCmd([]string{"display-message", fmt.Sprintf("Error pushing: %s", err)})
		}

		for _, paneID := range m.createdPanes {
			tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		for _, wt := range m.createdWorktrees {
			wtPath := filepath.Join(parentDir, wt)
			cmd = exec.Command("git", "worktree", "remove", wtPath, "--force")
			cmd.Run()

			cmd = exec.Command("git", "branch", "-D", wt)
			cmd.Run()
		}

		tmux.RunCmd([]string{"display-message", fmt.Sprintf("Wrap complete: merged %s and cleaned up", modelName)})

		return wrapCompleteMsg{}
	}
}

func sendToModelPaneCmd(paneID string, modelName string, prompt string, m model) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			return nil
		}

		shellQuote := func(s string) string {
			return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
		}

		// Use bound provider/base model for this instance label
		provider := m.instanceProvider[modelName]
		base := m.instanceBaseModel[modelName]
		if provider == "" || base == "" {
			// Fallback to currentProvider and given modelName
			provider = m.currentProvider()
			base = modelName
		}
		modelFull := provider + "/" + base
		bashCmd := fmt.Sprintf("opencode run -m %s %s", shellQuote(modelFull), shellQuote(prompt))

		_, _, _ = tmux.RunCmd([]string{"send-keys", "-t", paneID, "C-c"})
		_, _, _ = tmux.RunCmd([]string{"send-keys", "-t", paneID, bashCmd, "Enter"})
		_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Sent to @%s: %s", modelName, prompt)})

		return nil
	}
}

func cleanupCmd(m model) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			return cleanupCompleteMsg{}
		}

		for _, paneID := range m.createdPanes {
			tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		cwd, err := os.Getwd()
		if err != nil {
			return cleanupCompleteMsg{}
		}
		parentDir := filepath.Dir(cwd)

		for _, worktree := range m.createdWorktrees {
			worktreePath := filepath.Join(parentDir, worktree)

			cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
			cmd.Run()

			cmd = exec.Command("git", "branch", "-D", worktree)
			cmd.Run()
		}

		if len(m.createdPanes) > 0 || len(m.createdWorktrees) > 0 {
			tmux.RunCmd([]string{"display-message", "Cleanup complete: closed panes, removed worktrees and branches"})
		}

		return cleanupCompleteMsg{}
	}
}

func (m model) View() string {
	if m.screen == screenIteration {
		return m.viewIteration()
	}
	if m.screen == screenNewTask {
		return m.viewNewTask()
	}
	if m.screen == screenProgress {
		return m.viewProgress()
	}
	// Header and spacing
	header := rainbowHeader(m.width)
	spacer := "\n\n"

	// Dimensions
	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}

	// Prompt box size
	promptWidth := maxWidth / 2
	if promptWidth < 50 {
		promptWidth = 50
	}
	promptHeight := 10

	// Branch box size (single line)
	branchWidth := m.width / 4
	if branchWidth < 24 {
		branchWidth = 24
	}
	if branchWidth > 40 {
		branchWidth = 40
	}

	// Selected column size
	selectedWidth := m.width / 5
	if selectedWidth < 24 {
		selectedWidth = 24
	}
	if selectedWidth > 32 {
		selectedWidth = 32
	}

	// Render branch single-line with cursor
	bline := m.branch
	if m.branchCursor > len(bline) {
		m.branchCursor = len(bline)
	}
	bLeft := bline[:m.branchCursor]
	bRight := bline[m.branchCursor:]
	branchInner := bLeft + bRight
	if m.focus == focusBranch && m.cursorVisible {
		cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
		branchInner = bLeft + cursor + bRight
	}

	// Render task single-line with cursor
	tline := m.task
	if m.taskCursor > len(tline) {
		m.taskCursor = len(tline)
	}
	tLeft := tline[:m.taskCursor]
	tRight := tline[m.taskCursor:]
	taskInner := tLeft + tRight
	if m.focus == focusTask && m.cursorVisible {
		cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
		taskInner = tLeft + cursor + tRight
	}

	branchBorder := lipgloss.Color("#6BCB77")
	if m.focus == focusBranch {
		branchBorder = lipgloss.Color("#4D96FF")
	}
	// task border highlights when focused
	taskBorder := lipgloss.Color("#6BCB77")
	if m.focus == focusTask {
		taskBorder = lipgloss.Color("#4D96FF")
	}
	branchBox := lipgloss.NewStyle().
		Width(branchWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(branchBorder).
		Padding(0, 2)
	// task box shares width with branch box
	taskBox := lipgloss.NewStyle().
		Width(branchWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(taskBorder).
		Padding(0, 2)

	branchLabel := lipgloss.NewStyle().Faint(true).Render("branch-name")
	taskLabel := lipgloss.NewStyle().Faint(true).Render("task-name")
	branchView := branchLabel + "\n" + branchBox.Render(branchInner) + "\n\n" + taskLabel + "\n" + taskBox.Render(taskInner)

	// Render prompt buffer with block cursor
	var pb strings.Builder
	for i, line := range m.input {
		if i == m.cursor.row {
			col := m.cursor.col
			if col > len(line) {
				col = len(line)
			}
			pb.WriteString(line[:col])
			if m.focus == focusPrompt && m.cursorVisible {
				curBlock := lipgloss.NewStyle().Reverse(true).Render(" ")
				pb.WriteString(curBlock)
			}
			pb.WriteString(line[col:])
		} else {
			pb.WriteString(line)
		}
		if i < len(m.input)-1 {
			pb.WriteString("\n")
		}
	}

	promptBorder := lipgloss.Color("#6BCB77")
	if m.focus == focusPrompt {
		promptBorder = lipgloss.Color("#4D96FF")
	}
	promptBox := lipgloss.NewStyle().
		Width(promptWidth).Height(promptHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(promptBorder).
		Padding(1, 2)

	promptView := promptBox.Render(pb.String())

	// Selected models column next to the prompt
	selectedCol := m.renderSelectedColumn(selectedWidth)

	topGap := "  "
	row := lipgloss.JoinHorizontal(lipgloss.Top, branchView, topGap, promptView, topGap, selectedCol)
	centeredRow := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, row)

	// Provider + Models dropdown row (same visual width as prompt)
	// Compute widths
	provWidth := promptWidth / 2
	if provWidth < 24 {
		provWidth = 24
	}
	gap := "  "
	modelsWidth := promptWidth - provWidth - lipgloss.Width(gap)
	if modelsWidth < 24 {
		modelsWidth = 24
	}

	// Provider view
	provBorder := lipgloss.Color("#6BCB77")
	if m.focus == focusProvider {
		provBorder = lipgloss.Color("#4D96FF")
	}
	provLabel := lipgloss.NewStyle().Faint(true).Render("model provider")
	if !m.providerOpen {
		current := m.providers[m.providerIndex]
		provBox := lipgloss.NewStyle().
			Width(provWidth).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(provBorder).
			Padding(0, 2)
		provView := provLabel + "\n" + provBox.Render(current+"  ▾")

		// Models collapsed or open
		modelsView := m.renderModelsDropdown(modelsWidth)

		pair := lipgloss.JoinHorizontal(lipgloss.Top, provView, gap, modelsView)
		pairCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pair)

		hint := lipgloss.NewStyle().Faint(true).Render("tab: next field • ↑↓: navigate • space: select models • enter: submit")
		hintCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hint)

		return header + spacer + centeredRow + "\n\n" + pairCentered + "\n\n" + hintCentered
	}

	// Provider open view
	var list strings.Builder
	for i, opt := range m.providers {
		item := opt
		if i == m.providerHover {
			item = lipgloss.NewStyle().Reverse(true).Render(opt)
		}
		list.WriteString(item)
		if i < len(m.providers)-1 {
			list.WriteString("\n")
		}
	}
	provOpenBox := lipgloss.NewStyle().
		Width(provWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(provBorder).
		Padding(0, 2)
	provOpenView := provLabel + "\n" + provOpenBox.Render(list.String())

	modelsView := m.renderModelsDropdown(modelsWidth)
	pair := lipgloss.JoinHorizontal(lipgloss.Top, provOpenView, gap, modelsView)
	pairCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pair)

	hint := lipgloss.NewStyle().Faint(true).Render("tab: next field • ↑↓: navigate • space: select models • enter: submit")
	hintCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hint)

	return header + spacer + centeredRow + "\n\n" + pairCentered + "\n\n" + hintCentered
}

func (m model) viewIteration() string {
	header := rainbowHeader(m.width)

	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}

	promptWidth := maxWidth - 20
	if promptWidth < 60 {
		promptWidth = 60
	}
	if promptWidth > 100 {
		promptWidth = 100
	}
	promptHeight := m.height - 20
	if promptHeight < 10 {
		promptHeight = 10
	}

	// Prefer opened instance labels for mention/highlight; fallback to selections
	var mentionables []string
	if len(m.modelToWorktree) > 0 {
		for name := range m.modelToWorktree {
			mentionables = append(mentionables, name)
		}
	} else {
		mentionables = m.selectedModels()
	}

	var pb strings.Builder
	for i, line := range m.iterationInput {
		if i == m.iterationCursor.row {
			col := m.iterationCursor.col
			if col > len(line) {
				col = len(line)
			}

			leftPart := highlightCommandLine(line[:col], mentionables)
			rightPart := highlightCommandLine(line[col:], mentionables)

			pb.WriteString(leftPart)
			if m.cursorVisible {
				curBlock := lipgloss.NewStyle().Reverse(true).Render(" ")
				pb.WriteString(curBlock)
			}
			pb.WriteString(rightPart)
		} else {
			pb.WriteString(highlightCommandLine(line, mentionables))
		}
		if i < len(m.iterationInput)-1 {
			pb.WriteString("\n")
		}
	}

	promptBox := lipgloss.NewStyle().
		Width(promptWidth).Height(promptHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4D96FF")).
		Padding(1, 2)

	label := lipgloss.NewStyle().Faint(true).Render("iteration prompt")
	hint := lipgloss.NewStyle().Faint(true).Render("commands: /bail /next <instance> /wrap <instance> | @<instance> <prompt>")
	tmuxHint := lipgloss.NewStyle().Faint(true).Render("tmux: Ctrl-b then arrow keys to move between panes")
	promptView := label + "\n" + promptBox.Render(pb.String()) + "\n" + hint + "\n" + tmuxHint

	if m.autocompleteActive && len(m.autocompleteOptions) > 0 {
		var acList strings.Builder
		for i, opt := range m.autocompleteOptions {
			if i == m.autocompleteIndex {
				acList.WriteString(lipgloss.NewStyle().Reverse(true).Render(opt))
			} else {
				acList.WriteString(opt)
			}
			if i < len(m.autocompleteOptions)-1 {
				acList.WriteString("\n")
			}
		}

		acBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F7B801")).
			Padding(0, 1)
		acView := acBox.Render(acList.String())

		promptView = promptView + "\n\n" + acView
	}

	centeredPrompt := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, promptView)
	centeredVertical := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, centeredPrompt)

	return header + "\n\n" + centeredVertical
}

func (m model) viewNewTask() string {
	header := rainbowHeader(m.width)

	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}

	taskNameWidth := maxWidth / 4
	if taskNameWidth < 24 {
		taskNameWidth = 24
	}
	if taskNameWidth > 40 {
		taskNameWidth = 40
	}

	promptWidth := maxWidth / 2
	if promptWidth < 50 {
		promptWidth = 50
	}
	promptHeight := 10

	tline := m.newTaskName
	if m.newTaskNameCursor > len(tline) {
		m.newTaskNameCursor = len(tline)
	}
	tLeft := tline[:m.newTaskNameCursor]
	tRight := tline[m.newTaskNameCursor:]
	taskInner := tLeft + tRight
	if m.newTaskFocus == focusTask && m.cursorVisible {
		cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
		taskInner = tLeft + cursor + tRight
	}

	taskBorder := lipgloss.Color("#6BCB77")
	if m.newTaskFocus == focusTask {
		taskBorder = lipgloss.Color("#4D96FF")
	}
	taskBox := lipgloss.NewStyle().
		Width(taskNameWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(taskBorder).
		Padding(0, 2)

	taskLabel := lipgloss.NewStyle().Faint(true).Render("task-name")
	taskView := taskLabel + "\n" + taskBox.Render(taskInner)

	var pb strings.Builder
	for i, line := range m.newTaskPrompt {
		if i == m.newTaskCursor.row {
			col := m.newTaskCursor.col
			if col > len(line) {
				col = len(line)
			}
			pb.WriteString(line[:col])
			if m.newTaskFocus == focusPrompt && m.cursorVisible {
				curBlock := lipgloss.NewStyle().Reverse(true).Render(" ")
				pb.WriteString(curBlock)
			}
			pb.WriteString(line[col:])
		} else {
			pb.WriteString(line)
		}
		if i < len(m.newTaskPrompt)-1 {
			pb.WriteString("\n")
		}
	}

	promptBorder := lipgloss.Color("#6BCB77")
	if m.newTaskFocus == focusPrompt {
		promptBorder = lipgloss.Color("#4D96FF")
	}
	promptBox := lipgloss.NewStyle().
		Width(promptWidth).Height(promptHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(promptBorder).
		Padding(1, 2)

	promptView := promptBox.Render(pb.String())

	topGap := "  "
	row := lipgloss.JoinHorizontal(lipgloss.Top, taskView, topGap, promptView)
	centeredRow := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, row)

	return header + "\n\n" + centeredRow
}

func (m model) viewProgress() string {
	header := rainbowHeader(m.width)
	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// center a simple spinner with message
	spinner := ""
	if len(m.spinnerFrames) > 0 {
		spinner = m.spinnerFrames[m.spinnerIndex%len(m.spinnerFrames)]
	}
	msg := m.progressMsg
	if msg == "" {
		msg = "Working..."
	}
	line := fmt.Sprintf(" %s  %s", spinner, msg)
	centered := lipgloss.PlaceHorizontal(maxWidth, lipgloss.Center, line)
	centeredVertical := lipgloss.Place(maxWidth, m.height, lipgloss.Center, lipgloss.Center, centered)
	return header + "\n\n" + centeredVertical
}

func highlightCommandLine(line string, selectedModels []string) string {
	if line == "" {
		return ""
	}

	var result strings.Builder
	i := 0
	runes := []rune(line)

	slashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F7B801")).Bold(true)
	atStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6BCB77")).Bold(true)

	validSlashCommands := map[string]bool{
		"/bail": true,
		"/next": true,
		"/wrap": true,
	}

	modelSet := make(map[string]bool)
	for _, m := range selectedModels {
		modelSet[m] = true
	}

	for i < len(runes) {
		if runes[i] == '/' {
			start := i
			i++
			for i < len(runes) && (runes[i] >= 'a' && runes[i] <= 'z' || runes[i] >= 'A' && runes[i] <= 'Z' || runes[i] == '-' || runes[i] == '_') {
				i++
			}
			cmd := string(runes[start:i])
			if validSlashCommands[cmd] {
				result.WriteString(slashStyle.Render(cmd))
			} else {
				result.WriteString(cmd)
			}
		} else if runes[i] == '@' {
			start := i
			i++
			for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' && runes[i] != '\n' {
				i++
			}
			mention := string(runes[start:i])
			modelName := mention[1:]
			if modelSet[modelName] {
				result.WriteString(atStyle.Render(mention))
			} else {
				result.WriteString(mention)
			}
		} else {
			result.WriteRune(runes[i])
			i++
		}
	}

	return result.String()
}

func (m model) renderModelsDropdown(width int) string {
	border := lipgloss.Color("#6BCB77")
	if m.focus == focusModels {
		border = lipgloss.Color("#4D96FF")
	}
	label := lipgloss.NewStyle().Faint(true).Render("models")
	box := lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 2)

	opts := m.providerModels()
	if !m.modelsOpen {
		// collapsed: show total count selected
		count := 0
		p := m.currentProvider()
		if m.selected[p] != nil {
			for _, v := range m.selected[p] {
				if v > 0 {
					count += v
				}
			}
		}
		labelText := "Select models…  ▾"
		if count > 0 {
			labelText = fmt.Sprintf("%d selected  ▾", count)
		}
		return label + "\n" + box.Render(labelText)
	}

	// open: list with counts
	var list strings.Builder
	p := m.currentProvider()
	sel := m.selected[p]
	for i, opt := range opts {
		c := 0
		if sel != nil {
			c = sel[opt]
		}
		row := opt
		if c > 0 {
			row = fmt.Sprintf("%s ×%d", opt, c)
		}
		if i == m.modelsHover {
			row = lipgloss.NewStyle().Reverse(true).Render(row)
		}
		list.WriteString(row)
		if i < len(opts)-1 {
			list.WriteString("\n")
		}
	}
	return label + "\n" + box.Render(list.String())
}

func (m model) renderSelectedColumn(width int) string {
	label := lipgloss.NewStyle().Faint(true).Render("selected models")
	p := m.currentProvider()
	sel := m.selected[p]
	var lines []string
	for _, name := range m.models[p] {
		if sel != nil {
			if c := sel[name]; c > 0 {
				if c == 1 {
					lines = append(lines, "• "+name)
				} else {
					lines = append(lines, fmt.Sprintf("• %s ×%d", name, c))
				}
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{"• none"}
	}
	box := lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6BCB77")).
		Padding(0, 2)
	return label + "\n" + box.Render(strings.Join(lines, "\n"))
}

func rainbowHeader(width int) string {
	lines := bigBlockKALEIDOSCOPE()

	// Determine the widest line to size our gradient
	maxCols := 0
	for _, ln := range lines {
		if l := len([]rune(ln)); l > maxCols {
			maxCols = l
		}
	}
	if maxCols == 0 {
		return ""
	}

	// Color stops for a pleasant rainbow sweep (left → right)
	stops := []string{
		"#4D96FF", // blue
		"#6BCB77", // green
		"#F7B801", // yellow
		"#FF6B6B", // coral
		"#B967FF", // violet
	}
	palette := gradientColors(maxCols, stops)

	var out strings.Builder
	// Add vertical spacing above the banner
	out.WriteString("\n\n\n")
	for _, ln := range lines {
		var row strings.Builder
		cols := []rune(ln)
		for i, r := range cols {
			if r == ' ' {
				row.WriteRune(' ')
				continue
			}
			c := lipgloss.Color(palette[i])
			row.WriteString(lipgloss.NewStyle().Bold(true).Foreground(c).Render(string(r)))
		}
		centered := lipgloss.PlaceHorizontal(width, lipgloss.Center, row.String())
		out.WriteString(centered)
		out.WriteString("\n")
	}
	// Add matching vertical spacing below the banner
	out.WriteString("\n\n\n")
	return out.String()
}

// bigBlockKALEIDOSCOPE returns a blocky ASCII banner for "KALEIDOSCOPE".
// Each string is one row; characters are built from '█' and spaces.
func bigBlockKALEIDOSCOPE() []string {
	font := map[rune][]string{
		'A': {
			"  ██   ",
			" █  █  ",
			"█    █ ",
			"██████ ",
			"█    █ ",
			"█    █ ",
			"█    █ ",
		},
		'C': {
			" ████  ",
			"█      ",
			"█      ",
			"█      ",
			"█      ",
			"█      ",
			" ████  ",
		},
		'D': {
			"█████  ",
			"█   █  ",
			"█    █ ",
			"█    █ ",
			"█    █ ",
			"█   █  ",
			"█████  ",
		},
		'E': {
			"██████ ",
			"█      ",
			"█      ",
			"█████  ",
			"█      ",
			"█      ",
			"██████ ",
		},
		'I': {
			"██████ ",
			"  █    ",
			"  █    ",
			"  █    ",
			"  █    ",
			"  █    ",
			"██████ ",
		},
		'K': {
			"█   █  ",
			"█  █   ",
			"█ █    ",
			"██     ",
			"█ █    ",
			"█  █   ",
			"█   █  ",
		},
		'L': {
			"█      ",
			"█      ",
			"█      ",
			"█      ",
			"█      ",
			"█      ",
			"██████ ",
		},
		'O': {
			" ████  ",
			"█    █ ",
			"█    █ ",
			"█    █ ",
			"█    █ ",
			"█    █ ",
			" ████  ",
		},
		'P': {
			"█████  ",
			"█   █  ",
			"█   █  ",
			"█████  ",
			"█      ",
			"█      ",
			"█      ",
		},
		'S': {
			" █████ ",
			"█      ",
			"█      ",
			" ████  ",
			"     █ ",
			"     █ ",
			"█████  ",
		},
	}

	word := "KALEIDOSCOPE"
	// Height from any glyph
	glyph := font['A']
	height := len(glyph)
	lines := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		for _, ch := range word {
			g, ok := font[ch]
			if !ok {
				// Fallback to blanks roughly the width of an 'A'
				b.WriteString("       ")
				b.WriteString("  ")
				continue
			}
			b.WriteString(g[row])
			b.WriteString("  ") // gap between letters
		}
		lines[row] = b.String()
	}
	return lines
}

// gradientColors creates a width-sized palette interpolating across the given
// hex color stops (e.g., ["#ff0000", "#00ff00", "#0000ff"]).
func gradientColors(width int, stops []string) []string {
	if width < 1 {
		return nil
	}
	if len(stops) == 0 {
		stops = []string{"#FFFFFF", "#FFFFFF"}
	}
	if len(stops) == 1 {
		stops = append(stops, stops[0])
	}

	nSeg := len(stops) - 1
	res := make([]string, width)
	for i := 0; i < width; i++ {
		pos := float64(i) / float64(width-1)
		seg := int(pos * float64(nSeg))
		if seg >= nSeg {
			seg = nSeg - 1
		}
		segStart := float64(seg) / float64(nSeg)
		segEnd := float64(seg+1) / float64(nSeg)
		t := 0.0
		if segEnd > segStart {
			t = (pos - segStart) / (segEnd - segStart)
		}

		r1, g1, b1 := hexToRGB(stops[seg])
		r2, g2, b2 := hexToRGB(stops[seg+1])

		r := int(math.Round((1-t)*float64(r1) + t*float64(r2)))
		g := int(math.Round((1-t)*float64(g1) + t*float64(g2)))
		b := int(math.Round((1-t)*float64(b1) + t*float64(b2)))
		res[i] = fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	return res
}

func hexToRGB(h string) (int, int, int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) != 6 {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 64)
	g, _ := strconv.ParseInt(h[2:4], 16, 64)
	b, _ := strconv.ParseInt(h[4:6], 16, 64)
	return int(r), int(g), int(b)
}

// selectedModels returns selected model names for the current provider
func (m model) selectedModels() []string {
	p := m.currentProvider()
	sel := m.selected[p]
	var out []string
	if sel == nil {
		return out
	}
	for _, name := range m.models[p] {
		if c, ok := sel[name]; ok && c > 0 {
			for i := 0; i < c; i++ {
				out = append(out, name)
			}
		}
	}
	return out
}

func (m model) getAutocompletePrefix(line string, cursorPos int) (string, int) {
	if cursorPos > len(line) {
		cursorPos = len(line)
	}

	// First: detect cases like "/next <partial>" or "/wrap <partial>" where the
	// cursor is inside the model argument (after a space). We want to return a
	// prefix that includes the command (so getAutocompleteOptions can detect the
	// context) but return a start index that points to the beginning of the
	// current token (so only the model name is replaced on completion).
	curStart := cursorPos
	for curStart > 0 && line[curStart-1] != ' ' && line[curStart-1] != '\t' && line[curStart-1] != '\n' {
		curStart--
	}
	currentToken := line[curStart:cursorPos]

	// find previous token (skip spaces backwards)
	prevEnd := curStart - 1
	for prevEnd >= 0 && (line[prevEnd] == ' ' || line[prevEnd] == '\t' || line[prevEnd] == '\n') {
		prevEnd--
	}
	if prevEnd >= 0 {
		prevStart := prevEnd
		for prevStart > 0 && line[prevStart-1] != ' ' && line[prevStart-1] != '\t' && line[prevStart-1] != '\n' {
			prevStart--
		}
		prevToken := line[prevStart : prevEnd+1]
		if len(prevToken) > 0 && (prevToken[0] == '/' || prevToken[0] == '@') {
			// return combined prefix (e.g. "/next gpt") but start at the current
			// token so replacement only swaps the model name.
			return prevToken + " " + currentToken, curStart
		}
	}

	// Fallback to original behavior: detect if we're inside a token that starts
	// with '/' or '@' (no space between command and cursor), or a contiguous
	// token that contains '/' or '@' when scanning left.
	start := cursorPos - 1
	if start < 0 {
		return "", 0
	}

	if line[start] == '/' || line[start] == '@' {
		for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' && line[start-1] != '\n' {
			start--
		}
		return line[start:cursorPos], start
	}

	for start >= 0 && line[start] != ' ' && line[start] != '\t' && line[start] != '\n' {
		if line[start] == '/' || line[start] == '@' {
			return line[start:cursorPos], start
		}
		start--
	}

	return "", 0
}

func (m model) getAutocompleteOptions(prefix string) []string {
	if len(prefix) == 0 {
		return nil
	}

	// Slash-command completions. Support two modes:
	// - completing the command itself (e.g. "/n" → "/next")
	// - completing the argument to a command (e.g. "/next g" → model names)
	if prefix[0] == '/' {
		// If this looks like a command with an argument (contains a space), handle
		// the "/next" and "/wrap" cases by returning available model names.
		if strings.HasPrefix(prefix, "/next ") || strings.HasPrefix(prefix, "/wrap ") {
			searchPrefix := ""
			if len(prefix) > 6 {
				// "/next " length is 6, "/wrap " length is 6 as well
				// extract everything after the space
				parts := strings.SplitN(prefix, " ", 2)
				if len(parts) == 2 {
					searchPrefix = parts[1]
				}
			}
			// Prefer models that currently have worktrees (i.e., were opened).
			var candidates []string
			for modelName := range m.modelToWorktree {
				candidates = append(candidates, modelName)
			}
			// Fallback to selected models if no worktrees known
			if len(candidates) == 0 {
				candidates = m.selectedModels()
			}
			var matches []string
			for _, c := range candidates {
				if strings.HasPrefix(c, searchPrefix) {
					matches = append(matches, c)
				}
			}
			return matches
		}

		// Otherwise complete top-level slash commands as before.
		commands := []string{"/bail", "/next", "/wrap"}
		var matches []string
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, prefix) {
				matches = append(matches, cmd)
			}
		}
		return matches
	}

	// @-mentions for sending input to a model
	if prefix[0] == '@' {
		var matches []string
		// Prefer opened instance labels (keys of modelToWorktree); fallback to selected models
		var candidates []string
		for name := range m.modelToWorktree {
			candidates = append(candidates, name)
		}
		if len(candidates) == 0 {
			candidates = m.selectedModels()
		}
		searchPrefix := prefix[1:]
		for _, name := range candidates {
			if strings.HasPrefix(name, searchPrefix) {
				matches = append(matches, "@"+name)
			}
		}
		return matches
	}

	return nil
}

func main() {
	run := flag.String("run", "", "run command (required)")
	setDefault := flag.Bool("set-default", false, "save chosen provider and models as defaults in .kaleidoscope")
	flag.Parse()

	if *run == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if !tmux.IsInsideTmux() {
		fmt.Fprintln(os.Stderr, "Error: not inside a tmux session; please start tmux and re-run")
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel(*run, *setDefault), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
