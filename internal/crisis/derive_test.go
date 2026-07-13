package crisis

// Context Checkpoint: done_criteria → test mapping (derive)
// functional[0] SpreadBp=(sofr−effr)×100 → TestSpreadBp
// functional[1] WowPct ≥6 观测返回 close_t/close_{t-5}−1, ok=true → TestWowPct
// functional[2] MomChange=window[last]−window[last−n]; Percentile 0–1 分位+实际观测数 → TestMomChange / TestPercentile
// boundary[0]   WowPct 不足6观测/基期为0 → ok=false; MomChange 不足 n+1/n≤0 → ok=false; Percentile 空窗 (-1,0) → TestWowPct / TestMomChange / TestPercentile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obsSeq 生成从 2026-01-01 起逐日递增的观测序列（多个测试文件复用）。
func obsSeq(vals ...float64) []Observation {
	out := make([]Observation, len(vals))
	for i, v := range vals {
		out[i] = Observation{Date: addDays("2026-01-01", i), Value: v}
	}
	return out
}

func TestSpreadBp(t *testing.T) {
	assert.InDelta(t, 25.0, SpreadBp(4.55, 4.30), 1e-9)
	assert.InDelta(t, -10.0, SpreadBp(4.30, 4.40), 1e-9)
}

func TestWowPct(t *testing.T) {
	w := obsSeq(100, 101, 102, 103, 104, 98) // t-5 观测 = 100 → -2%
	got, ok := WowPct(w)
	require.True(t, ok)
	assert.InDelta(t, -0.02, got, 1e-9)

	_, ok = WowPct(obsSeq(1, 2, 3, 4, 5)) // 不足 6 观测
	assert.False(t, ok)
	_, ok = WowPct(obsSeq(0, 1, 2, 3, 4, 5)) // 基期为 0
	assert.False(t, ok)
}

func TestMomChange(t *testing.T) {
	w := obsSeq(300, 310, 320, 450)
	got, ok := MomChange(w, 3)
	require.True(t, ok)
	assert.InDelta(t, 150.0, got, 1e-9)

	_, ok = MomChange(w, 4) // 观测数不足 n+1
	assert.False(t, ok)
	_, ok = MomChange(w, 0) // n≤0 守卫
	assert.False(t, ok)
	_, ok = MomChange(w, -1) // n≤0 守卫
	assert.False(t, ok)
}

func TestPercentile(t *testing.T) {
	p, n := Percentile(obsSeq(1, 2, 3, 4), 3.5)
	assert.Equal(t, 4, n)
	assert.InDelta(t, 0.75, p, 1e-9) // 4 个值中 3 个严格小于 3.5

	p, n = Percentile(nil, 1)
	assert.Equal(t, -1.0, p)
	assert.Equal(t, 0, n)
}
