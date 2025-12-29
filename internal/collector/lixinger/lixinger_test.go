package lixinger

import (
	"testing"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

func TestLixinger_ImplementsFundamentalCollector(t *testing.T) {
	var _ collector.FundamentalCollector = (*Lixinger)(nil)
}

func TestLixinger_Name(t *testing.T) {
	l := New("test-key")
	if l.Name() != "lixinger" {
		t.Errorf("expected 'lixinger', got %s", l.Name())
	}
}

func TestLixinger_SupportedMarkets(t *testing.T) {
	l := New("test-key")
	markets := l.SupportedMarkets()
	if len(markets) != 1 || markets[0] != core.MarketCNA {
		t.Errorf("expected [CN_A], got %v", markets)
	}
}

func TestLixinger_Init_RequiresAPIKey(t *testing.T) {
	l := &Lixinger{}
	err := l.Init(collector.Config{})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestLixinger_Init_WithAPIKey(t *testing.T) {
	l := &Lixinger{}
	err := l.Init(collector.Config{APIKey: "test-key"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
