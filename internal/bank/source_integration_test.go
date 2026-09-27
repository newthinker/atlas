//go:build integration

// Context Checkpoint: done_criteria → test mapping
// functional[0]      "600036.SH MissingFields 空、最新期近 270 天、三项非 NaN；ATLAS_AKTOOLS_URL 可覆盖" → TestEMSourceIntegration600036
// boundary[0]        "不带 tag 不参与编译"                                          → 本文件首行 build tag（go list 核对）
// error_handling[0]  "ATLAS_AKTOOLS_URL=http://127.0.0.1:1 时失败而非 skip"          → TestEMSourceIntegration600036 的 require.NoError

package bank

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实调用本地 aktools：守护东方财富字段名（设计 §2 的 live 校验点）。
// 运行：go test -tags integration ./internal/bank/ -run Integration -v
// 地址不可达时必须失败而不是 skip——冒烟测试静默跳过等于没跑。
func TestEMSourceIntegration600036(t *testing.T) {
	base := os.Getenv("ATLAS_AKTOOLS_URL")
	if base == "" {
		base = defaultAktoolsURL
	}
	s, err := NewEMSource(base).Fetch("600036.SH")
	require.NoError(t, err)
	assert.Empty(t, s.MissingFields, "字段名变化 ⇒ 更新 source.go 的 emFieldKeys")
	latest, ind := Analyze(s.Obs)
	assert.WithinDuration(t, time.Now(), latest, 270*24*time.Hour, "最新期应在近 9 个月内")
	for _, k := range Indicators {
		assert.False(t, math.IsNaN(ind[k].Value), "%s 应有值", k.Label())
	}
}
