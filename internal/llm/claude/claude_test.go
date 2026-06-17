// internal/llm/claude/claude_test.go
package claude

import (
	"net/http"
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

func TestNew_WithProxySucceeds(t *testing.T) {
	p, err := New("key", "model", WithProxy("http://127.0.0.1:7897"))
	if err != nil || p == nil {
		t.Fatalf("New with proxy failed: %v", err)
	}
}

func TestProxiedClient(t *testing.T) {
	if proxiedClient("") != nil {
		t.Error("empty proxy should yield nil (direct)")
	}
	hc := proxiedClient("http://127.0.0.1:7897")
	tr, ok := hc.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("expected proxied transport, got %T", hc.Transport)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.anthropic.com/", nil)
	u, err := tr.Proxy(req)
	if err != nil || u == nil || u.String() != "http://127.0.0.1:7897" {
		t.Errorf("proxy = %v (err %v), want http://127.0.0.1:7897", u, err)
	}
}
