// Package twelvedata 直连 Twelve Data 的 time_series HTTP API(GET,apikey 鉴权)。
// 角色 = 美股价格备源:yahoo 价格失败时的下一跳(spec §2 降级链)。
// 免费层 8 req/min,故客户端内置 8s 最小间隔节流;apikey 只入 runtime
// configs/config.yaml(gitignored),不入仓不入日志。
//
// 凭证一律走 Authorization 请求头,**绝不进 URL query**(ADR#7)。原因是本包的
// error 会经 prism Report.Failed 一路外发到 Telegram 与 launchd 日志:传输层失败时
// Go 返回的 *url.Error 携带完整 URL(net/url 的 stripPassword 只去 userinfo 密码、
// 不去 query),query 里的 key 就此明文外泄且 Telegram 侧不可撤回。TD 是备源,
// 只在主源已失败时调用,面对的正是网络抖动/代理故障这类传输层错误,触发面真实。
// key 进 URL 还会落进正向代理的 access log。两道防线:凭证不进 URL(根因),
// 且所有 error 出口经 redact 兜底(见 wrapErr)。
package twelvedata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

const (
	defaultBaseURL = "https://api.twelvedata.com"
	// defaultMinInterval 对应免费层 8 req/min;字段可在测试中覆盖以缩短用例耗时。
	defaultMinInterval = 8 * time.Second
	// outputSize 是单次请求的最大返回条数(TD 上限 5000),覆盖 10Y 日线。
	outputSize = "5000"
)

type Client struct {
	apiKey  string
	baseURL string
	hc      *http.Client

	mu          sync.Mutex
	lastReq     time.Time
	minInterval time.Duration
}

// New 指向生产端点。
func New(apiKey string) *Client { return NewWithBaseURL(apiKey, defaultBaseURL) }

// NewWithBaseURL 允许注入自定义端点(测试用 httptest server)。
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:      apiKey,
		baseURL:     baseURL,
		hc:          &http.Client{Timeout: 30 * time.Second},
		minInterval: defaultMinInterval,
	}
}

// wrapErr 是本包**唯一**的 error 出口:统一加前缀,并把 apiKey 从最终文本里抹掉。
// 凭证已不进 URL,这层是兜底——error 文本的来源(*url.Error、传输层、第三方库)
// 无法穷举,而这些 error 会外发到 Telegram。故不做类型判别,一律对成文文本脱敏。
// 空 key 时不替换:strings.ReplaceAll(s, "", x) 会在每个字符间插入 x。
//
// 刻意**不保留 error 链**(调用点用 %v 而非 %w):留了链,errors.Unwrap 就能取回
// 未脱敏的原始 error,脱敏形同虚设。本包无 sentinel error,消费方(prism)只格式化
// 文本,不做 errors.Is/As,故无损失。
func (c *Client) wrapErr(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if c.apiKey != "" {
		msg = strings.ReplaceAll(msg, c.apiKey, "<redacted>")
	}
	return fmt.Errorf("twelvedata: %s", msg)
}

// throttle 阻塞到距上次请求满 minInterval 为止。
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.minInterval - time.Since(c.lastReq); wait > 0 {
		time.Sleep(wait)
	}
	c.lastReq = time.Now()
}

// timeSeriesResponse 只取本包需要的字段;数值在 TD 响应中一律是字符串。
type timeSeriesResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    int    `json:"code"`
	Values  []struct {
		Datetime string `json:"datetime"`
		Close    string `json:"close"`
	} `json:"values"`
}

// FetchHistory 拉取 [start, end] 闭区间的日线,返回按 Time 升序的 OHLCV
// (仅 Symbol/Interval/Time/Close 填充——降级链只消费收盘价)。
// 单行 datetime 或 close 不可解析时跳过该行,不中断整段解析:TD 停牌/缺数据
// 会给出空串或 "null",丢一天好过丢整段。
//
// TD 的 end_date 是**排他**的(2026-08-02 实测 NVDA:end_date=07-31 拿不到
// 07-31,=08-01 才拿到),故此处发 end+1 天以兑现闭区间契约——否则作为 yahoo
// 价格备源时会静默丢掉最新一根收盘价。start_date 是包含的,不作补偿。
func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	c.throttle()

	q := url.Values{
		"symbol":     {symbol},
		"interval":   {"1day"},
		"start_date": {start.Format("2006-01-02")},
		"end_date":   {end.AddDate(0, 0, 1).Format("2006-01-02")},
		"outputsize": {outputSize},
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/time_series?"+q.Encode(), nil)
	if err != nil {
		return nil, c.wrapErr("%s: build request: %v", symbol, err)
	}
	// 2026-08-02 实测:Authorization 头可用(无凭证与 X-TD-APIKEY 均 401)。
	req.Header.Set("Authorization", "apikey "+c.apiKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.wrapErr("%s: %v", symbol, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, c.wrapErr("%s: read body: %v", symbol, err)
	}

	var env timeSeriesResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, c.wrapErr("%s: decode: %v", symbol, err)
	}
	// 凭证无效时 TD 回 HTTP 401,但 body 仍是同一个 JSON 错误信封
	// (2026-08-02 实测),故仍由这一条分支报出,无需按状态码分路。
	if env.Status == "error" {
		return nil, c.wrapErr("%s: code %d: %s", symbol, env.Code, env.Message)
	}

	out := make([]core.OHLCV, 0, len(env.Values))
	for _, v := range env.Values {
		d, err := time.Parse("2006-01-02", v.Datetime)
		if err != nil {
			continue
		}
		px, err := strconv.ParseFloat(v.Close, 64)
		if err != nil {
			continue
		}
		out = append(out, core.OHLCV{Symbol: symbol, Interval: "1d", Time: d, Close: px})
	}
	// values 为倒序(最新在前),统一转升序
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}
