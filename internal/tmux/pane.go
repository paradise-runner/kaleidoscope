package tmux

import (
	tmux "github.com/jubnzv/go-tmux"
)

// IsInside checks if the process is running inside a tmux session
func IsInside() bool {
	return tmux.IsInsideTmux()
}

// RunCommand executes a tmux command and returns stdout, stderr, and error
func RunCommand(args []string) (string, string, error) {
	return tmux.RunCmd(args)
}

// DisplayMessage shows a message in the tmux status line
func DisplayMessage(message string) error {
	_, _, err := tmux.RunCmd([]string{"display-message", message})
	return err
}
