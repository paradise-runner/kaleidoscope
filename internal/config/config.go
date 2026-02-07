package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Defaults represents the kaleidoscope configuration stored in .kaleidoscope file
type Defaults struct {
	Provider string                    `json:"provider"`
	Models   map[string][]string       `json:"models"`
	Choices  map[string]map[string]int `json:"choices"`
}

// LoadDefaults reads the .kaleidoscope config file from the current working directory
func LoadDefaults() *Defaults {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var defaults Defaults
	if err := json.Unmarshal(data, &defaults); err != nil {
		return nil
	}

	return &defaults
}

// IncrementChoice increments the usage count for a specific provider/model combination
func IncrementChoice(provider string, model string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")

	defaults := LoadDefaults()
	if defaults == nil {
		defaults = &Defaults{
			Provider: provider,
			Models:   make(map[string][]string),
			Choices:  make(map[string]map[string]int),
		}
	}

	if defaults.Choices == nil {
		defaults.Choices = make(map[string]map[string]int)
	}

	if defaults.Choices[provider] == nil {
		defaults.Choices[provider] = make(map[string]int)
	}

	defaults.Choices[provider][model]++

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveDefaults persists the provider and selected models to .kaleidoscope config file
func SaveDefaults(provider string, selected map[string]map[string]int) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, ".kaleidoscope")

	existing := LoadDefaults()
	var choices map[string]map[string]int
	if existing != nil && existing.Choices != nil {
		choices = existing.Choices
	} else {
		choices = make(map[string]map[string]int)
	}

	models := make(map[string][]string)
	for prov, sel := range selected {
		var selectedModels []string
		for model, count := range sel {
			if count > 0 {
				for i := 0; i < count; i++ {
					selectedModels = append(selectedModels, model)
				}
			}
		}
		if len(selectedModels) > 0 {
			models[prov] = selectedModels
		}
	}

	defaults := Defaults{
		Provider: provider,
		Models:   models,
		Choices:  choices,
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
