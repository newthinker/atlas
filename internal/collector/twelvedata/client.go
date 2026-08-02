// Package twelvedata 直连 Twelve Data 的 time_series HTTP API(GET,apikey 鉴权)。
// 角色 = 美股价格备源:yahoo 价格失败时的下一跳(spec §2 降级链)。
// 免费层 8 req/min,故客户端内置 8s 最小间隔节流;apikey 只入 runtime
// configs/config.yaml(gitignored),不入仓不入日志。
package twelvedata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
		"apikey":     {c.apiKey},
	}
	resp, err := c.hc.Get(c.baseURL + "/time_series?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("twelvedata: %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("twelvedata: %s: read body: %w", symbol, err)
	}

	var env timeSeriesResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("twelvedata: %s: decode: %w", symbol, err)
	}
	if env.Status == "error" {
		return nil, fmt.Errorf("twelvedata: %s: code %d: %s", symbol, env.Code, env.Message)
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
