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
const exitDoublePressWindow = 500 * time.Millisecond
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
	// Collapsed paste handling for main prompt
	promptPastes map[string]string // token -> original content
	pasteCounter int               // to generate unique tokens

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

	// exitPending indicates one press of ESC or Ctrl+C has occurred; a second
	// press within exitDoublePressWindow will exit the TUI. Cleared by a timer.
	exitPending bool

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
		"opencode":       {"qwen3-coder", "claude-opus-4-1", "kimi-k2", "claude-haiku-4-5", "minimax-m2", "claude-sonnet-4-5", "an-gd4", "gpt-5-codex", "big-pickle", "claude-3-5-haiku", "glm-4.6", "grok-code", "claude-sonnet-4", "gpt-5"},
		"openai":         {"gpt-4.1-mini", "text-embedding-3-small", "gpt-4", "o1-pro", "gpt-4o-2024-05-13", "gpt-4o-2024-08-06", "gpt-4.1-mini", "o3-deep-research", "gpt-3.5-turbo", "text-embedding-3-large", "gpt-4-turbo", "o1-preview", "o3-mini", "codex-mini-latest", "gpt-5-nano", "gpt-5-codex", "gpt-4o", "gpt-4.1", "o4-mini", "o1", "gpt-5-mini", "o1-mini", "text-embedding-ada-002", "o3-pro", "gpt-4o-2024-11-20", "o3", "o4-mini-deep-research", "gpt-5-chat-latest", "gpt-4o-mini", "gpt-5", "gpt-5-pro"},
		"openrouter":     {"moonshotai/kimi-k2", "moonshotai/kimi-k2-0905", "moonshotai/kimi-dev-72b:free", "moonshotai/kimi-k2-thinking", "moonshotai/kimi-k2-0905:exacto", "moonshotai/kimi-k2:free", "thudm/glm-z1-32b:free", "nousresearch/hermes-4-70b", "nousresearch/hermes-4-405b", "nousresearch/deephermes-3-llama-3-8b-preview", "nvidia/nemotron-nano-9b-v2", "x-ai/grok-4", "x-ai/grok-code-fast-1", "x-ai/grok-3", "x-ai/grok-4-fast", "x-ai/grok-3-beta", "x-ai/grok-3-mini-beta", "x-ai/grok-3-mini", "cognitivecomputations/dolphin3.0-mistral-24b", "cognitivecomputations/dolphin3.0-r1-mistral-24b", "deepseek/deepseek-chat-v3.1", "deepseek/deepseek-r1:free", "deepseek/deepseek-v3-base:free", "deepseek/deepseek-v3.1-terminus", "deepseek/deepseek-r1-0528-qwen3-8b:free", "deepseek/deepseek-chat-v3-0324", "deepseek/deepseek-r1-0528:free", "deepseek/deepseek-r1-distill-llama-70b", "deepseek/deepseek-r1-distill-qwen-14b", "deepseek/deepseek-v3.1-terminus:exacto", "featherless/qwerky-72b", "tngtech/deepseek-r1t2-chimera:free", "minimax/minimax-m1", "minimax/minimax-m2:free", "minimax/minimax-01", "google/gemini-2.0-flash-001", "google/gemma-2-9b-it:free", "google/gemini-2.5-flash", "google/gemini-2.5-pro-preview-05-06", "google/gemma-3n-e4b-it", "google/gemini-2.5-flash-lite", "google/gemini-2.5-pro-preview-06-05", "google/gemini-2.5-flash-preview-09-2025", "google/gemini-2.5-pro", "google/gemma-3-12b-it", "google/gemma-3n-e4b-it:free", "google/gemini-2.5-flash-lite-preview-09-2025", "google/gemini-2.0-flash-exp:free", "google/gemma-3-27b-it", "microsoft/mai-ds-r1:free", "openai/gpt-4.1-mini", "openai/gpt-5-chat", "openai/gpt-5-nano", "openai/gpt-5-codex", "openai/gpt-4.1", "openai/gpt-oss-120b:exacto", "openai/o4-mini", "openai/gpt-5-mini", "openai/gpt-5-image", "openai/gpt-oss-20b", "openai/gpt-oss-120b", "openai/gpt-4o-mini", "openai/gpt-5", "openai/gpt-5-pro", "openrouter/horizon-alpha", "openrouter/polaris-alpha", "openrouter/sonoma-sky-alpha", "openrouter/cypher-alpha:free", "openrouter/sonoma-dusk-alpha", "openrouter/horizon-beta", "z-ai/glm-4.5", "z-ai/glm-4.5-air", "z-ai/glm-4.5v", "z-ai/glm-4.6", "z-ai/glm-4.6:exacto", "z-ai/glm-4.5-air:free", "qwen/qwen3-coder", "qwen/qwen3-32b:free", "qwen/qwen3-next-80b-a3b-instruct", "qwen/qwen-2.5-coder-32b-instruct", "qwen/qwen3-235b-a22b:free", "qwen/qwq-32b:free", "qwen/qwen3-30b-a3b-thinking-2507", "qwen/qwen3-30b-a3b:free", "qwen/qwen2.5-vl-72b-instruct", "qwen/qwen3-14b:free", "qwen/qwen3-30b-a3b-instruct-2507", "qwen/qwen3-235b-a22b-thinking-2507", "qwen/qwen2.5-vl-32b-instruct:free", "qwen/qwen2.5-vl-72b-instruct:free", "qwen/qwen3-235b-a22b-07-25:free", "qwen/qwen3-coder:free", "qwen/qwen3-235b-a22b-07-25", "qwen/qwen3-8b:free", "qwen/qwen3-max", "qwen/qwen3-next-80b-a3b-thinking", "qwen/qwen3-coder:exacto", "mistralai/devstral-medium-2507", "mistralai/codestral-2508", "mistralai/mistral-7b-instruct:free", "mistralai/devstral-small-2505", "mistralai/mistral-small-3.2-24b-instruct", "mistralai/devstral-small-2505:free", "mistralai/mistral-small-3.2-24b-instruct:free", "mistralai/mistral-medium-3", "mistralai/mistral-small-3.1-24b-instruct", "mistralai/devstral-small-2507", "mistralai/mistral-medium-3.1", "mistralai/mistral-nemo:free", "rekaai/reka-flash-3", "meta-llama/llama-3.2-11b-vision-instruct", "meta-llama/llama-3.3-70b-instruct:free", "meta-llama/llama-4-scout:free", "anthropic/claude-opus-4", "anthropic/claude-haiku-4.5", "anthropic/claude-opus-4.1", "anthropic/claude-3.7-sonnet", "anthropic/claude-3.5-haiku", "anthropic/claude-sonnet-4", "anthropic/claude-sonnet-4.5", "sarvamai/sarvam-m:free"},
		"lmstudio":       {"openai/gpt-oss-20b", "qwen/qwen3-30b-a3b-2507", "qwen/qwen3-coder-30b"},
		"anthropic":      {"claude-opus-4-0", "claude-3-5-sonnet-20241022", "claude-opus-4-1", "claude-haiku-4-5", "claude-3-5-sonnet-20240620", "claude-3-5-haiku-latest", "claude-3-opus-20240229", "claude-sonnet-4-5", "claude-sonnet-4-5-20250929", "claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-3-5-haiku-20241022", "claude-3-haiku-20240307", "claude-3-7-sonnet-20250219", "claude-3-7-sonnet-latest", "claude-sonnet-4-0", "claude-opus-4-1-20250805", "claude-3-sonnet-20240229", "claude-haiku-4-5-20251001"},
		"amazon-bedrock": {"cohere.command-r-plus-v1:0", "anthropic.claude-v2", "anthropic.claude-3-7-sonnet-20250219-v1:0", "anthropic.claude-sonnet-4-20250514-v1:0", "qwen.qwen3-coder-30b-a3b-v1:0", "meta.llama3-2-11b-instruct-v1:0", "anthropic.claude-3-haiku-20240307-v1:0", "meta.llama3-2-90b-instruct-v1:0", "meta.llama3-2-1b-instruct-v1:0", "anthropic.claude-v2:1", "deepseek.v3-v1:0", "cohere.command-light-text-v14", "ai21.jamba-1-5-large-v1:0", "meta.llama3-3-70b-instruct-v1:0", "anthropic.claude-3-opus-20240229-v1:0", "amazon.nova-pro-v1:0", "meta.llama3-1-8b-instruct-v1:0", "qwen.qwen3-32b-v1:0", "anthropic.claude-3-5-sonnet-20240620-v1:0", "anthropic.claude-haiku-4-5-20251001-v1:0", "cohere.command-r-v1:0", "amazon.nova-micro-v1:0", "meta.llama3-1-70b-instruct-v1:0", "meta.llama3-70b-instruct-v1:0", "deepseek.r1-v1:0", "anthropic.claude-3-5-sonnet-20241022-v2:0", "cohere.command-text-v14", "anthropic.claude-opus-4-20250514-v1:0", "qwen.qwen3-coder-480b-a35b-v1:0", "anthropic.claude-sonnet-4-5-20250929-v1:0", "meta.llama3-2-3b-instruct-v1:0", "anthropic.claude-instant-v1", "amazon.nova-premier-v1:0", "anthropic.claude-opus-4-1-20250805-v1:0", "meta.llama4-scout-17b-instruct-v1:0", "ai21.jamba-1-5-mini-v1:0", "meta.llama3-8b-instruct-v1:0", "anthropic.claude-3-sonnet-20240229-v1:0", "meta.llama4-maverick-17b-instruct-v1:0", "qwen.qwen3-235b-a22b-2507-v1:0", "amazon.nova-lite-v1:0", "anthropic.claude-3-5-haiku-20241022-v1:0"},
		"github-copilot": {"claude-sonnet-4.5", "claude-haiku-4.5", "gpt-5-mini", "gpt-5", "gemini-2.0-flash-001", "claude-opus-4", "grok-code-fast-1", "claude-3.5-sonnet", "o3-mini", "gpt-5-codex", "gpt-4o", "gpt-4.1", "o4-mini", "claude-opus-41", "claude-3.7-sonnet", "gemini-2.5-pro", "o3", "claude-sonnet-4", "claude-3.7-sonnet-thought"},
	}
	sel := map[string]map[string]int{
		"opencode":       {},
		"openai":         {},
		"openrouter":     {},
		"lmstudio":       {},
		"anthropic":      {},
		"amazon-bedrock": {},
		"github-copilot": {},
		"OpenAI":         {},
	}

	providerIndex := 0

	defaults := loadDefaults()
	if defaults != nil {
		providers := []string{"github-copilot", "openai", "anthropic", "opencode", "openrouter", "lmstudio", "amazon-bedrock", "OpenAI"}
		for i, provider := range providers {
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
		providers:        []string{"github-copilot", "openai", "anthropic", "opencode", "openrouter", "lmstudio", "amazon-bedrock"},
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
		promptPastes:     map[string]string{},
		pasteCounter:     0,
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

// Paste placeholder token markers
const pasteTokenPrefix = "[[PASTE#"
const pasteTokenSuffix = "]]"

func (m *model) makePasteToken() string {
	m.pasteCounter++
	return fmt.Sprintf("%s%d%s", pasteTokenPrefix, m.pasteCounter, pasteTokenSuffix)
}

// promptDisplayWidth approximates inner content width of the prompt box.
func (m model) promptDisplayWidth() int {
	// Mirror logic from View() for promptWidth and renderRainbowBox paddings
	smallScreen := (m.width > 0 && m.width < 90) || (m.height > 0 && m.height < 20)
	promptWidth := 10
	if smallScreen {
		promptWidth = m.width - 4
		if promptWidth < 20 {
			promptWidth = m.width
		}
	} else {
		promptWidth = int(float64(m.width) * 0.5)
		if promptWidth < 40 {
			promptWidth = 40
		}
	}
	inner := promptWidth - 2 - 2*1 // borders and padH=1 in renderRainbowBox
	if inner < 1 {
		inner = 1
	}
	return inner
}

// isLongPaste determines if pasted text should be collapsed.
// Heuristic: collapse if there are >2 explicit lines, or if word-wrapped
// into ~contentWidth/6 words per line would exceed 2 lines.
func (m model) isLongPaste(s string) bool {
	if s == "" {
		return false
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 2 {
		return true
	}
	// Words-based approximation
	words := len(strings.Fields(s))
	cw := m.promptDisplayWidth()
	wordsPerLine := cw / 6
	if wordsPerLine < 1 {
		wordsPerLine = 1
	}
	approxLines := (words + wordsPerLine - 1) / wordsPerLine
	return approxLines > 2
}

// tokenRangesInLine finds all paste tokens in a line and returns byte ranges.
func tokenRangesInLine(line string) []struct {
	start, end int
	token      string
} {
	var out []struct {
		start, end int
		token      string
	}
	search := line
	base := 0
	for {
		i := strings.Index(search, pasteTokenPrefix)
		if i < 0 {
			break
		}
		start := base + i
		j := strings.Index(search[i:], pasteTokenSuffix)
		if j < 0 {
			break
		}
		end := start + (j + len(pasteTokenSuffix) + i)
		// extract token string
		tok := line[start:end]
		out = append(out, struct {
			start, end int
			token      string
		}{start: start, end: end, token: tok})
		// advance
		advance := i + j + len(pasteTokenSuffix)
		base += advance
		if advance >= len(search) {
			break
		}
		search = search[advance:]
	}
	return out
}

// tokenRangeContaining returns the token range that contains index idx, if any.
func tokenRangeContaining(line string, idx int) (start, end int, token string, ok bool) {
	for _, r := range tokenRangesInLine(line) {
		if idx >= r.start && idx < r.end {
			return r.start, r.end, r.token, true
		}
	}
	return 0, 0, "", false
}

// clampCursorOutsideToken moves col to token boundary if inside a token.
func clampCursorOutsideToken(line string, col int, moveRight bool) int {
	if col < 0 {
		return 0
	}
	if col > len(line) {
		return len(line)
	}
	if start, end, _, ok := tokenRangeContaining(line, col); ok {
		if moveRight {
			return end
		}
		return start
	}
	return col
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
		// Remove the pane/worktree from tracked state and transition to NewTask.
		// Clearing iteration input so user can enter the next task.
		if msg.PaneID != "" {
			// remove pane id from createdPanes
			var remaining []string
			for _, p := range m.createdPanes {
				if p != msg.PaneID {
					remaining = append(remaining, p)
				}
			}
			m.createdPanes = remaining
		}
		if msg.Worktree != "" {
			var remainingWT []string
			for _, w := range m.createdWorktrees {
				if w != msg.Worktree {
					remainingWT = append(remainingWT, w)
				}
			}
			m.createdWorktrees = remainingWT
		}
		if msg.Model != "" {
			delete(m.modelToPaneID, msg.Model)
			delete(m.modelToWorktree, msg.Model)
			delete(m.modelPrompts, msg.Model)
			delete(m.instanceProvider, msg.Model)
			delete(m.instanceBaseModel, msg.Model)
		}
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
		// Surface any error message
		if msg.ErrorText != "" {
			_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: %s", msg.ErrorText)})
		}
		return m, nil
	case wrapCompleteMsg:
		// Similar cleanup/state update as nextCompleteMsg, but after wrapping
		// we should quit the TUI instead of returning to the new-task screen.
		if msg.PaneID != "" {
			var remaining []string
			for _, p := range m.createdPanes {
				if p != msg.PaneID {
					remaining = append(remaining, p)
				}
			}
			m.createdPanes = remaining
		}
		if msg.Worktree != "" {
			var remainingWT []string
			for _, w := range m.createdWorktrees {
				if w != msg.Worktree {
					remainingWT = append(remainingWT, w)
				}
			}
			m.createdWorktrees = remainingWT
		}
		if msg.Model != "" {
			delete(m.modelToPaneID, msg.Model)
			delete(m.modelToWorktree, msg.Model)
			delete(m.modelPrompts, msg.Model)
			delete(m.instanceProvider, msg.Model)
			delete(m.instanceBaseModel, msg.Model)
		}
		// Reset iteration state (not strictly necessary before quitting, but keeps
		// state consistent for any callers that inspect the model).
		m.iterationInput = []string{""}
		m.iterationCursor.row = 0
		m.iterationCursor.col = 0
		m.iterationHistoryIndex = -1
		m.draftIterationInput = nil
		m.autocompleteActive = false
		m.autocompleteOptions = nil
		if msg.ErrorText != "" {
			_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: %s", msg.ErrorText)})
		}
		// Quit the TUI after wrap completes.
		return m, tea.Quit
	case cleanupCompleteMsg:
		return m, tea.Quit
	case panesOpenedMsg:
		// If any panes were successfully opened, proceed to the iteration screen
		// and record their metadata. If an error was returned but some panes
		// did open, still continue but surface a warning to the user.
		if msg.count > 0 {
			m.screen = screenIteration
			m.createdPanes = append(m.createdPanes, msg.paneIDs...)
			m.createdWorktrees = append(m.createdWorktrees, msg.worktrees...)
			// Expand any collapsed paste tokens before sending
			initialPrompt := strings.TrimSpace(m.expandTokens(strings.Join(m.input, "\n")))
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
			if msg.err != nil {
				_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Warning: some panes failed to open: %s", msg.err)})
			}
		} else if msg.err != nil {
			// Nothing opened and we have an error
			_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Failed to open panes: %s", msg.err)})
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case escTimeoutMsg:
		if m.pendingEsc {
			m.pendingEsc = false
			// ESC was pressed alone (no following key). If an exit is already
			// pending, treat this as the confirm press; otherwise start the
			// double-press exit window so the UI can show a warning.
			if m.exitPending {
				return m, cleanupCmd(m)
			}
			m.exitPending = true
			return m, tea.Tick(exitDoublePressWindow, func(t time.Time) tea.Msg { return exitTimeoutMsg{} })
		}
		return m, nil
	case exitTimeoutMsg:
		// Exit confirmation window expired; clear pending flag so the warning
		// disappears and normal input resumes.
		if m.exitPending {
			m.exitPending = false
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
					// clamp to token boundary if inside
					line := m.input[m.cursor.row]
					m.cursor.col = clampCursorOutsideToken(line, m.cursor.col, false)
				} else {
					m.cursor.row, m.cursor.col = moveWordRightLines(m.input, m.cursor.row, m.cursor.col)
					line := m.input[m.cursor.row]
					m.cursor.col = clampCursorOutsideToken(line, m.cursor.col, true)
				}
				return m, nil
			}
			// If on provider/models, ignore
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.exitPending {
				return m, cleanupCmd(m)
			}
			m.exitPending = true
			return m, tea.Tick(exitDoublePressWindow, func(t time.Time) tea.Msg { return exitTimeoutMsg{} })
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
			line := m.input[m.cursor.row]
			// prevent splitting in middle of a paste token
			if _, end, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
				// if inside token, move cursor to end before inserting newline
				m.cursor.col = end
				line = m.input[m.cursor.row]
			}
			before := line[:m.cursor.col]
			after := line[m.cursor.col:]
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
				// Do not insert spaces inside placeholder tokens
				if _, _, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
					return m, nil
				}
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
			line := m.input[m.cursor.row]
			// If cursor is inside a token, delete the whole token
			if s, e, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
				m.input[m.cursor.row] = line[:s] + line[e:]
				m.cursor.col = s
				return m, nil
			}
			if m.cursor.col > 0 {
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
				// If inside a token, delete the entire token from the line start
				if _, end, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
					m.input[m.cursor.row] = line[end:]
					m.cursor.col = 0
					return m, nil
				}
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
				newCol := m.cursor.col - 1
				line := m.input[m.cursor.row]
				if s, _, _, ok := tokenRangeContaining(line, newCol); ok {
					m.cursor.col = s
				} else {
					m.cursor.col = newCol
				}
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
				newCol := m.cursor.col + 1
				// If newCol is inside a token, jump to end of that token
				if s, e, _, ok := tokenRangeContaining(line, newCol); ok {
					_ = s
					m.cursor.col = e
				} else {
					m.cursor.col = newCol
				}
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
				// Handle bracketed paste specially in the main prompt
				if msg.Paste && m.focus == focusPrompt {
					paste := string(msg.Runes)
					line := m.input[m.cursor.row]
					// Don't insert inside a token
					if _, _, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
						return m, nil
					}
					if m.isLongPaste(paste) {
						if m.promptPastes == nil {
							m.promptPastes = make(map[string]string)
						}
						tok := m.makePasteToken()
						m.promptPastes[tok] = paste
						m.input[m.cursor.row] = line[:m.cursor.col] + tok + line[m.cursor.col:]
						m.cursor.col += len(tok)
						return m, nil
					}
					// Short paste (<=2 lines by heuristic): insert with newlines
					parts := strings.Split(paste, "\n")
					before := line[:m.cursor.col]
					after := line[m.cursor.col:]
					if len(parts) == 1 {
						m.input[m.cursor.row] = before + parts[0] + after
						m.cursor.col = len(before) + len(parts[0])
						return m, nil
					}
					// multiple lines: splice
					m.input[m.cursor.row] = before + parts[0]
					inserted := make([]string, 0, len(parts)-1)
					inserted = append(inserted, parts[1:len(parts)-1]...)
					inserted = append(inserted, parts[len(parts)-1]+after)
					m.input = append(m.input[:m.cursor.row+1], append(inserted, m.input[m.cursor.row+1:]...)...)
					m.cursor.row += len(parts) - 1
					m.cursor.col = len(parts[len(parts)-1])
					return m, nil
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
				// Avoid inserting in the middle of a placeholder token
				if _, _, _, ok := tokenRangeContaining(line, m.cursor.col); ok {
					return m, nil
				}
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
		if m.exitPending {
			return m, cleanupCmd(m)
		}
		m.exitPending = true
		return m, tea.Tick(exitDoublePressWindow, func(t time.Time) tea.Msg { return exitTimeoutMsg{} })
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

			if currentLine == "/retry" {
				m.screen = screenProgress
				m.progressMsg = "Retrying command: cleaning up and re-opening panes..."
				return m, retryCmd(m)
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
					prompt := m.expandTokens(parts[1])
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
		if m.exitPending {
			return m, cleanupCmd(m)
		}
		m.exitPending = true
		return m, tea.Tick(exitDoublePressWindow, func(t time.Time) tea.Msg { return exitTimeoutMsg{} })
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

type exitTimeoutMsg struct{}

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

type nextCompleteMsg struct {
	Model     string
	PaneID    string
	Worktree  string
	ErrorText string
}

type wrapCompleteMsg struct {
	Model     string
	PaneID    string
	Worktree  string
	ErrorText string
}

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
			prompt := m.expandTokens(strings.Join(m.input, "\n"))
			modelFull := provider + "/" + baseName
			// Append configured run command if provided
			runPart := ""
			if strings.TrimSpace(m.runCmd) != "" {
				runPart = "; " + m.runCmd
			}
			bashCmd := fmt.Sprintf("git worktree add -b %s ../%s %s || true; cd ../%s; opencode run -m %s %s%s; exec $SHELL",
				shellQuote(id), shellQuote(id), shellQuote(branchName), shellQuote(id), shellQuote(modelFull), shellQuote(prompt), runPart)

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

func retryCmd(m model) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			_, _, _ = tmux.RunCmd([]string{"display-message", "Not inside tmux; cannot retry"})
			return nil
		}

		// Kill existing panes
		for _, paneID := range m.createdPanes {
			_, _, _ = tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		// Remove existing worktrees and branches
		cwd, err := os.Getwd()
		if err != nil {
			_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Retry error: %s", err)})
			return nil
		}
		parentDir := filepath.Dir(cwd)
		for _, wt := range m.createdWorktrees {
			worktreePath := filepath.Join(parentDir, wt)
			cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
			cmd.Run()
			cmd = exec.Command("git", "branch", "-D", wt)
			cmd.Run()
		}

		// Re-open panes for the currently selected models
		models := m.selectedModels()
		if len(models) == 0 {
			_, _, _ = tmux.RunCmd([]string{"display-message", "Retry: no models selected to open"})
			return nil
		}

		// Call openPanesCmd to create panes and return its message
		return openPanesCmd(models, m)()
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

		// Kill all panes opened by this session
		for _, p := range m.createdPanes {
			_, _, _ = tmux.RunCmd([]string{"kill-pane", "-t", p})
		}

		// Remove all worktrees and branches created by this session
		for _, wt := range m.createdWorktrees {
			wtPath := filepath.Join(parentDir, wt)
			cmd = exec.Command("git", "worktree", "remove", wtPath, "--force")
			cmd.Run()
			cmd = exec.Command("git", "branch", "-D", wt)
			cmd.Run()
		}

		msgText := fmt.Sprintf("Next complete: merged %s and cleaned up %d pane(s)", modelName, len(m.createdPanes))
		_, _, _ = tmux.RunCmd([]string{"display-message", msgText})

		return nextCompleteMsg{Model: modelName, PaneID: "", Worktree: ""}
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

		// Kill the pane associated with this model (if known)
		paneID := m.modelToPaneID[modelName]
		if paneID != "" {
			_, _, _ = tmux.RunCmd([]string{"kill-pane", "-t", paneID})
		}

		// Also close any other panes opened by this session
		for _, p := range m.createdPanes {
			if p == paneID {
				continue
			}
			_, _, _ = tmux.RunCmd([]string{"kill-pane", "-t", p})
		}

		// Remove this worktree/branch
		wtPath := filepath.Join(parentDir, worktree)
		cmd = exec.Command("git", "worktree", "remove", wtPath, "--force")
		cmd.Run()

		cmd = exec.Command("git", "branch", "-D", worktree)
		cmd.Run()

		// Remove other worktrees/branches created by this session
		for _, other := range m.createdWorktrees {
			if other == worktree {
				continue
			}
			otherPath := filepath.Join(parentDir, other)
			cmd = exec.Command("git", "worktree", "remove", otherPath, "--force")
			cmd.Run()
			cmd = exec.Command("git", "branch", "-D", other)
			cmd.Run()
		}

		msgText := fmt.Sprintf("Wrap complete: merged %s and cleaned up %d worktree(s)", modelName, len(m.createdWorktrees))
		if paneID == "" {
			msgText = fmt.Sprintf("Wrap complete: merged %s (pane not found) and cleaned up %d worktree(s)", modelName, len(m.createdWorktrees))
		}
		_, _, _ = tmux.RunCmd([]string{"display-message", msgText})

		return wrapCompleteMsg{Model: modelName, PaneID: paneID, Worktree: worktree}

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
		// Append configured run command if provided, mirroring openPanesCmd behavior
		runPart := ""
		if strings.TrimSpace(m.runCmd) != "" {
			runPart = "; " + m.runCmd
		}
		bashCmd := fmt.Sprintf("opencode run -m %s %s%s", shellQuote(modelFull), shellQuote(prompt), runPart)

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

func compactHeader(width int) string {
	// Small, single-line header used for narrow/small terminals
	if width <= 0 {
		width = 80
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6BCB77")).Render("KALEIDOSCOPE")
	subtitle := lipgloss.NewStyle().Faint(true).Render(" • opencode")
	line := title + subtitle
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, line)
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

	// Choose compact header for small screens to avoid huge ASCII art
	smallScreen := (m.width > 0 && m.width < 90) || (m.height > 0 && m.height < 20)
	var header string
	if smallScreen {
		header = compactHeader(m.width)
	} else {
		header = rainbowHeader(m.width)
	}
	spacer := "\n\n"

	// Dimensions (be conservative when terminal reports 0)
	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}
	maxHeight := m.height
	if maxHeight <= 0 {
		maxHeight = 24
	}

	// Adaptive sizing
	var promptWidth, branchWidth, selectedWidth int
	if smallScreen {
		// Stack elements vertically and use almost full width
		promptWidth = maxWidth - 4
		if promptWidth < 20 {
			promptWidth = maxWidth
		}
		branchWidth = promptWidth
		selectedWidth = promptWidth
	} else {
		// Wider layout: split into columns but allow smaller minimums
		promptWidth = int(float64(maxWidth) * 0.5)
		if promptWidth < 40 {
			promptWidth = 40
		}
		branchWidth = int(float64(maxWidth) * 0.25)
		if branchWidth < 20 {
			branchWidth = 20
		}
		if branchWidth > 40 {
			branchWidth = 40
		}
		selectedWidth = int(float64(maxWidth) * 0.18)
		if selectedWidth < 16 {
			selectedWidth = 16
		}
		if selectedWidth > 36 {
			selectedWidth = 36
		}
	}

	// Prompt height should adapt to available space
	promptHeight := 10
	if smallScreen {
		// leave minimal vertical space for prompt when height is small
		promptHeight = max(3, maxHeight/4)
	} else {
		if maxHeight < 30 {
			promptHeight = max(6, maxHeight/3)
		}
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
		if len(bRight) > 0 {
			// Replace the character under the cursor with a reversed-style version
			cursor := lipgloss.NewStyle().Reverse(true).Render(string(bRight[0]))
			branchInner = bLeft + cursor + bRight[1:]
		} else {
			cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
			branchInner = bLeft + cursor + bRight
		}
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
		if len(tRight) > 0 {
			cursor := lipgloss.NewStyle().Reverse(true).Render(string(tRight[0]))
			taskInner = tLeft + cursor + tRight[1:]
		} else {
			cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
			taskInner = tLeft + cursor + tRight
		}
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
		Padding(0, 1)
	// task box shares width with branch box
	taskBox := lipgloss.NewStyle().
		Width(branchWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(taskBorder).
		Padding(0, 1)

	branchLabel := lipgloss.NewStyle().Faint(true).Render("branch-name")
	taskLabel := lipgloss.NewStyle().Faint(true).Render("task-name")
	branchView := branchLabel + "\n" + branchBox.Render(branchInner) + "\n\n" + taskLabel + "\n" + taskBox.Render(taskInner)

	// Render prompt buffer with block cursor, showing collapsed paste tokens
	var pb strings.Builder
	pasteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B967FF")).Bold(true)
	for i, line := range m.input {
		// Render line with tokens highlighted
		renderLine := func(s string) string {
			if s == "" {
				return ""
			}
			var b strings.Builder
			pos := 0
			for _, r := range tokenRangesInLine(s) {
				if r.start > pos {
					b.WriteString(s[pos:r.start])
				}
				b.WriteString(pasteStyle.Render("[PASTED TEXT]"))
				pos = r.end
			}
			if pos < len(s) {
				b.WriteString(s[pos:])
			}
			return b.String()
		}

		if i == m.cursor.row {
			col := m.cursor.col
			if col > len(line) {
				col = len(line)
			}
			// If cursor inside a token, render cursor as reverse on the placeholder
			if start, end, _, ok := tokenRangeContaining(line, col); ok {
				left := renderLine(line[:start])
				mid := pasteStyle.Render("[PASTED TEXT]")
				if m.focus == focusPrompt && m.cursorVisible {
					mid = lipgloss.NewStyle().Reverse(true).Render(mid)
				}
				right := renderLine(line[end:])
				pb.WriteString(left + mid + right)
			} else {
				// Normal cursor behavior on non-token text: show reversed space at boundary
				pb.WriteString(renderLine(line[:col]))
				if m.focus == focusPrompt && m.cursorVisible {
					pb.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
				}
				pb.WriteString(renderLine(line[col:]))
			}
		} else {
			pb.WriteString(renderLine(line))
		}
		if i < len(m.input)-1 {
			pb.WriteString("\n")
		}
	}

	promptView := renderRainbowBox(pb.String(), promptWidth, promptHeight, 1, 1)

	// Selected models column next to the prompt
	selectedCol := m.renderSelectedColumn(selectedWidth)

	if smallScreen {
		// Stack vertically: header, branch, prompt, selected, provider/models
		provView := ""
		// provider+models row simplified for small screens
		if !m.providerOpen {
			current := m.providers[m.providerIndex]
			provLabel := lipgloss.NewStyle().Faint(true).Render("provider")
			provBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#6BCB77")).Padding(0, 1)
			provView = provLabel + "\n" + provBox.Render(current+"  ▾")
		} else {
			provView = m.renderModelsDropdown(promptWidth)
		}

		parts := []string{header, spacer, branchView, "\n", promptView, "\n", selectedCol, "\n", provView}
		body := strings.Join(parts, "\n")
		return m.renderWithBottomBar(body)
	}

	// Wider layout: place branch | prompt | selected horizontally
	topGap := "  "
	row := lipgloss.JoinHorizontal(lipgloss.Top, branchView, topGap, promptView, topGap, selectedCol)
	centeredRow := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, row)

	// Provider + Models dropdown row (same visual width as prompt)
	// Compute widths
	provWidth := promptWidth / 2
	if provWidth < 20 {
		provWidth = 20
	}
	gap := "  "
	modelsWidth := promptWidth - provWidth - lipgloss.Width(gap)
	if modelsWidth < 16 {
		modelsWidth = 16
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
			Padding(0, 1)
		provView := provLabel + "\n" + provBox.Render(current+"  ▾")

		// Models collapsed or open
		modelsView := m.renderModelsDropdown(modelsWidth)

		pair := lipgloss.JoinHorizontal(lipgloss.Top, provView, gap, modelsView)
		pairCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pair)

		hint := lipgloss.NewStyle().Faint(true).Render("tab: next field • ↑↓: navigate • space: select models • enter: submit")
		hintCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hint)

		body := header + spacer + centeredRow + "\n\n" + pairCentered + "\n\n" + hintCentered
		return m.renderWithBottomBar(body)
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
		Padding(0, 1)
	provOpenView := provLabel + "\n" + provOpenBox.Render(list.String())

	modelsView := m.renderModelsDropdown(modelsWidth)
	pair := lipgloss.JoinHorizontal(lipgloss.Top, provOpenView, gap, modelsView)
	pairCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pair)

	hint := lipgloss.NewStyle().Faint(true).Render("tab: next field • ↑↓: navigate • space: select models • enter: submit")
	hintCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hint)

	body := header + spacer + centeredRow + "\n\n" + pairCentered + "\n\n" + hintCentered
	return m.renderWithBottomBar(body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m model) viewIteration() string {
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

	// Calculate available height for prompt area, accounting for header, hints, and bottom bar
	// Hints (label, commands hint, tmux hint) = 3 lines
	// Bottom bar = 1-2 lines
	// Leave small buffer above bottom bar for breathing room
	reservedTop := 2    // minimal spacing at top
	reservedBottom := 5 // hints + buffer + bottom bar
	availableHeight := m.height - reservedTop - reservedBottom
	if availableHeight < 8 {
		availableHeight = 8
	}
	if availableHeight > 20 {
		availableHeight = 20
	}
	promptHeight := availableHeight

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

			// If cursor is visible, prefer replacing the character under it.
			if m.cursorVisible {
				// compute rune index corresponding to byte col
				runes := []rune(line)
				byteIndex := 0
				runeIndex := 0
				for i := range runes {
					if byteIndex >= col {
						break
					}
					byteIndex += len(string(runes[i]))
					runeIndex++
				}
				// If cursor is inside the line, render highlighted with cursor at runeIndex
				if runeIndex < len(runes) {
					pb.WriteString(highlightCommandLineWithCursor(line, mentionables, runeIndex))
					continue
				}
				// End-of-line: render highlighted left, reversed space, then highlighted right
				pb.WriteString(highlightCommandLine(line[:col], mentionables))
				pb.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
				pb.WriteString(highlightCommandLine(line[col:], mentionables))
				continue
			}

			pb.WriteString(highlightCommandLine(line, mentionables))

		} else {
			pb.WriteString(highlightCommandLine(line, mentionables))
		}
		if i < len(m.iterationInput)-1 {
			pb.WriteString("\n")
		}
	}

	label := lipgloss.NewStyle().Faint(true).Render("iteration prompt")
	hint := lipgloss.NewStyle().Faint(true).Render("commands: /bail /retry /next <instance> /wrap <instance> | @<instance> <prompt>")
	tmuxHint := lipgloss.NewStyle().Faint(true).Render("tmux: Ctrl-b then arrow keys to move between panes")
	// Use the rainbow border renderer for the main iteration prompt
	promptView := label + "\n" + renderRainbowBox(pb.String(), promptWidth, promptHeight, 1, 2) + "\n" + hint + "\n" + tmuxHint

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

	// Position content with minimal spacing at top
	body := centeredPrompt
	return m.renderWithBottomBar(body)
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
				if col < len(line) {
					// Replace rune under cursor
					runes := []rune(line)
					byteIndex := 0
					runeIndex := 0
					for i := range runes {
						if byteIndex >= col {
							break
						}
						byteIndex += len(string(runes[i]))
						runeIndex++
					}
					if runeIndex < len(runes) {
						curBlock := lipgloss.NewStyle().Reverse(true).Render(string(runes[runeIndex]))
						pb.WriteString(curBlock)
						pb.WriteString(string(runes[runeIndex+1:]))
						continue
					}
				}
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

	// Use rainbow border renderer for the new-task prompt instead of lipgloss BorderForeground
	promptView := renderRainbowBox(pb.String(), promptWidth, promptHeight, 1, 2)

	topGap := "  "
	row := lipgloss.JoinHorizontal(lipgloss.Top, taskView, topGap, promptView)
	centeredRow := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, row)

	body := header + "\n\n" + centeredRow
	return m.renderWithBottomBar(body)
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
	body := header + "\n\n" + centeredVertical
	return m.renderWithBottomBar(body)
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
		"/bail":  true,
		"/next":  true,
		"/wrap":  true,
		"/retry": true,
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

// highlightCommandLineWithCursor renders a command line with the same
// highlighting rules as highlightCommandLine but applies a reverse
// style to the rune at cursorRuneIndex (if it is within the line).
// If cursorRuneIndex is out of range (<0 or >= len(runes)) the line is
// returned highlighted without a reversed cursor. This keeps cursor
// rendering in a single pass and ensures slash/mention tokens are
// styled and reversed together.
func highlightCommandLineWithCursor(line string, selectedModels []string, cursorRuneIndex int) string {
	// If no content, nothing to highlight; caller should handle EOL cursor.
	if line == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(line)

	slashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F7B801")).Bold(true)
	atStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6BCB77")).Bold(true)

	validSlashCommands := map[string]bool{
		"/bail":  true,
		"/next":  true,
		"/wrap":  true,
		"/retry": true,
	}

	modelSet := make(map[string]bool)
	for _, m := range selectedModels {
		modelSet[m] = true
	}

	for i := 0; i < len(runes); {
		if runes[i] == '/' {
			start := i
			i++
			for i < len(runes) && (runes[i] >= 'a' && runes[i] <= 'z' || runes[i] >= 'A' && runes[i] <= 'Z' || runes[i] == '-' || runes[i] == '_') {
				i++
			}
			cmd := string(runes[start:i])
			out := cmd
			if validSlashCommands[cmd] {
				out = slashStyle.Render(cmd)
			}
			if cursorRuneIndex >= start && cursorRuneIndex < i {
				out = lipgloss.NewStyle().Reverse(true).Render(out)
			}
			result.WriteString(out)
		} else if runes[i] == '@' {
			start := i
			i++
			for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' && runes[i] != '\n' {
				i++
			}
			mention := string(runes[start:i])
			modelName := ""
			if len(mention) > 1 {
				modelName = mention[1:]
			}
			out := mention
			if modelSet[modelName] {
				out = atStyle.Render(mention)
			}
			if cursorRuneIndex >= start && cursorRuneIndex < i {
				out = lipgloss.NewStyle().Reverse(true).Render(out)
			}
			result.WriteString(out)
		} else {
			// Normal rune
			if i == cursorRuneIndex {
				result.WriteString(lipgloss.NewStyle().Reverse(true).Render(string(runes[i])))
			} else {
				result.WriteRune(runes[i])
			}
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

func (m model) renderPathBar() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	if home, e := os.UserHomeDir(); e == nil && home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + strings.TrimPrefix(cwd, home)
	}

	// Prepare label and path content (make text blue)
	label := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#6BCB77")).Render("cwd")
	path := cwd

	// Determine available inner width (subtract padding/borders)
	available := m.width - 10
	if available < 10 {
		available = 10
	}
	if lipgloss.Width(path) > available {
		r := []rune(path)
		keep := available - 3
		if keep <= 0 {
			path = "…"
		} else {
			start := keep / 2
			end := keep - start
			if start < 0 {
				start = 0
			}
			if end < 0 {
				end = 0
			}
			if start+end > len(r) {
				// Fallback simple truncation
				path = string(r[:available])
			} else {
				path = string(r[:start]) + "…" + string(r[len(r)-end:])
			}
		}
	}

	content := label + " " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6BCB77")).Render("> "+path)
	// Let the bar size to the content instead of forcing full terminal width
	bar := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#D896FF")).Padding(0, 2).Render(content)
	// Return the styled bar itself (no placement), caller will anchor it to the bottom
	return bar
}

// renderWithBottomBar pads `body` so that the path bar appears on the last
// terminal line (anchored to the bottom-left). If the terminal height is
// unknown or too small, fall back to appending the bar with a small gap.
func (m model) renderWithBottomBar(body string) string {
	bar := m.renderPathBar()
	// If exit warning is active, render a red warning line above the bar and
	// account for its height when padding.
	warning := ""
	warningLines := 0
	if m.exitPending {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true)
		warning = warnStyle.Render("Press Esc or Ctrl+C again to exit")
		warningLines = 1
	}
	if m.height <= 0 {
		// Unknown height: preserve previous spacing
		if warning != "" {
			return body + "\n\n" + warning + "\n" + bar
		}
		return body + "\n\n" + bar
	}
	// Count lines in body, warning, and bar
	bodyLines := 0
	if body != "" {
		bodyLines = strings.Count(body, "\n") + 1
	}
	barLines := 0
	if bar != "" {
		barLines = strings.Count(bar, "\n") + 1
	}
	pad := m.height - bodyLines - barLines - warningLines
	if pad <= 0 {
		// Not enough room: keep one blank line between content, optional warning, and bar
		if warning != "" {
			return body + "\n\n" + warning + "\n" + bar
		}
		return body + "\n\n" + bar
	}
	out := body + strings.Repeat("\n", pad)
	if warning != "" {
		out += warning + "\n"
	}
	out += bar
	return out
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

// renderRainbowBox renders `content` inside a rounded box whose border is
// colored with a left→right rainbow gradient. `width` and `height` indicate
// the full box dimensions including borders. `padV` and `padH` specify the
// inner vertical and horizontal padding (in spaces).
func renderRainbowBox(content string, width, height, padV, padH int) string {
	// Fallback: if width is too small, just return content
	if width <= 0 {
		return content
	}
	if width < 4 || height < 3 {
		// Not enough room for a border; just trim/pad content to width
		lines := strings.Split(content, "\n")
		for i := range lines {
			runes := []rune(lines[i])
			if len(runes) > width {
				lines[i] = string(runes[:width])
			}
		}
		return strings.Join(lines, "\n")
	}

	// Compute inner width (space available for padding + content)
	totalInner := width - 2 // exclude left/right border chars
	if totalInner < 1 {
		return content
	}

	// Build a palette spanning the horizontal span of the inner area
	stops := []string{"#4D96FF", "#6BCB77", "#F7B801", "#FF6B6B", "#B967FF"}
	palette := gradientColors(totalInner, stops)

	// Prepare content lines and wrap/pad them to the inner content width
	innerContentWidth := totalInner - 2*padH
	if innerContentWidth < 0 {
		innerContentWidth = 0
	}
	rawLines := strings.Split(content, "\n")
	// Ensure we have exactly the number of content lines that fit into height
	maxContentLines := height - 2 - 2*padV // exclude top/bottom borders and vertical padding
	if maxContentLines < 0 {
		maxContentLines = 0
	}
	var contentLines []string

	// Helper: take up to `innerContentWidth` display-width worth of runes
	for _, ln := range rawLines {
		r := []rune(ln)
		for len(r) > 0 {
			if innerContentWidth == 0 {
				// If no room, push an empty padded line and break to avoid infinite loop
				contentLines = append(contentLines, strings.Repeat(" ", innerContentWidth))
				break
			}
			// Find the largest slice of runes that fits within innerContentWidth
			chunkRunes := 0
			for j := 1; j <= len(r); j++ {
				if lipgloss.Width(string(r[:j])) > innerContentWidth {
					break
				}
				chunkRunes = j
			}
			if chunkRunes == 0 {
				// force at least one rune to avoid infinite loop; it may overflow visually
				chunkRunes = 1
			}
			chunk := string(r[:chunkRunes])
			// right-pad the chunk to innerContentWidth using display width
			padNeeded := innerContentWidth - lipgloss.Width(chunk)
			if padNeeded < 0 {
				padNeeded = 0
			}
			chunk = chunk + strings.Repeat(" ", padNeeded)
			contentLines = append(contentLines, chunk)
			// advance
			if chunkRunes >= len(r) {
				r = r[:0]
			} else {
				r = r[chunkRunes:]
			}
			// If we've collected more than enough lines, we can optionally stop early
			if len(contentLines) >= maxContentLines {
				break
			}
		}
		if len(contentLines) >= maxContentLines {
			break
		}
	}

	// Trim or pad contentLines to fit maxContentLines
	if len(contentLines) > maxContentLines {
		contentLines = contentLines[:maxContentLines]
	} else if len(contentLines) < maxContentLines {
		for len(contentLines) < maxContentLines {
			contentLines = append(contentLines, strings.Repeat(" ", innerContentWidth))
		}
	}

	// Build top border colored across totalInner columns
	var topBuilder strings.Builder
	// Color the left-top corner with the first palette color
	topBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette[0])).Render("╭"))
	for i := 0; i < totalInner; i++ {
		c := lipgloss.Color(palette[i])
		topBuilder.WriteString(lipgloss.NewStyle().Foreground(c).Render("─"))
	}
	// Color the right-top corner with the last palette color
	topBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette[len(palette)-1])).Render("╮"))
	top := topBuilder.String()

	// Build bottom border
	var bottomBuilder strings.Builder
	// Color the left-bottom corner with the first palette color
	bottomBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette[0])).Render("╰"))
	for i := 0; i < totalInner; i++ {
		c := lipgloss.Color(palette[i])
		bottomBuilder.WriteString(lipgloss.NewStyle().Foreground(c).Render("─"))
	}
	// Color the right-bottom corner with the last palette color
	bottomBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette[len(palette)-1])).Render("╯"))
	bottom := bottomBuilder.String()

	// Colors for left and right vertical bars: use ends of palette
	leftColor := lipgloss.Color(palette[0])
	rightColor := lipgloss.Color(palette[len(palette)-1])

	var bodyBuilder strings.Builder
	bodyBuilder.WriteString(top)
	bodyBuilder.WriteString("\n")

	// Top vertical padding
	for pv := 0; pv < padV; pv++ {
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(leftColor).Render("│"))
		bodyBuilder.WriteString(strings.Repeat(" ", totalInner))
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(rightColor).Render("│"))
		bodyBuilder.WriteString("\n")
	}

	// Content rows
	for _, ln := range contentLines {
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(leftColor).Render("│"))
		// left horizontal padding
		bodyBuilder.WriteString(strings.Repeat(" ", padH))
		// content (already padded/truncated to innerContentWidth)
		bodyBuilder.WriteString(ln)
		// right horizontal padding
		bodyBuilder.WriteString(strings.Repeat(" ", padH))
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(rightColor).Render("│"))
		bodyBuilder.WriteString("\n")
	}

	// Bottom vertical padding
	for pv := 0; pv < padV; pv++ {
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(leftColor).Render("│"))
		bodyBuilder.WriteString(strings.Repeat(" ", totalInner))
		bodyBuilder.WriteString(lipgloss.NewStyle().Foreground(rightColor).Render("│"))
		bodyBuilder.WriteString("\n")
	}

	bodyBuilder.WriteString(bottom)

	// Place horizontally to requested width (so caller can center)
	return lipgloss.PlaceHorizontal(width, lipgloss.Left, bodyBuilder.String())
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

// expandTokens replaces all placeholder paste tokens with their original content.
func (m model) expandTokens(s string) string {
	if m.promptPastes == nil || len(m.promptPastes) == 0 || s == "" {
		return s
	}
	out := s
	for tok, val := range m.promptPastes {
		out = strings.ReplaceAll(out, tok, val)
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
		commands := []string{"/bail", "/next", "/wrap", "/retry"}
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
	run := flag.String("run", "", "run command (optional)")
	setDefault := flag.Bool("set-default", false, "save chosen provider and models as defaults in .kaleidoscope")
	// tmux session name can be set via env var or flag
	tmuxDefault := os.Getenv("KALEIDOSCOPE_TMUX_SESSION")
	if tmuxDefault == "" {
		tmuxDefault = "kaleidoscope"
	}
	tmuxSession := flag.String("tmux-session", tmuxDefault, "tmux session name (env KALEIDOSCOPE_TMUX_SESSION)")
	flag.Parse()

	// --run is optional; when omitted no extra command will be appended after the opencode call.
	if !tmux.IsInsideTmux() {
		// Ensure tmux binary exists before attempting to exec it so we can
		// provide a clear error message if it's missing.
		if _, err := exec.LookPath("tmux"); err != nil {
			fmt.Fprintln(os.Stderr, "tmux not found in PATH; please install tmux or run this program inside a tmux session")
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, "Not inside a tmux session; attempting to start or attach one...")

		// Build tmux args to create a new, unique session and run this program inside it.
		// To avoid multiple invocations attaching to the same session we always create
		// a session with a unique suffix (pid + timestamp) rather than using '-A'.
		sessionName := *tmuxSession
		uniqueSession := fmt.Sprintf("%s-%d-%d", sessionName, os.Getpid(), time.Now().UnixNano())
		fmt.Fprintln(os.Stderr, "Starting tmux session:", uniqueSession)
		tmuxArgs := append([]string{"new-session", "-s", uniqueSession, "--", os.Args[0]}, os.Args[1:]...)
		cmd := exec.Command("tmux", tmuxArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Run will start a new tmux session and execute the program inside it.
		// If it succeeds, the child tmux session will run the program and we should
		// exit this parent process.
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error: failed to start tmux session:", err)
			fmt.Fprintln(os.Stderr, "Please start tmux and re-run")
			os.Exit(1)
		}
		// If tmux started and ran our program, exit the original process now.
		os.Exit(0)
	}

	p := tea.NewProgram(initialModel(*run, *setDefault), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
