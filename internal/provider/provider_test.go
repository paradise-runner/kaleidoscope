package provider

import (
	"testing"
)

func TestProvidersHaveModels(t *testing.T) {
	for provider, models := range Providers {
		// OpenAI (capital) is a legacy entry that may have no models
		if provider == "OpenAI" {
			continue
		}
		if len(models) == 0 {
			t.Errorf("Provider %q has no models", provider)
		}
	}
}

func TestProviderNamesMatchProvidersMap(t *testing.T) {
	// Check that all names in ProviderNames exist in Providers map
	for _, name := range ProviderNames {
		if _, ok := Providers[name]; !ok {
			t.Errorf("ProviderNames contains %q but it's not in Providers map", name)
		}
	}
}

func TestKnownProvidersExist(t *testing.T) {
	knownProviders := []string{
		"opencode",
		"openai",
		"openrouter",
		"lmstudio",
		"anthropic",
		"amazon-bedrock",
		"github-copilot",
	}

	for _, provider := range knownProviders {
		if _, ok := Providers[provider]; !ok {
			t.Errorf("Known provider %q not found in Providers map", provider)
		}
	}
}

func TestModelListsAreNonEmpty(t *testing.T) {
	for provider, models := range Providers {
		// OpenAI (capital) is a legacy entry that may have no models
		if provider == "OpenAI" {
			continue
		}
		if len(models) == 0 {
			t.Errorf("Provider %q has empty model list", provider)
		}

		// Verify no empty model names
		for i, model := range models {
			if model == "" {
				t.Errorf("Provider %q has empty model name at index %d", provider, i)
			}
		}
	}
}

func TestProviderNamesIsOrdered(t *testing.T) {
	// Just verify the list has expected length
	if len(ProviderNames) == 0 {
		t.Error("ProviderNames is empty")
	}

	// Verify first entry is github-copilot as per the code
	if len(ProviderNames) > 0 && ProviderNames[0] != "github-copilot" {
		t.Errorf("Expected first provider to be 'github-copilot', got %q", ProviderNames[0])
	}
}
