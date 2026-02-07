package tmux

import (
	gotmux "github.com/jubnzv/go-tmux"
)

// IsInside checks if the process is running inside a tmux session
func IsInside() bool {
	return gotmux.IsInsideTmux()
}

// RunCommand executes a tmux command and returns stdout, stderr, and error
func RunCommand(args []string) (string, string, error) {
	return gotmux.RunCmd(args)
}

// DisplayMessage shows a message in the tmux status line
func DisplayMessage(message string) error {
	_, _, err := gotmux.RunCmd([]string{"display-message", message})
	return err
}
