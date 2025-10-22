# Agent Guidelines for Kaleidoscope

## Build/Test/Lint Commands
- **Build**: `go build .`
- **Run**: `go run main.go --run "echo 'hello world'"` (requires tmux)
- **Test**: No test suite currently exists
- **Format**: `gofmt -w .`
- **Vet**: `go vet ./...`

## Code Style
- **Language**: Go 1.24+
- **Imports**: Standard library first, then external packages (charmbracelet, jubnzv/go-tmux)
- **Formatting**: Use gofmt, tabs for indentation
- **Naming**: camelCase for functions/vars, PascalCase for exported types
- **Types**: Explicit struct types with JSON tags where needed
- **Error Handling**: Return errors up the stack, use tmux.RunCmd for user messages
- **State Management**: Bubble Tea model pattern - immutable updates, commands for side effects
- **Git Operations**: Use exec.Command for git worktree/branch operations
- **Tmux Integration**: Use jubnzv/go-tmux library for pane management

## Architecture Notes
- Single-file TUI application using Bubble Tea framework
- Three screen types: screenSetup, screenIteration, screenNewTask
- State stored in model struct with focus-based input handling
- Git worktrees isolate each model's workspace
- Config persisted in `.kaleidoscope` JSON file
