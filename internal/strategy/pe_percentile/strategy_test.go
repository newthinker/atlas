package pe_percentile

import (
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0] "分档 5→strong_buy/15→buy/50→无/85→sell/95→strong_sell" → TestAnalyze_PEBands
// functional[1] "Source method:fallback_reason 解析进 Metadata"          → TestAnalyze_MethodMetadata
// functional[2] "RequiredData Fundamentals=true/AssetTypes=[stock,index]/PriceHistory=lookback*252" → TestRequiredData_AssetTypes
// boundary[0]   "Fundamental nil 或 PEPercentile<0 → (nil,nil)"          → TestAnalyze_Unavailable
//
// TASK-004 done_criteria → test mapping (percentile_step):
// functional[0] "Init(percentile_step:3 int) → Metadata[percentile_step]==3.0" → TestInit_PercentileStepParam
// boundary[0]   "未配置 → Metadata 不含 percentile_step 键"                    → TestAnalyze_NoStepParam_NoStepMetadata
// boundary[1]   "percentile_step ≤ 0 → 视为未配置，不写元数据"                  → TestInit_PercentileStepNonPositive_NoMetadata
//
// Task 3 since-inception done_criteria → test mapping:
// functional[0] "lookback_years=0 → RequiredData().PriceHistory==SinceInceptionBars" → TestRequiredData_SinceInception
// functional[1] "Init(lookback_years:0) 不报错"                                       → TestInit_AcceptsZeroLookback
// boundary[0]   "Init(lookback_years:-1) 报错"                                        → TestInit_RejectsNegativeLookback
// functional[2] "lookback_years=0 + Fundamental 有效 → Reason 含 'full history'"      → TestAnalyze_FullHistoryReasonText
// boundary[1]   "lookback_years=0 + Fundamental nil → 无信号"                         → TestAnalyze_InceptionNoFundamentalNoSignal

func peCtx(pePct float64, source string) strategy.AnalysisContext {
	return strategy.AnalysisContext{
		Symbol: "TEST", Now: time.Now(),
		Fundamental: &core.Fundamental{Symbol: "TEST", PEPercentile: pePct, Source: source},
	}
}

func TestAnalyze_PEBands(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{}) // 默认 20/80/10/90

	cases := []struct {
		pct  float64
		want core.Action // "" = 无信号
	}{
		{5, core.ActionStrongBuy}, {15, core.ActionBuy},
		{50, ""}, {85, core.ActionSell}, {95, core.ActionStrongSell},
	}
	for _, c := range cases {
		sigs, err := s.Analyze(peCtx(c.pct, "lixinger_cvpos"))
		if err != nil {
			t.Fatal(err)
		}
		if c.want == "" && len(sigs) != 0 {
			t.Errorf("pct=%v: want no signal, got %+v", c.pct, sigs)
		}
		if c.want != "" && (len(sigs) != 1 || sigs[0].Action != c.want) {
			t.Errorf("pct=%v: want %s, got %+v", c.pct, c.want, sigs)
		}
	}
}

func TestAnalyze_MethodMetadata(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	sigs, _ := s.Analyze(peCtx(5, "lixinger_cvpos:yahoo_eps_insufficient"))
	if len(sigs) != 1 {
		t.Fatal("expected one signal")
	}
	if sigs[0].Metadata["method"] != "lixinger_cvpos" ||
		sigs[0].Metadata["fallback_reason"] != "yahoo_eps_insufficient" {
		t.Errorf("metadata = %+v", sigs[0].Metadata)
	}
	// 无冒号时不设 fallback_reason
	sigs2, _ := s.Analyze(peCtx(5, "lixinger_cvpos"))
	if len(sigs2) != 1 {
		t.Fatal("expected one signal")
	}
	if sigs2[0].Metadata["method"] != "lixinger_cvpos" {
		t.Errorf("method = %v", sigs2[0].Metadata["method"])
	}
	if _, ok := sigs2[0].Metadata["fallback_reason"]; ok {
		t.Errorf("fallback_reason should be absent for source without colon, got %+v", sigs2[0].Metadata)
	}
}

func TestAnalyze_Unavailable(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	// Fundamental 缺失 / PEPercentile 负值 → 无信号
	if sigs, _ := s.Analyze(strategy.AnalysisContext{Symbol: "T"}); len(sigs) != 0 {
		t.Errorf("nil fundamental should yield no signal")
	}
	if sigs, _ := s.Analyze(peCtx(-1, "")); len(sigs) != 0 {
		t.Errorf("negative percentile should yield no signal")
	}
}

func TestRequiredData_AssetTypes(t *testing.T) {
	s := New()
	rd := s.RequiredData()
	if !rd.Fundamentals {
		t.Errorf("Fundamentals = %v, want true", rd.Fundamentals)
	}
	if len(rd.AssetTypes) != 2 || rd.AssetTypes[0] != core.AssetStock || rd.AssetTypes[1] != core.AssetIndex {
		t.Errorf("AssetTypes = %v, want [stock index]", rd.AssetTypes)
	}
	if rd.PriceHistory != 5*252 { // 默认 lookback 5
		t.Errorf("PriceHistory = %d, want %d", rd.PriceHistory, 5*252)
	}
}

func TestInit_InvalidThresholds(t *testing.T) {
	s := New()
	// extreme_low < low < high < extreme_high 不成立 → error
	err := s.Init(strategy.Config{Params: map[string]any{"low": 90, "high": 20}})
	if err == nil {
		t.Errorf("expected error for low >= high")
	}
}

// TestInit_PercentileStepParam: functional[0] — int 形态参数被 numParam 双形态
// helper 读取，>0 时写入信号 Metadata（float64）。float64 形态由既有 numParam
// 用例覆盖。peCtx(5,...) 触发一条 strong_buy 信号。
func TestInit_PercentileStepParam(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"percentile_step": 3}}); err != nil {
		t.Fatal(err)
	}
	sigs, _ := s.Analyze(peCtx(5, "lixinger_cvpos"))
	if len(sigs) == 0 {
		t.Fatal("expected a signal")
	}
	if sigs[0].Metadata["percentile_step"] != 3.0 {
		t.Errorf("metadata percentile_step = %v, want 3.0", sigs[0].Metadata["percentile_step"])
	}
}

// TestAnalyze_NoStepParam_NoStepMetadata: boundary[0] — 未配置时 Metadata 不含
// percentile_step 键（router 回退全局默认）。RED 阶段守卫测试，本就 PASS。
func TestAnalyze_NoStepParam_NoStepMetadata(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	sigs, _ := s.Analyze(peCtx(5, "lixinger_cvpos"))
	if len(sigs) == 0 {
		t.Fatal("expected a signal")
	}
	if _, ok := sigs[0].Metadata["percentile_step"]; ok {
		t.Error("percentile_step must be absent when not configured (router falls back to global)")
	}
}

// TestInit_PercentileStepNonPositive_NoMetadata: boundary[1] — ≤0 视为未配置，
// 不写元数据（int 0 / 负 / float64 0 三态）。
func TestInit_PercentileStepNonPositive_NoMetadata(t *testing.T) {
	for _, step := range []any{0, -1, 0.0} {
		s := New()
		if err := s.Init(strategy.Config{Params: map[string]any{"percentile_step": step}}); err != nil {
			t.Fatalf("step=%v: %v", step, err)
		}
		sigs, _ := s.Analyze(peCtx(5, "lixinger_cvpos"))
		if len(sigs) == 0 {
			t.Fatalf("step=%v: expected a signal", step)
		}
		if _, ok := sigs[0].Metadata["percentile_step"]; ok {
			t.Errorf("step=%v: percentile_step <= 0 must be treated as unconfigured", step)
		}
	}
}

// Task 3: since-inception tests

// TestRequiredData_SinceInception: functional[0] — lookback_years=0 时
// RequiredData().PriceHistory 应等于 strategy.SinceInceptionBars。
func TestRequiredData_SinceInception(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init with lookback_years=0 failed: %v", err)
	}
	rd := s.RequiredData()
	if rd.PriceHistory != strategy.SinceInceptionBars {
		t.Errorf("PriceHistory = %d, want SinceInceptionBars (%d)", rd.PriceHistory, strategy.SinceInceptionBars)
	}
}

// TestInit_AcceptsZeroLookback: functional[1] — lookback_years=0 合法，Init 不报错。
func TestInit_AcceptsZeroLookback(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Errorf("Init(lookback_years=0) should not error, got: %v", err)
	}
}

// TestInit_RejectsNegativeLookback: boundary[0] — lookback_years=-1 非法，Init 报错。
func TestInit_RejectsNegativeLookback(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": -1}}); err == nil {
		t.Error("Init(lookback_years=-1) should return an error")
	}
}

// TestAnalyze_FullHistoryReasonText: functional[2] — lookback_years=0 + Fundamental
// 有效时，Signal.Reason 应含 "full history"，且 metadata lookback_years==0。
func TestAnalyze_FullHistoryReasonText(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	sigs, err := s.Analyze(peCtx(5, "lixinger_cvpos"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	if !strings.Contains(sigs[0].Reason, "full history") {
		t.Errorf("Reason = %q, want it to contain 'full history'", sigs[0].Reason)
	}
	if sigs[0].Metadata["lookback_years"] != 0 {
		t.Errorf("metadata lookback_years = %v, want 0", sigs[0].Metadata["lookback_years"])
	}
}

// TestAnalyze_InceptionNoFundamentalNoSignal: boundary[1] — lookback_years=0 +
// Fundamental nil（数据不足/未算出分位）→ 无信号。
// pe_percentile 的 Analyze 顶部已有 Fundamental nil 守卫，本测试确认 inception
// 模式下该守卫仍然生效（与 price_percentile minSampleBars 门槛不同，pe_percentile
// 不直接检查 OHLCV 长度，依赖 Fundamental 是否可用来判断数据充足性）。
func TestAnalyze_InceptionNoFundamentalNoSignal(t *testing.T) {
	s := New()
	if err := s.Init(strategy.Config{Params: map[string]any{"lookback_years": 0}}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	ctx := strategy.AnalysisContext{Symbol: "TEST", Now: time.Now(), Fundamental: nil}
	sigs, err := s.Analyze(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 0 {
		t.Errorf("expected no signal when Fundamental is nil, got %+v", sigs)
	}
}
