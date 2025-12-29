// internal/llm/factory/factory.go
package factory

import (
	"fmt"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/llm"
	"github.com/newthinker/atlas/internal/llm/claude"
	"github.com/newthinker/atlas/internal/llm/ollama"
	"github.com/newthinker/atlas/internal/llm/openai"
)

// New creates an LLM provider based on configuration.
func New(cfg config.LLMConfig) (llm.Provider, error) {
	switch cfg.Provider {
	case "claude":
		return claude.New(cfg.Claude.APIKey, cfg.Claude.Model)
	case "openai":
		return openai.New(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
	case "ollama":
		return ollama.New(cfg.Ollama.Endpoint, cfg.Ollama.Model)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
}
