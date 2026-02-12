package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// testModel builds a model pre-configured for tests.  It bypasses
// initialModel so we avoid git/file-system side-effects but still
// get a realistic starting state.
func testModel() model {
	providers := []string{"github-copilot", "openai", "anthropic"}
	models := map[string][]string{
		"github-copilot": {"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"},
		"openai":         {"gpt-5", "gpt-5-mini"},
		"anthropic":      {"claude-sonnet-4-5"},
	}
	selected := map[string]map[string]int{
		"github-copilot": {
			"claude-sonnet-4.5": 1,
			"gpt-5-mini":        1,
			"gemini-2.5-pro":    1,
		},
		"openai":    {},
		"anthropic": {},
	}

	return model{
		width:            120,
		height:           40,
		input:            []string{"Implement JWT authentication"},
		branch:           "feature-auth",
		branchCursor:     len("feature-auth"),
		task:             "auth",
		taskCursor:       len("auth"),
		providers:        providers,
		providerIndex:    0,
		models:           models,
		selected:         selected,
		focus:            focusPrompt,
		screen:           screenSetup,
		iterationInput:   []string{""},
		createdPanes:     []string{},
		createdWorktrees: []string{},
		modelToPaneID:    map[string]string{},
		modelToWorktree:  map[string]string{},
		modelPrompts:     map[string][]string{},
		instanceProvider: map[string]string{},
		instanceBaseModel: map[string]string{},
		promptPastes:     map[string]string{},
		newTaskPrompt:    []string{""},
		newTaskFocus:     focusTask,
		cursorVisible:    true,
		spinnerFrames:    []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		history:          []string{},
		historyIndex:     -1,
		iterationHistoryIndex: -1,
	}
}

// threePanesOpened returns a panesOpenedMsg that simulates 3 models being
// opened successfully, as if openPanesCmd ran.
func threePanesOpened() panesOpenedMsg {
	return panesOpenedMsg{
		count:      3,
		err:        nil,
		paneIDs:    []string{"%10", "%11", "%12"},
		worktrees:  []string{"repo_feature-auth_auth_claude-sonnet-4.5", "repo_feature-auth_auth_gpt-5-mini", "repo_feature-auth_auth_gemini-2.5-pro"},
		modelNames: []string{"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"},
		providers:  []string{"github-copilot", "github-copilot", "github-copilot"},
		baseModels: []string{"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"},
	}
}

// --------------------------------------------------------------------------
// Tests: branch + workspace creation
// --------------------------------------------------------------------------

func TestPanesOpenedTransitionsToIterationScreen(t *testing.T) {
	m := testModel()

	// Before panes open, should NOT be on iteration screen.
	if m.screen == screenIteration {
		t.Fatal("expected screen to NOT be screenIteration before panes opened")
	}

	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	if m.screen != screenIteration {
		t.Fatalf("expected screenIteration after panesOpenedMsg, got %d", m.screen)
	}
}

func TestPanesOpenedRecordsPaneIDs(t *testing.T) {
	m := testModel()
	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	if len(m.createdPanes) != 3 {
		t.Fatalf("expected 3 created panes, got %d", len(m.createdPanes))
	}
	expected := []string{"%10", "%11", "%12"}
	for i, id := range expected {
		if m.createdPanes[i] != id {
			t.Errorf("createdPanes[%d] = %q, want %q", i, m.createdPanes[i], id)
		}
	}
}

func TestPanesOpenedRecordsWorktrees(t *testing.T) {
	m := testModel()
	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	if len(m.createdWorktrees) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(m.createdWorktrees))
	}

	for _, wt := range m.createdWorktrees {
		if wt == "" {
			t.Error("worktree entry should not be empty")
		}
	}
}

func TestPanesOpenedPopulatesModelMappings(t *testing.T) {
	m := testModel()
	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	modelNames := []string{"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"}
	paneIDs := []string{"%10", "%11", "%12"}
	worktrees := []string{
		"repo_feature-auth_auth_claude-sonnet-4.5",
		"repo_feature-auth_auth_gpt-5-mini",
		"repo_feature-auth_auth_gemini-2.5-pro",
	}

	for i, name := range modelNames {
		if got := m.modelToPaneID[name]; got != paneIDs[i] {
			t.Errorf("modelToPaneID[%q] = %q, want %q", name, got, paneIDs[i])
		}
		if got := m.modelToWorktree[name]; got != worktrees[i] {
			t.Errorf("modelToWorktree[%q] = %q, want %q", name, got, worktrees[i])
		}
		if prompts := m.modelPrompts[name]; len(prompts) != 1 {
			t.Errorf("modelPrompts[%q] has %d entries, want 1", name, len(prompts))
		}
	}
}

func TestPanesOpenedRecordsInstanceMetadata(t *testing.T) {
	m := testModel()
	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	modelNames := []string{"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"}
	for _, name := range modelNames {
		if prov := m.instanceProvider[name]; prov != "github-copilot" {
			t.Errorf("instanceProvider[%q] = %q, want %q", name, prov, "github-copilot")
		}
	}

	expectedBase := []string{"claude-sonnet-4.5", "gpt-5-mini", "gemini-2.5-pro"}
	for i, name := range modelNames {
		if base := m.instanceBaseModel[name]; base != expectedBase[i] {
			t.Errorf("instanceBaseModel[%q] = %q, want %q", name, base, expectedBase[i])
		}
	}
}

func TestPanesOpenedWithZeroCountStaysOnCurrentScreen(t *testing.T) {
	m := testModel()
	m.screen = screenSetup

	msg := panesOpenedMsg{count: 0, err: fmt.Errorf("not inside tmux")}
	result, _ := m.Update(msg)
	m = result.(model)

	if m.screen != screenSetup {
		t.Fatalf("expected screen to remain screenSetup when 0 panes opened, got %d", m.screen)
	}
	if len(m.createdPanes) != 0 {
		t.Fatalf("expected no panes tracked, got %d", len(m.createdPanes))
	}
}

func TestPanesOpenedPartialErrorStillTransitions(t *testing.T) {
	// If 2 of 3 panes opened but one failed, we should still transition.
	msg := panesOpenedMsg{
		count:      2,
		err:        fmt.Errorf("one pane failed"),
		paneIDs:    []string{"%10", "%11"},
		worktrees:  []string{"wt1", "wt2"},
		modelNames: []string{"model-a", "model-b"},
		providers:  []string{"openai", "openai"},
		baseModels: []string{"model-a", "model-b"},
	}
	m := testModel()
	result, _ := m.Update(msg)
	m = result.(model)

	if m.screen != screenIteration {
		t.Fatal("expected screenIteration even when partial error")
	}
	if len(m.createdPanes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(m.createdPanes))
	}
}

// --------------------------------------------------------------------------
// Tests: selecting the 1st choice (/next) and teardown
// --------------------------------------------------------------------------

// setupIterationModel creates a model that is already on the iteration screen
// with 3 panes/worktrees tracked, simulating the state after openPanesCmd.
func setupIterationModel() model {
	m := testModel()
	result, _ := m.Update(threePanesOpened())
	return result.(model)
}

func TestNextCompleteRemovesSelectedModelState(t *testing.T) {
	m := setupIterationModel()

	// Verify pre-conditions: model "claude-sonnet-4.5" is tracked
	if _, ok := m.modelToPaneID["claude-sonnet-4.5"]; !ok {
		t.Fatal("precondition failed: claude-sonnet-4.5 not in modelToPaneID")
	}

	// Simulate the completion of /next claude-sonnet-4.5
	msg := nextCompleteMsg{
		Model:    "claude-sonnet-4.5",
		PaneID:   "",
		Worktree: "",
	}
	result, _ := m.Update(msg)
	m = result.(model)

	// The selected model should be removed from all tracking maps
	if _, ok := m.modelToPaneID["claude-sonnet-4.5"]; ok {
		t.Error("claude-sonnet-4.5 should be removed from modelToPaneID after next")
	}
	if _, ok := m.modelToWorktree["claude-sonnet-4.5"]; ok {
		t.Error("claude-sonnet-4.5 should be removed from modelToWorktree after next")
	}
	if _, ok := m.modelPrompts["claude-sonnet-4.5"]; ok {
		t.Error("claude-sonnet-4.5 should be removed from modelPrompts after next")
	}
	if _, ok := m.instanceProvider["claude-sonnet-4.5"]; ok {
		t.Error("claude-sonnet-4.5 should be removed from instanceProvider after next")
	}
	if _, ok := m.instanceBaseModel["claude-sonnet-4.5"]; ok {
		t.Error("claude-sonnet-4.5 should be removed from instanceBaseModel after next")
	}
}

func TestNextCompleteTransitionsToNewTaskScreen(t *testing.T) {
	m := setupIterationModel()

	if m.screen != screenIteration {
		t.Fatalf("precondition: expected screenIteration, got %d", m.screen)
	}

	msg := nextCompleteMsg{Model: "claude-sonnet-4.5"}
	result, _ := m.Update(msg)
	m = result.(model)

	if m.screen != screenNewTask {
		t.Fatalf("expected screenNewTask after nextCompleteMsg, got %d", m.screen)
	}
	if m.newTaskFocus != focusTask {
		t.Fatalf("expected newTaskFocus == focusTask after next, got %d", m.newTaskFocus)
	}
}

func TestNextCompleteClearsIterationInput(t *testing.T) {
	m := setupIterationModel()

	// Simulate the user having typed something in the iteration prompt
	m.iterationInput = []string{"/next claude-sonnet-4.5"}
	m.iterationCursor.row = 0
	m.iterationCursor.col = len("/next claude-sonnet-4.5")

	msg := nextCompleteMsg{Model: "claude-sonnet-4.5"}
	result, _ := m.Update(msg)
	m = result.(model)

	if len(m.iterationInput) != 1 || m.iterationInput[0] != "" {
		t.Errorf("iterationInput should be reset to [\"\"], got %v", m.iterationInput)
	}
	if m.iterationCursor.row != 0 || m.iterationCursor.col != 0 {
		t.Errorf("iterationCursor should be {0,0}, got {%d,%d}", m.iterationCursor.row, m.iterationCursor.col)
	}
}

func TestNextCompleteResetsAutocompleteState(t *testing.T) {
	m := setupIterationModel()
	m.autocompleteActive = true
	m.autocompleteOptions = []string{"claude-sonnet-4.5", "gpt-5-mini"}
	m.autocompleteIndex = 1

	msg := nextCompleteMsg{Model: "claude-sonnet-4.5"}
	result, _ := m.Update(msg)
	m = result.(model)

	if m.autocompleteActive {
		t.Error("autocompleteActive should be false after nextCompleteMsg")
	}
	if m.autocompleteOptions != nil {
		t.Error("autocompleteOptions should be nil after nextCompleteMsg")
	}
}

func TestNextCompleteWithPaneIDRemovesFromCreatedPanes(t *testing.T) {
	m := setupIterationModel()

	// nextCmd in the real code returns PaneID="" and Worktree="" (the cleanup
	// loop already removed them all). But the Update handler also supports
	// removing specific entries when PaneID/Worktree are set.
	msg := nextCompleteMsg{
		Model:    "claude-sonnet-4.5",
		PaneID:   "%10",
		Worktree: "repo_feature-auth_auth_claude-sonnet-4.5",
	}
	result, _ := m.Update(msg)
	m = result.(model)

	for _, p := range m.createdPanes {
		if p == "%10" {
			t.Error("pane %10 should have been removed from createdPanes")
		}
	}
	for _, w := range m.createdWorktrees {
		if w == "repo_feature-auth_auth_claude-sonnet-4.5" {
			t.Error("worktree should have been removed from createdWorktrees")
		}
	}
}

func TestNextCompleteDoesNotReturnQuitCmd(t *testing.T) {
	m := setupIterationModel()

	msg := nextCompleteMsg{Model: "claude-sonnet-4.5"}
	_, cmd := m.Update(msg)

	// /next should NOT quit — it returns to the new-task screen.
	if cmd != nil {
		t.Error("nextCompleteMsg should return nil cmd (no quit), but got non-nil")
	}
}

// --------------------------------------------------------------------------
// Tests: /bail teardown
// --------------------------------------------------------------------------

func TestBailCompleteQuitsApp(t *testing.T) {
	m := setupIterationModel()

	result, cmd := m.Update(bailCompleteMsg{})
	_ = result.(model)

	// bailCompleteMsg should trigger tea.Quit
	if cmd == nil {
		t.Fatal("expected tea.Quit command from bailCompleteMsg, got nil")
	}

	// Execute the cmd and check the resulting message is the Quit message.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg from bail cmd, got %T", msg)
	}
}

// --------------------------------------------------------------------------
// Tests: /wrap teardown
// --------------------------------------------------------------------------

func TestWrapCompleteQuitsApp(t *testing.T) {
	m := setupIterationModel()

	msg := wrapCompleteMsg{
		Model:    "claude-sonnet-4.5",
		PaneID:   "%10",
		Worktree: "repo_feature-auth_auth_claude-sonnet-4.5",
	}
	result, cmd := m.Update(msg)
	_ = result.(model)

	if cmd == nil {
		t.Fatal("expected tea.Quit command from wrapCompleteMsg, got nil")
	}

	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg from wrap cmd, got %T", quitMsg)
	}
}

func TestWrapCompleteRemovesModelState(t *testing.T) {
	m := setupIterationModel()

	msg := wrapCompleteMsg{
		Model:    "gpt-5-mini",
		PaneID:   "%11",
		Worktree: "repo_feature-auth_auth_gpt-5-mini",
	}
	result, _ := m.Update(msg)
	m = result.(model)

	if _, ok := m.modelToPaneID["gpt-5-mini"]; ok {
		t.Error("gpt-5-mini should be removed from modelToPaneID")
	}
	if _, ok := m.modelToWorktree["gpt-5-mini"]; ok {
		t.Error("gpt-5-mini should be removed from modelToWorktree")
	}
	if _, ok := m.instanceProvider["gpt-5-mini"]; ok {
		t.Error("gpt-5-mini should be removed from instanceProvider")
	}
}

// --------------------------------------------------------------------------
// Tests: cleanupCompleteMsg
// --------------------------------------------------------------------------

func TestCleanupCompleteQuitsApp(t *testing.T) {
	m := setupIterationModel()

	_, cmd := m.Update(cleanupCompleteMsg{})
	if cmd == nil {
		t.Fatal("expected tea.Quit from cleanupCompleteMsg")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", msg)
	}
}

// --------------------------------------------------------------------------
// Tests: full flow — open 3 panes, /next on 1st, verify teardown
// --------------------------------------------------------------------------

func TestFullFlowOpenThreePanesSelectFirstThenTeardown(t *testing.T) {
	// Step 1: Start with a fresh model
	m := testModel()

	// Verify initial state: no panes or worktrees
	if len(m.createdPanes) != 0 || len(m.createdWorktrees) != 0 {
		t.Fatal("precondition: should start with no panes or worktrees")
	}

	// Step 2: Simulate panes opening (branch + workspace creation for 3 models)
	result, _ := m.Update(threePanesOpened())
	m = result.(model)

	// Verify we're on the iteration screen with all 3 models tracked
	if m.screen != screenIteration {
		t.Fatalf("step 2: expected screenIteration, got %d", m.screen)
	}
	if len(m.createdPanes) != 3 {
		t.Fatalf("step 2: expected 3 panes, got %d", len(m.createdPanes))
	}
	if len(m.createdWorktrees) != 3 {
		t.Fatalf("step 2: expected 3 worktrees, got %d", len(m.createdWorktrees))
	}
	if len(m.modelToPaneID) != 3 {
		t.Fatalf("step 2: expected 3 model-to-pane mappings, got %d", len(m.modelToPaneID))
	}
	if len(m.modelToWorktree) != 3 {
		t.Fatalf("step 2: expected 3 model-to-worktree mappings, got %d", len(m.modelToWorktree))
	}

	// Step 3: Simulate selecting the 1st choice (claude-sonnet-4.5) via /next
	// The real flow: user types "/next claude-sonnet-4.5" → updateIteration
	// parses it → calls nextCmd → nextCmd returns nextCompleteMsg.
	// We test the Update handler for nextCompleteMsg directly since nextCmd
	// has side-effects (git, tmux).
	nextMsg := nextCompleteMsg{
		Model:    "claude-sonnet-4.5",
		PaneID:   "",
		Worktree: "",
	}
	result, cmd := m.Update(nextMsg)
	m = result.(model)

	// Step 4: Verify teardown

	// 4a: Should transition back to new task screen
	if m.screen != screenNewTask {
		t.Fatalf("step 4a: expected screenNewTask, got %d", m.screen)
	}

	// 4b: The selected model should be completely removed from state
	if _, ok := m.modelToPaneID["claude-sonnet-4.5"]; ok {
		t.Error("step 4b: claude-sonnet-4.5 should be removed from modelToPaneID")
	}
	if _, ok := m.modelToWorktree["claude-sonnet-4.5"]; ok {
		t.Error("step 4b: claude-sonnet-4.5 should be removed from modelToWorktree")
	}
	if _, ok := m.modelPrompts["claude-sonnet-4.5"]; ok {
		t.Error("step 4b: claude-sonnet-4.5 should be removed from modelPrompts")
	}
	if _, ok := m.instanceProvider["claude-sonnet-4.5"]; ok {
		t.Error("step 4b: claude-sonnet-4.5 should be removed from instanceProvider")
	}
	if _, ok := m.instanceBaseModel["claude-sonnet-4.5"]; ok {
		t.Error("step 4b: claude-sonnet-4.5 should be removed from instanceBaseModel")
	}

	// 4c: Iteration state should be reset
	if len(m.iterationInput) != 1 || m.iterationInput[0] != "" {
		t.Errorf("step 4c: iterationInput not reset, got %v", m.iterationInput)
	}
	if m.iterationCursor.row != 0 || m.iterationCursor.col != 0 {
		t.Errorf("step 4c: cursor not reset, got {%d,%d}", m.iterationCursor.row, m.iterationCursor.col)
	}
	if m.autocompleteActive {
		t.Error("step 4c: autocomplete should be inactive")
	}

	// 4d: Focus should be on task name input (ready for next task)
	if m.newTaskFocus != focusTask {
		t.Errorf("step 4d: expected focusTask, got %d", m.newTaskFocus)
	}

	// 4e: Should NOT quit the app (that's what /wrap does)
	if cmd != nil {
		t.Error("step 4e: /next should not produce a quit command")
	}
}

// --------------------------------------------------------------------------
// Tests: iteration screen command parsing via key input
// --------------------------------------------------------------------------

func TestIterationEnterBailTransitionsToProgress(t *testing.T) {
	m := setupIterationModel()

	// Type "/bail" into the iteration input
	m.iterationInput = []string{"/bail"}
	m.iterationCursor.col = len("/bail")

	// Press Enter
	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.updateIteration(keyMsg)
	m = result.(model)

	if m.screen != screenProgress {
		t.Fatalf("expected screenProgress after /bail enter, got %d", m.screen)
	}
	if m.progressMsg == "" {
		t.Error("progressMsg should be set after /bail")
	}
	if cmd == nil {
		t.Fatal("expected bailCmd to be returned")
	}
}

func TestIterationEnterNextTransitionsToProgress(t *testing.T) {
	m := setupIterationModel()

	// Type "/next claude-sonnet-4.5" into the iteration input
	m.iterationInput = []string{"/next claude-sonnet-4.5"}
	m.iterationCursor.col = len("/next claude-sonnet-4.5")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.updateIteration(keyMsg)
	m = result.(model)

	if m.screen != screenProgress {
		t.Fatalf("expected screenProgress after /next, got %d", m.screen)
	}
	if !strings.Contains(m.progressMsg, "claude-sonnet-4.5") {
		t.Errorf("progressMsg should mention model name, got %q", m.progressMsg)
	}
	if cmd == nil {
		t.Fatal("expected nextCmd to be returned")
	}
}

func TestIterationEnterWrapTransitionsToProgress(t *testing.T) {
	m := setupIterationModel()

	m.iterationInput = []string{"/wrap gpt-5-mini"}
	m.iterationCursor.col = len("/wrap gpt-5-mini")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.updateIteration(keyMsg)
	m = result.(model)

	if m.screen != screenProgress {
		t.Fatalf("expected screenProgress after /wrap, got %d", m.screen)
	}
	if !strings.Contains(m.progressMsg, "gpt-5-mini") {
		t.Errorf("progressMsg should mention model name, got %q", m.progressMsg)
	}
	if cmd == nil {
		t.Fatal("expected wrapCmd to be returned")
	}
}

func TestIterationEnterRetryTransitionsToProgress(t *testing.T) {
	m := setupIterationModel()

	m.iterationInput = []string{"/retry"}
	m.iterationCursor.col = len("/retry")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.updateIteration(keyMsg)
	m = result.(model)

	if m.screen != screenProgress {
		t.Fatalf("expected screenProgress after /retry, got %d", m.screen)
	}
	if cmd == nil {
		t.Fatal("expected retryCmd to be returned")
	}
}

// --------------------------------------------------------------------------
// Tests: selectedModels helper
// --------------------------------------------------------------------------

func TestSelectedModelsReturnsCorrectCount(t *testing.T) {
	m := testModel()

	models := m.selectedModels()
	if len(models) != 3 {
		t.Fatalf("expected 3 selected models, got %d: %v", len(models), models)
	}
}

func TestSelectedModelsWithDuplicates(t *testing.T) {
	m := testModel()
	// Select claude-sonnet-4.5 twice
	m.selected["github-copilot"]["claude-sonnet-4.5"] = 2

	models := m.selectedModels()
	count := 0
	for _, name := range models {
		if name == "claude-sonnet-4.5" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected claude-sonnet-4.5 to appear twice, got %d times in %v", count, models)
	}
}

func TestSelectedModelsEmptyWhenNoneSelected(t *testing.T) {
	m := testModel()
	m.selected["github-copilot"] = map[string]int{}

	models := m.selectedModels()
	if len(models) != 0 {
		t.Fatalf("expected 0 selected models, got %d", len(models))
	}
}

// --------------------------------------------------------------------------
// Tests: identifierFor
// --------------------------------------------------------------------------

func TestIdentifierForComposesCorrectly(t *testing.T) {
	m := testModel()
	m.branch = "feat-xyz"
	m.task = "setup"

	id := m.identifierFor("claude-sonnet-4.5")
	// identifierFor uses "_" separator and includes repo, branch, task, model
	if !strings.Contains(id, "feat-xyz") {
		t.Errorf("identifier should contain branch, got %q", id)
	}
	if !strings.Contains(id, "setup") {
		t.Errorf("identifier should contain task, got %q", id)
	}
	if !strings.Contains(id, "claude-sonnet-4.5") {
		t.Errorf("identifier should contain model name, got %q", id)
	}
	// Parts are underscore-separated
	parts := strings.Split(id, "_")
	if len(parts) < 3 {
		t.Errorf("expected at least 3 underscore-separated parts, got %d in %q", len(parts), id)
	}
}

// --------------------------------------------------------------------------
// Tests: window resize
// --------------------------------------------------------------------------

func TestWindowSizeUpdatesModelDimensions(t *testing.T) {
	m := testModel()

	result, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = result.(model)

	if m.width != 200 {
		t.Errorf("expected width 200, got %d", m.width)
	}
	if m.height != 50 {
		t.Errorf("expected height 50, got %d", m.height)
	}
}

// --------------------------------------------------------------------------
// Tests: cursor blink and spinner tick (periodic messages)
// --------------------------------------------------------------------------

func TestCursorBlinkTogglesVisibility(t *testing.T) {
	m := testModel()
	m.cursorVisible = true

	result, cmd := m.Update(cursorBlinkMsg{})
	m = result.(model)

	if m.cursorVisible != false {
		t.Error("cursor should toggle from visible to invisible")
	}
	if cmd == nil {
		t.Error("cursorBlinkMsg should schedule next tick")
	}

	// Toggle back
	result, _ = m.Update(cursorBlinkMsg{})
	m = result.(model)
	if m.cursorVisible != true {
		t.Error("cursor should toggle back to visible")
	}
}

func TestSpinnerTickAdvancesIndex(t *testing.T) {
	m := testModel()
	m.spinnerIndex = 0

	result, cmd := m.Update(spinnerTickMsg{})
	m = result.(model)

	if m.spinnerIndex != 1 {
		t.Errorf("expected spinner index 1, got %d", m.spinnerIndex)
	}
	if cmd == nil {
		t.Error("spinnerTickMsg should schedule next tick")
	}
}

func TestSpinnerTickWrapsAround(t *testing.T) {
	m := testModel()
	m.spinnerIndex = len(m.spinnerFrames) - 1 // last frame

	result, _ := m.Update(spinnerTickMsg{})
	m = result.(model)

	if m.spinnerIndex != 0 {
		t.Errorf("expected spinner to wrap to 0, got %d", m.spinnerIndex)
	}
}

// --------------------------------------------------------------------------
// Tests: nextCompleteMsg with ErrorText
// --------------------------------------------------------------------------

func TestNextCompleteWithErrorTextStillTransitions(t *testing.T) {
	m := setupIterationModel()

	msg := nextCompleteMsg{
		Model:     "claude-sonnet-4.5",
		ErrorText: "push to origin failed",
	}
	result, _ := m.Update(msg)
	m = result.(model)

	// Should still transition to newTask screen despite error
	if m.screen != screenNewTask {
		t.Fatalf("expected screenNewTask even with error, got %d", m.screen)
	}
}

// --------------------------------------------------------------------------
// Tests: expandTokens
// --------------------------------------------------------------------------

func TestExpandTokensReplacesPlaceholders(t *testing.T) {
	m := testModel()
	m.promptPastes = map[string]string{
		"[[PASTE#1]]": "full paste content here\nwith multiple lines",
	}

	result := m.expandTokens("Before [[PASTE#1]] after")
	if result != "Before full paste content here\nwith multiple lines after" {
		t.Errorf("unexpected expand result: %q", result)
	}
}

func TestExpandTokensNoopWhenNoPastes(t *testing.T) {
	m := testModel()
	input := "no tokens here"
	result := m.expandTokens(input)
	if result != input {
		t.Errorf("expected no change, got %q", result)
	}
}

// --------------------------------------------------------------------------
// Tests: currentProvider
// --------------------------------------------------------------------------

func TestCurrentProviderReturnsFirst(t *testing.T) {
	m := testModel()
	if p := m.currentProvider(); p != "github-copilot" {
		t.Errorf("expected github-copilot, got %q", p)
	}
}

func TestCurrentProviderEmptyWhenNoProviders(t *testing.T) {
	m := testModel()
	m.providers = nil
	if p := m.currentProvider(); p != "" {
		t.Errorf("expected empty provider, got %q", p)
	}
}

// --------------------------------------------------------------------------
// Tests: wrapCompleteMsg resets iteration state
// --------------------------------------------------------------------------

func TestWrapCompleteClearsIterationState(t *testing.T) {
	m := setupIterationModel()
	m.iterationInput = []string{"/wrap claude-sonnet-4.5"}
	m.autocompleteActive = true
	m.autocompleteOptions = []string{"opt1"}
	m.iterationHistoryIndex = 3
	m.draftIterationInput = []string{"draft"}

	msg := wrapCompleteMsg{Model: "claude-sonnet-4.5", PaneID: "%10", Worktree: "wt1"}
	result, _ := m.Update(msg)
	m = result.(model)

	if len(m.iterationInput) != 1 || m.iterationInput[0] != "" {
		t.Errorf("iterationInput not reset: %v", m.iterationInput)
	}
	if m.iterationCursor.row != 0 || m.iterationCursor.col != 0 {
		t.Error("iterationCursor not reset")
	}
	if m.iterationHistoryIndex != -1 {
		t.Errorf("iterationHistoryIndex not reset: %d", m.iterationHistoryIndex)
	}
	if m.draftIterationInput != nil {
		t.Error("draftIterationInput not reset")
	}
	if m.autocompleteActive {
		t.Error("autocomplete should be inactive")
	}
	if m.autocompleteOptions != nil {
		t.Error("autocompleteOptions should be nil")
	}
}
