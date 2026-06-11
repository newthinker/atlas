package valuation

import (
	"errors"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping
// functional[1] "EPS 上升+价格恒定 → 当前 PE 分位 < 50"        → TestReconstructPEPercentile_StepAlignment
// functional[2] "EPS 跳升期重建 PE 分位与价格分位差 ≥ 1"        → TestReconstructPEPercentile_NotEqualToPricePercentile
// boundary[0]   "有效点 < 8 → ErrInsufficientEPS；中间负季度剔除后 ≥8 仍计算" → TestReconstructPEPercentile_Errors
// boundary[1]   "剔除后 PE 序列为空 → ErrInsufficientEPS"        → TestReconstructPEPercentile_EmptyAfterDrop
// error[0]      "当前 EPS(TTM) ≤ 0 → ErrNonPositiveEPS"          → TestReconstructPEPercentile_Errors

func bars(start time.Time, closes ...float64) []core.OHLCV {
	out := make([]core.OHLCV, len(closes))
	for i, c := range closes {
		out[i] = core.OHLCV{Close: c, Time: start.AddDate(0, 0, i)}
	}
	return out
}

func quarterlyEPS(start time.Time, eps ...float64) []core.EPSPoint {
	out := make([]core.EPSPoint, len(eps))
	for i, e := range eps {
		out[i] = core.EPSPoint{Date: start.AddDate(0, 3*i, 0), EPS: e}
	}
	return out
}

func repeat(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func linear(from, to float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		if n == 1 {
			out[i] = from
			continue
		}
		out[i] = from + (to-from)*float64(i)/float64(n-1)
	}
	return out
}

func closesOf(b []core.OHLCV) []float64 {
	out := make([]float64, len(b))
	for i, bar := range b {
		out[i] = bar.Close
	}
	return out
}

func TestReconstructPEPercentile_StepAlignment(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 8 个季度 EPS 全为正（满足门槛），价格恒定 100：
	// EPS 上升 → PE 下降 → 当前 PE 处于序列低位
	eps := quarterlyEPS(start, 4, 4, 4, 4, 5, 5, 5, 5)
	closes := bars(start.AddDate(0, 0, 1), repeat(100, 700)...) // ~23 个月日线
	got, err := ReconstructPEPercentile(closes, eps)
	if err != nil {
		t.Fatalf("ReconstructPEPercentile: %v", err)
	}
	if got > 50 {
		t.Errorf("rising EPS with flat price should put current PE in lower half, got %v", got)
	}
}

func TestReconstructPEPercentile_NotEqualToPricePercentile(t *testing.T) {
	// 回归用例：EPS 变动期，重建 PE 分位 ≠ 价格分位（设计 §6）
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	eps := quarterlyEPS(start, 2, 2, 2, 2, 8, 8, 8, 8) // EPS 跳升
	closes := bars(start.AddDate(0, 0, 1), linear(100, 200, 700)...)
	pePct, err := ReconstructPEPercentile(closes, eps)
	if err != nil {
		t.Fatal(err)
	}
	pricePct := PercentileRank(closesOf(closes), closes[len(closes)-1].Close)
	if diff := pePct - pricePct; diff > -1 && diff < 1 {
		t.Errorf("PE percentile (%v) should differ from price percentile (%v)", pePct, pricePct)
	}
}

func TestReconstructPEPercentile_Errors(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 有效季度点不足（<8）→ ErrInsufficientEPS（数据缺失 → 调用方可兜底）
	if _, err := ReconstructPEPercentile(bars(start, 100, 100), quarterlyEPS(start, 4, 4, 4)); !errors.Is(err, ErrInsufficientEPS) {
		t.Errorf("want ErrInsufficientEPS, got %v", err)
	}
	// 当前 EPS ≤ 0（真实亏损）→ ErrNonPositiveEPS（调用方直接跳过，不兜底）
	eps := quarterlyEPS(start, 4, 4, 4, 4, 4, 4, 4, 4, -1)
	if _, err := ReconstructPEPercentile(bars(start.AddDate(2, 0, 0), 100, 100), eps); !errors.Is(err, ErrNonPositiveEPS) {
		t.Errorf("want ErrNonPositiveEPS, got %v", err)
	}
	// 亏损季度剔除：中间一个负 EPS 季度的交易日不进 PE 序列，剩余 ≥8 个有效点仍可计算
	eps2 := quarterlyEPS(start, 4, 4, -2, 4, 4, 4, 4, 4, 4)
	if _, err := ReconstructPEPercentile(bars(start.AddDate(0, 1, 0), repeat(100, 800)...), eps2); err != nil {
		t.Errorf("one loss quarter among 8 valid should still compute, got %v", err)
	}
}

func TestReconstructPEPercentile_EmptyAfterDrop(t *testing.T) {
	// load-bearing：8 个有效正 EPS 点满足门槛，但所有 close 都早于首个 EPS 点
	// → 无 bar 可对齐 → PE 序列为空 → 必须 ErrInsufficientEPS，绝不 -1+nil 冒充成功
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	eps := quarterlyEPS(start, 4, 4, 4, 4, 4, 4, 4, 4)
	closes := bars(start.AddDate(-1, 0, 0), 100, 100, 100) // 全部早于 2024-01-01
	got, err := ReconstructPEPercentile(closes, eps)
	if !errors.Is(err, ErrInsufficientEPS) {
		t.Errorf("empty PE series must return ErrInsufficientEPS, got (%v, %v)", got, err)
	}
}
