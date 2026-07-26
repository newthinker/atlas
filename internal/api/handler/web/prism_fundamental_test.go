package web

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping (TASK-013 /prism/fundamental)
//
//	functional[0] 200 + id="fund-chart" + /static/echarts.min.js
//	                → TestPrismFundamentalPage
//	functional[1] 无 fundamental_q 数据 → 404
//	                → TestPrismFundamentalDistinct404s/no_data
//	functional[2] 7 个指标 / 折线柱状 / 季度年度 / dataZoom 容器
//	                → TestPrismFundamentalPageControls
//	functional[3] prism_board 卡片含 /prism/fundamental/ 入口链接
//	                → TestPrismBoardLinksToFundamental(两份模板都查)
//	functional[4] null 传播 (a): 含 null 的序列不得被当成 0
//	                → TestPrismFundamentalNullsSurviveToPage(+对偶)
//	boundary[0]   provider 未启用(nil) → 404 且不 panic
//	                → TestPrismFundamentalDistinct404s/not_enabled
//	boundary[1]   null 传播 (b)(c) 已按 AD-7 降级为人工验收;
//	              **可验证的代偿措施** = 静态守卫钉住不得出现把 null 变 0 的写法
//	                → TestPrismFundamentalJSHasNoNullToZeroCoercion
//	non_func[0]   双模板目录逐字节一致 + 磁盘模式可渲染
//	                → TestFundamentalTemplateDirsInSync / TestNewHandlerDiskModeRendersFundamental
//	non_func[1]   AD-8 resp.data 等 → verify_by: review(静态守卫见上)

// fakeFundamental records the symbol it was asked for so a handler that ignores
// its argument cannot hide behind a page that still renders.
type fakeFundamental struct {
	v      *sankey.FundamentalView
	err    error
	symbol string
	calls  int
}

func (f *fakeFundamental) Fundamental(symbol string) (*sankey.FundamentalView, error) {
	f.symbol = symbol
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.v, nil
}

// sampleFundamental is four quarters. roe_ttm is NaN for the first three: the
// engine refuses to pass a partial sum off as a TTM figure, so this is the
// shape every real symbol has, not a contrived edge case.
func sampleFundamental() *sankey.FundamentalView {
	return &sankey.FundamentalView{
		Symbol:  "MSFT",
		Periods: []string{"2025Q1", "2025Q2", "2025Q3", "2025Q4"},
		// 报告期在日历上的真实位置——股价按它重采样到与指标同一刻度（TASK-004）。
		PeriodEnds: []string{"2025-03-31", "2025-06-30", "2025-09-30", "2025-12-31"},
		Metrics: map[string][]float64{
			"revenue":          {1000, 1100, 1200, 1300},
			"gross_profit":     {600, 660, 720, 780},
			"operating_income": {300, 330, 360, 390},
			"net_income":       {200, 220, 240, 260},
			"equity":           {5000, 5100, 5200, 5300},
			"roe_ttm":          {math.NaN(), math.NaN(), math.NaN(), 0.176},
		},
		Dates:  []string{"2025-12-30", "2025-12-31"},
		Closes: []float64{101.5, 102.5},
	}
}

func newFundamentalHandler(t *testing.T, f *fakeFundamental) *Handler {
	t.Helper()
	h, err := NewHandlerWithFS(TemplateFS())
	require.NoError(t, err)
	h.SetPrismFundamental(f)
	return h
}

func getFundamentalPage(t *testing.T, h *Handler, symbol string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.PrismFundamental(rec, httptest.NewRequest(http.MethodGet, "/prism/fundamental/"+symbol, nil), symbol)
	return rec
}

func TestPrismFundamentalPage(t *testing.T) {
	f := &fakeFundamental{v: sampleFundamental()}
	rec := getFundamentalPage(t, newFundamentalHandler(t, f), "MSFT")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="fund-chart"`)
	assert.Contains(t, body, "/static/echarts.min.js")
	assert.Equal(t, 1, f.calls, "页面必须服务端取一次数据（否则无法区分 404）")
}

// TestPrismFundamentalSymbolReachesService: 两个**不同**的 symbol，各自断言。
// 单个 symbol 时「把 symbol 硬编码成该值」也能让断言通过（ME12 形态），两个就不能。
func TestPrismFundamentalSymbolReachesService(t *testing.T) {
	for _, sym := range []string{"MSFT", "NVDA"} {
		f := &fakeFundamental{v: sampleFundamental()}
		rec := getFundamentalPage(t, newFundamentalHandler(t, f), sym)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, sym, f.symbol, "symbol 必须原样透传到 service")
		assert.Contains(t, rec.Body.String(), sym, "页面标题应显示该 symbol")
	}
}

func TestPrismFundamentalPageControls(t *testing.T) {
	rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: sampleFundamental()}), "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	for _, want := range []string{
		`id="fund-chart"`,  // 主图容器
		`id="metric-btns"`, // 指标切换组
		`id="kind-btns"`,   // 折线 ↔ 柱状
		`data-k="line"`, `data-k="bar"`,
		`id="gran-btns"`, // 季度 ↔ 年度
		`data-g="q"`, `data-g="fy"`,
		`id="price-toggle"`, // 股价叠加（双轴）
		`id="fund-zoom-note"`,
		// 7 个指标（design-spec §5.2）
		`data-m="revenue"`, `data-m="net_income"`, `data-m="gross_margin"`,
		`data-m="op_margin"`, `data-m="roe_ttm"`, `data-m="revenue_yoy"`, `data-m="net_income_yoy"`,
	} {
		assert.Contains(t, body, want, "缺少交互元素 %s（design-spec §5.2）", want)
	}
	assert.Contains(t, body, "dataZoom", "需有 dataZoom 双端滑块")
}

// TestPrismFundamentalNullsSurviveToPage —— functional[4] (a)。
//
// 页面把序列服务端内联进来（也正因如此年度切换与增速现算都不需要新请求）。
// NaN 必须成为 JSON 的 null 而**不是 0**：一条把缺失画成 0 的曲线，在图上与
// 「该季 ROE 真的是 0」完全无法区分，而 roe_ttm 前 3 季必然缺失。
func TestPrismFundamentalNullsSurviveToPage(t *testing.T) {
	rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: sampleFundamental()}), "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, `"roe_ttm":[null,null,null,0.176]`,
		"NaN 必须序列化为 null；写成 0 会画出一条假的归零曲线")
	assert.NotContains(t, body, `"roe_ttm":[0,0,0,`, "缺失值不得被当成 0")

	t.Run("非缺失值不受影响", func(t *testing.T) {
		assert.Contains(t, body, `"revenue":[1000,1100,1200,1300]`,
			"正常序列必须原样保留 —— 否则「全变 null」也能让上面的断言通过")
	})

	t.Run("±Inf 同样映射为 null", func(t *testing.T) {
		v := sampleFundamental()
		v.Metrics["roe_ttm"] = []float64{math.Inf(1), math.Inf(-1), 0.1, 0.2}
		rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: v}), "MSFT")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"roe_ttm":[null,null,0.1,0.2]`,
			"AD-14: ±Inf 也必须是 null，否则 JSON 直接不合法")
	})
}

// TestPrismFundamentalJSHasNoNullToZeroCoercion —— boundary[1] 的代偿措施。
//
// (b) 4 季聚合与 (c) 增速现算是纯 JS，已按 AD-7 转人工验收。可自动验证的部分是
// 这条静态守卫：把 null 静默变成 0 的惯用写法一旦出现在模板里就报警。
func TestPrismFundamentalJSHasNoNullToZeroCoercion(t *testing.T) {
	src, err := os.ReadFile("templates/prism_fundamental.html")
	require.NoError(t, err)
	page := string(src)

	for _, bad := range []struct{ pattern, why string }{
		{`|| 0`, "`x || 0` 会把 null 静默变 0，画出假的归零曲线"},
		{`||0`, "同上（无空格写法）"},
		{`Number(null)`, "Number(null) === 0"},
		{`+null`, "一元 + 会把 null 变 0"},
	} {
		assert.NotContains(t, page, bad.pattern, "模板 JS 不得出现 %q：%s", bad.pattern, bad.why)
	}

	// 正向：必须显式处理 null，而不是靠强制转换蒙混过去。
	assert.Regexp(t, regexp.MustCompile(`===\s*null|!==\s*null`), page,
		"必须有显式的 null 判断")

	// AD-8（resp.data 解包）在本页不适用：整份序列由服务端内联，页面不发 XHR，
	// 故不存在「忘记解 {data, meta} 包裹层」这个缺陷面。钉住这个前提 —— 一旦有人
	// 加了 fetch，本断言会红，提醒他同时补上 resp.data 解包。
	assert.NotContains(t, page, "fetch(",
		"本页数据由服务端内联，不应有 XHR；若确需新增 fetch，必须同时用 resp.data 解包（AD-8）")
}

// TestPrismFundamentalDistinct404s: 两种 404 的文案必须不同 —— 「功能未启用」是
// 运维问题，「该标的无财报数据」是数据问题，混为一句会让人查错方向。
func TestPrismFundamentalDistinct404s(t *testing.T) {
	cases := []struct {
		name    string
		handler func(t *testing.T) *Handler
		keyword string
	}{
		{
			name: "not_enabled", // provider 为 nil
			handler: func(t *testing.T) *Handler {
				h, err := NewHandlerWithFS(TemplateFS()) // 未 SetPrismFundamental
				require.NoError(t, err)
				return h
			},
			keyword: "未启用",
		},
		{
			name: "no_data", // 该标的没有 fundamental_q 数据
			handler: func(t *testing.T) *Handler {
				return newFundamentalHandler(t, &fakeFundamental{
					err: fmt.Errorf("%w: MSFT has no fundamentals", prismstore.ErrNotFound)})
			},
			keyword: "无财报数据",
		},
	}

	bodies := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := getFundamentalPage(t, c.handler(t), "MSFT")
			require.Equal(t, http.StatusNotFound, rec.Code)
			assert.Contains(t, rec.Body.String(), c.keyword)
			bodies[c.name] = rec.Body.String()
		})
	}

	require.Len(t, bodies, 2)
	for _, c := range cases {
		for name, body := range bodies {
			if name == c.name {
				continue
			}
			assert.NotContains(t, body, c.keyword,
				"%s 的文案不得含 %s 的关键词 %q —— 两种 404 必须能被用户区分", name, c.name, c.keyword)
		}
	}
}

func TestPrismFundamentalAnalyzeErrorIs500(t *testing.T) {
	h := newFundamentalHandler(t, &fakeFundamental{err: fmt.Errorf("db down")})
	rec := getFundamentalPage(t, h, "MSFT")
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "存储故障不是 404")
	assert.NotContains(t, rec.Body.String(), "db down", "500 不回显内部错误细节")
}

// TestPrismBoardLinksToFundamental: functional[3]。**两份模板都要查** —— 只改 embed
// 那份时单测走 embed 会全绿，而 atlas serve 读磁盘那份，线上就是没有入口（AD-4）。
func TestPrismBoardLinksToFundamental(t *testing.T) {
	for _, p := range []string{"templates/prism_board.html", "../../templates/prism_board.html"} {
		src, err := os.ReadFile(p)
		require.NoError(t, err, "%s 必须存在", p)
		assert.Contains(t, string(src), "/prism/fundamental/",
			"%s 的卡片必须有财务趋势页入口", p)
	}
}

// TestFundamentalTemplateDirsInSync: AD-4。
func TestFundamentalTemplateDirsInSync(t *testing.T) {
	embedded, err := os.ReadFile("templates/prism_fundamental.html")
	require.NoError(t, err)
	disk, err := os.ReadFile("../../templates/prism_fundamental.html")
	require.NoError(t, err, "internal/api/templates/ 下必须有 prism_fundamental.html（serve.go 读这里）")
	assert.Equal(t, string(embedded), string(disk), "两份模板必须逐字节一致")
}

func TestNewHandlerDiskModeRendersFundamental(t *testing.T) {
	h, err := NewHandler("../../templates")
	require.NoError(t, err, "磁盘模式必须能解析 prism_fundamental.html")
	h.SetPrismFundamental(&fakeFundamental{v: sampleFundamental()})

	rec := getFundamentalPage(t, h, "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `id="fund-chart"`)
}
