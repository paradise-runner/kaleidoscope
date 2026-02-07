package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
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

	// Test: LoadDefaults should return nil when file doesn't exist
	defaults := LoadDefaults()
	if defaults != nil {
		t.Error("LoadDefaults should return nil when file doesn't exist")
	}

	// Create test data
	selected := map[string]map[string]int{
		"openai": {
			"gpt-4":         2,
			"gpt-3.5-turbo": 1,
		},
		"anthropic": {
			"claude-3": 1,
		},
	}

	// Save defaults
	if err := SaveDefaults("openai", selected); err != nil {
		t.Fatalf("SaveDefaults failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".kaleidoscope")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify
	loaded := LoadDefaults()
	if loaded == nil {
		t.Fatal("LoadDefaults returned nil after save")
	}

	if loaded.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", loaded.Provider)
	}

	// Check models
	if len(loaded.Models["openai"]) != 3 { // 2 gpt-4 + 1 gpt-3.5-turbo
		t.Errorf("Expected 3 openai models, got %d", len(loaded.Models["openai"]))
	}

	if len(loaded.Models["anthropic"]) != 1 {
		t.Errorf("Expected 1 anthropic model, got %d", len(loaded.Models["anthropic"]))
	}

	// Verify the config file is valid JSON
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var jsonCheck Defaults
	if err := json.Unmarshal(data, &jsonCheck); err != nil {
		t.Errorf("Config file is not valid JSON: %v", err)
	}
}

func TestIncrementChoice(t *testing.T) {
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

	// Test: Increment on non-existent file should create it
	if err := IncrementChoice("openai", "gpt-4"); err != nil {
		t.Fatalf("IncrementChoice failed on non-existent file: %v", err)
	}

	// Load and verify count is 1
	defaults := LoadDefaults()
	if defaults == nil {
		t.Fatal("LoadDefaults returned nil after increment")
	}

	if defaults.Choices["openai"]["gpt-4"] != 1 {
		t.Errorf("Expected count 1, got %d", defaults.Choices["openai"]["gpt-4"])
	}

	// Increment again
	if err := IncrementChoice("openai", "gpt-4"); err != nil {
		t.Fatalf("Second IncrementChoice failed: %v", err)
	}

	// Verify count is 2
	defaults = LoadDefaults()
	if defaults.Choices["openai"]["gpt-4"] != 2 {
		t.Errorf("Expected count 2, got %d", defaults.Choices["openai"]["gpt-4"])
	}

	// Increment different model
	if err := IncrementChoice("openai", "gpt-3.5-turbo"); err != nil {
		t.Fatalf("IncrementChoice for different model failed: %v", err)
	}

	defaults = LoadDefaults()
	if defaults.Choices["openai"]["gpt-3.5-turbo"] != 1 {
		t.Errorf("Expected count 1 for gpt-3.5-turbo, got %d", defaults.Choices["openai"]["gpt-3.5-turbo"])
	}

	// Increment different provider
	if err := IncrementChoice("anthropic", "claude-3"); err != nil {
		t.Fatalf("IncrementChoice for different provider failed: %v", err)
	}

	defaults = LoadDefaults()
	if defaults.Choices["anthropic"]["claude-3"] != 1 {
		t.Errorf("Expected count 1 for anthropic/claude-3, got %d", defaults.Choices["anthropic"]["claude-3"])
	}

	// Verify openai counts are preserved
	if defaults.Choices["openai"]["gpt-4"] != 2 {
		t.Errorf("Expected openai/gpt-4 count 2 to be preserved, got %d", defaults.Choices["openai"]["gpt-4"])
	}
}

func TestSaveDefaultsPreservesChoices(t *testing.T) {
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

	// Create initial config with choices
	if err := IncrementChoice("openai", "gpt-4"); err != nil {
		t.Fatalf("IncrementChoice failed: %v", err)
	}
	if err := IncrementChoice("openai", "gpt-4"); err != nil {
		t.Fatalf("Second IncrementChoice failed: %v", err)
	}

	// Save new defaults
	selected := map[string]map[string]int{
		"anthropic": {
			"claude-3": 1,
		},
	}
	if err := SaveDefaults("anthropic", selected); err != nil {
		t.Fatalf("SaveDefaults failed: %v", err)
	}

	// Verify choices were preserved
	defaults := LoadDefaults()
	if defaults.Choices["openai"]["gpt-4"] != 2 {
		t.Errorf("Expected choices to be preserved, got %d", defaults.Choices["openai"]["gpt-4"])
	}

	// Verify provider was updated
	if defaults.Provider != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got '%s'", defaults.Provider)
	}
}
