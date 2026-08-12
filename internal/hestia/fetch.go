package hestia

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxResponseBytes 是单次响应的上限。index 页实测 38KB，文章页同量级；
// 10MB 宽松到不会误伤，又不会让一个异常响应把内存吃光。
const maxResponseBytes = 10 << 20

// Fetcher 取一个 URL 的原始字节。discover 与 4b 的 ingest 共用。
//
// 定义成接口而不是直接用 *http.Client：discover 的测试要用 index 页快照喂数据，
// 不该碰网络。
type Fetcher interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// NewPBOCFetcher 返回抓央行站点用的 Fetcher。
//
// Transport 不设 Proxy —— 这是**有意的**，不是忘了配。http.DefaultTransport
// 用 ProxyFromEnvironment，而 Atlas 全局设了 http_proxy=127.0.0.1:7897
// （Yahoo 直连恒 403 才加的）。
//
// 绕过的理由不是「走代理会失败」——2026-08-12 实测走代理同样返回 200。真实理由是
// **代理未必在跑**：launchd 唤起时若 clash 没启动，走代理会连接失败，而直连本来
// 能成。境内站点绕境外代理本也是多余的一跳。
//
// 这一层必须在 client 里做，不能只靠 plist 的 no_proxy —— 那是进程级的，
// 会把 Telegram 和 Sheets 一起放行（约束 C6 要求同进程内三种策略）。
func NewPBOCFetcher(timeout time.Duration) Fetcher {
	return pbocFetcher{c: &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{}, // Proxy 留零值：直连
	}}
}

type pbocFetcher struct{ c *http.Client }

// Get 带 ctx —— *http.Client.Get 本身不收 ctx，必须走 NewRequestWithContext，
// 否则 launchd 杀进程时正在飞的请求不会被取消。
func (f pbocFetcher) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("hestia fetch %s: %w", url, err)
	}
	resp, err := f.c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hestia fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// 带上状态码与 URL：404 与 403 的处置完全不同（前者是页面没了，
		// 后者多半是被限速或代理串进来了），只说「抓取失败」看不出来。
		return nil, fmt.Errorf("hestia fetch %s: HTTP %d", url, resp.StatusCode)
	}

	// LimitReader 多读一个字节，用来区分「恰好等于上限」与「超过上限」。
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("hestia fetch %s: reading body: %w", url, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("hestia fetch %s: response exceeds %d bytes", url, maxResponseBytes)
	}
	return body, nil
}
