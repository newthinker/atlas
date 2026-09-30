//go:build integration

package prism

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/tiingo"
)

// prism 降级演练：yahoo 用 fake 强制失败，tiingo 走真实 API；不碰生产配置与数据库。
// token 只从环境变量读，由人类提供（AD-9）。运行：
// ATLAS_TIINGO_TOKEN=... go test -tags integration ./internal/prism/ -run LiveDrill -v
func TestFetchClosesTiingoLiveDrill(t *testing.T) {
	tok := os.Getenv("ATLAS_TIINGO_TOKEN")
	if tok == "" {
		t.Skip("ATLAS_TIINGO_TOKEN 未设置")
	}
	now := time.Now()
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("drill: yahoo down")}}
	closes, deg, err := fetchCloses(us, []PriceHop{{Name: "tiingo", Client: tiingo.New(tok)}}, "NVDA", now.AddDate(0, 0, -30), now)
	require.NoError(t, err)
	require.NotEmpty(t, closes)
	for _, b := range closes {
		assert.Greater(t, b.Close, 0.0, "%s close", b.Time)
	}
	// P17：单跳 tiingo 成功时文案带 yahoo 失败原因，且无前序失败跳。
	assert.Equal(t, "NVDA: yahoo price failed (drill: yahoo down), tiingo fallback ok", deg)
}
