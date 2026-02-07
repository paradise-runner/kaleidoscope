package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveHistoryRoundTrip(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "kaleidoscope-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Test: LoadForRepo should return nil/empty when no history exists
	history := LoadForRepo()
	if history != nil && len(history) > 0 {
		t.Errorf("LoadForRepo should return empty history, got %v", history)
	}

	// Save some history
	testHistory := []string{
		"First prompt",
		"Second prompt",
		"Third prompt",
	}
	if err := SaveForRepo(testHistory); err != nil {
		t.Fatalf("SaveForRepo failed: %v", err)
	}

	// Load and verify
	loaded := LoadForRepo()
	if len(loaded) != 3 {
		t.Errorf("Expected 3 history items, got %d", len(loaded))
	}

	for i, expected := range testHistory {
		if loaded[i] != expected {
			t.Errorf("History item %d: expected %q, got %q", i, expected, loaded[i])
		}
	}
}

func TestSaveAndLoadLargeHistory(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "kaleidoscope-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Create history with more than MaxHistory items
	var largeHistory []string
	for i := 0; i < MaxHistory+5; i++ {
		largeHistory = append(largeHistory, fmt.Sprintf("Prompt %d", i))
	}

	// Note: The actual trimming logic would be in main.go when adding to history
	// This test verifies we can save and load history larger than MaxHistory
	if err := SaveForRepo(largeHistory); err != nil {
		t.Fatalf("SaveForRepo failed: %v", err)
	}

	loaded := LoadForRepo()
	if len(loaded) != MaxHistory+5 {
		t.Errorf("Expected %d history items, got %d", MaxHistory+5, len(loaded))
	}
}

func TestMigrationFromOldFormat(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "kaleidoscope-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Create old format history file in current directory
	oldHistory := []string{"Old prompt 1", "Old prompt 2"}
	oldPath := filepath.Join(tmpDir, ".kaleidoscope_history.json")

	data, err := json.Marshal(oldHistory)
	if err != nil {
		t.Fatalf("Failed to marshal old history: %v", err)
	}

	if err := os.WriteFile(oldPath, data, 0644); err != nil {
		t.Fatalf("Failed to write old history file: %v", err)
	}

	// Load should migrate from old format
	loaded := LoadForRepo()
	if len(loaded) != 2 {
		t.Errorf("Expected 2 migrated history items, got %d", len(loaded))
	}

	if loaded[0] != "Old prompt 1" || loaded[1] != "Old prompt 2" {
		t.Errorf("Migrated history doesn't match: got %v", loaded)
	}

	// Old file should be removed after migration
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Old history file should be removed after migration")
	}
}

func TestHistoryInDifferentDirectories(t *testing.T) {
	// Create two temp directories for testing
	tmpDir1, err := os.MkdirTemp("", "kaleidoscope-test-1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "kaleidoscope-test-2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	// Save original directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	// Save history in first directory
	if err := os.Chdir(tmpDir1); err != nil {
		t.Fatalf("Failed to change to temp dir 1: %v", err)
	}
	history1 := []string{"Dir1 prompt 1", "Dir1 prompt 2"}
	if err := SaveForRepo(history1); err != nil {
		t.Fatalf("SaveForRepo in dir1 failed: %v", err)
	}

	// Save history in second directory
	if err := os.Chdir(tmpDir2); err != nil {
		t.Fatalf("Failed to change to temp dir 2: %v", err)
	}
	history2 := []string{"Dir2 prompt 1", "Dir2 prompt 2", "Dir2 prompt 3"}
	if err := SaveForRepo(history2); err != nil {
		t.Fatalf("SaveForRepo in dir2 failed: %v", err)
	}

	// Verify histories are separate
	loaded2 := LoadForRepo()
	if len(loaded2) != 3 {
		t.Errorf("Expected 3 items in dir2 history, got %d", len(loaded2))
	}

	// Switch back to first directory and verify its history
	if err := os.Chdir(tmpDir1); err != nil {
		t.Fatalf("Failed to change back to temp dir 1: %v", err)
	}
	loaded1 := LoadForRepo()
	if len(loaded1) != 2 {
		t.Errorf("Expected 2 items in dir1 history, got %d", len(loaded1))
	}

	if loaded1[0] != "Dir1 prompt 1" {
		t.Errorf("Dir1 history corrupted: got %q", loaded1[0])
	}
}
