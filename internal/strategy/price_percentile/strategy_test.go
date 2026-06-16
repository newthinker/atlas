package price_percentile

import (
	"strings"
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

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0] "Init(percentile_step:3 int) → 信号 Metadata[percentile_step]==3.0" → TestInit_PercentileStepParam
// boundary[0]   "未配置 percentile_step → 信号 Metadata 不含该键"                    → TestAnalyze_NoStepParam_NoStepMetadata
// boundary[1]   "percentile_step ≤ 0 → 视为未配置,不写元数据"                        → TestInit_PercentileStepNonPositive

// analyzeWithExtremeLowPercentile drives one extreme-low (strong_buy) signal,
// reusing the SignalBands construction (300 bars, last close at historical low).
func analyzeWithExtremeLowPercentile(t *testing.T, s *Strategy) []core.Signal {
	t.Helper()
	closes := make([]float64, 300)
	for i := range closes {
		closes[i] = 100 + float64(i%50)
	}
	closes[299] = 50 // historical low → extreme-low percentile → strong_buy
	sigs, err := s.Analyze(ctxWithCloses(closes))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	return sigs
}

// functional[0]: int-form percentile_step param propagates to signal metadata.
// (float64 form is covered by the existing numParam dual-form tests.)
func TestInit_PercentileStepParam(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"percentile_step": 3}}); err != nil {
		t.Fatal(err)
	}
	sigs := analyzeWithExtremeLowPercentile(t, s)
	if len(sigs) == 0 {
		t.Fatal("expected a signal")
	}
	if sigs[0].Metadata["percentile_step"] != 3.0 {
		t.Errorf("metadata percentile_step = %v, want 3.0", sigs[0].Metadata["percentile_step"])
	}
}

// boundary[0]: guard test — absent when not configured (router falls back to global).
func TestAnalyze_NoStepParam_NoStepMetadata(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	sigs := analyzeWithExtremeLowPercentile(t, s)
	if len(sigs) == 0 {
		t.Fatal("expected a signal")
	}
	if _, ok := sigs[0].Metadata["percentile_step"]; ok {
		t.Error("percentile_step must be absent when not configured (router falls back to global)")
	}
}

// boundary[1]: percentile_step ≤ 0 is treated as unconfigured (no metadata written).
func TestInit_PercentileStepNonPositive(t *testing.T) {
	for _, v := range []any{0, -1, 0.0, -2.5} {
		s := New()
		if err := s.Init(strategy.Config{Params: map[string]any{"percentile_step": v}}); err != nil {
			t.Fatalf("step=%v: Init error %v", v, err)
		}
		sigs := analyzeWithExtremeLowPercentile(t, s)
		if len(sigs) == 0 {
			t.Fatalf("step=%v: expected a signal", v)
		}
		if _, ok := sigs[0].Metadata["percentile_step"]; ok {
			t.Errorf("step=%v: percentile_step ≤ 0 must not write metadata", v)
		}
	}
}

// Context Checkpoint: done_criteria → test mapping (Task 2 — since inception)
// functional[0] "lookback_years=0 → RequiredData().PriceHistory==SinceInceptionBars"          → TestRequiredData_SinceInception
// functional[1] "Init with lookback_years=0 does not error"                                   → TestInit_AcceptsZeroLookback
// error_handling[0] "Init with lookback_years=-1 returns error"                               → TestInit_RejectsNegativeLookback
// functional[2] "lookback_years=0 + signal → Reason contains 'full history'"                  → TestAnalyze_FullHistoryReasonText
// boundary[0]   "lookback_years=0 but ctx.OHLCV < 252 bars → nil (minSampleBars still guards)" → TestAnalyze_InceptionRespectsMinSample

// TestRequiredData_SinceInception verifies lookback_years==0 returns SinceInceptionBars.
func TestRequiredData_SinceInception(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init(lookback_years=0) unexpected error: %v", err)
	}
	rd := s.RequiredData()
	if rd.PriceHistory != strategy.SinceInceptionBars {
		t.Errorf("PriceHistory = %d, want SinceInceptionBars (%d)", rd.PriceHistory, strategy.SinceInceptionBars)
	}
}

// TestInit_AcceptsZeroLookback verifies Init does not reject lookback_years==0.
func TestInit_AcceptsZeroLookback(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Errorf("Init(lookback_years=0) should not error, got: %v", err)
	}
}

// TestInit_RejectsNegativeLookback verifies Init rejects lookback_years < 0.
func TestInit_RejectsNegativeLookback(t *testing.T) {
	for _, neg := range []any{-1, -1.0, -5} {
		s := New()
		if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": neg}}); err == nil {
			t.Errorf("Init(lookback_years=%v) should return error", neg)
		}
	}
}

// TestAnalyze_FullHistoryReasonText verifies Reason text contains "full history"
// when lookback_years==0 and a signal is generated.
func TestAnalyze_FullHistoryReasonText(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// 300 bars with extreme-low current close → strong_buy signal
	closes := make([]float64, 300)
	for i := range closes {
		closes[i] = 100 + float64(i%50)
	}
	closes[299] = 50 // historical low → extreme-low percentile
	sigs, err := s.Analyze(ctxWithCloses(closes))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(sigs) == 0 {
		t.Fatal("expected at least one signal")
	}
	reason := sigs[0].Reason
	if !strings.Contains(reason, "full history") {
		t.Errorf("Reason = %q, want it to contain 'full history'", reason)
	}
	// metadata lookback_years should be 0 (inception marker preserved for downstream)
	if v, ok := sigs[0].Metadata["lookback_years"]; !ok || v != 0 {
		t.Errorf("metadata lookback_years = %v, want 0", v)
	}
}

// TestAnalyze_InceptionRespectsMinSample (B3 강화):
// lookback_years==0 but fewer than 252 bars → no signal (minSampleBars guard remains active).
func TestAnalyze_InceptionRespectsMinSample(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// 100 bars — below minSampleBars=252; all prices at extreme low so the classify
	// logic would signal if it were reached. We assert it is NOT reached.
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 1.0 // all same → current is at 0th percentile
	}
	closes[99] = 0.5 // extreme low
	sigs, err := s.Analyze(ctxWithCloses(closes))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("inception with <252 bars must return nil (no signal), got %+v", sigs)
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
