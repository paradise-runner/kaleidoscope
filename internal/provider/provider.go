package provider

// Providers is a map of provider names to their available models
var Providers = map[string][]string{
	"opencode":       {"qwen3-coder", "claude-opus-4-1", "kimi-k2", "claude-haiku-4-5", "minimax-m2", "claude-sonnet-4-5", "an-gd4", "gpt-5-codex", "big-pickle", "claude-3-5-haiku", "glm-4.6", "grok-code", "claude-sonnet-4", "gpt-5"},
	"openai":         {"gpt-4.1-mini", "text-embedding-3-small", "gpt-4", "o1-pro", "gpt-4o-2024-05-13", "gpt-4o-2024-08-06", "gpt-4.1-mini", "o3-deep-research", "gpt-3.5-turbo", "text-embedding-3-large", "gpt-4-turbo", "o1-preview", "o3-mini", "codex-mini-latest", "gpt-5-nano", "gpt-5-codex", "gpt-4o", "gpt-4.1", "o4-mini", "o1", "gpt-5-mini", "o1-mini", "text-embedding-ada-002", "o3-pro", "gpt-4o-2024-11-20", "o3", "o4-mini-deep-research", "gpt-5-chat-latest", "gpt-4o-mini", "gpt-5", "gpt-5-pro"},
	"openrouter":     {"moonshotai/kimi-k2", "moonshotai/kimi-k2-0905", "moonshotai/kimi-dev-72b:free", "moonshotai/kimi-k2-thinking", "moonshotai/kimi-k2-0905:exacto", "moonshotai/kimi-k2:free", "thudm/glm-z1-32b:free", "nousresearch/hermes-4-70b", "nousresearch/hermes-4-405b", "nousresearch/deephermes-3-llama-3-8b-preview", "nvidia/nemotron-nano-9b-v2", "x-ai/grok-4", "x-ai/grok-code-fast-1", "x-ai/grok-3", "x-ai/grok-4-fast", "x-ai/grok-3-beta", "x-ai/grok-3-mini-beta", "x-ai/grok-3-mini", "cognitivecomputations/dolphin3.0-mistral-24b", "cognitivecomputations/dolphin3.0-r1-mistral-24b", "deepseek/deepseek-chat-v3.1", "deepseek/deepseek-r1:free", "deepseek/deepseek-v3-base:free", "deepseek/deepseek-v3.1-terminus", "deepseek/deepseek-r1-0528-qwen3-8b:free", "deepseek/deepseek-chat-v3-0324", "deepseek/deepseek-r1-0528:free", "deepseek/deepseek-r1-distill-llama-70b", "deepseek/deepseek-r1-distill-qwen-14b", "deepseek/deepseek-v3.1-terminus:exacto", "featherless/qwerky-72b", "tngtech/deepseek-r1t2-chimera:free", "minimax/minimax-m1", "minimax/minimax-m2:free", "minimax/minimax-01", "google/gemini-2.0-flash-001", "google/gemma-2-9b-it:free", "google/gemini-2.5-flash", "google/gemini-2.5-pro-preview-05-06", "google/gemma-3n-e4b-it", "google/gemini-2.5-flash-lite", "google/gemini-2.5-pro-preview-06-05", "google/gemini-2.5-flash-preview-09-2025", "google/gemini-2.5-pro", "google/gemma-3-12b-it", "google/gemma-3n-e4b-it:free", "google/gemini-2.5-flash-lite-preview-09-2025", "google/gemini-2.0-flash-exp:free", "google/gemma-3-27b-it", "microsoft/mai-ds-r1:free", "openai/gpt-4.1-mini", "openai/gpt-5-chat", "openai/gpt-5-nano", "openai/gpt-5-codex", "openai/gpt-4.1", "openai/gpt-oss-120b:exacto", "openai/o4-mini", "openai/gpt-5-mini", "openai/gpt-5-image", "openai/gpt-oss-20b", "openai/gpt-oss-120b", "openai/gpt-4o-mini", "openai/gpt-5", "openai/gpt-5-pro", "openrouter/horizon-alpha", "openrouter/polaris-alpha", "openrouter/sonoma-sky-alpha", "openrouter/cypher-alpha:free", "openrouter/sonoma-dusk-alpha", "openrouter/horizon-beta", "z-ai/glm-4.5", "z-ai/glm-4.5-air", "z-ai/glm-4.5v", "z-ai/glm-4.6", "z-ai/glm-4.6:exacto", "z-ai/glm-4.5-air:free", "qwen/qwen3-coder", "qwen/qwen3-32b:free", "qwen/qwen3-next-80b-a3b-instruct", "qwen/qwen-2.5-coder-32b-instruct", "qwen/qwen3-235b-a22b:free", "qwen/qwq-32b:free", "qwen/qwen3-30b-a3b-thinking-2507", "qwen/qwen3-30b-a3b:free", "qwen/qwen2.5-vl-72b-instruct", "qwen/qwen3-14b:free", "qwen/qwen3-30b-a3b-instruct-2507", "qwen/qwen3-235b-a22b-thinking-2507", "qwen/qwen2.5-vl-32b-instruct:free", "qwen/qwen2.5-vl-72b-instruct:free", "qwen/qwen3-235b-a22b-07-25:free", "qwen/qwen3-coder:free", "qwen/qwen3-235b-a22b-07-25", "qwen/qwen3-8b:free", "qwen/qwen3-max", "qwen/qwen3-next-80b-a3b-thinking", "qwen/qwen3-coder:exacto", "mistralai/devstral-medium-2507", "mistralai/codestral-2508", "mistralai/mistral-7b-instruct:free", "mistralai/devstral-small-2505", "mistralai/mistral-small-3.2-24b-instruct", "mistralai/devstral-small-2505:free", "mistralai/mistral-small-3.2-24b-instruct:free", "mistralai/mistral-medium-3", "mistralai/mistral-small-3.1-24b-instruct", "mistralai/devstral-small-2507", "mistralai/mistral-medium-3.1", "mistralai/mistral-nemo:free", "rekaai/reka-flash-3", "meta-llama/llama-3.2-11b-vision-instruct", "meta-llama/llama-3.3-70b-instruct:free", "meta-llama/llama-4-scout:free", "anthropic/claude-opus-4", "anthropic/claude-haiku-4.5", "anthropic/claude-opus-4.1", "anthropic/claude-3.7-sonnet", "anthropic/claude-3.5-haiku", "anthropic/claude-sonnet-4", "anthropic/claude-sonnet-4.5", "sarvamai/sarvam-m:free"},
	"lmstudio":       {"openai/gpt-oss-20b", "qwen/qwen3-30b-a3b-2507", "qwen/qwen3-coder-30b"},
	"anthropic":      {"claude-opus-4-0", "claude-3-5-sonnet-20241022", "claude-opus-4-1", "claude-haiku-4-5", "claude-3-5-sonnet-20240620", "claude-3-5-haiku-latest", "claude-3-opus-20240229", "claude-sonnet-4-5", "claude-sonnet-4-5-20250929", "claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-3-5-haiku-20241022", "claude-3-haiku-20240307", "claude-3-7-sonnet-20250219", "claude-3-7-sonnet-latest", "claude-sonnet-4-0", "claude-opus-4-1-20250805", "claude-3-sonnet-20240229", "claude-haiku-4-5-20251001"},
	"amazon-bedrock": {"cohere.command-r-plus-v1:0", "anthropic.claude-v2", "anthropic.claude-3-7-sonnet-20250219-v1:0", "anthropic.claude-sonnet-4-20250514-v1:0", "qwen.qwen3-coder-30b-a3b-v1:0", "meta.llama3-2-11b-instruct-v1:0", "anthropic.claude-3-haiku-20240307-v1:0", "meta.llama3-2-90b-instruct-v1:0", "meta.llama3-2-1b-instruct-v1:0", "anthropic.claude-v2:1", "deepseek.v3-v1:0", "cohere.command-light-text-v14", "ai21.jamba-1-5-large-v1:0", "meta.llama3-3-70b-instruct-v1:0", "anthropic.claude-3-opus-20240229-v1:0", "amazon.nova-pro-v1:0", "meta.llama3-1-8b-instruct-v1:0", "qwen.qwen3-32b-v1:0", "anthropic.claude-3-5-sonnet-20240620-v1:0", "anthropic.claude-haiku-4-5-20251001-v1:0", "cohere.command-r-v1:0", "amazon.nova-micro-v1:0", "meta.llama3-1-70b-instruct-v1:0", "meta.llama3-70b-instruct-v1:0", "deepseek.r1-v1:0", "anthropic.claude-3-5-sonnet-20241022-v2:0", "cohere.command-text-v14", "anthropic.claude-opus-4-20250514-v1:0", "qwen.qwen3-coder-480b-a35b-v1:0", "anthropic.claude-sonnet-4-5-20250929-v1:0", "meta.llama3-2-3b-instruct-v1:0", "anthropic.claude-instant-v1", "amazon.nova-premier-v1:0", "anthropic.claude-opus-4-1-20250805-v1:0", "meta.llama4-scout-17b-instruct-v1:0", "ai21.jamba-1-5-mini-v1:0", "meta.llama3-8b-instruct-v1:0", "anthropic.claude-3-sonnet-20240229-v1:0", "meta.llama4-maverick-17b-instruct-v1:0", "qwen.qwen3-235b-a22b-2507-v1:0", "amazon.nova-lite-v1:0", "anthropic.claude-3-5-haiku-20241022-v1:0"},
	"github-copilot": {"claude-sonnet-4.5", "claude-haiku-4.5", "gpt-5-mini", "gpt-5", "gemini-2.0-flash-001", "claude-opus-4", "grok-code-fast-1", "claude-3.5-sonnet", "o3-mini", "gpt-5-codex", "gpt-4o", "gpt-4.1", "o4-mini", "claude-opus-41", "claude-3.7-sonnet", "gemini-2.5-pro", "o3", "claude-sonnet-4", "claude-3.7-sonnet-thought"},
	"OpenAI":         {}, // Legacy entry, preserved for compatibility
}

// ProviderNames is the ordered list of provider names for UI display
// This excludes legacy aliases like "OpenAI" which are only used for config compatibility
var ProviderNames = []string{
	"github-copilot",
	"openai",
	"anthropic",
	"opencode",
	"openrouter",
	"lmstudio",
	"amazon-bedrock",
}

// AllProviderNames includes legacy provider names for config compatibility
// Use ProviderNames for UI display instead
var AllProviderNames = []string{
	"github-copilot",
	"openai",
	"anthropic",
	"opencode",
	"openrouter",
	"lmstudio",
	"amazon-bedrock",
	"OpenAI",
}
