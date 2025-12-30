// internal/meta/synthesizer_test.go
package meta

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

func TestSynthesizer_Synthesize(t *testing.T) {
	synth := NewSynthesizer(
		&mockSynthLLM{},
		atlasctx.NewInMemoryTrackRecord(),
		nil, // logger
		SynthesizerConfig{},
	)

	ctx := context.Background()
	result, err := synth.Synthesize(ctx, SynthesisRequest{
		TimeRange: TimeRange{
			Start: time.Now().AddDate(0, -1, 0),
			End:   time.Now(),
		},
		Trades: []backtest.Trade{
			{EntrySignal: core.Signal{Symbol: "AAPL", Action: core.ActionBuy}, Return: 0.05},
			{EntrySignal: core.Signal{Symbol: "GOOG", Action: core.ActionBuy}, Return: -0.02},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Explanation == "" {
		t.Error("expected explanation")
	}
}

func TestSynthesizer_NoData(t *testing.T) {
	synth := NewSynthesizer(
		&mockSynthLLM{},
		atlasctx.NewInMemoryTrackRecord(),
		nil, // logger
		SynthesizerConfig{},
	)

	ctx := context.Background()
	_, err := synth.Synthesize(ctx, SynthesisRequest{})

	if err == nil {
		t.Error("expected error for no data")
	}
}

func TestSynthesizer_BuildPrompt(t *testing.T) {
	synth := &Synthesizer{}

	req := SynthesisRequest{
		TimeRange: TimeRange{
			Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
		},
		Trades: []backtest.Trade{
			{EntrySignal: core.Signal{Symbol: "AAPL", Action: core.ActionBuy}, Return: 0.05},
		},
		Signals: []core.Signal{
			{Strategy: "MA_Crossover", Action: core.ActionBuy},
		},
	}

	stats := map[string]*atlasctx.StrategyStats{
		"MA_Crossover": {
			Strategy:     "MA_Crossover",
			TotalSignals: 100,
			WinRate:      0.6,
		},
	}

	prompt := synth.buildPrompt(req, stats)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "2024-01-01") {
		t.Error("expected date in prompt")
	}
	if !contains(prompt, "MA_Crossover") {
		t.Error("expected strategy name in prompt")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock LLM for synthesizer tests
type mockSynthLLM struct{}

func (m *mockSynthLLM) Name() string { return "mock" }

func (m *mockSynthLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: `{
			"parameter_suggestions": [],
			"new_rules": [],
			"combination_rules": [],
			"explanation": "Test analysis complete"
		}`,
	}, nil
}
