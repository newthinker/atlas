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

// Arbitrator uses LLM to resolve conflicting trading signals.
type Arbitrator struct {
	llm           llm.Provider
	marketContext atlasctx.MarketContextProvider
	trackRecord   atlasctx.TrackRecordProvider
	newsProvider  atlasctx.NewsProvider
	contextDays   int
}

// ArbitratorConfig holds arbitrator configuration.
type ArbitratorConfig struct {
	ContextDays int
}

// NewArbitrator creates a new signal arbitrator.
func NewArbitrator(
	llmProvider llm.Provider,
	marketCtx atlasctx.MarketContextProvider,
	trackRecord atlasctx.TrackRecordProvider,
	newsProvider atlasctx.NewsProvider,
	cfg ArbitratorConfig,
) *Arbitrator {
	if cfg.ContextDays <= 0 {
		cfg.ContextDays = 7
	}
	return &Arbitrator{
		llm:           llmProvider,
		marketContext: marketCtx,
		trackRecord:   trackRecord,
		newsProvider:  newsProvider,
		contextDays:   cfg.ContextDays,
	}
}

// ArbitrationRequest contains the data for arbitration.
type ArbitrationRequest struct {
	Symbol             string
	Market             core.Market
	ConflictingSignals []core.Signal
}

// ArbitrationResult contains the LLM's decision.
type ArbitrationResult struct {
	Decision     core.Action `json:"decision"`
	Confidence   float64     `json:"confidence"`
	Reasoning    string      `json:"reasoning"`
	WeightedFrom []string    `json:"weighted_from"`
}

// Arbitrate resolves conflicting signals using LLM analysis.
func (a *Arbitrator) Arbitrate(ctx context.Context, req ArbitrationRequest) (*ArbitrationResult, error) {
	if len(req.ConflictingSignals) == 0 {
		return nil, fmt.Errorf("no signals to arbitrate")
	}

	// If all signals agree, no arbitration needed
	if allSignalsAgree(req.ConflictingSignals) {
		return &ArbitrationResult{
			Decision:     req.ConflictingSignals[0].Action,
			Confidence:   avgConfidence(req.ConflictingSignals),
			Reasoning:    "All strategies agree on the action",
			WeightedFrom: getStrategyNames(req.ConflictingSignals),
		}, nil
	}

	// Gather context
	marketCtx, _ := a.marketContext.GetContext(ctx, req.Market)
	news, _ := a.newsProvider.GetNews(ctx, req.Symbol, a.contextDays)
	allStats, _ := a.trackRecord.GetAllStats(ctx)

	// Build prompt
	prompt := a.buildPrompt(req, marketCtx, news, allStats)

	// Call LLM
	llmReq := llm.ChatRequest{
		SystemPrompt: arbitratorSystemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
		JSONMode:    true,
	}

	resp, err := a.llm.Chat(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Parse response
	var result ArbitrationResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// If JSON parsing fails, try to extract from text
		return a.parseTextResponse(resp.Content, req.ConflictingSignals)
	}

	return &result, nil
}

func (a *Arbitrator) buildPrompt(
	req ArbitrationRequest,
	marketCtx *atlasctx.MarketContext,
	news []atlasctx.NewsItem,
	stats map[string]*atlasctx.StrategyStats,
) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Symbol: %s\n\n", req.Symbol))

	// Conflicting signals
	sb.WriteString("## Conflicting Signals:\n")
	for _, sig := range req.ConflictingSignals {
		sb.WriteString(fmt.Sprintf("- **%s**: %s (confidence: %.2f)\n",
			sig.Strategy, sig.Action, sig.Confidence))
		if sig.Reason != "" {
			sb.WriteString(fmt.Sprintf("  Reason: %s\n", sig.Reason))
		}
	}
	sb.WriteString("\n")

	// Market context
	if marketCtx != nil {
		sb.WriteString("## Market Context:\n")
		sb.WriteString(fmt.Sprintf("- Regime: %s\n", marketCtx.Regime))
		sb.WriteString(fmt.Sprintf("- Volatility: %.2f%%\n", marketCtx.Volatility*100))
		sb.WriteString("\n")
	}

	// Strategy performance
	if len(stats) > 0 {
		sb.WriteString("## Strategy Track Records:\n")
		for name, s := range stats {
			if s.TotalSignals > 0 {
				sb.WriteString(fmt.Sprintf("- **%s**: Win rate %.1f%%, Avg return %.2f%%, Signals: %d\n",
					name, s.WinRate*100, s.AvgReturn*100, s.TotalSignals))
			}
		}
		sb.WriteString("\n")
	}

	// Recent news
	if len(news) > 0 {
		sb.WriteString("## Recent News:\n")
		for i, n := range news {
			if i >= 5 {
				break
			}
			sentiment := "neutral"
			if n.Sentiment > 0.3 {
				sentiment = "positive"
			} else if n.Sentiment < -0.3 {
				sentiment = "negative"
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", n.PublishedAt.Format("Jan 2"), n.Title, sentiment))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Task:\n")
	sb.WriteString("Analyze the conflicting signals and determine the best action.\n")
	sb.WriteString("Consider strategy track records, market conditions, and recent news.\n")
	sb.WriteString("Respond with JSON containing: decision (BUY/SELL/HOLD), confidence (0-1), reasoning, weighted_from (strategy names).\n")

	return sb.String()
}

func (a *Arbitrator) parseTextResponse(text string, signals []core.Signal) (*ArbitrationResult, error) {
	// Default to HOLD if we can't parse
	result := &ArbitrationResult{
		Decision:     core.ActionHold,
		Confidence:   0.5,
		Reasoning:    text,
		WeightedFrom: getStrategyNames(signals),
	}

	// Try to find action keywords
	textUpper := strings.ToUpper(text)
	if strings.Contains(textUpper, "BUY") && !strings.Contains(textUpper, "SELL") {
		result.Decision = core.ActionBuy
	} else if strings.Contains(textUpper, "SELL") && !strings.Contains(textUpper, "BUY") {
		result.Decision = core.ActionSell
	}

	return result, nil
}

func allSignalsAgree(signals []core.Signal) bool {
	if len(signals) <= 1 {
		return true
	}
	action := signals[0].Action
	for _, s := range signals[1:] {
		if s.Action != action {
			return false
		}
	}
	return true
}

func avgConfidence(signals []core.Signal) float64 {
	if len(signals) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range signals {
		sum += s.Confidence
	}
	return sum / float64(len(signals))
}

func getStrategyNames(signals []core.Signal) []string {
	names := make([]string, len(signals))
	for i, s := range signals {
		names[i] = s.Strategy
	}
	return names
}

const arbitratorSystemPrompt = `You are a trading signal arbitrator. Your role is to analyze conflicting trading signals from multiple strategies and determine the best course of action.

Consider:
1. Strategy track records - favor strategies with better historical performance
2. Market regime - adapt recommendations to current market conditions
3. Recent news - factor in sentiment and fundamental developments
4. Signal confidence - weight by confidence levels

Always respond with valid JSON in this format:
{
  "decision": "BUY" | "SELL" | "HOLD",
  "confidence": 0.0-1.0,
  "reasoning": "explanation of your decision",
  "weighted_from": ["strategy1", "strategy2"]
}

Be conservative when uncertain. HOLD is appropriate when signals are mixed and context is unclear.`
