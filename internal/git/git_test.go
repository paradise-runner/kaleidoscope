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

func TestIsStandardBranch(t *testing.T) {
	tests := []struct {
		branch   string
		expected bool
	}{
		{"main", true},
		{"master", true},
		{"dev", true},
		{"develop", true},
		{"development", true},
		{"feature/my-feature", false},
		{"fix/bug-123", false},
		{"my-branch", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			result := IsStandardBranch(tt.branch)
			if result != tt.expected {
				t.Errorf("IsStandardBranch(%q) = %v, want %v", tt.branch, result, tt.expected)
			}
		})
	}
}

func TestGetFeatureBranchDefault(t *testing.T) {
	// This test verifies the logic without depending on the actual git state
	// In a real repo, we can't control what branch we're on, so we just verify
	// it returns a string (possibly empty)
	result := GetFeatureBranchDefault()

	// Result should be either empty (if on standard branch or error) or a branch name
	t.Logf("GetFeatureBranchDefault returned: %q", result)

	// If we got a result, it should not be a standard branch
	if result != "" && IsStandardBranch(result) {
		t.Errorf("GetFeatureBranchDefault returned standard branch %q, should return empty string", result)
	}
}
