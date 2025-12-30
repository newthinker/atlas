// internal/meta/synthesizer.go
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
	"go.uber.org/zap"
)

// Synthesizer uses LLM to analyze trading history and suggest improvements.
type Synthesizer struct {
	llm         llm.Provider
	trackRecord atlasctx.TrackRecordProvider
	logger      *zap.Logger
}

// SynthesizerConfig holds synthesizer configuration.
type SynthesizerConfig struct {
	MinTrades int
}

// NewSynthesizer creates a new strategy synthesizer.
func NewSynthesizer(llmProvider llm.Provider, trackRecord atlasctx.TrackRecordProvider, logger *zap.Logger, cfg SynthesizerConfig) *Synthesizer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Synthesizer{
		llm:         llmProvider,
		trackRecord: trackRecord,
		logger:      logger,
	}
}

// SynthesisRequest contains the data for synthesis analysis.
type SynthesisRequest struct {
	TimeRange  TimeRange
	Strategies []string
	Trades     []backtest.Trade
	Signals    []core.Signal
}

// TimeRange represents a time period.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// SynthesisResult contains LLM-generated improvement suggestions.
type SynthesisResult struct {
	ParameterSuggestions []ParameterSuggestion `json:"parameter_suggestions"`
	NewRules             []RuleProposal        `json:"new_rules"`
	CombinationRules     []CombinationRule     `json:"combination_rules"`
	Explanation          string                `json:"explanation"`
}

// ParameterSuggestion suggests a change to strategy parameters.
type ParameterSuggestion struct {
	Strategy     string  `json:"strategy"`
	Parameter    string  `json:"parameter"`
	CurrentVal   any     `json:"current_val"`
	SuggestedVal any     `json:"suggested_val"`
	Rationale    string  `json:"rationale"`
	BacktestDiff float64 `json:"backtest_diff"`
}

// RuleProposal proposes a new trading rule.
type RuleProposal struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Condition   string `json:"condition"`
	Action      string `json:"action"`
	Evidence    string `json:"evidence"`
}

// CombinationRule proposes combining multiple signals.
type CombinationRule struct {
	Conditions []SignalCondition `json:"conditions"`
	Action     core.Action       `json:"action"`
	Confidence float64           `json:"confidence"`
	Evidence   string            `json:"evidence"`
}

// SignalCondition describes a condition on a signal.
type SignalCondition struct {
	Strategy  string      `json:"strategy"`
	Action    core.Action `json:"action"`
	MinConf   float64     `json:"min_confidence,omitempty"`
}

// Synthesize analyzes trading history and generates improvement suggestions.
func (s *Synthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	if len(req.Trades) == 0 && len(req.Signals) == 0 {
		return nil, fmt.Errorf("no trades or signals to analyze")
	}

	// Get strategy stats with graceful degradation
	allStats, err := s.trackRecord.GetAllStats(ctx)
	if err != nil {
		s.logger.Warn("failed to get track records, proceeding without",
			zap.Error(err))
		allStats = make(map[string]*atlasctx.StrategyStats)
	}

	// Build prompt
	prompt := s.buildPrompt(req, allStats)

	// Call LLM
	llmReq := llm.ChatRequest{
		SystemPrompt: synthesizerSystemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   2048,
		Temperature: 0.4,
		JSONMode:    true,
	}

	resp, err := s.llm.Chat(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Parse response
	var result SynthesisResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// Return basic result if JSON parsing fails
		return &SynthesisResult{
			Explanation: resp.Content,
		}, nil
	}

	return &result, nil
}

func (s *Synthesizer) buildPrompt(req SynthesisRequest, stats map[string]*atlasctx.StrategyStats) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Analysis Period: %s to %s\n\n",
		req.TimeRange.Start.Format("2006-01-02"),
		req.TimeRange.End.Format("2006-01-02")))

	// Trade summary
	if len(req.Trades) > 0 {
		sb.WriteString("## Trade Summary:\n")
		wins, losses := 0, 0
		totalReturn := 0.0
		for _, t := range req.Trades {
			if t.Return > 0 {
				wins++
			} else {
				losses++
			}
			totalReturn += t.Return
		}
		sb.WriteString(fmt.Sprintf("- Total Trades: %d\n", len(req.Trades)))
		sb.WriteString(fmt.Sprintf("- Wins: %d, Losses: %d\n", wins, losses))
		sb.WriteString(fmt.Sprintf("- Total Return: %.2f%%\n", totalReturn*100))
		sb.WriteString("\n")

		// Sample trades
		sb.WriteString("## Recent Trades (sample):\n")
		limit := 10
		if len(req.Trades) < limit {
			limit = len(req.Trades)
		}
		for i := 0; i < limit; i++ {
			t := req.Trades[len(req.Trades)-1-i]
			result := "WIN"
			if t.Return < 0 {
				result = "LOSS"
			}
			sb.WriteString(fmt.Sprintf("- %s %s: %.2f%% (%s)\n",
				t.EntrySignal.Symbol, t.EntrySignal.Action, t.Return*100, result))
		}
		sb.WriteString("\n")
	}

	// Strategy performance
	if len(stats) > 0 {
		sb.WriteString("## Strategy Performance:\n")
		for name, stat := range stats {
			if stat.TotalSignals > 0 {
				sb.WriteString(fmt.Sprintf("- **%s**: Win rate %.1f%%, Avg return %.2f%%, Max drawdown %.1f%%\n",
					name, stat.WinRate*100, stat.AvgReturn*100, stat.MaxDrawdown*100))
			}
		}
		sb.WriteString("\n")
	}

	// Signal patterns
	if len(req.Signals) > 0 {
		sb.WriteString("## Signal Patterns:\n")
		signalCounts := make(map[string]map[core.Action]int)
		for _, sig := range req.Signals {
			if signalCounts[sig.Strategy] == nil {
				signalCounts[sig.Strategy] = make(map[core.Action]int)
			}
			signalCounts[sig.Strategy][sig.Action]++
		}
		for strat, counts := range signalCounts {
			sb.WriteString(fmt.Sprintf("- %s: BUY=%d, SELL=%d, HOLD=%d\n",
				strat, counts[core.ActionBuy], counts[core.ActionSell], counts[core.ActionHold]))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Task:\n")
	sb.WriteString("Analyze the trading history and suggest improvements:\n")
	sb.WriteString("1. Parameter adjustments for existing strategies\n")
	sb.WriteString("2. New trading rules based on observed patterns\n")
	sb.WriteString("3. Signal combination rules (when multiple strategies align)\n")
	sb.WriteString("\nRespond with JSON containing: parameter_suggestions, new_rules, combination_rules, explanation.\n")

	return sb.String()
}

const synthesizerSystemPrompt = `You are a trading strategy analyst. Your role is to analyze historical trading performance and suggest improvements.

Focus on:
1. Parameter optimization - identify parameters that could be tuned for better results
2. Pattern recognition - find trading patterns in successful/failed trades
3. Signal combinations - identify when multiple signals together predict success
4. Risk management - suggest improvements to reduce drawdowns

Always respond with valid JSON:
{
  "parameter_suggestions": [
    {
      "strategy": "strategy_name",
      "parameter": "param_name",
      "current_val": current_value,
      "suggested_val": suggested_value,
      "rationale": "why this change",
      "backtest_diff": estimated_improvement
    }
  ],
  "new_rules": [
    {
      "name": "rule_name",
      "description": "what the rule does",
      "condition": "when to apply",
      "action": "BUY/SELL/HOLD",
      "evidence": "supporting data"
    }
  ],
  "combination_rules": [
    {
      "conditions": [{"strategy": "name", "action": "BUY", "min_confidence": 0.7}],
      "action": "BUY",
      "confidence": 0.8,
      "evidence": "supporting data"
    }
  ],
  "explanation": "overall summary"
}

Be specific and evidence-based. Only suggest changes with clear supporting data.`
