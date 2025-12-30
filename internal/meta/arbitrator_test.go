// internal/meta/arbitrator_test.go
package meta

import (
	"context"
	"testing"
	"time"

	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

func TestArbitrator_AllSignalsAgree(t *testing.T) {
	arb := NewArbitrator(
		&mockLLMProvider{},
		&mockMarketCtx{},
		atlasctx.NewInMemoryTrackRecord(),
		atlasctx.NewStaticNewsProvider(nil),
		nil, // logger
		ArbitratorConfig{},
	)

	signals := []core.Signal{
		{Strategy: "MA_Crossover", Action: core.ActionBuy, Confidence: 0.8},
		{Strategy: "RSI", Action: core.ActionBuy, Confidence: 0.7},
	}

	ctx := context.Background()
	result, err := arb.Arbitrate(ctx, ArbitrationRequest{
		Symbol:             "AAPL",
		Market:             core.MarketUS,
		ConflictingSignals: signals,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != core.ActionBuy {
		t.Errorf("expected BUY, got %s", result.Decision)
	}
	if result.Confidence != 0.75 {
		t.Errorf("expected confidence 0.75, got %f", result.Confidence)
	}
}

func TestArbitrator_NoSignals(t *testing.T) {
	arb := NewArbitrator(
		&mockLLMProvider{},
		&mockMarketCtx{},
		atlasctx.NewInMemoryTrackRecord(),
		atlasctx.NewStaticNewsProvider(nil),
		nil, // logger
		ArbitratorConfig{},
	)

	ctx := context.Background()
	_, err := arb.Arbitrate(ctx, ArbitrationRequest{
		Symbol:             "AAPL",
		Market:             core.MarketUS,
		ConflictingSignals: []core.Signal{},
	})

	if err == nil {
		t.Error("expected error for no signals")
	}
}

func TestAllSignalsAgree(t *testing.T) {
	tests := []struct {
		name    string
		signals []core.Signal
		want    bool
	}{
		{
			name:    "empty",
			signals: []core.Signal{},
			want:    true,
		},
		{
			name:    "single",
			signals: []core.Signal{{Action: core.ActionBuy}},
			want:    true,
		},
		{
			name: "all buy",
			signals: []core.Signal{
				{Action: core.ActionBuy},
				{Action: core.ActionBuy},
			},
			want: true,
		},
		{
			name: "mixed",
			signals: []core.Signal{
				{Action: core.ActionBuy},
				{Action: core.ActionSell},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allSignalsAgree(tt.signals)
			if got != tt.want {
				t.Errorf("allSignalsAgree() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAvgConfidence(t *testing.T) {
	signals := []core.Signal{
		{Confidence: 0.8},
		{Confidence: 0.6},
	}

	avg := avgConfidence(signals)
	if avg != 0.7 {
		t.Errorf("expected 0.7, got %f", avg)
	}
}

func TestParseTextResponse(t *testing.T) {
	arb := &Arbitrator{}

	tests := []struct {
		text string
		want core.Action
	}{
		{"I recommend to BUY", core.ActionBuy},
		{"You should SELL", core.ActionSell},
		{"Mixed signals, HOLD", core.ActionHold},
		{"unclear response", core.ActionHold},
	}

	signals := []core.Signal{{Strategy: "test"}}
	for _, tt := range tests {
		result, _ := arb.parseTextResponse(tt.text, signals)
		if result.Decision != tt.want {
			t.Errorf("parseTextResponse(%q) = %s, want %s", tt.text, result.Decision, tt.want)
		}
	}
}

// Mock implementations for testing

type mockLLMProvider struct{}

func (m *mockLLMProvider) Name() string { return "mock" }

func (m *mockLLMProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: `{"decision": "BUY", "confidence": 0.7, "reasoning": "test", "weighted_from": ["test"]}`,
	}, nil
}

type mockMarketCtx struct{}

func (m *mockMarketCtx) GetContext(ctx context.Context, market core.Market) (*atlasctx.MarketContext, error) {
	return &atlasctx.MarketContext{
		Market:     market,
		Regime:     atlasctx.RegimeBull,
		Volatility: 0.15,
		UpdatedAt:  time.Now(),
	}, nil
}
