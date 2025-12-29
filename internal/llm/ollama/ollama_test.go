// internal/llm/ollama/ollama_test.go
package ollama

import (
	"testing"

	"github.com/newthinker/atlas/internal/llm"
)

func TestProvider_ImplementsInterface(t *testing.T) {
	var _ llm.Provider = (*Provider)(nil)
}

func TestNew_DefaultEndpoint(t *testing.T) {
	p, err := New("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.endpoint != "http://localhost:11434" {
		t.Errorf("expected default endpoint http://localhost:11434, got %s", p.endpoint)
	}
}

func TestNew_DefaultModel(t *testing.T) {
	p, err := New("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "qwen2.5:32b" {
		t.Errorf("expected default model qwen2.5:32b, got %s", p.model)
	}
}

func TestNew_CustomValues(t *testing.T) {
	p, err := New("http://custom:8080", "llama3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.endpoint != "http://custom:8080" {
		t.Errorf("expected custom endpoint, got %s", p.endpoint)
	}
	if p.model != "llama3" {
		t.Errorf("expected custom model, got %s", p.model)
	}
}
