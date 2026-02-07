package git

import (
	"os/exec"
	"strings"
)

// GetCurrentBranch returns the name of the current git branch
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsStandardBranch returns true if the branch name is a standard branch
// (main, master, dev, develop, development) that shouldn't be used as a feature branch
func IsStandardBranch(branch string) bool {
	standardBranches := map[string]bool{
		"main":        true,
		"master":      true,
		"dev":         true,
		"develop":     true,
		"development": true,
	}
	return standardBranches[branch]
}

// GetFeatureBranchDefault returns the current branch name if it's not a standard branch,
// otherwise returns an empty string (for use as a default feature branch name)
func GetFeatureBranchDefault() string {
	branch, err := GetCurrentBranch()
	if err != nil || IsStandardBranch(branch) {
		return ""
	}
	return branch
}
