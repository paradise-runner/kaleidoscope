package history

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MaxHistory is the maximum number of history entries to keep per repository
const MaxHistory = 20

// repoHistoryFilePath computes the path to the history file for the current working directory
func repoHistoryFilePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	hash := sha1.Sum([]byte(abs))
	dir := filepath.Join(os.TempDir(), "kaleidoscope-history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	file := filepath.Join(dir, fmt.Sprintf("%x.json", hash))
	return file, nil
}

// LoadForRepo loads the prompt history for the current repository from the temp directory
func LoadForRepo() []string {
	path, err := repoHistoryFilePath()
	if err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var h []string
			if jsonErr := json.Unmarshal(data, &h); jsonErr == nil {
				return h
			}
		}
	}

	// Migrate from old per-repo file if present
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	oldPath := filepath.Join(cwd, ".kaleidoscope_history.json")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil
	}
	var h []string
	if jsonErr := json.Unmarshal(data, &h); jsonErr != nil {
		return nil
	}
	if newPath, e := repoHistoryFilePath(); e == nil {
		_ = os.WriteFile(newPath, data, 0644)
		_ = os.Remove(oldPath)
	}
	return h
}

// SaveForRepo persists the prompt history for the current repository to the temp directory
func SaveForRepo(h []string) error {
	path, err := repoHistoryFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
