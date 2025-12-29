// internal/llm/openai/openai_test.go
package openai

import (
	"testing"

	"github.com/newthinker/atlas/internal/llm"
)

func TestProvider_ImplementsInterface(t *testing.T) {
	var _ llm.Provider = (*Provider)(nil)
}

func TestNew_RequiresAPIKey(t *testing.T) {
	_, err := New("", "model")
	if err == nil {
		t.Error("expected error for empty API key")
	}
}

func TestNew_DefaultModel(t *testing.T) {
	p, err := New("test-key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "gpt-4o" {
		t.Errorf("expected default model gpt-4o, got %s", p.model)
	}
}
