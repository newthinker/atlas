// internal/llm/openai/openai.go
package openai

import (
	"context"
	"fmt"

	"github.com/newthinker/atlas/internal/llm"
	"github.com/sashabaranov/go-openai"
)

// Provider implements the LLM interface for OpenAI.
type Provider struct {
	client *openai.Client
	model  string
}

// New creates a new OpenAI provider.
func New(apiKey, model string) (*Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	if model == "" {
		model = "gpt-4o"
	}
	client := openai.NewClient(apiKey)
	return &Provider{client: client, model: model}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "openai"
}

// Chat sends a chat request to the OpenAI API.
func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages)+1)

	// Add system prompt as first message if provided
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	// Add user/assistant messages
	for _, m := range req.Messages {
		role := openai.ChatMessageRoleUser
		if m.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	chatReq := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: float32(req.Temperature),
	}

	if req.JSONMode {
		chatReq.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	resp, err := p.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai API error: %w", err)
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	finishReason := ""
	if len(resp.Choices) > 0 {
		finishReason = string(resp.Choices[0].FinishReason)
	}

	return &llm.ChatResponse{
		Content: content,
		Usage: llm.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
		FinishReason: finishReason,
	}, nil
}
