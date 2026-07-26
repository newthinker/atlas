package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping (TASK-012 /prism/sankey)
//
//	functional[0] 有模板 symbol → 200,含 id="sankey-grid" 与 /static/echarts.min.js
//	                → TestPrismSankeyPage
//	functional[1] 未配置模板的 symbol → 404
//	                → TestPrismSankeyThreeDistinct404s/no_template
//	functional[2] 粒度/语言/视图/报告期范围切换 + 矩阵 table 容器
//	                → TestPrismSankeyPageControls(容器与选项) +
//	                  TestPrismSankeyQueryParamsReachService(参数真的透传到 service)
//	functional[3] AD-22 Notes 渲染为可见 footnote(而非丢弃)
//	                → TestPrismSankeyNotesRenderedAsFootnote(含对偶断言)
//	                  另: TestPrismSankeyConflictsRendered(AD-26 降级提示)
//	functional[4] G4 端到端四环贯通 → internal/api/sankey_e2e_test.go
//	                  TestSankeyEndToEndEngineNoteReachesPage(本包只能到 fake 边界,
//	                  真实引擎链路在 api 包,见该文件头注释)
//	boundary      prism 未启用(nil) → 404 且不 panic
//	                → TestPrismSankeyThreeDistinct404s/not_enabled
//	non_func[0]   双模板目录 byte 级一致 + 磁盘模式可渲染
//	                → TestSankeyTemplateDirsInSync / TestNewHandlerDiskModeRendersSankey
//	non_func[1]   JS 用 resp.data 解包(AD-8) + 裸 404 守卫
//	                → TestPrismSankeyJSUnwrapsDataAndGuardsBare404(静态文本守卫,
//	                  Go 测试无法执行 JS,故此条主体是 verify_by: review)
//	error_handling 三种 404 页面文案互不相同
//	                → TestPrismSankeyThreeDistinct404s(两两不等 + 各含自身关键词、不含他者关键词)

// fakeSankey captures what the page handler passes down so a dropped query
// parameter cannot hide behind a page that still renders.
type fakeSankey struct {
	a   *sankey.Analysis
	err error

	symbol, from, to, gran, lang string
	calls                        int
}

func (f *fakeSankey) Analyze(symbol, from, to, granularity, lang string) (*sankey.Analysis, error) {
	f.symbol, f.from, f.to, f.gran, f.lang = symbol, from, to, granularity, lang
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.a, nil
}

// sampleAnalysis is two quarters with one segment, one matrix row and no notes.
func sampleAnalysis() *sankey.Analysis {
	a := &sankey.Analysis{
		Symbol: "MSFT", Granularity: "q",
		Periods: []sankey.PeriodView{
			{Period: "2025Q3", Graph: sankey.Graph{
				Nodes: []sankey.Node{{Name: "收入", Value: 1000}},
				Links: []sankey.Link{{Source: "收入", Target: "毛利", Value: 600}},
			}, Metrics: sankey.PeriodMetrics{Period: "2025Q3", PeriodEnd: "2025-09-30", Revenue: 1000}},
			{Period: "2025Q4", Graph: sankey.Graph{
				Nodes: []sankey.Node{{Name: "收入", Value: 1100}},
				Links: []sankey.Link{{Source: "收入", Target: "毛利", Value: 660}},
			}, Metrics: sankey.PeriodMetrics{Period: "2025Q4", PeriodEnd: "2025-12-31", Revenue: 1100}},
		},
		Matrix: []sankey.MatrixRow{
			{Key: "revenue", Label: "收入", Values: []float64{1000, 1100}, Change: 0.1, ChangeKind: "yoy"},
		},
	}
	a.Template.Version = 2
	a.Template.Lang = map[string]string{"cloud": "云业务"}
	return a
}

func newSankeyHandler(t *testing.T, f *fakeSankey) *Handler {
	t.Helper()
	h, err := NewHandlerWithFS(TemplateFS())
	require.NoError(t, err)
	h.SetPrismSankey(f)
	return h
}

func getSankeyPage(t *testing.T, h *Handler, target, symbol string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.PrismSankey(rec, httptest.NewRequest(http.MethodGet, target, nil), symbol)
	return rec
}

func TestPrismSankeyPage(t *testing.T) {
	f := &fakeSankey{a: sampleAnalysis()}
	rec := getSankeyPage(t, newSankeyHandler(t, f), "/prism/sankey/MSFT", "MSFT")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="sankey-grid"`)
	assert.Contains(t, body, "/static/echarts.min.js")
	assert.Contains(t, body, "MSFT")
	assert.Equal(t, "MSFT", f.symbol, "symbol 必须透传到 service")
}

func TestPrismSankeyPageControls(t *testing.T) {
	rec := getSankeyPage(t, newSankeyHandler(t, &fakeSankey{a: sampleAnalysis()}), "/prism/sankey/MSFT", "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	for _, want := range []string{
		`id="range-from"`, `id="range-to"`, // 报告期范围两个 select
		`id="gran-btns"`, `data-g="q"`, `data-g="fy"`, // 粒度 季/年
		`id="lang-btns"`, `data-l="zh"`, `data-l="en"`, // 语言 中/英
		`id="view-btns"`, `data-v="grid"`, `data-v="single"`, `data-v="stack"`, // 视图 网格/单期大图/堆叠柱
		`id="sankey-matrix"`,                      // 矩阵 table 容器
		`id="sankey-single"`, `id="sankey-stack"`, // 另两个视图容器
	} {
		assert.Contains(t, body, want, "缺少交互元素 %s(design-spec §5.2)", want)
	}
	// 范围 select 的选项来自 API 返回的期列表
	assert.Contains(t, body, "2025Q3")
	assert.Contains(t, body, "2025Q4")
}

func TestPrismSankeyQueryParamsReachService(t *testing.T) {
	// 参数原样透传:少传一个,页面照样渲染,只有这里能发现。
	f := &fakeSankey{a: sampleAnalysis()}
	h := newSankeyHandler(t, f)
	getSankeyPage(t, h, "/prism/sankey/MSFT?from=2024Q1&to=2025Q4&granularity=fy&lang=en", "MSFT")

	assert.Equal(t, "2024Q1", f.from)
	assert.Equal(t, "2025Q4", f.to)
	assert.Equal(t, "fy", f.gran)
	assert.Equal(t, "en", f.lang)

	t.Run("非法 granularity 按未指定处理", func(t *testing.T) {
		f := &fakeSankey{a: sampleAnalysis()}
		getSankeyPage(t, newSankeyHandler(t, f), "/prism/sankey/MSFT?granularity=x", "MSFT")
		assert.Equal(t, "", f.gran, "非法粒度不得当成 q 传下去")
	})
}

func TestPrismSankeyNotesRenderedAsFootnote(t *testing.T) {
	const note = "营业利润→税项及其他 为负（-500），按 0 宽绘制，不计入守恒"
	a := sampleAnalysis()
	a.Periods[0].Graph.Notes = []string{note}

	rec := getSankeyPage(t, newSankeyHandler(t, &fakeSankey{a: a}), "/prism/sankey/MSFT", "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="sankey-notes"`)
	assert.Contains(t, body, note, "AD-22: Notes 必须渲染为可见 footnote,不得丢弃")
	assert.Contains(t, body, "2025Q3", "footnote 要说明是哪一期")

	t.Run("对偶:无 note 时页面不凭空造 footnote", func(t *testing.T) {
		rec := getSankeyPage(t, newSankeyHandler(t, &fakeSankey{a: sampleAnalysis()}), "/prism/sankey/MSFT", "MSFT")
		assert.NotContains(t, rec.Body.String(), "按 0 宽绘制")
	})
}

func TestPrismSankeyConflictsRendered(t *testing.T) {
	const detail = "收到 5 个季度(应为 4),标签冲突使该财年无法聚合,已跳过"
	a := sampleAnalysis()
	a.Conflicts = []sankey.PeriodConflict{{Period: "FY2025", Detail: detail}}

	rec := getSankeyPage(t, newSankeyHandler(t, &fakeSankey{a: a}), "/prism/sankey/MSFT", "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="sankey-conflicts"`)
	assert.Contains(t, body, detail, "AD-26: conflicts 是「某个财年为什么不见了」的唯一解释来源")
	assert.Contains(t, body, "FY2025")

	t.Run("对偶:无 conflict 时不显示降级提示", func(t *testing.T) {
		rec := getSankeyPage(t, newSankeyHandler(t, &fakeSankey{a: sampleAnalysis()}), "/prism/sankey/MSFT", "MSFT")
		assert.NotContains(t, rec.Body.String(), detail)
	})
}

// TestPrismSankeyThreeDistinct404s: 三种 404 的页面文案必须互不相同。
// 混为一句「无数据」会让「功能未启用」的运维问题被当成数据问题查。
func TestPrismSankeyThreeDistinct404s(t *testing.T) {
	cases := []struct {
		name    string
		handler func(t *testing.T) *Handler
		keyword string
	}{
		{
			name: "not_enabled", // (c) prism 未启用 / 模板全部加载失败
			handler: func(t *testing.T) *Handler {
				h, err := NewHandlerWithFS(TemplateFS()) // 未 SetPrismSankey
				require.NoError(t, err)
				return h
			},
			keyword: "未启用",
		},
		{
			name: "no_template", // (a) 该标的没配模板
			handler: func(t *testing.T) *Handler {
				return newSankeyHandler(t, &fakeSankey{
					err: fmt.Errorf("%w: MSFT", sankey.ErrNoTemplate)})
			},
			keyword: "未配置财报桥模板",
		},
		{
			name: "unknown_symbol", // (b) 标的不在库里
			handler: func(t *testing.T) *Handler {
				return newSankeyHandler(t, &fakeSankey{
					err: fmt.Errorf("%w: MSFT", prismstore.ErrNotFound)})
			},
			keyword: "不在库中",
		},
	}

	bodies := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := getSankeyPage(t, c.handler(t), "/prism/sankey/MSFT", "MSFT")
			require.Equal(t, http.StatusNotFound, rec.Code)
			assert.Contains(t, rec.Body.String(), c.keyword)
			bodies[c.name] = rec.Body.String()
		})
	}

	require.Len(t, bodies, 3)
	for _, c := range cases {
		for name, body := range bodies {
			if name == c.name {
				continue
			}
			assert.NotContains(t, body, c.keyword,
				"%s 的文案不得含 %s 的关键词 %q —— 三种 404 必须能被用户区分", name, c.name, c.keyword)
		}
	}
}

func TestPrismSankeyAnalyzeErrorIs500(t *testing.T) {
	h := newSankeyHandler(t, &fakeSankey{err: errors.New("db down")})
	rec := getSankeyPage(t, h, "/prism/sankey/MSFT", "MSFT")
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "存储故障不是 404")
	assert.NotContains(t, rec.Body.String(), "db down", "500 不回显内部错误细节")
}

// TestSankeyTemplateDirsInSync: AD-4 —— embed 目录与 serve.go 实际读取的磁盘目录
// 必须逐字节一致,否则单测(走 embed)全绿而 atlas serve(走磁盘)启动即失败。
func TestSankeyTemplateDirsInSync(t *testing.T) {
	embedded, err := os.ReadFile("templates/prism_sankey.html")
	require.NoError(t, err, "handler/web/templates/ 下必须有 prism_sankey.html")
	disk, err := os.ReadFile("../../templates/prism_sankey.html")
	require.NoError(t, err, "internal/api/templates/ 下必须有 prism_sankey.html(serve.go 读的是这里)")
	assert.Equal(t, string(embedded), string(disk), "两份模板必须逐字节一致")
}

// TestNewHandlerDiskModeRendersSankey: serve.go 用 NewHandler(磁盘目录) 构造,
// 故 pages 列表与磁盘文件都必须就位。
func TestNewHandlerDiskModeRendersSankey(t *testing.T) {
	h, err := NewHandler("../../templates")
	require.NoError(t, err, "磁盘模式必须能解析 prism_sankey.html")
	h.SetPrismSankey(&fakeSankey{a: sampleAnalysis()})

	rec := getSankeyPage(t, h, "/prism/sankey/MSFT", "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `id="sankey-grid"`)
}

// TestPrismSankeyJSUnwrapsDataAndGuardsBare404 是静态文本守卫: Go 测试跑不了 JS,
// 但两个最容易回归的点(AD-8 的 resp.data 解包、裸 404 的非 JSON 分支)至少要在
// 模板里存在。真正的判定归 verify_by: review。
func TestPrismSankeyJSUnwrapsDataAndGuardsBare404(t *testing.T) {
	src, err := os.ReadFile("templates/prism_sankey.html")
	require.NoError(t, err)
	page := string(src)

	assert.Contains(t, page, "resp.data", "AD-8: 必须用 resp.data 解包(prism_compare.html 此处有缺陷,勿照抄)")
	assert.Contains(t, page, "JSON.parse", "裸 404 的响应体不是 JSON,必须自己 parse 并捕获失败")
	assert.Contains(t, page, "接口未注册", "非 JSON 响应要给出「功能未启用」而不是「无数据」")
	assert.Contains(t, page, "resp.data.error", "400/404 的错误在 resp.data.error(response.JSON 会包一层),不在 resp.error")
}
