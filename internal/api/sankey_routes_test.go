package api

// Context Checkpoint: done_criteria → test mapping
//
//	TASK-013 error_handling  prism 未启用时 /api/prism/{sankey,fundamental} 必须
//	  **无条件注册**并返回 JSON 404，而不是落到 "/" catch-all 的 200 dashboard →
//	  TestPrismAPIsAnswerJSON404WhenDisabled
//	TASK-011 boundary（语义已反转，见下）→ TestSankeyRoutesRegistration
//
// ⚠ 本文件的 fixture 必须带 TemplatesDir。
// 原 newTestServer 传的是 Config{Host, Port}——没有 TemplatesDir，而
// server.go 的 `mux.HandleFunc("/", Dashboard)` 在 `if cfg.TemplatesDir != ""` 内。
// 于是未注册的 /api/prism/* 在这个阉割 server 上得到 mux 的裸 404，
// 而在生产（TemplatesDir 已配）上得到 **200 + 2896 字节 dashboard HTML**。
// 实测两种 fixture 对同一份代码的结果：
//
//	无 TemplatesDir → 404 / 19B  / text/plain
//	有 TemplatesDir → 200 / 2896B / text/html   ← 生产真实行为
//
// 也就是说旧断言看到的 404 是生产根本不会出现的，它绿了三个 sprint 却对真实缺陷
// 完全无感。负向断言的 fixture 必须在「兜底路径」上与生产一致，否则断言恒真。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism/sankey"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
)

// newTestServer builds a server shaped like production: TemplatesDir is set, so
// the "/" dashboard catch-all is registered. Without it the negative assertions
// below would pass for a reason that cannot occur in production.
func newTestServer(t *testing.T, svc *sankey.Service) *Server {
	t.Helper()
	srv, err := NewServer(Config{Host: "localhost", Port: 0, TemplatesDir: "templates"}, Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Strategies:  strategy.NewEngine(),
		PrismSankey: svc,
	}, zap.NewNop())
	require.NoError(t, err)
	return srv
}

func getPath(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// requireDashboardCatchAllPresent is the guard that keeps this file honest: if
// someone drops TemplatesDir from the fixture, every negative assertion below
// silently degrades into "an unregistered path 404s", which is exactly the bug
// they exist to catch. Fail loudly instead.
func requireDashboardCatchAllPresent(t *testing.T, srv *Server) {
	t.Helper()
	w := getPath(t, srv, "/definitely-not-a-registered-route")
	require.Equal(t, http.StatusOK, w.Code,
		"fixture 必须含 \"/\" dashboard catch-all（即 TemplatesDir 已配），"+
			"否则下面的负向断言会因为一个生产不存在的裸 404 而恒真")
	require.Contains(t, strings.ToLower(w.Body.String()), "<html",
		"catch-all 应当返回 dashboard HTML —— 这正是缺陷的形态")
}

var prismSankeyAPIs = []string{"/api/prism/sankey", "/api/prism/fundamental"}

// TestPrismAPIsAnswerJSON404WhenDisabled: prism 未启用时两个 API 必须**无条件注册**
// 并返回带原因的 JSON 404。
//
// 这里只断 404 是不够的（test-agent-9 验收预告第 1 条）：落到 dashboard 时状态码是
// 200，但若有人把 catch-all 改成返回 404 + HTML，纯状态码断言照样绿。故同时钉住
// 「响应体是 JSON」「不含 dashboard 标志」「能解出原因」三点。
func TestPrismAPIsAnswerJSON404WhenDisabled(t *testing.T) {
	srv := newTestServer(t, nil) // prism 未启用
	requireDashboardCatchAllPresent(t, srv)

	for _, p := range prismSankeyAPIs {
		t.Run(p, func(t *testing.T) {
			w := getPath(t, srv, p+"?symbol=MSFT")
			body := w.Body.String()

			require.Equal(t, http.StatusNotFound, w.Code,
				"未启用时必须是 404，落到 dashboard 会是 200")
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json",
				"响应体必须是 JSON —— 前端 r.json() 拿到 HTML 会直接抛")
			assert.NotContains(t, strings.ToLower(body), "<html",
				"不得返回 dashboard HTML")
			assert.NotContains(t, body, "sankey-grid", "不得渲染出任何页面")

			// response.JSON 包了一层：错误在 resp.data.error（AD-8）。
			var resp struct {
				Data struct {
					Error string `json:"error"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp),
				"响应体必须可被 JSON.parse —— 这正是缺陷的直接后果")
			assert.Contains(t, resp.Data.Error, "not enabled",
				"必须说明是「功能未启用」，而不是让人以为该标的无数据")
		})
	}
}

// TestSankeyRoutesRegistration —— ⚠ 语义相对 TASK-011 已**反转**。
//
// 原断言是「nil 时不注册（否则会暴露一个必然报错的端点）」。该理由在 mux 有
// catch-all 时不成立：不注册并不会让请求消失，只会让它落到 dashboard 并返回
// 200 + HTML，比一个诚实的 JSON 404 糟得多。故现在断言「注册且答 JSON 404」。
// 这不是回归失败，是 TASK-013 error_handling 要求的修正。
func TestSankeyRoutesRegistration(t *testing.T) {
	t.Run("registered even without a service（答 JSON 404，不落到 dashboard）", func(t *testing.T) {
		srv := newTestServer(t, nil)
		requireDashboardCatchAllPresent(t, srv)
		for _, p := range prismSankeyAPIs {
			w := getPath(t, srv, p+"?symbol=MSFT")
			assert.Equal(t, http.StatusNotFound, w.Code, "%s 应已注册并由 handler 答 404", p)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json", p)
		}
	})

	t.Run("registered with a service", func(t *testing.T) {
		// 空模板集：路由已注册，故请求走到 handler 而不是 mux 的 404。
		srv := newTestServer(t, sankey.NewService(nil, map[string]*sankey.Template{}))
		for _, p := range prismSankeyAPIs {
			w := getPath(t, srv, p)
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"%s 应已注册并由 handler 处理（缺 symbol → 400）", p)
		}
	})
}
