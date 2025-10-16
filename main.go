package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tmux "github.com/jubnzv/go-tmux"
)

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
			if sel[name] {
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
	selected    map[string]map[string]bool // provider -> model -> selected
	modelsOpen  bool
	modelsHover int

	// Focus
	focus focusType
}

func initialModel() model {
	mods := map[string][]string{
		"Github": {"sonnet-4.5", "gpt-5", "gemini-2.5"},
		"OpenAI": {"gpt-5", "gpt-5-codex", "gpt-5-mini"},
	}
	// initialize empty selections per provider
	sel := map[string]map[string]bool{
		"Github": {},
		"OpenAI": {},
	}
	m := model{
		input:         []string{""},
		branch:        "",
		task:          "",
		providers:     []string{"Github", "OpenAI"},
		providerIndex: 0,
		providerOpen:  false,
		providerHover: 0,
		models:        mods,
		selected:      sel,
		modelsOpen:    false,
		modelsHover:   0,
		focus:         focusPrompt,
	}
	return m
}

func (m model) Init() tea.Cmd { return nil }

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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
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
			// Space toggles selection when in models multiselect and open.
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
					m.selected[p] = map[string]bool{}
				}
				name := opts[m.modelsHover]
				m.selected[p][name] = !m.selected[p][name]
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
				if m.modelsOpen {
					m.modelsOpen = false
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
				if m.cursor.row > 0 {
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
				if m.cursor.row < len(m.input)-1 {
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

func (m model) View() string {
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
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	branchInner := bLeft + cursor + bRight

	// Render task single-line with cursor
	tline := m.task
	if m.taskCursor > len(tline) {
		m.taskCursor = len(tline)
	}
	tLeft := tline[:m.taskCursor]
	tRight := tline[m.taskCursor:]
	taskInner := tLeft + cursor + tRight

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
			curBlock := lipgloss.NewStyle().Reverse(true).Render(" ")
			pb.WriteString(line[:col])
			pb.WriteString(curBlock)
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

		return header + spacer + centeredRow + "\n\n" + pairCentered
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

	return header + spacer + centeredRow + "\n\n" + pairCentered
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
		// collapsed: show count selected
		count := 0
		p := m.currentProvider()
		if m.selected[p] != nil {
			for _, v := range m.selected[p] {
				if v {
					count++
				}
			}
		}
		labelText := "Select models…  ▾"
		if count > 0 {
			labelText = fmt.Sprintf("%d selected  ▾", count)
		}
		return label + "\n" + box.Render(labelText)
	}

	// open: list with checkboxes
	var list strings.Builder
	p := m.currentProvider()
	sel := m.selected[p]
	for i, opt := range opts {
		checked := "[ ]"
		if sel != nil && sel[opt] {
			checked = "[x]"
		}
		row := checked + " " + opt
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
		if sel != nil && sel[name] {
			lines = append(lines, "• "+name)
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
		if sel[name] {
			out = append(out, name)
		}
	}
	return out
}

// panesOpenedMsg reports how many panes were opened and any error
type panesOpenedMsg struct {
	count int
	err   error
}

// openPanesCmd splits the current tmux window once per model and tiles layout
func openPanesCmd(models []string, m model) tea.Cmd {
	return func() tea.Msg {
		if !tmux.IsInsideTmux() {
			_, _, _ = tmux.RunCmd([]string{"display-message", "Not inside tmux; cannot open panes"})
			return panesOpenedMsg{0, fmt.Errorf("not inside tmux")}
		}

		// Capture the current pane id to restore focus later
		paneOut, _, err := tmux.RunCmd([]string{"display-message", "-p", "#{pane_id}"})
		if err != nil {
			return panesOpenedMsg{0, err}
		}
		origPaneID := strings.TrimSpace(paneOut)

		opened := 0
		var lastErr error
		for _, name := range models {
			id := m.identifierFor(name)
			// Use split-window to run the git commands in the new pane directly.
			// Request the new pane id with -P -F "#{pane_id}" so we can target it if needed.
			bashCmd := fmt.Sprintf("git worktree add -b '%s' ../%s || true; cd ../%s; exec $SHELL", id, id, id)
			out, _, err := tmux.RunCmd([]string{"split-window", "-v", "-P", "-F", "#{pane_id}", "bash", "-lc", bashCmd})
			if err != nil {
				lastErr = err
				// continue attempting remaining panes
				continue
			}
			_ = out
			opened++
		}

		// Arrange panes nicely
		_, _, _ = tmux.RunCmd([]string{"select-layout", "tiled"})

		// Restore focus to the original pane
		_, _, _ = tmux.RunCmd([]string{"select-pane", "-t", origPaneID})

		// Inform in tmux status line
		_, _, _ = tmux.RunCmd([]string{"display-message", fmt.Sprintf("Opened %d pane(s)", opened)})

		return panesOpenedMsg{opened, lastErr}
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
