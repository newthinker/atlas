package api

// Context Checkpoint: done_criteria → test mapping (TASK-012 functional[4] = G4 收口)
//
//	functional[4] 四环链 007 产生 → 009 装配 → 011 API → 012 渲染 端到端贯通
//	   → TestSankeyEndToEndEngineNoteReachesPage
//	     链路每一环都是真货: 真 YAML → 真 sankey.LoadTemplates → 真 sqlite Store →
//	     真 FundamentalRow(NetIncome > OperatingIncome) → 真 BuildPeriods(经 Service.Analyze)
//	     → 真 api.SankeyHandler(HTTP) 与真 web.Handler(HTTP),全部挂在真 Server.mux 上。
//	     **无 fake、无手工注入 Notes** —— 那正是本条要消除的缺口
//	     (既有 sankey_test.go:151 TestSankeyNotesPassThrough 直接把 note 塞进 fake,
//	      因此「引擎真的会在该输入下产生这条 note」从未被验证过)。
//	boundary(同一用例的对偶): NetIncome < OperatingIncome 的标的不得出现该 footnote
//	   → TestSankeyEndToEndEngineNoteReachesPage/正常数据不产生该 footnote
//
// 覆盖范围的诚实说明: 浏览器里 JS 再次 fetch API 并重绘 footnote 的那一步,
// Go 测试无法驱动(无 JS 引擎)。故本用例贯通的是「引擎 → 装配 → API/页面 HTTP 响应」,
// 页面侧断言的是服务端渲染出的 footnote(页面本就把 Notes 服务端渲染一份,
// 正是为了让这条链在无 JS 的情况下也成立)。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
)

// e2eTemplate is a real template YAML, parsed by the real loader.
const e2eTemplate = `company: %s
cik: "0000123456"
version: 1
segments:
  - key: cloud
    name_zh: 云业务
    name_en: Cloud
    xbrl_member: CloudMember
`

// e2eQuarters returns four real quarters. When interestIncome is true the
// issuer earns more after tax than it does from operations (large interest
// income), which is what drives the engine to emit the negative
// 营业利润→税项及其他 flow and its note (AD-13/AD-22).
func e2eQuarters(interestIncome bool) []prismstore.FundamentalRow {
	ends := []struct{ period, end string }{
		{"2025Q1", "2025-03-31"}, {"2025Q2", "2025-06-30"},
		{"2025Q3", "2025-09-30"}, {"2025Q4", "2025-12-31"},
	}
	net := 200.0 // < OperatingIncome: 正常纳税
	if interestIncome {
		net = 350.0 // > OperatingIncome: tax_other 为负
	}
	rows := make([]prismstore.FundamentalRow, 0, len(ends))
	for _, e := range ends {
		rows = append(rows, prismstore.FundamentalRow{
			FiscalPeriod: e.period, PeriodEnd: e.end, FilingDate: e.end, Source: "edgar",
			Revenue: 1000, GrossProfit: 600, OperatingIncome: 300, NetIncome: net,
			RnD: 100, SGnA: 150, IncomeTax: 40, EPSDiluted: 1.2, Equity: 5000,
		})
	}
	return rows
}

// sankeyNotesResp is the sliver of the API response these tests read: the
// engine notes, per period.
type sankeyNotesResp struct {
	Data struct {
		Periods []struct {
			Period string `json:"period"`
			Graph  struct {
				Notes []string `json:"notes"`
			} `json:"graph"`
		} `json:"periods"`
	} `json:"data"`
}

// newSankeyPageServer builds a real Server with the web pages wired
// (TemplatesDir is what makes /prism/sankey/ renderable) and the given store
// and service, either of which may be nil to model a disabled feature.
func newSankeyPageServer(t *testing.T, store *prismstore.Store, svc *sankey.Service) *Server {
	t.Helper()
	srv, err := NewServer(Config{Host: "localhost", Port: 0, TemplatesDir: "templates"}, Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Strategies:  strategy.NewEngine(),
		PrismStore:  store,
		PrismSankey: svc,
	}, zap.NewNop())
	require.NoError(t, err)
	return srv
}

// newE2EServer wires a real store and a real sankey service into a real Server.
func newE2EServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	tmplDir := filepath.Join(dir, "templates")
	require.NoError(t, os.MkdirAll(tmplDir, 0o755))
	for _, sym := range []string{"BRIDGE", "NORMAL"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmplDir, sym+".yaml"),
			[]byte(fmt.Sprintf(e2eTemplate, sym)), 0o644))
	}
	templates, err := sankey.LoadTemplates(tmplDir) // 真实模板加载器
	require.NoError(t, err)
	require.Len(t, templates, 2)

	store, err := prismstore.Open(filepath.Join(dir, "prism.db")) // 真实 sqlite
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	for _, c := range []struct {
		symbol         string
		interestIncome bool
	}{{"BRIDGE", true}, {"NORMAL", false}} {
		id, err := store.UpsertInstrument(prismstore.Instrument{
			Symbol: c.symbol, Name: c.symbol + " Inc", Market: "US", Type: "stock", Source: "edgar",
		})
		require.NoError(t, err)
		require.NoError(t, store.UpsertFundamentals(id, e2eQuarters(c.interestIncome)))
	}

	return newSankeyPageServer(t, store, sankey.NewService(store, templates)) // 真实 Service
}

func TestSankeyEndToEndEngineNoteReachesPage(t *testing.T) {
	srv := newE2EServer(t)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	t.Run("API 环: 引擎产生的 note 出现在 JSON 里", func(t *testing.T) {
		w := get("/api/prism/sankey?symbol=BRIDGE&granularity=q")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp sankeyNotesResp
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotEmpty(t, resp.Data.Periods)

		var notes []string
		for _, p := range resp.Data.Periods {
			notes = append(notes, p.Graph.Notes...)
		}
		require.NotEmpty(t, notes, "引擎应为 NetIncome > OperatingIncome 的期产生 note")
		assert.Contains(t, notes[0], "税项及其他",
			"note 必须是引擎真算出来的负残差说明,而不是任何一层塞进去的")
	})

	t.Run("页面环: 同一条 note 渲染为可见 footnote", func(t *testing.T) {
		w := get("/prism/sankey/BRIDGE?granularity=q")
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `id="sankey-notes"`)
		assert.Contains(t, body, "税项及其他",
			"AD-22: 引擎 → 装配 → 页面 四环贯通,footnote 必须可见")
		assert.Contains(t, body, "按 0 宽绘制")
	})

	t.Run("正常数据不产生该 footnote", func(t *testing.T) {
		// 对偶断言: 没有这一条,「页面恒印这句话」也能让上面两条全绿。
		w := get("/prism/sankey/NORMAL?granularity=q")
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "税项及其他")

		w = get("/api/prism/sankey?symbol=NORMAL&granularity=q")
		require.Equal(t, http.StatusOK, w.Code)
		var resp sankeyNotesResp
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotEmpty(t, resp.Data.Periods)
		for _, p := range resp.Data.Periods {
			assert.Empty(t, p.Graph.Notes, "正常数据不应有负残差 note")
		}
	})
}

// TestSankeyPageRouteRegistration: 页面路由**无条件注册**,PrismSankey 为 nil 时
// 给「未启用」的解释性 404。
//
// 第二个子用例是本任务发现的既有陷阱: mux 里没注册的 /prism/... 会落到 "/" 的
// catch-all(dashboard),于是「功能未关」的书签打开后是一个 **200 的 dashboard**——
// 比裸 404 更容易误导(用户根本不知道自己看的不是财报桥)。既有 /prism/detail/
// 等页面至今如此(不在本任务 scope,已写入 discovery)。
func TestSankeyPageRouteRegistration(t *testing.T) {
	store, err := prismstore.Open(filepath.Join(t.TempDir(), "prism.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	t.Run("sankey service 为 nil 时页面仍解释原因", func(t *testing.T) {
		srv := newSankeyPageServer(t, store, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/prism/sankey/MSFT", nil))
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "未启用",
			"必须是解释性 404,不是 mux 的裸 404")
	})

	t.Run("prism 完全未启用时也不落到 dashboard", func(t *testing.T) {
		srv := newSankeyPageServer(t, nil, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/prism/sankey/MSFT", nil))
		require.Equal(t, http.StatusNotFound, w.Code,
			"未注册的话会命中 \"/\" 的 catch-all,返回 200 的 dashboard")
		assert.Contains(t, w.Body.String(), "未启用")
		assert.NotContains(t, w.Body.String(), "sankey-grid", "不得渲染出任何页面")
	})
}
