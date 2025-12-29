// internal/llm/factory/factory_test.go
package factory

import (
	"testing"

	"github.com/newthinker/atlas/internal/config"
)

func TestNew_Claude(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "claude",
		Claude: config.ClaudeConfig{
			APIKey: "test-key",
			Model:  "claude-3-sonnet",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("expected claude provider, got %s", p.Name())
	}
}

func TestNew_OpenAI(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai provider, got %s", p.Name())
	}
}

func TestNew_Ollama(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "ollama",
		Ollama: config.OllamaConfig{
			Endpoint: "http://localhost:11434",
			Model:    "llama3",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("expected ollama provider, got %s", p.Name())
	}
}

func TestNew_Unknown(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "unknown",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNew_ClaudeMissingKey(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "claude",
		Claude: config.ClaudeConfig{
			APIKey: "",
		},
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for missing API key")
	}
}
