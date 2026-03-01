package llm

import "fmt"

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderOllama    = "ollama" // alias for openai with custom base URL
)

// Config holds the LLM provider configuration.
type Config struct {
	Provider string // "openai", "anthropic", or "ollama" (OpenAI-compatible)
	APIKey   string
	BaseURL  string // for OpenAI-compatible endpoints (Ollama, LM Studio, OpenRouter, etc.)
	Model    string // override default model
}

// IsConfigured returns true if a provider is set with required credentials.
// Ollama doesn't require an API key.
func (c Config) IsConfigured() bool {
	if c.Provider == "" {
		return false
	}
	if c.Provider == ProviderOllama {
		return true // Ollama doesn't need an API key
	}
	return c.APIKey != ""
}

// NewProvider creates a Provider from the given configuration.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case ProviderOpenAI:
		return newOpenAIProvider(cfg)
	case ProviderOllama:
		// Ollama uses the OpenAI-compatible API
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		if cfg.APIKey == "" {
			cfg.APIKey = "ollama" // Ollama requires a non-empty key but doesn't validate it
		}
		return newOpenAIProvider(cfg)
	case ProviderAnthropic:
		return newAnthropicProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown AI provider: %q (supported: openai, anthropic, ollama)", cfg.Provider)
	}
}
