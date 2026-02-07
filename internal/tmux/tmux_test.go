package tmux

import (
	"testing"
)

func TestDisplayMessage(t *testing.T) {
	// This is a basic smoke test that won't actually display a message
	// unless running in tmux, but ensures the function signature is correct
	_ = DisplayMessage("test message")
}

func TestRunCommand(t *testing.T) {
	// This is a basic smoke test
	_, _, _ = RunCommand([]string{"display-message", "-p", "test"})
}

func TestIsInside(t *testing.T) {
	// Just verify the function can be called
	_ = IsInside()
}
