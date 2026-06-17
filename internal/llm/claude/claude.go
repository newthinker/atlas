// internal/llm/claude/claude.go
package claude

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/newthinker/atlas/internal/llm"
)

// Provider implements the LLM interface for Claude/Anthropic.
type Provider struct {
	client anthropic.Client
	model  string
}

// Option configures a Claude provider.
type Option func(*config)

type config struct{ proxy string }

// WithProxy routes Anthropic API calls through an HTTP/HTTPS/SOCKS5 proxy
// (e.g. "http://127.0.0.1:7890"), needed where api.anthropic.com is blocked.
// Empty or unparseable → direct.
func WithProxy(proxyURL string) Option {
	return func(c *config) { c.proxy = proxyURL }
}

// proxiedClient builds an *http.Client routed through proxyURL, or nil for direct.
func proxiedClient(proxyURL string) *http.Client {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
}

// New creates a new Claude provider.
func New(apiKey, model string, opts ...Option) (*Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	reqOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if hc := proxiedClient(cfg.proxy); hc != nil {
		reqOpts = append(reqOpts, option.WithHTTPClient(hc))
	}
	client := anthropic.NewClient(reqOpts...)
	return &Provider{client: client, model: model}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "claude"
}

// Chat sends a chat request to the Claude API.
func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	messages := make([]anthropic.MessageParam, len(req.Messages))
	for i, m := range req.Messages {
		if m.Role == "user" {
			messages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content))
		} else {
			messages[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content))
		}
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude API error: %w", err)
	}

	content := ""
	if len(resp.Content) > 0 && resp.Content[0].Type == "text" {
		content = resp.Content[0].Text
	}

	return &llm.ChatResponse{
		Content: content,
		Usage: llm.Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		},
		FinishReason: string(resp.StopReason),
	}, nil
}
