package api

// Context Checkpoint: done_criteria → test mapping
//   boundary  Dependencies.PrismSankey 为 nil 时两条 sankey 路由**不注册**（负向断言） →
//             TestSankeyRoutesRegistration

import (
	"net/http"
	"net/http/httptest"
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

func newTestServer(t *testing.T, svc *sankey.Service) *Server {
	t.Helper()
	srv, err := NewServer(Config{Host: "localhost", Port: 0}, Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Strategies:  strategy.NewEngine(),
		PrismSankey: svc,
	}, zap.NewNop())
	require.NoError(t, err)
	return srv
}

func TestSankeyRoutesRegistration(t *testing.T) {
	paths := []string{"/api/prism/sankey", "/api/prism/fundamental"}

	t.Run("not registered without a service", func(t *testing.T) {
		srv := newTestServer(t, nil)
		for _, p := range paths {
			w := httptest.NewRecorder()
			srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p+"?symbol=MSFT", nil))
			assert.Equal(t, http.StatusNotFound, w.Code,
				"prism 未启用时 %s 不应注册（否则会暴露一个必然报错的端点）", p)
		}
	})

	t.Run("registered with a service", func(t *testing.T) {
		// 空模板集：路由已注册，故请求走到 handler 而不是 mux 的 404。
		srv := newTestServer(t, sankey.NewService(nil, map[string]*sankey.Template{}))
		for _, p := range paths {
			w := httptest.NewRecorder()
			srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"%s 应已注册并由 handler 处理（缺 symbol → 400）", p)
		}
	})
}
