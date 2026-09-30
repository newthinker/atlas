//go:build integration

package tiingo

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实调用 Tiingo（约 2 次）。token 只从环境变量读，由人类提供（AD-9）。运行：
// ATLAS_TIINGO_TOKEN=... go test -tags integration ./internal/collector/tiingo/ -run Integration -v
// 本包 TestMain 已装零策略 Gate（不缓存、不计配额），New 构造时快照它。
func TestTiingoIntegration(t *testing.T) {
	tok := os.Getenv("ATLAS_TIINGO_TOKEN")
	if tok == "" {
		t.Skip("ATLAS_TIINGO_TOKEN 未设置")
	}
	c := New(tok)
	now := time.Now()
	for _, sym := range []string{"AAPL", "BRK.B"} {
		bars, err := c.FetchHistory(sym, now.AddDate(0, 0, -30), now)
		require.NoError(t, err, sym)
		require.NotEmpty(t, bars, sym)
		for _, b := range bars {
			assert.False(t, math.IsNaN(b.Close) || b.Close <= 0, "%s %s close=%v", sym, b.Time, b.Close)
		}
	}
}
