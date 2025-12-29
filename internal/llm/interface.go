package llm

import "context"

// Provider defines the interface for LLM providers
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// ChatRequest holds the request parameters
type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	MaxTokens    int
	Temperature  float64
	JSONMode     bool
}

// Message represents a chat message
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// ChatResponse holds the response from the LLM
type ChatResponse struct {
	Content      string
	Usage        Usage
	FinishReason string
}

// Usage tracks token consumption
type Usage struct {
	InputTokens  int
	OutputTokens int
}
