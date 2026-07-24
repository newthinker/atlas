package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/api/response"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] board 状态标签(10y 优先缺用 5y)+ NaN→null → TestPrismBoardStatusAndNulls
//   functional[1] series 窗口过滤 + null                       → TestPrismSeriesWindowAndErrors
//   boundary      缺 symbol→400、未知 symbol→404、全 NaN→na    → TestPrismBoardStatusAndNulls / TestPrismSeriesWindowAndErrors

type fakePrismStore struct {
	board   []prismstore.BoardRow
	series  map[string]*prismstore.SeriesData
	err     error
	gotFrom string // 捕获 handler 传入的 from,验证 window→from 映射透传
}

func (f *fakePrismStore) Board() ([]prismstore.BoardRow, error) { return f.board, f.err }
func (f *fakePrismStore) Series(symbol, from string) (*prismstore.SeriesData, error) {
	f.gotFrom = from
	if f.err != nil {
		return nil, f.err
	}
	sd, ok := f.series[symbol]
	if !ok {
		return nil, prismstore.ErrNotFound
	}
	return sd, nil
}

func TestPrismBoardStatusAndNulls(t *testing.T) {
	h := NewPrismHandler(&fakePrismStore{board: []prismstore.BoardRow{
		{Instrument: prismstore.Instrument{Symbol: "000300.SH", Name: "沪深300", Group: "A股指数", Market: "CN_A", Source: "lixinger"},
			AsOf: "2026-07-22", PETTM: 12.2, PB: 1.3, Pctl5Y: 10, Pctl10Y: 8},
		{Instrument: prismstore.Instrument{Symbol: "NVDA", Name: "NVIDIA", Group: "美股公司", Market: "US", Source: "engine"},
			AsOf: "2026-07-22", PETTM: 45.5, PB: math.NaN(), Pctl5Y: 90, Pctl10Y: math.NaN()},
		{Instrument: prismstore.Instrument{Symbol: "NA.X", Name: "无分位", Group: "其它", Market: "US", Source: "engine"},
			AsOf: "2026-07-22", PETTM: 1, PB: 1, Pctl5Y: math.NaN(), Pctl10Y: math.NaN()},
		{Instrument: prismstore.Instrument{Symbol: "MID.X", Name: "区间内", Group: "其它", Market: "US", Source: "engine"},
			AsOf: "2026-07-22", PETTM: 20, PB: 2, Pctl5Y: 50, Pctl10Y: 50},
	}}, 15, 85)

	rec := httptest.NewRecorder()
	h.Board(rec, httptest.NewRequest(http.MethodGet, "/api/prism/board", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp response.SuccessResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data := resp.Data.(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 4)
	assert.Equal(t, 15.0, data["low_pct"])
	assert.Equal(t, 85.0, data["high_pct"])
	i0 := items[0].(map[string]any)
	i1 := items[1].(map[string]any)
	i2 := items[2].(map[string]any)
	i3 := items[3].(map[string]any)
	assert.Equal(t, "low", i0["status"])     // pctl_10y=8 < 15
	assert.Equal(t, "high", i1["status"])    // 10y 缺→用 5y=90 > 85
	assert.Equal(t, "na", i2["status"])      // 5y/10y 全 NaN
	assert.Equal(t, "neutral", i3["status"]) // pctl_10y=50 在 [15,85] 区间内
	assert.Nil(t, i1["pb"], "NaN 必须序列化为 null")
	assert.Nil(t, i1["pctl_10y"], "NaN 必须序列化为 null")
}

func TestPrismStoreErrorReturns500(t *testing.T) {
	// error_handling: store 层错误 → 500 + {error}
	h := NewPrismHandler(&fakePrismStore{err: errors.New("db down")}, 15, 85)

	rec := httptest.NewRecorder()
	h.Board(rec, httptest.NewRequest(http.MethodGet, "/api/prism/board", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = httptest.NewRecorder()
	h.Series(rec, httptest.NewRequest(http.MethodGet, "/api/prism/series?symbol=NVDA", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPrismSeriesWindowAndErrors(t *testing.T) {
	h := NewPrismHandler(&fakePrismStore{series: map[string]*prismstore.SeriesData{
		"NVDA": {Symbol: "NVDA", Name: "NVIDIA", Source: "engine",
			Dates: []string{"2026-07-22"}, PETTM: []float64{45.5},
			Pctl5Y: []float64{90}, Pctl10Y: []float64{math.NaN()}},
	}}, 15, 85)

	rec := httptest.NewRecorder()
	h.Series(rec, httptest.NewRequest(http.MethodGet, "/api/prism/series?symbol=NVDA&window=5y", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.SuccessResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data := resp.Data.(map[string]any)
	assert.Equal(t, "NVDA", data["symbol"])
	p10, ok := data["pctl_10y"].([]any)
	require.True(t, ok)
	require.Len(t, p10, 1)
	assert.Nil(t, p10[0], "series NaN 必须序列化为 null")

	rec = httptest.NewRecorder()
	h.Series(rec, httptest.NewRequest(http.MethodGet, "/api/prism/series", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "缺 symbol → 400")

	rec = httptest.NewRecorder()
	h.Series(rec, httptest.NewRequest(http.MethodGet, "/api/prism/series?symbol=NOPE", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "未知 symbol → 404")
}

// functional[1] window→from 映射:1y/3y/5y 映射为 now 回看对应年数的日期串,
// "max"/空/未知窗口一律映射为 ""(store 层视作全区间)。
func TestWindowFrom(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		window string
		want   string
	}{
		{"1y", now.AddDate(-1, 0, 0).Format("2006-01-02")},
		{"3y", now.AddDate(-3, 0, 0).Format("2006-01-02")},
		{"5y", now.AddDate(-5, 0, 0).Format("2006-01-02")},
		{"max", ""},
		{"", ""},
		{"7y", ""}, // 未知窗口降级为全区间
	}
	for _, tc := range tests {
		t.Run(tc.window, func(t *testing.T) {
			assert.Equal(t, tc.want, windowFrom(tc.window, now))
		})
	}
}

// functional[1] 透传:handler 应把 windowFrom 结果原样传给 store.Series。
func TestSeriesPassesWindowFromToStore(t *testing.T) {
	fs := &fakePrismStore{series: map[string]*prismstore.SeriesData{
		"NVDA": {Symbol: "NVDA", Dates: []string{"2026-07-22"}, PETTM: []float64{45.5},
			Pctl5Y: []float64{90}, Pctl10Y: []float64{math.NaN()}},
	}}
	h := NewPrismHandler(fs, 15, 85)
	rec := httptest.NewRecorder()
	h.Series(rec, httptest.NewRequest(http.MethodGet, "/api/prism/series?symbol=NVDA&window=3y", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, time.Now().AddDate(-3, 0, 0).Format("2006-01-02"), fs.gotFrom,
		"3y 窗口应透传为 now-3y 的 from")
}
