package hestia

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transportOf 取出 Fetcher 内部的 Transport。同包测试可以碰未导出字段。
func transportOf(t *testing.T, f Fetcher) *http.Transport {
	t.Helper()
	pf, ok := f.(pbocFetcher)
	require.True(t, ok, "NewPBOCFetcher 应返回 pbocFetcher")
	tr, ok := pf.c.Transport.(*http.Transport)
	require.True(t, ok, "Transport 应是 *http.Transport")
	return tr
}

// PBOC 请求不得走代理。
//
// ⚠️ 不要把这条改写成「httptest server + Setenv HTTP_PROXY + 断言请求成功」——
// ProxyFromEnvironment 对 127.0.0.1 从不返回代理（实测），那种写法对代理行为
// 零鉴别力，用 http.DefaultTransport 的实现也能通过。必须问真实域名。
func TestPBOCFetcherDoesNotProxyPBOC(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	tr := transportOf(t, NewPBOCFetcher(time.Second))
	if tr.Proxy == nil {
		return // 直连，正是要的
	}
	// 也允许实现用一个对 PBOC 返回 nil 的 Proxy 函数——测行为，不测结构。
	req, err := http.NewRequest(http.MethodGet,
		"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html", nil)
	require.NoError(t, err)

	u, err := tr.Proxy(req)
	require.NoError(t, err)
	assert.Nil(t, u, "PBOC 请求不得走代理：代理未必在跑，直连本来能成")
}

func TestPBOCFetcherGet(t *testing.T) {
	t.Run("正常返回体", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>ok</html>"))
		}))
		defer srv.Close()

		got, err := NewPBOCFetcher(5*time.Second).Get(context.Background(), srv.URL)
		require.NoError(t, err)
		assert.Equal(t, "<html>ok</html>", string(got))
	})

	t.Run("非 200 报错且带状态码与 URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := NewPBOCFetcher(5*time.Second).Get(context.Background(), srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
		assert.Contains(t, err.Error(), srv.URL, "错误信息要带 URL，否则翻页时不知道是哪一页失败的")
	})

	t.Run("ctx 取消能中断", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立刻取消

		_, err := NewPBOCFetcher(5*time.Second).Get(ctx, srv.URL)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("超大响应被拒", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// 比上限多一个字节
			_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))
		}))
		defer srv.Close()

		_, err := NewPBOCFetcher(30*time.Second).Get(context.Background(), srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})

	// 恰好等于上限必须放行。这条与上一条**成对**，不是重复：
	// 上一条单独存在时，把实现写成 io.LimitReader(resp.Body, maxResponseBytes)
	// 配 len(body) >= maxResponseBytes 也照样绿——那个实现会把恰好 10MB 的正常
	// 响应误杀。DoD 的 boundary 明写「多读一个字节才能区分『恰好等于上限』与
	// 『超过上限』」，两侧都钉住才算区分。删掉任一条都会让另一侧无人守。
	t.Run("恰好等于上限被接受", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", maxResponseBytes)))
		}))
		defer srv.Close()

		got, err := NewPBOCFetcher(30*time.Second).Get(context.Background(), srv.URL)
		require.NoError(t, err)
		assert.Len(t, got, maxResponseBytes, "恰好等于上限不得被截断，也不得报错")
	})

	// 构造请求就失败的路径（URL 里有控制字符）。它与下面「读响应体中途断流」是
	// Get 仅有的另外两条错误出口，DoD 的 error_handling 要求错误信息带 URL——
	// 这两条出口同样要带，否则翻页时「哪一页挂了」在这两种失败上就断线。
	t.Run("URL 非法时报错且带 URL 与底层错误", func(t *testing.T) {
		const bad = "http://exa\x7fmple.com/x"

		_, err := NewPBOCFetcher(5*time.Second).Get(context.Background(), bad)
		require.Error(t, err)
		assert.Contains(t, err.Error(), bad, "错误信息要带 URL")
		require.NotNil(t, errors.Unwrap(err), "底层错误必须被 %w 包住，调用方才能 errors.As 出去")
	})

	t.Run("读响应体中途断流报错", func(t *testing.T) {
		// 声明 Content-Length 远大于实际写入的字节数再关连接，让 io.ReadAll
		// 拿到 unexpected EOF —— 这是「200 但body 不完整」的形态，不能被
		// 当成一次成功的抓取悄悄返回半截页面。
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 1024\r\n\r\nshort")
		}))
		defer srv.Close()

		_, err := NewPBOCFetcher(5*time.Second).Get(context.Background(), srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), srv.URL, "错误信息要带 URL")
		require.NotNil(t, errors.Unwrap(err), "底层错误必须被 %w 包住")
	})
}
