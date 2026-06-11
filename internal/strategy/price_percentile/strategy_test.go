package price_percentile

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0] 历史极低→strong_buy[0.8,0.95]+percentile / 中位无信号 / 极高→strong_sell → TestAnalyze_SignalBands
// functional[1] RequiredData PriceHistory=756 + 六类 AssetTypes                          → TestRequiredData
// functional[2] 信号 Price=当前收盘(非0, W1 教训)                                          → TestAnalyze_SignalBands (Price 断言)
// boundary[0]   <252 根 → (nil,nil)                                                      → TestAnalyze_InsufficientHistory
// boundary[1]   Init 参数 int 与 float64 两形态均解析                                      → TestInit_ParamTypes
// error_handling[0] 阈值乱序 Init 返回 error                                              → TestInit_ThresholdDisorder

func ctxWithCloses(closes []float64) strategy.AnalysisContext {
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	ohlcv := make([]core.OHLCV, len(closes))
	for i, c := range closes {
		ohlcv[i] = core.OHLCV{Symbol: "TEST", Close: c, Time: start.AddDate(0, 0, i)}
	}
	return strategy.AnalysisContext{Symbol: "TEST", OHLCV: ohlcv, Now: time.Now()}
}

func TestAnalyze_SignalBands(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{Params: map[string]any{}}) // 默认参数 25/75/10/90

	// 300 根K线（>252 门槛）。构造当前价为历史极低位 → strong_buy
	closes := make([]float64, 300)
	for i := range closes {
		closes[i] = 100 + float64(i%50)
	}
	closes[299] = 50 // 历史最低
	sigs, err := s.Analyze(ctxWithCloses(closes))
	if err != nil || len(sigs) != 1 || sigs[0].Action != core.ActionStrongBuy {
		t.Fatalf("want strong_buy, got %+v err=%v", sigs, err)
	}
	if sigs[0].Confidence < 0.8 || sigs[0].Confidence > 0.95 {
		t.Errorf("strong zone confidence out of [0.8,0.95]: %v", sigs[0].Confidence)
	}
	if _, ok := sigs[0].Metadata["percentile"]; !ok {
		t.Error("missing percentile metadata")
	}
	// functional[2]: signal Price must be the current close (W1 教训：非 0)
	if sigs[0].Price != 50 {
		t.Errorf("signal Price = %v, want current close 50 (non-zero, W1 lesson)", sigs[0].Price)
	}

	// 中位 → 无信号
	closes[299] = 125
	if sigs, _ := s.Analyze(ctxWithCloses(closes)); len(sigs) != 0 {
		t.Errorf("mid percentile should yield no signal, got %+v", sigs)
	}

	// 历史最高 → strong_sell
	closes[299] = 500
	if sigs, _ := s.Analyze(ctxWithCloses(closes)); len(sigs) != 1 || sigs[0].Action != core.ActionStrongSell {
		t.Errorf("want strong_sell, got %+v", sigs)
	}
}

func TestAnalyze_InsufficientHistory(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	if sigs, err := s.Analyze(ctxWithCloses(make([]float64, 100))); err != nil || len(sigs) != 0 {
		t.Errorf("‹252 bars must yield no signal, got %+v err=%v", sigs, err)
	}
}

func TestRequiredData(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{Params: map[string]any{"lookback_years": 3}})
	rd := s.RequiredData()
	if rd.PriceHistory != 3*252 {
		t.Errorf("PriceHistory = %d, want %d", rd.PriceHistory, 3*252)
	}
	if len(rd.AssetTypes) != 6 { // stock/index/etf/fund/commodity/crypto
		t.Errorf("AssetTypes = %v", rd.AssetTypes)
	}
}

// boundary[1]: viper may decode YAML numbers as int or float64; both must parse.
func TestInit_ParamTypes(t *testing.T) {
	intParams := map[string]any{
		"lookback_years": 3, "low": 25, "high": 75, "extreme_low": 10, "extreme_high": 90,
	}
	floatParams := map[string]any{
		"lookback_years": 3.0, "low": 25.0, "high": 75.0, "extreme_low": 10.0, "extreme_high": 90.0,
	}
	for name, p := range map[string]map[string]any{"int": intParams, "float64": floatParams} {
		s := New()
		if err := s.Init(strategy.Config{Params: p}); err != nil {
			t.Fatalf("%s params: unexpected error %v", name, err)
		}
		if rd := s.RequiredData(); rd.PriceHistory != 3*252 {
			t.Errorf("%s params: PriceHistory = %d, want 756", name, rd.PriceHistory)
		}
	}
}

// error_handling[0]: thresholds out of order must fail Init.
func TestInit_ThresholdDisorder(t *testing.T) {
	cases := []map[string]any{
		{"extreme_low": 30, "low": 25, "high": 75, "extreme_high": 90}, // extreme_low >= low
		{"extreme_low": 10, "low": 80, "high": 75, "extreme_high": 90}, // low >= high
		{"extreme_low": 10, "low": 25, "high": 95, "extreme_high": 90}, // high >= extreme_high
	}
	for i, p := range cases {
		s := New()
		if err := s.Init(strategy.Config{Params: p}); err == nil {
			t.Errorf("case %d: expected error for disordered thresholds %v", i, p)
		}
	}
}
