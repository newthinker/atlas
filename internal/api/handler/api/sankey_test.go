package api

// Context Checkpoint: done_criteria → test mapping
//   functional[0] /api/prism/sankey 200 + Periods/Matrix/Granularity/Template；
//                 **AD-22 Graph.Notes 透传**（对偶：正常数据下为空）           → TestSankeyResponseShape, TestSankeyNotesPassThrough
//                 **AD-26 Analysis.Conflicts 透传**（断具体值，非 != nil）      → TestSankeyConflictsPassThrough
//   functional[1] granularity/from/to/lang 四个查询参数透传到 Service.Analyze  → TestSankeyQueryParamsReachService
//   functional[2] 无模板 → 404 且响应体含 no sankey template                   → TestSankeyErrorBranches
//   functional[3] /api/prism/fundamental 200 含 Periods/Metrics/Dates/Closes；
//                 无数据 → 404                                                 → TestFundamentalResponse
//   functional[4] AD-14 Inf 防护：含 Inf 的 Analysis 仍返回 200 而非 500        → TestSankeyInfNeverBreaksJSON
//   boundary      缺 symbol → 400；granularity 非法 → 400；三种 404 文案互不混淆 → TestSankeyErrorBranches

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

type fakeSankeyService struct {
	analysis    *sankey.Analysis
	fundamental *sankey.FundamentalView
	err         error
	fundErr     error

	// 捕获 handler 传入的参数，验证查询串确实透传下去了。
	gotSymbol, gotFrom, gotTo, gotGranularity, gotLang string
	gotFundSymbol                                      string
}

func (f *fakeSankeyService) Analyze(symbol, from, to, granularity, lang string) (*sankey.Analysis, error) {
	f.gotSymbol, f.gotFrom, f.gotTo, f.gotGranularity, f.gotLang = symbol, from, to, granularity, lang
	if f.err != nil {
		return nil, f.err
	}
	return f.analysis, nil
}

func (f *fakeSankeyService) Fundamental(symbol string) (*sankey.FundamentalView, error) {
	f.gotFundSymbol = symbol
	if f.fundErr != nil {
		return nil, f.fundErr
	}
	return f.fundamental, nil
}

// analysisFixture 造一份结构完整的 Analysis：一期、含分部与主干节点、矩阵一行。
func analysisFixture() *sankey.Analysis {
	a := &sankey.Analysis{
		Symbol: "MSFT", Granularity: "fy", Truncated: false,
		Periods: []sankey.PeriodView{{
			Period: "FY2026",
			Graph: sankey.Graph{
				Nodes: []sankey.Node{{Name: "云业务", Value: 40e9}, {Name: "收入", Value: 65.585e9}},
				Links: []sankey.Link{{Source: "云业务", Target: "收入", Value: 40e9}},
			},
			Metrics: sankey.PeriodMetrics{
				Period: "FY2026", PeriodEnd: "2026-06-30",
				Segments: map[string]float64{"cloud": 40e9},
				Revenue:  65.585e9, GrossProfit: 45e9, OperatingIncome: 30e9, NetIncome: 24.7e9,
				GrossMargin: 0.686, OpMargin: 0.457, NetMargin: 0.377,
			},
		}},
		Matrix: []sankey.MatrixRow{{
			Key: "revenue", Label: "收入", Values: []float64{65.585e9}, Change: 0.12, ChangeKind: "yoy",
		}},
	}
	a.Template.Version = 1
	a.Template.Lang = map[string]string{"cloud": "云业务"}
	return a
}

func doGet(t *testing.T, h *SankeyHandler, target string, fn func(http.ResponseWriter, *http.Request)) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(http.MethodGet, target, nil))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"响应必须是合法 JSON，实得: %s", rec.Body.String())
	return rec.Code, body
}

// data 取 response.JSON 的 {"data": ...} 包裹层（AD-8：前端必须用 resp.data）。
func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	d, ok := body["data"].(map[string]any)
	require.True(t, ok, "响应应有 data 包裹层，实得 %v", body)
	return d
}

func TestSankeyResponseShape(t *testing.T) {
	svc := &fakeSankeyService{analysis: analysisFixture()}
	h := NewSankeyHandler(svc)

	code, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
	require.Equal(t, http.StatusOK, code)
	data := dataOf(t, body)

	assert.Equal(t, "MSFT", data["symbol"])
	assert.Equal(t, "fy", data["granularity"])
	assert.Equal(t, false, data["truncated"])

	periods, ok := data["periods"].([]any)
	require.True(t, ok, "data 应含 periods，实得 %v", data)
	require.Len(t, periods, 1)
	p := periods[0].(map[string]any)
	assert.Equal(t, "FY2026", p["period"])

	graph, ok := p["graph"].(map[string]any)
	require.True(t, ok, "每期应含 graph")
	assert.Len(t, graph["nodes"], 2)
	assert.Len(t, graph["links"], 1)

	metrics, ok := p["metrics"].(map[string]any)
	require.True(t, ok, "每期应含 metrics")
	assert.Equal(t, 65.585e9, metrics["revenue"])
	assert.Equal(t, map[string]any{"cloud": 40e9}, metrics["segments"])

	matrix, ok := data["matrix"].([]any)
	require.True(t, ok)
	require.Len(t, matrix, 1)
	row := matrix[0].(map[string]any)
	assert.Equal(t, "revenue", row["key"])
	assert.Equal(t, "收入", row["label"])
	assert.Equal(t, "yoy", row["change_kind"])

	tmpl, ok := data["template"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), tmpl["version"])
	assert.Equal(t, map[string]any{"cloud": "云业务"}, tmpl["lang"])
}

// AD-22：透传链 TASK-007 产生 → TASK-009 装配 → **本任务 API** → TASK-012 渲染 footnote。
// 本环断掉则负残差退回「图上凭空少一块且无人知情」。
func TestSankeyNotesPassThrough(t *testing.T) {
	t.Run("negative residual reaches the response", func(t *testing.T) {
		a := analysisFixture()
		// tax_other 为负时 BuildSankey 记的那类 footnote。
		a.Periods[0].Graph.Notes = []string{"营业利润→税项及其他 为负（-3000000000），按 0 宽绘制，不计入守恒"}
		h := NewSankeyHandler(&fakeSankeyService{analysis: a})

		code, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
		require.Equal(t, http.StatusOK, code)

		graph := dataOf(t, body)["periods"].([]any)[0].(map[string]any)["graph"].(map[string]any)
		notes, ok := graph["notes"].([]any)
		require.True(t, ok, "graph 必须携带 notes，实得 %v", graph)
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0].(string), "税项及其他", "footnote 必须点名为负的节点")
	})

	// 对偶：正常数据下不得凭空造出 notes。
	t.Run("normal data carries no notes", func(t *testing.T) {
		h := NewSankeyHandler(&fakeSankeyService{analysis: analysisFixture()})
		code, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
		require.Equal(t, http.StatusOK, code)

		graph := dataOf(t, body)["periods"].([]any)[0].(map[string]any)["graph"].(map[string]any)
		assert.Empty(t, graph["notes"], "没有负流时不该有 footnote")
	})
}

// AD-26：Conflicts 是「本该有但被拒绝的东西」的唯一数据源，断链则前端无法给降级提示。
func TestSankeyConflictsPassThrough(t *testing.T) {
	a := analysisFixture()
	a.Conflicts = []sankey.PeriodConflict{{
		Period: "FY2025",
		Detail: "收到 5 个季度（应为 4），标签冲突使该财年无法聚合，已跳过",
	}}
	h := NewSankeyHandler(&fakeSankeyService{analysis: a})

	code, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
	require.Equal(t, http.StatusOK, code)

	conflicts, ok := dataOf(t, body)["conflicts"].([]any)
	require.True(t, ok, "data 必须携带 conflicts，实得 %v", dataOf(t, body))
	require.Len(t, conflicts, 1)
	c := conflicts[0].(map[string]any)
	assert.Equal(t, "FY2025", c["period"])
	assert.Contains(t, c["detail"], "收到 5 个季度", "被拒的理由必须原样到达前端")

	t.Run("normal data carries no conflicts", func(t *testing.T) {
		h := NewSankeyHandler(&fakeSankeyService{analysis: analysisFixture()})
		_, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
		assert.Empty(t, dataOf(t, body)["conflicts"])
	})
}

func TestSankeyQueryParamsReachService(t *testing.T) {
	svc := &fakeSankeyService{analysis: analysisFixture()}
	h := NewSankeyHandler(svc)

	code, _ := doGet(t, h,
		"/api/prism/sankey?symbol=MSFT&from=2025Q1&to=2026Q2&granularity=fy&lang=en", h.Sankey)
	require.Equal(t, http.StatusOK, code)

	assert.Equal(t, "MSFT", svc.gotSymbol)
	assert.Equal(t, "2025Q1", svc.gotFrom)
	assert.Equal(t, "2026Q2", svc.gotTo)
	assert.Equal(t, "fy", svc.gotGranularity)
	assert.Equal(t, "en", svc.gotLang)
}

func TestSankeyErrorBranches(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		svc      *fakeSankeyService
		wantCode int
		wantText string
	}{
		{
			name: "missing symbol", target: "/api/prism/sankey",
			svc:      &fakeSankeyService{analysis: analysisFixture()},
			wantCode: http.StatusBadRequest, wantText: "symbol required",
		},
		{
			// 静默当季度处理会让用户以为自己筛的是别的粒度。
			name: "invalid granularity", target: "/api/prism/sankey?symbol=MSFT&granularity=x",
			svc:      &fakeSankeyService{analysis: analysisFixture()},
			wantCode: http.StatusBadRequest, wantText: "granularity",
		},
		{
			name: "no template", target: "/api/prism/sankey?symbol=NOPE",
			svc:      &fakeSankeyService{err: sankey.ErrNoTemplate},
			wantCode: http.StatusNotFound, wantText: "no sankey template",
		},
		{
			name: "unknown symbol", target: "/api/prism/sankey?symbol=NOPE",
			svc:      &fakeSankeyService{err: prismstore.ErrNotFound},
			wantCode: http.StatusNotFound, wantText: "unknown symbol",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewSankeyHandler(tc.svc)
			code, body := doGet(t, h, tc.target, h.Sankey)
			assert.Equal(t, tc.wantCode, code)
			assert.Contains(t, jsonString(t, body), tc.wantText)
		})
	}

	// 三种 404 文案互不混淆。
	t.Run("404 messages stay distinct", func(t *testing.T) {
		noTmpl := NewSankeyHandler(&fakeSankeyService{err: sankey.ErrNoTemplate})
		_, b1 := doGet(t, noTmpl, "/api/prism/sankey?symbol=A", noTmpl.Sankey)
		unknown := NewSankeyHandler(&fakeSankeyService{err: prismstore.ErrNotFound})
		_, b2 := doGet(t, unknown, "/api/prism/sankey?symbol=A", unknown.Sankey)
		assert.NotEqual(t, jsonString(t, b1), jsonString(t, b2),
			"「无模板」与「未知 symbol」必须能从文案区分开")
	})

	t.Run("service failure is 500", func(t *testing.T) {
		h := NewSankeyHandler(&fakeSankeyService{err: errors.New("boom")})
		code, _ := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
		assert.Equal(t, http.StatusInternalServerError, code)
	})
}

// AD-14 硬故障：encoding/json 对 ±Inf 直接报错，会让整个响应 500。
// 引擎侧已保证不产出 Inf，这里是第二道防线。
func TestSankeyInfNeverBreaksJSON(t *testing.T) {
	a := analysisFixture()
	a.Periods[0].Metrics.Revenue = math.Inf(1)
	a.Periods[0].Metrics.GrossMargin = math.Inf(-1)
	a.Periods[0].Graph.Nodes[0].Value = math.Inf(1)
	a.Periods[0].Graph.Links[0].Value = math.Inf(-1)
	a.Periods[0].Metrics.Segments["cloud"] = math.Inf(1)
	a.Matrix[0].Values = []float64{math.Inf(1), math.NaN()}
	a.Matrix[0].Change = math.Inf(-1)
	h := NewSankeyHandler(&fakeSankeyService{analysis: a})

	code, body := doGet(t, h, "/api/prism/sankey?symbol=MSFT", h.Sankey)
	require.Equal(t, http.StatusOK, code, "含 Inf 时仍须 200，不得因 json.Marshal 失败变 500")

	data := dataOf(t, body)
	p := data["periods"].([]any)[0].(map[string]any)
	assert.Nil(t, p["metrics"].(map[string]any)["revenue"], "±Inf 应映射为 null")
	assert.Nil(t, p["metrics"].(map[string]any)["gross_margin"])
	assert.Nil(t, p["metrics"].(map[string]any)["segments"].(map[string]any)["cloud"])
	assert.Nil(t, p["graph"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["value"])
	assert.Nil(t, p["graph"].(map[string]any)["links"].([]any)[0].(map[string]any)["value"])

	row := data["matrix"].([]any)[0].(map[string]any)
	assert.Equal(t, []any{nil, nil}, row["values"], "Inf 与 NaN 都应为 null")
	assert.Nil(t, row["change"])
}

func TestFundamentalResponse(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := &fakeSankeyService{fundamental: &sankey.FundamentalView{
			Symbol:  "MSFT",
			Periods: []string{"2026Q1", "2026Q2"},
			Metrics: map[string][]float64{
				"revenue": {100e9, math.NaN()},
				"roe_ttm": {math.Inf(1), 0.35},
			},
			Dates:  []string{"2026-06-29", "2026-06-30"},
			Closes: []float64{101.5, math.NaN()},
		}}
		h := NewSankeyHandler(svc)

		code, body := doGet(t, h, "/api/prism/fundamental?symbol=MSFT", h.Fundamental)
		require.Equal(t, http.StatusOK, code)
		data := dataOf(t, body)

		assert.Equal(t, "MSFT", svc.gotFundSymbol)
		assert.Equal(t, []any{"2026Q1", "2026Q2"}, data["periods"])
		assert.Equal(t, []any{"2026-06-29", "2026-06-30"}, data["dates"])
		assert.Equal(t, []any{101.5, nil}, data["closes"], "NaN 收盘价应为 null")

		metrics := data["metrics"].(map[string]any)
		assert.Equal(t, []any{100e9, nil}, metrics["revenue"])
		assert.Equal(t, []any{nil, 0.35}, metrics["roe_ttm"], "Inf 也应为 null")
	})

	t.Run("no data is 404", func(t *testing.T) {
		h := NewSankeyHandler(&fakeSankeyService{fundErr: prismstore.ErrNotFound})
		code, body := doGet(t, h, "/api/prism/fundamental?symbol=NOPE", h.Fundamental)
		assert.Equal(t, http.StatusNotFound, code)
		assert.Contains(t, jsonString(t, body), "unknown symbol")
	})

	t.Run("missing symbol is 400", func(t *testing.T) {
		h := NewSankeyHandler(&fakeSankeyService{})
		code, body := doGet(t, h, "/api/prism/fundamental", h.Fundamental)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, jsonString(t, body), "symbol required")
	})
}

func jsonString(t *testing.T, body map[string]any) string {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return string(b)
}
