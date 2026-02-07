package git

import (
	"testing"
)

func TestGetCurrentBranch(t *testing.T) {
	// This test will only pass in a git repository
	// It's a basic smoke test to ensure the function works
	branch, err := GetCurrentBranch()

	// In a git repo, we should get a branch name
	// Outside a git repo, we'll get an error
	if err == nil && branch == "" {
		t.Error("GetCurrentBranch returned empty branch name without error")
	}

	// If we're in a git repo (likely), verify we got something
	// This test is permissive to work in various environments
	if err == nil {
		t.Logf("Current branch: %s", branch)
	} else {
		t.Logf("Not in a git repo or git not available: %v", err)
	}
}
