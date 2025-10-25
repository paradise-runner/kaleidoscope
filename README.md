# Kaleidoscope

<image src="assets/logo.png" alt="logo" width="200"/>

## Overview

Kaleidoscope is a command-line tool that enables developers to run multiple AI models in parallel on the same coding task, compare their outputs, and choose the best solution seamlessly. It integrates with `tmux` and `git worktrees` to provide an efficient and safe workflow for AI-assisted coding.

<image src="assets/tui.png" alt="kaleidoscope tui" width="600"/>

## Features

- **Multi-model parallel execution**: Run multiple AI models (Claude, GPT, etc.) on the same prompt simultaneously
- **Git worktree integration**: Each model works in its own isolated git worktree
- **Branch management**: Automatically creates and manages feature branches
- **Interactive iteration**: Send follow-up prompts to specific models using `@model` syntax
- **Smart cleanup**: Choose winning solutions and automatically merge, or bail and cleanup everything
- **Defaults persistence**: Save your preferred provider and models in `.kaleidoscope` config
- **Command autocomplete**: Tab completion for commands and model names

## Prerequisites
> Currently only MacOS is supported.
- **tmux**: Must be running inside a tmux session
    - `brew install tmux` (macOS)
- **opencode**: The `opencode` CLI tool must be installed and configured
    - `brew install sst/tap/opencode` (macOS)

## Installation

```bash
brew install paradise-runner/tap/kaleidoscope
```

## Usage

### Basic Usage

Run Kaleidoscope with the required `--run` flag specifying the command to execute after opencode completes:

```bash
# start a new tmux session
tmux

# run kaleidoscope with your test command
kaleidoscope --run "npm test"
```

Let the fireworks begin!

<image src="assets/kaleidoscope-demo.gif" alt="kaleidoscope demo" width="600"/>


### Interface

When launched, Kaleidoscope presents a TUI with the following fields:

1. **branch-name**: Name of the feature branch to create
2. **task-name**: Description of the task
3. **prompt**: Multi-line prompt to send to AI models
4. **model provider**: Select between github-copilot, OpenAI, etc.
5. **models**: Multi-select dropdown to choose which models to run

Navigate with:
- `Tab`: Cycle between fields
- `↑↓`: Navigate dropdowns and multi-line text
- `Space`: Toggle model selection
- `Enter`: Submit (creates worktrees and opens panes)
- `Ctrl+C` or `Esc`: Cancel and cleanup

### Iteration Commands

Once models are running in separate panes, you can use these commands in the iteration prompt:

- `/bail`: Cancel everything and cleanup all panes, worktrees, and branches
- `/next <model>`: Merge the specified model's changes to the feature branch, push, and cleanup
- `/wrap <model>`: Similar to next, but returns to new task screen instead of exiting
- `@<model> <prompt>`: Send a follow-up prompt to a specific model

Example:
```
@claude-sonnet-4.5 add error handling to the login function
```

### Saving Defaults

Save your preferred provider and model selections:

```bash
kaleidoscope --run "npm test" --set-default
```

This creates a `.kaleidoscope` file in your current directory with your preferences. The file includes:
- Default provider
- Selected models per provider
- Usage statistics for each model (tracked when using `/next`)

## Configuration

The `.kaleidoscope` file is a JSON file storing:

```json
{
  "provider": "github-copilot",
  "models": {
    "github-copilot": ["claude-sonnet-4.5", "gpt-5-mini"]
  },
  "choices": {
    "github-copilot": {
      "claude-sonnet-4.5": 5,
      "gpt-5-mini": 2
    }
  }
}
```

## Workflow Example

1. Start kaleidoscope in a tmux session:
   ```bash
   kaleidoscope --run "go test ./..."
   ```

2. Fill in the form:
   - branch-name: `feature/add-auth`
   - task-name: `add-jwt-authentication`
   - prompt: `Add JWT authentication to the API`
   - Select provider and models (e.g., claude-sonnet-4.5, gpt-5)

3. Press Enter - creates worktrees and opens panes for each model

4. Models run in parallel in separate panes

5. Review outputs and send follow-up prompts:
   ```
   @claude-sonnet-4.5 add rate limiting
   ```

6. Choose the best solution:
   ```
   /next claude-sonnet-4.5
   ```

7. Kaleidoscope commits changes, merges to feature branch, pushes, and cleans up

## How It Works

1. **Setup**: Creates a feature branch from your current branch
2. **Worktrees**: For each selected model, creates a git worktree in `../<repo>-<branch>-<task>-<model>/`
3. **Execution**: Opens a tmux pane for each worktree and runs `opencode run -m <provider>/<model> <prompt>`
4. **Iteration**: Allows sending additional prompts to specific models
5. **Selection**: When you `/next` a model:
   - Commits all changes in that model's worktree
   - Merges to the feature branch with `--no-ff`
   - Pushes to origin
   - Cleans up all panes, worktrees, and temporary branches
6. **Cleanup**: `/bail` removes everything without merging

## Using Kaleidoscope to develop Kaleidoscope

To use Kaleidoscope to help develop itself, you can run:

```bash
go build . && kaleidoscope --run "go run main.go --run 'echo \"hello world\"'"
```
This command will run Kaleidoscope to help implement changes to its own codebase, and then build the updated binary. The inner `go run main.go --run 'echo "hello world"'` command just spins up a simple test command to verify functionality for you to see in the panes.

## License

MIT License - see [LICENSE](LICENSE) file for details

## Author

Edward Champion

- personal website: [hec.works](https://hec.works)
- github: [paradise-runner](https://github.com/paradise-runner)
