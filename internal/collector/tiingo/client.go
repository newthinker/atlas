// Package tiingo 直连 Tiingo 日线 API（GET /tiingo/daily/<ticker>/prices，Token 头鉴权）。
// 角色 = 美股价格备源（设计 docs/superpowers/specs/2026-09-30-tiingo-source-design.md）：
// prism 美股价格链 yahoo→tiingo→twelvedata 的第二跳，以及 serve 分析循环的最后一个外部兜底。
//
// 凭证只走 Authorization 请求头、绝不进 URL（同 twelvedata ADR#7：error 会经 prism
// 报告外发到 Telegram，*url.Error 携带完整 URL）；所有 error 出口经 wrapErr 脱敏断链。
package tiingo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
)

const (
	defaultBaseURL = "https://api.tiingo.com"
	// topicDaily 的配额（40 次/时）登记在 policy 内置表，本包不持有节流状态。
	topicDaily = "tiingo.daily"
	// maxBody 防御性上限：不带 endDate 的全量日线约 60 字节/行，30 年 ≈ 0.5MB。
	maxBody = 32 << 20
)

type Client struct {
	apiKey  string
	baseURL string
	hc      *http.Client
	gate    *policy.Gate
}

// New 指向生产端点。
func New(apiKey string) *Client { return NewWithBaseURL(apiKey, defaultBaseURL) }

// NewWithBaseURL 允许注入自定义端点（测试用 httptest server）。gate 在构造时快照
// policy.Default()：测试里的 policy.SetDefault 必须发生在本函数之前才生效。
func NewWithBaseURL(apiKey, baseURL string) *Client {
	// Proxy=nil 强制直连：经本机代理 127.0.0.1:7897 访问 api.tiingo.com 实测 TLS 握手
	// 必然失败，而部分 launchd 任务设置了进程级 https_proxy（设计 §2.1）。
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second, Transport: tr},
		gate:    policy.Default(),
	}
}

// wrapErr 是本包唯一的 error 出口：加前缀、抹掉 apiKey、用 %v 断链
// （留链则 errors.Unwrap 可取回未脱敏原文）。
func (c *Client) wrapErr(format string, args ...any) error {
	return fmt.Errorf("tiingo: %s", c.redact(fmt.Sprintf(format, args...)))
}

// redact 抹掉 apiKey。空 key 不替换。
func (c *Client) redact(s string) string {
	if c.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.apiKey, "<redacted>")
}

// mapPolicyErr 把 policy 哨兵错误换成本包的临时错误；必须在 policy.Fetch 返回处调用。
// 临时性绝不可映射成永久性：文本显式写 retryable。
func (c *Client) mapPolicyErr(err error) error {
	if errors.Is(err, policy.ErrTimeout) || errors.Is(err, policy.ErrQuotaExceeded) {
		return c.wrapErr("temporary gate failure (retryable): %v", err)
	}
	return err
}

// FetchHistory 经 Gate 拉取 [start, end] 闭区间（按日）的日线，已折算为 yahoo close 口径。
// 返回值 slices.Clone：Gate 命中缓存时不复制，多个调用方共享底层数组。
func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	key := fmt.Sprintf("%s|%s|%s", symbol, start.Format("2006-01-02"), end.Format("2006-01-02"))
	out, err := policy.Fetch(c.gate, topicDaily, key, func() ([]core.OHLCV, error) {
		return c.fetchHistory(symbol, start, end)
	})
	if err != nil {
		return nil, c.mapPolicyErr(err)
	}
	return slices.Clone(out), nil
}

func (c *Client) fetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	// 刻意不带 endDate：end 之后的拆股必须可见，否则折算漏因子（设计 §2.4）。
	q := url.Values{"startDate": {start.Format("2006-01-02")}}
	u := fmt.Sprintf("%s/tiingo/daily/%s/prices?%s", c.baseURL, url.PathEscape(toTicker(symbol)), q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, c.wrapErr("%s: build request: %v", symbol, err)
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.wrapErr("%s: %v", symbol, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, c.wrapErr("%s: read body: %v", symbol, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.statusErr(symbol, resp.StatusCode, raw)
	}
	var rows []priceRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, c.wrapErr("%s: decode: %v", symbol, err)
	}
	out, err := normalize(symbol, rows, start, end)
	if err != nil {
		return nil, c.wrapErr("%v", err)
	}
	return out, nil
}

// statusErr 按运维动作分类：404 标的不存在；400/401/403 是配置/权限问题，重试无意义；
// 429 可重试。detail 优先取 Tiingo 的 {"detail": "..."}，否则取截断的 body。
func (c *Client) statusErr(symbol string, code int, body []byte) error {
	var e struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &e)
	detail := e.Detail
	if detail == "" {
		// 先脱敏再截断：先截断的话，横跨截断点的 key 只剩前缀，wrapErr 再也匹配不到。
		detail = c.redact(string(body))
		detail = strings.ToValidUTF8(detail[:min(len(detail), 200)], "")
	}
	switch code {
	case http.StatusNotFound:
		return c.wrapErr("%s: not found: %s", symbol, detail)
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return c.wrapErr("%s: permission/config error (not retryable): HTTP %d: %s", symbol, code, detail)
	case http.StatusTooManyRequests:
		return c.wrapErr("%s: rate limited (retryable): %s", symbol, detail)
	default:
		return c.wrapErr("%s: HTTP %d: %s", symbol, code, detail)
	}
}
