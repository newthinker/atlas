// internal/llm/claude/claude_test.go
package claude

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
