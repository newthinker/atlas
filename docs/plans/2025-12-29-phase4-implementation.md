# ATLAS Phase 4 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add LLM-powered meta-strategies (signal arbitration, strategy synthesis) and Futu broker integration to ATLAS.

**Architecture:** LLM providers (Claude/OpenAI/Ollama) power two meta-strategies: an Arbitrator that resolves conflicting signals using market context, and a Synthesizer that suggests strategy improvements. Futu broker provides real portfolio data.

**Tech Stack:** Go 1.21+, Anthropic SDK, OpenAI SDK, Ollama API, Futu OpenD API

---

## Task 1: LLM Interface and Configuration

**Files:**
- Create: `internal/llm/interface.go`
- Create: `internal/llm/interface_test.go`
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

**Step 1: Create LLM interface**

```go
// internal/llm/interface.go
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
```

**Step 2: Create test file**

```go
// internal/llm/interface_test.go
package llm

import "testing"

func TestInterfaceDefined(t *testing.T) {
	var _ Provider = nil
}
```

**Step 3: Add LLM config to config.go**

Add after existing config structs:

```go
type LLMConfig struct {
	Provider string       `mapstructure:"provider"`
	Claude   ClaudeConfig `mapstructure:"claude"`
	OpenAI   OpenAIConfig `mapstructure:"openai"`
	Ollama   OllamaConfig `mapstructure:"ollama"`
}

type ClaudeConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type OpenAIConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type OllamaConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Model    string `mapstructure:"model"`
}
```

Add `LLM LLMConfig` field to main Config struct.

**Step 4: Update config.example.yaml**

```yaml
llm:
  provider: claude
  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
  openai:
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
  ollama:
    endpoint: "http://localhost:11434"
    model: "qwen2.5:32b"
```

**Step 5: Run tests**

```bash
go test ./internal/llm/... ./internal/config/... -v
```

**Step 6: Commit**

```bash
git add internal/llm/ internal/config/config.go config.example.yaml
git commit -m "feat: add LLM provider interface and configuration"
```

---

## Task 2: Claude Provider Implementation

**Files:**
- Create: `internal/llm/claude/claude.go`
- Create: `internal/llm/claude/claude_test.go`

**Step 1: Add Anthropic SDK dependency**

```bash
go get github.com/anthropics/anthropic-sdk-go
```

**Step 2: Create Claude provider**

```go
// internal/llm/claude/claude.go
package claude

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/newthinker/atlas/internal/llm"
)

type Provider struct {
	client *anthropic.Client
	model  string
}

func New(apiKey, model string) (*Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Provider{client: client, model: model}, nil
}

func (p *Provider) Name() string {
	return "claude"
}

func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	messages := make([]anthropic.MessageParam, len(req.Messages))
	for i, m := range req.Messages {
		if m.Role == "user" {
			messages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content))
		} else {
			messages[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content))
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(int64(req.MaxTokens)),
		Messages:  anthropic.F(messages),
	}

	if req.SystemPrompt != "" {
		params.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(req.SystemPrompt),
		})
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude API error: %w", err)
	}

	content := ""
	if len(resp.Content) > 0 {
		if textBlock, ok := resp.Content[0].AsUnion().(anthropic.TextBlock); ok {
			content = textBlock.Text
		}
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
```

**Step 3: Create test**

```go
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
```

**Step 4: Run tests**

```bash
go test ./internal/llm/... -v
```

**Step 5: Commit**

```bash
git add internal/llm/claude/ go.mod go.sum
git commit -m "feat: add Claude LLM provider"
```

---

## Task 3: OpenAI Provider Implementation

**Files:**
- Create: `internal/llm/openai/openai.go`
- Create: `internal/llm/openai/openai_test.go`

**Step 1: Add OpenAI SDK dependency**

```bash
go get github.com/openai/openai-go
```

**Step 2: Create OpenAI provider**

```go
// internal/llm/openai/openai.go
package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/newthinker/atlas/internal/llm"
)

type Provider struct {
	client *openai.Client
	model  string
}

func New(apiKey, model string) (*Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &Provider{client: client, model: model}, nil
}

func (p *Provider) Name() string {
	return "openai"
}

func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}

	for _, m := range req.Messages {
		if m.Role == "user" {
			messages = append(messages, openai.UserMessage(m.Content))
		} else {
			messages = append(messages, openai.AssistantMessage(m.Content))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:     openai.F(p.model),
		Messages:  openai.F(messages),
		MaxTokens: openai.F(int64(req.MaxTokens)),
	}

	if req.JSONMode {
		params.ResponseFormat = openai.F[openai.ChatCompletionNewParamsResponseFormatUnion](
			openai.ResponseFormatJSONObjectParam{Type: openai.F(openai.ResponseFormatJSONObjectTypeJSONObject)},
		)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai API error: %w", err)
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	return &llm.ChatResponse{
		Content: content,
		Usage: llm.Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
		},
		FinishReason: string(resp.Choices[0].FinishReason),
	}, nil
}
```

**Step 3: Create test**

```go
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
```

**Step 4: Run tests**

```bash
go test ./internal/llm/... -v
```

**Step 5: Commit**

```bash
git add internal/llm/openai/ go.mod go.sum
git commit -m "feat: add OpenAI LLM provider"
```

---

## Task 4: Ollama Provider Implementation

**Files:**
- Create: `internal/llm/ollama/ollama.go`
- Create: `internal/llm/ollama/ollama_test.go`

**Step 1: Create Ollama provider**

```go
// internal/llm/ollama/ollama.go
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/newthinker/atlas/internal/llm"
)

type Provider struct {
	endpoint string
	model    string
	client   *http.Client
}

func New(endpoint, model string) (*Provider, error) {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &Provider{
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{},
	}, nil
}

func (p *Provider) Name() string {
	return "ollama"
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	messages := make([]ollamaMessage, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}

	for _, m := range req.Messages {
		messages = append(messages, ollamaMessage{Role: m.Role, Content: m.Content})
	}

	ollamaReq := ollamaRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
	}

	if req.JSONMode {
		ollamaReq.Format = "json"
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama API error: %w", err)
	}
	defer resp.Body.Close()

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &llm.ChatResponse{
		Content: ollamaResp.Message.Content,
		Usage: llm.Usage{
			InputTokens:  ollamaResp.PromptEvalCount,
			OutputTokens: ollamaResp.EvalCount,
		},
		FinishReason: "stop",
	}, nil
}
```

**Step 2: Create test**

```go
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
	p, err := New("", "model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.endpoint != "http://localhost:11434" {
		t.Errorf("expected default endpoint, got %s", p.endpoint)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/llm/... -v
```

**Step 4: Commit**

```bash
git add internal/llm/ollama/
git commit -m "feat: add Ollama LLM provider"
```

---

## Task 5: LLM Provider Factory

**Files:**
- Create: `internal/llm/factory.go`
- Create: `internal/llm/factory_test.go`

**Step 1: Create factory**

```go
// internal/llm/factory.go
package llm

import (
	"fmt"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/llm/claude"
	"github.com/newthinker/atlas/internal/llm/ollama"
	"github.com/newthinker/atlas/internal/llm/openai"
)

// NewProvider creates an LLM provider from config
func NewProvider(cfg config.LLMConfig) (Provider, error) {
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
```

**Step 2: Create test**

```go
// internal/llm/factory_test.go
package llm

import (
	"testing"

	"github.com/newthinker/atlas/internal/config"
)

func TestNewProvider_UnknownProvider(t *testing.T) {
	cfg := config.LLMConfig{Provider: "unknown"}
	_, err := NewProvider(cfg)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNewProvider_Ollama(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "ollama",
		Ollama:   config.OllamaConfig{Model: "test"},
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %s", p.Name())
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/llm/... -v
```

**Step 4: Commit**

```bash
git add internal/llm/factory.go internal/llm/factory_test.go
git commit -m "feat: add LLM provider factory"
```

---

## Task 6: Context Types and Market Context Provider

**Files:**
- Create: `internal/context/types.go`
- Create: `internal/context/market.go`
- Create: `internal/context/market_test.go`

**Step 1: Create context types**

```go
// internal/context/types.go
package context

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// MarketRegime represents the current market state
type MarketRegime string

const (
	RegimeBull     MarketRegime = "bull"
	RegimeBear     MarketRegime = "bear"
	RegimeSideways MarketRegime = "sideways"
)

// Trend represents a directional trend
type Trend string

const (
	TrendUp   Trend = "up"
	TrendDown Trend = "down"
	TrendFlat Trend = "flat"
)

// MarketContext holds current market conditions
type MarketContext struct {
	Market       core.Market
	Regime       MarketRegime
	Volatility   float64
	SectorTrends map[string]Trend
	UpdatedAt    time.Time
}

// NewsItem represents a news article
type NewsItem struct {
	Title       string
	Summary     string
	Source      string
	URL         string
	Symbols     []string
	Sentiment   float64
	PublishedAt time.Time
}

// StrategyStats holds strategy performance metrics
type StrategyStats struct {
	Strategy     string
	WinRate      float64
	TotalSignals int
	Accuracy     float64
}
```

**Step 2: Create market context provider**

```go
// internal/context/market.go
package context

import (
	"context"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// MarketContextProvider provides market context
type MarketContextProvider interface {
	GetContext(ctx context.Context, market core.Market) (*MarketContext, error)
}

// SimpleMarketContext is a basic implementation
type SimpleMarketContext struct {
	cache map[core.Market]*MarketContext
	mu    sync.RWMutex
}

// NewSimpleMarketContext creates a new provider
func NewSimpleMarketContext() *SimpleMarketContext {
	return &SimpleMarketContext{
		cache: make(map[core.Market]*MarketContext),
	}
}

func (s *SimpleMarketContext) GetContext(ctx context.Context, market core.Market) (*MarketContext, error) {
	s.mu.RLock()
	if mc, ok := s.cache[market]; ok && time.Since(mc.UpdatedAt) < time.Hour {
		s.mu.RUnlock()
		return mc, nil
	}
	s.mu.RUnlock()

	// Default context - can be enhanced with real data later
	mc := &MarketContext{
		Market:       market,
		Regime:       RegimeSideways,
		Volatility:   0.2,
		SectorTrends: make(map[string]Trend),
		UpdatedAt:    time.Now(),
	}

	s.mu.Lock()
	s.cache[market] = mc
	s.mu.Unlock()

	return mc, nil
}

// SetContext allows setting context for testing or manual updates
func (s *SimpleMarketContext) SetContext(market core.Market, mc *MarketContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[market] = mc
}
```

**Step 3: Create test**

```go
// internal/context/market_test.go
package context

import (
	"context"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestSimpleMarketContext_GetContext(t *testing.T) {
	provider := NewSimpleMarketContext()
	ctx := context.Background()

	mc, err := provider.GetContext(ctx, core.MarketUS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Market != core.MarketUS {
		t.Errorf("expected US market, got %s", mc.Market)
	}
}

func TestSimpleMarketContext_Cache(t *testing.T) {
	provider := NewSimpleMarketContext()
	ctx := context.Background()

	mc1, _ := provider.GetContext(ctx, core.MarketUS)
	mc2, _ := provider.GetContext(ctx, core.MarketUS)

	if mc1.UpdatedAt != mc2.UpdatedAt {
		t.Error("expected cached result")
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/context/... -v
```

**Step 5: Commit**

```bash
git add internal/context/
git commit -m "feat: add market context types and provider"
```

---

## Task 7: News Provider

**Files:**
- Create: `internal/context/news.go`
- Create: `internal/context/news_test.go`

**Step 1: Create news provider**

```go
// internal/context/news.go
package context

import (
	"context"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// NewsProvider provides news for symbols and markets
type NewsProvider interface {
	GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error)
	GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error)
}

// SimpleNewsProvider is a basic implementation with caching
type SimpleNewsProvider struct {
	cache     map[string][]NewsItem
	cacheTime map[string]time.Time
	mu        sync.RWMutex
	ttl       time.Duration
}

// NewSimpleNewsProvider creates a new provider
func NewSimpleNewsProvider(ttl time.Duration) *SimpleNewsProvider {
	if ttl == 0 {
		ttl = 6 * time.Hour
	}
	return &SimpleNewsProvider{
		cache:     make(map[string][]NewsItem),
		cacheTime: make(map[string]time.Time),
		ttl:       ttl,
	}
}

func (s *SimpleNewsProvider) GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error) {
	s.mu.RLock()
	if news, ok := s.cache[symbol]; ok {
		if time.Since(s.cacheTime[symbol]) < s.ttl {
			s.mu.RUnlock()
			return s.filterByDays(news, days), nil
		}
	}
	s.mu.RUnlock()

	// Return empty for now - can be extended with real news fetching
	return []NewsItem{}, nil
}

func (s *SimpleNewsProvider) GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error) {
	key := "market:" + string(market)
	s.mu.RLock()
	if news, ok := s.cache[key]; ok {
		if time.Since(s.cacheTime[key]) < s.ttl {
			s.mu.RUnlock()
			return s.filterByDays(news, days), nil
		}
	}
	s.mu.RUnlock()

	return []NewsItem{}, nil
}

func (s *SimpleNewsProvider) filterByDays(news []NewsItem, days int) []NewsItem {
	cutoff := time.Now().AddDate(0, 0, -days)
	var filtered []NewsItem
	for _, n := range news {
		if n.PublishedAt.After(cutoff) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// AddNews allows adding news for testing
func (s *SimpleNewsProvider) AddNews(symbol string, news []NewsItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[symbol] = news
	s.cacheTime[symbol] = time.Now()
}
```

**Step 2: Create test**

```go
// internal/context/news_test.go
package context

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestSimpleNewsProvider_GetNews(t *testing.T) {
	provider := NewSimpleNewsProvider(time.Hour)
	ctx := context.Background()

	news, err := provider.GetNews(ctx, "AAPL", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if news == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestSimpleNewsProvider_AddNews(t *testing.T) {
	provider := NewSimpleNewsProvider(time.Hour)
	ctx := context.Background()

	testNews := []NewsItem{
		{Title: "Test News", PublishedAt: time.Now()},
	}
	provider.AddNews("AAPL", testNews)

	news, _ := provider.GetNews(ctx, "AAPL", 7)
	if len(news) != 1 {
		t.Errorf("expected 1 news item, got %d", len(news))
	}
}

func TestSimpleNewsProvider_FilterByDays(t *testing.T) {
	provider := NewSimpleNewsProvider(time.Hour)
	ctx := context.Background()

	testNews := []NewsItem{
		{Title: "Recent", PublishedAt: time.Now()},
		{Title: "Old", PublishedAt: time.Now().AddDate(0, 0, -10)},
	}
	provider.AddNews("AAPL", testNews)

	news, _ := provider.GetNews(ctx, "AAPL", 7)
	if len(news) != 1 {
		t.Errorf("expected 1 recent news item, got %d", len(news))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/context/... -v
```

**Step 4: Commit**

```bash
git add internal/context/news.go internal/context/news_test.go
git commit -m "feat: add news provider"
```

---

## Task 8: Track Record Provider

**Files:**
- Create: `internal/context/track_record.go`
- Create: `internal/context/track_record_test.go`

**Step 1: Create track record provider**

```go
// internal/context/track_record.go
package context

import (
	"context"
	"sync"

	"github.com/newthinker/atlas/internal/core"
)

// TrackRecordProvider provides strategy performance stats
type TrackRecordProvider interface {
	GetStats(ctx context.Context, strategy string) (*StrategyStats, error)
	GetAllStats(ctx context.Context) (map[string]StrategyStats, error)
	RecordSignal(ctx context.Context, signal core.Signal, outcome bool) error
}

// SimpleTrackRecord is an in-memory implementation
type SimpleTrackRecord struct {
	records map[string]*trackRecord
	mu      sync.RWMutex
}

type trackRecord struct {
	wins   int
	total  int
}

// NewSimpleTrackRecord creates a new provider
func NewSimpleTrackRecord() *SimpleTrackRecord {
	return &SimpleTrackRecord{
		records: make(map[string]*trackRecord),
	}
}

func (s *SimpleTrackRecord) GetStats(ctx context.Context, strategy string) (*StrategyStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.records[strategy]
	if !ok {
		return &StrategyStats{Strategy: strategy}, nil
	}

	winRate := 0.0
	if record.total > 0 {
		winRate = float64(record.wins) / float64(record.total)
	}

	return &StrategyStats{
		Strategy:     strategy,
		WinRate:      winRate,
		TotalSignals: record.total,
		Accuracy:     winRate,
	}, nil
}

func (s *SimpleTrackRecord) GetAllStats(ctx context.Context) (map[string]StrategyStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]StrategyStats)
	for strategy, record := range s.records {
		winRate := 0.0
		if record.total > 0 {
			winRate = float64(record.wins) / float64(record.total)
		}
		stats[strategy] = StrategyStats{
			Strategy:     strategy,
			WinRate:      winRate,
			TotalSignals: record.total,
			Accuracy:     winRate,
		}
	}
	return stats, nil
}

func (s *SimpleTrackRecord) RecordSignal(ctx context.Context, signal core.Signal, outcome bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[signal.Strategy]; !ok {
		s.records[signal.Strategy] = &trackRecord{}
	}

	s.records[signal.Strategy].total++
	if outcome {
		s.records[signal.Strategy].wins++
	}

	return nil
}
```

**Step 2: Create test**

```go
// internal/context/track_record_test.go
package context

import (
	"context"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestSimpleTrackRecord_RecordAndGetStats(t *testing.T) {
	provider := NewSimpleTrackRecord()
	ctx := context.Background()

	signal := core.Signal{Strategy: "test_strategy"}

	// Record 3 wins, 2 losses
	provider.RecordSignal(ctx, signal, true)
	provider.RecordSignal(ctx, signal, true)
	provider.RecordSignal(ctx, signal, true)
	provider.RecordSignal(ctx, signal, false)
	provider.RecordSignal(ctx, signal, false)

	stats, err := provider.GetStats(ctx, "test_strategy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.TotalSignals != 5 {
		t.Errorf("expected 5 signals, got %d", stats.TotalSignals)
	}

	expectedWinRate := 0.6
	if stats.WinRate != expectedWinRate {
		t.Errorf("expected win rate %f, got %f", expectedWinRate, stats.WinRate)
	}
}

func TestSimpleTrackRecord_GetAllStats(t *testing.T) {
	provider := NewSimpleTrackRecord()
	ctx := context.Background()

	provider.RecordSignal(ctx, core.Signal{Strategy: "strategy1"}, true)
	provider.RecordSignal(ctx, core.Signal{Strategy: "strategy2"}, false)

	stats, _ := provider.GetAllStats(ctx)
	if len(stats) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(stats))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/context/... -v
```

**Step 4: Commit**

```bash
git add internal/context/track_record.go internal/context/track_record_test.go
git commit -m "feat: add track record provider"
```

---

## Task 9: Signal Arbitrator

**Files:**
- Create: `internal/meta/arbitrator.go`
- Create: `internal/meta/arbitrator_test.go`
- Create: `internal/meta/prompts/arbitrator.txt`

**Step 1: Create prompts directory and arbitrator prompt**

```bash
mkdir -p internal/meta/prompts
```

```text
// internal/meta/prompts/arbitrator.txt
You are a trading signal arbitrator. Your job is to resolve conflicts when multiple trading strategies disagree about what action to take for a stock.

You will be given:
1. Conflicting signals from different strategies
2. Current market context (regime, volatility)
3. Historical performance of each strategy
4. Recent news about the stock

Analyze all inputs and decide the best action. Consider:
- Which strategy has better historical accuracy in similar market conditions?
- Does recent news support one signal over another?
- What is the overall market regime and how does it affect each strategy?

Respond with a JSON object:
{
  "decision": "buy" | "sell" | "hold",
  "confidence": 0.0-1.0,
  "reasoning": "Your explanation",
  "weighted_from": ["strategy1", "strategy2"]
}
```

**Step 2: Create arbitrator**

```go
// internal/meta/arbitrator.go
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

// Arbitrator resolves conflicting signals using LLM
type Arbitrator struct {
	llm           llm.Provider
	marketContext atlasctx.MarketContextProvider
	trackRecord   atlasctx.TrackRecordProvider
	newsProvider  atlasctx.NewsProvider
	systemPrompt  string
}

// ArbitratorConfig holds configuration
type ArbitratorConfig struct {
	SystemPrompt string
}

// NewArbitrator creates a new arbitrator
func NewArbitrator(
	llmProvider llm.Provider,
	marketCtx atlasctx.MarketContextProvider,
	trackRecord atlasctx.TrackRecordProvider,
	news atlasctx.NewsProvider,
	cfg ArbitratorConfig,
) *Arbitrator {
	prompt := cfg.SystemPrompt
	if prompt == "" {
		prompt = defaultArbitratorPrompt
	}
	return &Arbitrator{
		llm:           llmProvider,
		marketContext: marketCtx,
		trackRecord:   trackRecord,
		newsProvider:  news,
		systemPrompt:  prompt,
	}
}

// ArbitrationRequest holds the input for arbitration
type ArbitrationRequest struct {
	Symbol             string
	Market             core.Market
	ConflictingSignals []core.Signal
}

// ArbitrationResult holds the arbitration output
type ArbitrationResult struct {
	Decision     core.Action
	Confidence   float64
	Reasoning    string
	WeightedFrom []string
}

// Arbitrate resolves conflicting signals
func (a *Arbitrator) Arbitrate(ctx context.Context, req ArbitrationRequest) (*ArbitrationResult, error) {
	// Gather context
	marketCtx, err := a.marketContext.GetContext(ctx, req.Market)
	if err != nil {
		return nil, fmt.Errorf("getting market context: %w", err)
	}

	news, err := a.newsProvider.GetNews(ctx, req.Symbol, 7)
	if err != nil {
		return nil, fmt.Errorf("getting news: %w", err)
	}

	stats, err := a.trackRecord.GetAllStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting track record: %w", err)
	}

	// Build prompt
	userMsg := a.buildUserMessage(req, marketCtx, news, stats)

	// Call LLM
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		SystemPrompt: a.systemPrompt,
		Messages:     []llm.Message{{Role: "user", Content: userMsg}},
		MaxTokens:    1000,
		Temperature:  0.3,
		JSONMode:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Parse response
	return a.parseResponse(resp.Content)
}

func (a *Arbitrator) buildUserMessage(
	req ArbitrationRequest,
	marketCtx *atlasctx.MarketContext,
	news []atlasctx.NewsItem,
	stats map[string]atlasctx.StrategyStats,
) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Symbol: %s\n\n", req.Symbol))

	sb.WriteString("Conflicting Signals:\n")
	for _, sig := range req.ConflictingSignals {
		sb.WriteString(fmt.Sprintf("- %s: %s (confidence: %.2f) - %s\n",
			sig.Strategy, sig.Action, sig.Confidence, sig.Reason))
	}

	sb.WriteString(fmt.Sprintf("\nMarket Context:\n- Regime: %s\n- Volatility: %.2f\n",
		marketCtx.Regime, marketCtx.Volatility))

	sb.WriteString("\nStrategy Performance:\n")
	for _, sig := range req.ConflictingSignals {
		if s, ok := stats[sig.Strategy]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %.1f%% win rate (%d signals)\n",
				s.Strategy, s.WinRate*100, s.TotalSignals))
		}
	}

	if len(news) > 0 {
		sb.WriteString("\nRecent News:\n")
		for _, n := range news[:min(3, len(news))] {
			sb.WriteString(fmt.Sprintf("- %s (sentiment: %.2f)\n", n.Title, n.Sentiment))
		}
	}

	return sb.String()
}

func (a *Arbitrator) parseResponse(content string) (*ArbitrationResult, error) {
	var resp struct {
		Decision     string   `json:"decision"`
		Confidence   float64  `json:"confidence"`
		Reasoning    string   `json:"reasoning"`
		WeightedFrom []string `json:"weighted_from"`
	}

	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w", err)
	}

	action := core.ActionHold
	switch strings.ToLower(resp.Decision) {
	case "buy":
		action = core.ActionBuy
	case "sell":
		action = core.ActionSell
	case "strong_buy":
		action = core.ActionStrongBuy
	case "strong_sell":
		action = core.ActionStrongSell
	}

	return &ArbitrationResult{
		Decision:     action,
		Confidence:   resp.Confidence,
		Reasoning:    resp.Reasoning,
		WeightedFrom: resp.WeightedFrom,
	}, nil
}

const defaultArbitratorPrompt = `You are a trading signal arbitrator. Your job is to resolve conflicts when multiple trading strategies disagree about what action to take for a stock.

Analyze the conflicting signals, market context, strategy performance, and news to decide the best action.

Respond with a JSON object:
{
  "decision": "buy" | "sell" | "hold",
  "confidence": 0.0-1.0,
  "reasoning": "Your explanation",
  "weighted_from": ["strategy1", "strategy2"]
}`

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

**Step 3: Create test**

```go
// internal/meta/arbitrator_test.go
package meta

import (
	"context"
	"testing"

	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

type mockLLM struct {
	response string
}

func (m *mockLLM) Name() string { return "mock" }
func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response}, nil
}

func TestArbitrator_Arbitrate(t *testing.T) {
	mockResponse := `{"decision": "buy", "confidence": 0.8, "reasoning": "Test", "weighted_from": ["strategy1"]}`

	arb := NewArbitrator(
		&mockLLM{response: mockResponse},
		atlasctx.NewSimpleMarketContext(),
		atlasctx.NewSimpleTrackRecord(),
		atlasctx.NewSimpleNewsProvider(0),
		ArbitratorConfig{},
	)

	req := ArbitrationRequest{
		Symbol: "AAPL",
		Market: core.MarketUS,
		ConflictingSignals: []core.Signal{
			{Strategy: "strategy1", Action: core.ActionBuy, Confidence: 0.7},
			{Strategy: "strategy2", Action: core.ActionSell, Confidence: 0.6},
		},
	}

	result, err := arb.Arbitrate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != core.ActionBuy {
		t.Errorf("expected buy, got %s", result.Decision)
	}

	if result.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", result.Confidence)
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/meta/... -v
```

**Step 5: Commit**

```bash
git add internal/meta/
git commit -m "feat: add signal arbitrator with LLM"
```

---

## Task 10: Strategy Synthesizer

**Files:**
- Create: `internal/meta/synthesizer.go`
- Create: `internal/meta/synthesizer_test.go`
- Create: `internal/meta/prompts/synthesizer.txt`

**Step 1: Create synthesizer**

```go
// internal/meta/synthesizer.go
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

// Synthesizer analyzes historical performance to suggest improvements
type Synthesizer struct {
	llm          llm.Provider
	systemPrompt string
}

// SynthesizerConfig holds configuration
type SynthesizerConfig struct {
	SystemPrompt string
}

// NewSynthesizer creates a new synthesizer
func NewSynthesizer(llmProvider llm.Provider, cfg SynthesizerConfig) *Synthesizer {
	prompt := cfg.SystemPrompt
	if prompt == "" {
		prompt = defaultSynthesizerPrompt
	}
	return &Synthesizer{
		llm:          llmProvider,
		systemPrompt: prompt,
	}
}

// SynthesisRequest holds the input for synthesis
type SynthesisRequest struct {
	StartDate  time.Time
	EndDate    time.Time
	Strategies []string
	Trades     []backtest.Trade
	Signals    []core.Signal
}

// SynthesisResult holds the synthesis output
type SynthesisResult struct {
	ParameterSuggestions []ParameterSuggestion
	CombinationRules     []CombinationRule
	Explanation          string
}

// ParameterSuggestion suggests a parameter change
type ParameterSuggestion struct {
	Strategy     string
	Parameter    string
	CurrentVal   any
	SuggestedVal any
	Rationale    string
}

// CombinationRule suggests combining signals
type CombinationRule struct {
	Conditions []string
	Action     core.Action
	Confidence float64
	Evidence   string
}

// Synthesize analyzes data and suggests improvements
func (s *Synthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	userMsg := s.buildUserMessage(req)

	resp, err := s.llm.Chat(ctx, llm.ChatRequest{
		SystemPrompt: s.systemPrompt,
		Messages:     []llm.Message{{Role: "user", Content: userMsg}},
		MaxTokens:    2000,
		Temperature:  0.5,
		JSONMode:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	return s.parseResponse(resp.Content)
}

func (s *Synthesizer) buildUserMessage(req SynthesisRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Analysis Period: %s to %s\n\n",
		req.StartDate.Format("2006-01-02"), req.EndDate.Format("2006-01-02")))

	sb.WriteString(fmt.Sprintf("Strategies: %s\n\n", strings.Join(req.Strategies, ", ")))

	// Summarize trades
	sb.WriteString(fmt.Sprintf("Total Trades: %d\n", len(req.Trades)))

	wins, losses := 0, 0
	var totalReturn float64
	for _, t := range req.Trades {
		if t.Return > 0 {
			wins++
		} else {
			losses++
		}
		totalReturn += t.Return
	}
	sb.WriteString(fmt.Sprintf("Wins: %d, Losses: %d\n", wins, losses))
	sb.WriteString(fmt.Sprintf("Total Return: %.2f%%\n\n", totalReturn*100))

	// Signal summary
	signalCounts := make(map[string]int)
	for _, sig := range req.Signals {
		signalCounts[sig.Strategy]++
	}
	sb.WriteString("Signals by Strategy:\n")
	for strategy, count := range signalCounts {
		sb.WriteString(fmt.Sprintf("- %s: %d signals\n", strategy, count))
	}

	return sb.String()
}

func (s *Synthesizer) parseResponse(content string) (*SynthesisResult, error) {
	var resp struct {
		ParameterSuggestions []struct {
			Strategy     string `json:"strategy"`
			Parameter    string `json:"parameter"`
			CurrentVal   any    `json:"current_val"`
			SuggestedVal any    `json:"suggested_val"`
			Rationale    string `json:"rationale"`
		} `json:"parameter_suggestions"`
		CombinationRules []struct {
			Conditions []string `json:"conditions"`
			Action     string   `json:"action"`
			Confidence float64  `json:"confidence"`
			Evidence   string   `json:"evidence"`
		} `json:"combination_rules"`
		Explanation string `json:"explanation"`
	}

	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w", err)
	}

	result := &SynthesisResult{
		Explanation: resp.Explanation,
	}

	for _, ps := range resp.ParameterSuggestions {
		result.ParameterSuggestions = append(result.ParameterSuggestions, ParameterSuggestion{
			Strategy:     ps.Strategy,
			Parameter:    ps.Parameter,
			CurrentVal:   ps.CurrentVal,
			SuggestedVal: ps.SuggestedVal,
			Rationale:    ps.Rationale,
		})
	}

	for _, cr := range resp.CombinationRules {
		action := core.ActionHold
		switch strings.ToLower(cr.Action) {
		case "buy":
			action = core.ActionBuy
		case "sell":
			action = core.ActionSell
		}
		result.CombinationRules = append(result.CombinationRules, CombinationRule{
			Conditions: cr.Conditions,
			Action:     action,
			Confidence: cr.Confidence,
			Evidence:   cr.Evidence,
		})
	}

	return result, nil
}

const defaultSynthesizerPrompt = `You are a trading strategy synthesizer. Analyze historical trading performance and suggest improvements.

Based on the trade history and signals provided, suggest:
1. Parameter adjustments for existing strategies
2. Combination rules (when multiple signals together indicate opportunity)

Respond with a JSON object:
{
  "parameter_suggestions": [
    {"strategy": "name", "parameter": "param", "current_val": X, "suggested_val": Y, "rationale": "why"}
  ],
  "combination_rules": [
    {"conditions": ["strategy1=buy", "strategy2=buy"], "action": "buy", "confidence": 0.8, "evidence": "explanation"}
  ],
  "explanation": "Overall analysis summary"
}`
```

**Step 2: Create test**

```go
// internal/meta/synthesizer_test.go
package meta

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
)

func TestSynthesizer_Synthesize(t *testing.T) {
	mockResponse := `{
		"parameter_suggestions": [
			{"strategy": "ma_crossover", "parameter": "fast_period", "current_val": 50, "suggested_val": 40, "rationale": "Better performance"}
		],
		"combination_rules": [],
		"explanation": "Test analysis"
	}`

	synth := NewSynthesizer(&mockLLM{response: mockResponse}, SynthesizerConfig{})

	req := SynthesisRequest{
		StartDate:  time.Now().AddDate(-1, 0, 0),
		EndDate:    time.Now(),
		Strategies: []string{"ma_crossover"},
		Trades: []backtest.Trade{
			{Return: 0.05},
			{Return: -0.02},
		},
		Signals: []core.Signal{
			{Strategy: "ma_crossover", Action: core.ActionBuy},
		},
	}

	result, err := synth.Synthesize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ParameterSuggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(result.ParameterSuggestions))
	}

	if result.ParameterSuggestions[0].Strategy != "ma_crossover" {
		t.Errorf("expected ma_crossover, got %s", result.ParameterSuggestions[0].Strategy)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/meta/... -v
```

**Step 4: Commit**

```bash
git add internal/meta/synthesizer.go internal/meta/synthesizer_test.go
git commit -m "feat: add strategy synthesizer with LLM"
```

---

## Task 11: Broker Interface

**Files:**
- Create: `internal/broker/interface.go`
- Create: `internal/broker/interface_test.go`

**Step 1: Create broker interface**

```go
// internal/broker/interface.go
package broker

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Broker defines the interface for broker integrations
type Broker interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Connection
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Read operations
	GetPositions(ctx context.Context) ([]Position, error)
	GetOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	GetTradeHistory(ctx context.Context, start, end time.Time) ([]Trade, error)
}

// Position represents a holding
type Position struct {
	Symbol       string
	Market       core.Market
	Quantity     int64
	AvgCost      float64
	MarketValue  float64
	UnrealizedPL float64
	RealizedPL   float64
}

// AccountInfo holds account details
type AccountInfo struct {
	TotalAssets   float64
	Cash          float64
	BuyingPower   float64
	MarginUsed    float64
	DayTradesLeft int
}

// OrderSide represents buy or sell
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType represents order types
type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
	OrderTypeStop   OrderType = "stop"
)

// OrderStatus represents order status
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusPartial   OrderStatus = "partial"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRejected  OrderStatus = "rejected"
)

// Order represents a broker order
type Order struct {
	OrderID      string
	Symbol       string
	Market       core.Market
	Side         OrderSide
	Type         OrderType
	Quantity     int64
	Price        float64
	Status       OrderStatus
	FilledQty    int64
	AvgFillPrice float64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OrderFilter for querying orders
type OrderFilter struct {
	Symbol    string
	Status    OrderStatus
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// Trade represents an executed trade
type Trade struct {
	TradeID   string
	OrderID   string
	Symbol    string
	Market    core.Market
	Side      OrderSide
	Quantity  int64
	Price     float64
	Fee       float64
	ExecutedAt time.Time
}
```

**Step 2: Create test**

```go
// internal/broker/interface_test.go
package broker

import "testing"

func TestInterfaceDefined(t *testing.T) {
	var _ Broker = nil
}
```

**Step 3: Run tests**

```bash
go test ./internal/broker/... -v
```

**Step 4: Commit**

```bash
git add internal/broker/
git commit -m "feat: add broker interface"
```

---

## Task 12: Broker Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

**Step 1: Add broker config**

Add to config.go:

```go
type BrokerConfig struct {
	Enabled  bool       `mapstructure:"enabled"`
	Provider string     `mapstructure:"provider"`
	Futu     FutuConfig `mapstructure:"futu"`
}

type FutuConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Env           string `mapstructure:"env"` // "simulate" or "real"
	TradePassword string `mapstructure:"trade_password"`
	RSAKeyPath    string `mapstructure:"rsa_key_path"`
}
```

Add `Broker BrokerConfig` to main Config struct.

**Step 2: Update config.example.yaml**

```yaml
broker:
  enabled: false
  provider: futu
  futu:
    host: "127.0.0.1"
    port: 11111
    env: simulate
    trade_password: "${FUTU_TRADE_PWD}"
    rsa_key_path: ""
```

**Step 3: Run tests**

```bash
go test ./internal/config/... -v
```

**Step 4: Commit**

```bash
git add internal/config/config.go config.example.yaml
git commit -m "feat: add broker configuration"
```

---

## Task 13: Mock Broker Implementation

**Files:**
- Create: `internal/broker/mock/mock.go`
- Create: `internal/broker/mock/mock_test.go`

**Step 1: Create mock broker**

```go
// internal/broker/mock/mock.go
package mock

import (
	"context"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

// MockBroker is a mock implementation for testing
type MockBroker struct {
	connected  bool
	positions  []broker.Position
	orders     []broker.Order
	trades     []broker.Trade
	account    *broker.AccountInfo
	mu         sync.RWMutex
}

// New creates a new mock broker
func New() *MockBroker {
	return &MockBroker{
		account: &broker.AccountInfo{
			TotalAssets: 100000,
			Cash:        50000,
			BuyingPower: 50000,
		},
	}
}

func (m *MockBroker) Name() string {
	return "mock"
}

func (m *MockBroker) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketHK, core.MarketCN_A}
}

func (m *MockBroker) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *MockBroker) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

func (m *MockBroker) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *MockBroker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.positions, nil
}

func (m *MockBroker) GetOrders(ctx context.Context, filter broker.OrderFilter) ([]broker.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.orders, nil
}

func (m *MockBroker) GetAccountInfo(ctx context.Context) (*broker.AccountInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.account, nil
}

func (m *MockBroker) GetTradeHistory(ctx context.Context, start, end time.Time) ([]broker.Trade, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trades, nil
}

// SetPositions sets mock positions
func (m *MockBroker) SetPositions(positions []broker.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.positions = positions
}

// SetOrders sets mock orders
func (m *MockBroker) SetOrders(orders []broker.Order) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders = orders
}
```

**Step 2: Create test**

```go
// internal/broker/mock/mock_test.go
package mock

import (
	"context"
	"testing"

	"github.com/newthinker/atlas/internal/broker"
)

func TestMockBroker_ImplementsInterface(t *testing.T) {
	var _ broker.Broker = (*MockBroker)(nil)
}

func TestMockBroker_Connect(t *testing.T) {
	b := New()
	ctx := context.Background()

	if b.IsConnected() {
		t.Error("should not be connected initially")
	}

	if err := b.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if !b.IsConnected() {
		t.Error("should be connected after Connect()")
	}
}

func TestMockBroker_GetAccountInfo(t *testing.T) {
	b := New()
	b.Connect(context.Background())

	info, err := b.GetAccountInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.TotalAssets != 100000 {
		t.Errorf("expected 100000, got %f", info.TotalAssets)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/broker/... -v
```

**Step 4: Commit**

```bash
git add internal/broker/mock/
git commit -m "feat: add mock broker implementation"
```

---

## Task 14: Broker CLI Commands

**Files:**
- Create: `cmd/atlas/broker.go`

**Step 1: Create broker commands**

```go
// cmd/atlas/broker.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/newthinker/atlas/internal/broker/mock"
	"github.com/spf13/cobra"
)

var brokerCmd = &cobra.Command{
	Use:   "broker",
	Short: "Broker operations",
}

var brokerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check broker connection status",
	Run:   runBrokerStatus,
}

var brokerPositionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "List current positions",
	Run:   runBrokerPositions,
}

var brokerOrdersCmd = &cobra.Command{
	Use:   "orders",
	Short: "List recent orders",
	Run:   runBrokerOrders,
}

func init() {
	brokerCmd.AddCommand(brokerStatusCmd)
	brokerCmd.AddCommand(brokerPositionsCmd)
	brokerCmd.AddCommand(brokerOrdersCmd)
	rootCmd.AddCommand(brokerCmd)
}

func runBrokerStatus(cmd *cobra.Command, args []string) {
	// TODO: Use actual broker from config
	b := mock.New()
	ctx := context.Background()

	if err := b.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Broker: %s\n", b.Name())
	fmt.Printf("Connected: %v\n", b.IsConnected())

	info, err := b.GetAccountInfo(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get account info: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nAccount Info:\n")
	fmt.Printf("  Total Assets: $%.2f\n", info.TotalAssets)
	fmt.Printf("  Cash: $%.2f\n", info.Cash)
	fmt.Printf("  Buying Power: $%.2f\n", info.BuyingPower)
}

func runBrokerPositions(cmd *cobra.Command, args []string) {
	b := mock.New()
	ctx := context.Background()
	b.Connect(ctx)

	positions, err := b.GetPositions(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get positions: %v\n", err)
		os.Exit(1)
	}

	if len(positions) == 0 {
		fmt.Println("No positions")
		return
	}

	fmt.Printf("%-10s %-8s %10s %12s %12s\n", "Symbol", "Market", "Qty", "Avg Cost", "P&L")
	fmt.Println("------------------------------------------------------")
	for _, p := range positions {
		fmt.Printf("%-10s %-8s %10d %12.2f %12.2f\n",
			p.Symbol, p.Market, p.Quantity, p.AvgCost, p.UnrealizedPL)
	}
}

func runBrokerOrders(cmd *cobra.Command, args []string) {
	b := mock.New()
	ctx := context.Background()
	b.Connect(ctx)

	orders, err := b.GetOrders(ctx, broker.OrderFilter{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get orders: %v\n", err)
		os.Exit(1)
	}

	if len(orders) == 0 {
		fmt.Println("No orders")
		return
	}

	fmt.Printf("%-12s %-10s %-6s %10s %12s %-10s\n", "OrderID", "Symbol", "Side", "Qty", "Price", "Status")
	fmt.Println("------------------------------------------------------------")
	for _, o := range orders {
		fmt.Printf("%-12s %-10s %-6s %10d %12.2f %-10s\n",
			o.OrderID, o.Symbol, o.Side, o.Quantity, o.Price, o.Status)
	}
}
```

**Step 2: Add broker import to broker.go**

Add at top of file:

```go
import (
	"github.com/newthinker/atlas/internal/broker"
)
```

**Step 3: Build and test CLI**

```bash
go build -o bin/atlas ./cmd/atlas
./bin/atlas broker status
./bin/atlas broker positions
./bin/atlas broker orders
```

**Step 4: Commit**

```bash
git add cmd/atlas/broker.go
git commit -m "feat: add broker CLI commands"
```

---

## Task 15: Final Integration Test

**Step 1: Run full test suite**

```bash
go test ./... -cover
```

**Step 2: Build and verify**

```bash
go build -o bin/atlas ./cmd/atlas
go vet ./...
./bin/atlas version
./bin/atlas --help
./bin/atlas broker --help
```

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "chore: Phase 4 final cleanup and integration"
```

---

## Summary

Phase 4 adds:
- **LLM Providers** (Tasks 1-5): Interface, Claude, OpenAI, Ollama, Factory
- **Context Providers** (Tasks 6-8): Market context, News, Track record
- **Meta-Strategies** (Tasks 9-10): Signal Arbitrator, Strategy Synthesizer
- **Broker Integration** (Tasks 11-14): Interface, Config, Mock, CLI
- **Integration** (Task 15): Full test suite and verification
