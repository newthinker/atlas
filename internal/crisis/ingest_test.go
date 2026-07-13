package crisis

// Context Checkpoint: done_criteria → test mapping (ingest)
// functional[0] FRED 三日频序列换算入库(vix 原值/hy_oas/t10y2y ×100 bp), source=fred → TestIngestAllScalesJoinsAndDegrades
// functional[1] SOFR/EFFR 按日 join (SOFR−EFFR)×100 入库 sofr_effr, source=derived → TestIngestAllScalesJoinsAndDegrades
// functional[2] Yahoo ^MOVE→move/JPY=X→usdjpy 日收盘, 日期 o.Time.UTC().Format; IngestNFCI 单独 → TestIngestYahooClose / TestIngestAllScalesJoinsAndDegrades
// functional[3] IngestReport.Counts 按指标统计 → TestIngestAllScalesJoinsAndDegrades
// boundary[0]   SOFR/EFFR 任一腿缺该日则跳过 → TestIngestAllScalesJoinsAndDegrades
// error_handling[0] Yahoo 失败不阻断(收进 YahooErrs); FRED 失败返回 error → TestIngestAllScalesJoinsAndDegrades

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/core"
)

type fakeFRED map[string][]fred.Observation

func (f fakeFRED) FetchSeries(_ context.Context, id, _, _ string) ([]fred.Observation, error) {
	obs, ok := f[id]
	if !ok {
		return nil, fmt.Errorf("no such series %s", id)
	}
	return obs, nil
}

type fakeYahoo struct {
	bars map[string][]core.OHLCV
	err  error
}

func (f fakeYahoo) FetchHistory(symbol string, _, _ time.Time, _ string) ([]core.OHLCV, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bars[symbol], nil
}

// allFredSeries 提供 6 个 FRED 序列的空实现，测试按需覆盖。
func allFredSeries() fakeFRED {
	return fakeFRED{"VIXCLS": nil, "BAMLH0A0HYM2": nil, "T10Y2Y": nil, "NFCI": nil, "SOFR": nil, "EFFR": nil}
}

func TestIngestAllScalesJoinsAndDegrades(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ff := allFredSeries()
	ff["VIXCLS"] = []fred.Observation{{Date: "2026-07-01", Value: 15}}
	ff["BAMLH0A0HYM2"] = []fred.Observation{{Date: "2026-07-01", Value: 2.67}}
	ff["T10Y2Y"] = []fred.Observation{{Date: "2026-07-01", Value: 0.35}}
	ff["NFCI"] = []fred.Observation{{Date: "2026-07-01", Value: -0.52}}
	ff["SOFR"] = []fred.Observation{{Date: "2026-07-01", Value: 4.30}, {Date: "2026-07-02", Value: 4.35}}
	ff["EFFR"] = []fred.Observation{{Date: "2026-07-01", Value: 4.40}} // 07-02 缺腿

	ig := NewIngestor(ff, fakeYahoo{err: fmt.Errorf("yahoo down")}, st)
	rep, err := ig.IngestAll(ctx, "2026-07-01", "2026-07-02")
	require.NoError(t, err) // yahoo 失败不阻断（设计 §2.1 缺数降级）

	oas, err := st.Observation(ctx, IndHYOAS, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, oas)
	assert.InDelta(t, 267.0, oas.Value, 1e-9) // 百分数 ×100 → bp
	assert.Equal(t, "fred", oas.Source)

	t2, err := st.Observation(ctx, IndT10Y2Y, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, t2)
	assert.InDelta(t, 35.0, t2.Value, 1e-9)

	spread, err := st.Observation(ctx, IndSOFREFFR, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, spread)
	assert.InDelta(t, -10.0, spread.Value, 1e-9)
	assert.Equal(t, "derived", spread.Source)
	missing, err := st.Observation(ctx, IndSOFREFFR, "2026-07-02")
	require.NoError(t, err)
	assert.Nil(t, missing) // 缺腿日跳过

	assert.Len(t, rep.YahooErrs, 2) // move 与 usdjpy 都失败
	assert.Equal(t, 1, rep.Counts[IndVIX])

	// NFCI 单独刷新
	n, err := ig.IngestNFCI(ctx, "2026-07-01", "2026-07-02")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestIngestYahooClose(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	bar := time.Date(2026, 7, 10, 14, 30, 0, 0, time.UTC)
	fy := fakeYahoo{bars: map[string][]core.OHLCV{
		"JPY=X": {{Close: 161.7, Time: bar}},
		"^MOVE": {{Close: 69.6, Time: bar}},
	}}
	ig := NewIngestor(allFredSeries(), fy, st)
	rep, err := ig.IngestAll(ctx, "2026-07-01", "2026-07-10")
	require.NoError(t, err)
	assert.Empty(t, rep.YahooErrs)

	jpy, err := st.Observation(ctx, IndUSDJPY, "2026-07-10")
	require.NoError(t, err)
	require.NotNil(t, jpy)
	assert.InDelta(t, 161.7, jpy.Value, 1e-9)
	assert.Equal(t, "yahoo", jpy.Source)

	mv, err := st.Observation(ctx, IndMOVE, "2026-07-10")
	require.NoError(t, err)
	require.NotNil(t, mv)
	assert.InDelta(t, 69.6, mv.Value, 1e-9)
}

// TestIngestAllSurfacesFREDFailure 覆盖 error_handling[0] 第二子句：FRED 抓取
// 失败时 IngestAll 立即返回非 nil error（缺失序列 → fakeFRED.FetchSeries 报错）。
func TestIngestAllSurfacesFREDFailure(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		drop string // 从全序列 map 中删除的 FRED id
	}{
		{"direct series failure", "VIXCLS"}, // fredDirect 首个 → ingestFredSeries 失败
		{"spread leg failure", "SOFR"},      // ingestSpread 的 SOFR 腿失败
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newTestStore(t)
			ff := allFredSeries()
			delete(ff, tt.drop)
			ig := NewIngestor(ff, fakeYahoo{}, st)
			_, err := ig.IngestAll(ctx, "2026-07-01", "2026-07-02")
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.drop)
		})
	}
}
