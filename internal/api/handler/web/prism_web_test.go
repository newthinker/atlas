package web

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0]     board 渲染卡片 + 中文标签 + ?group= 筛选     → TestPrismBoardPageRendersCards
//   functional[1]     detail 页含 id="pe-chart" 与 echarts.min.js  → TestPrismDetailPage
//   functional[2]     /static/echarts.min.js → 200 + javascript    → TestStaticEcharts
//   boundary          provider nil→404 / 未知 symbol→404 / NaN→"—" / /static 非 echarts→404
//                       → TestPrismBoardNotEnabled / TestPrismDetailNotEnabled /
//                         TestPrismDetailUnknownSymbol / TestPrismBoardPageRendersCards(NaN) / TestStaticUnknownAsset
//   error_handling[0] Board() 出错 → 500                           → TestPrismBoardStoreError

type fakeBoard struct {
	rows  []prismstore.BoardRow
	err   error           // Board() 错误注入
	known map[string]bool // Series 已知 symbol;nil = 全部已知
}

func (f fakeBoard) Board() ([]prismstore.BoardRow, error) { return f.rows, f.err }
func (f fakeBoard) Series(symbol, from string) (*prismstore.SeriesData, error) {
	if f.known != nil && !f.known[symbol] {
		return nil, prismstore.ErrNotFound
	}
	return &prismstore.SeriesData{Symbol: symbol, Name: "NVIDIA", Source: "engine"}, nil
}

func newTestPrismHandler(t *testing.T, fb fakeBoard) *Handler {
	t.Helper()
	h, err := NewHandlerWithFS(TemplateFS())
	require.NoError(t, err)
	h.SetPrismProvider(fb, 15, 85)
	return h
}

func sampleBoard() fakeBoard {
	return fakeBoard{rows: []prismstore.BoardRow{
		{Instrument: prismstore.Instrument{Symbol: "000300.SH", Name: "沪深300", Group: "A股指数", Source: "lixinger"},
			AsOf: "2026-07-22", PETTM: 12.2, PB: 1.3, Pctl5Y: 10, Pctl10Y: 8},
		{Instrument: prismstore.Instrument{Symbol: "NVDA", Name: "NVIDIA", Group: "美股公司", Source: "engine"},
			AsOf: "2026-07-22", PETTM: 45.5, PB: math.NaN(), Pctl5Y: 90, Pctl10Y: math.NaN()},
	}}
}

func TestPrismBoardPageRendersCards(t *testing.T) {
	h := newTestPrismHandler(t, sampleBoard())
	rec := httptest.NewRecorder()
	h.PrismBoard(rec, httptest.NewRequest(http.MethodGet, "/prism/board", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "沪深300")
	assert.Contains(t, body, "低估") // 000300 pctl_10y=8 < 15
	assert.Contains(t, body, "高估") // NVDA pctl_10y 缺 → 用 pctl_5y=90 > 85
	assert.Contains(t, body, "—", "NaN 指标(NVDA PB)渲染为 —")

	// ?group= 只渲染该组卡片
	rec = httptest.NewRecorder()
	h.PrismBoard(rec, httptest.NewRequest(http.MethodGet, "/prism/board?group=美股公司", nil))
	body = rec.Body.String()
	assert.Contains(t, body, "NVIDIA")
	assert.NotContains(t, body, "沪深300")
}

func TestPrismDetailPage(t *testing.T) {
	h := newTestPrismHandler(t, sampleBoard())
	rec := httptest.NewRecorder()
	h.PrismDetail(rec, httptest.NewRequest(http.MethodGet, "/prism/detail/NVDA", nil), "NVDA")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="pe-chart"`)
	assert.Contains(t, body, "/static/echarts.min.js")
}

func TestStaticEcharts(t *testing.T) {
	h := newTestPrismHandler(t, sampleBoard())
	rec := httptest.NewRecorder()
	h.Static(rec, httptest.NewRequest(http.MethodGet, "/static/echarts.min.js", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "javascript")
}

func TestPrismBoardNotEnabled(t *testing.T) {
	h, err := NewHandlerWithFS(TemplateFS()) // 未 SetPrismProvider
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.PrismBoard(rec, httptest.NewRequest(http.MethodGet, "/prism/board", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPrismDetailNotEnabled(t *testing.T) {
	h, err := NewHandlerWithFS(TemplateFS())
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.PrismDetail(rec, httptest.NewRequest(http.MethodGet, "/prism/detail/NVDA", nil), "NVDA")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPrismDetailUnknownSymbol(t *testing.T) {
	fb := sampleBoard()
	fb.known = map[string]bool{"NVDA": true} // 仅 NVDA 已知
	h := newTestPrismHandler(t, fb)
	rec := httptest.NewRecorder()
	h.PrismDetail(rec, httptest.NewRequest(http.MethodGet, "/prism/detail/NOPE", nil), "NOPE")
	assert.Equal(t, http.StatusNotFound, rec.Code, "未知 symbol → 404")
}

func TestStaticUnknownAsset(t *testing.T) {
	h := newTestPrismHandler(t, sampleBoard())
	rec := httptest.NewRecorder()
	h.Static(rec, httptest.NewRequest(http.MethodGet, "/static/evil.js", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "/static 下非 echarts.min.js → 404")
}

func TestPrismBoardStoreError(t *testing.T) {
	h := newTestPrismHandler(t, fakeBoard{err: errors.New("db down")})
	rec := httptest.NewRecorder()
	h.PrismBoard(rec, httptest.NewRequest(http.MethodGet, "/prism/board", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "Board() 出错 → 500")
}
