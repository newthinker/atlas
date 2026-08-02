// Package baostock 经本地 Python 侧车桥(scripts/baostock/bridge.py)取 A 股日线,
// 作为 A 股行情降级链的第三跳(spec §2: yahoo → akshare → baostock)。
// 桥仅绑 127.0.0.1:8181,复权口径 adjustflag=3(不复权,与 PE 计算一致)。
package baostock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/newthinker/atlas/internal/core"
)

// Client 是 baostock 本地侧车桥的 HTTP 客户端。
type Client struct {
	baseURL string
	hc      *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 60 * time.Second}, // 全历史响应较大
	}
}

// bridgeCode 把 Atlas 形态 "600519.SH" 转成 baostock 形态 "sh.600519";
// 后缀非字母(已是 "sh.600519" 等桥形态)时原样透传。
func bridgeCode(symbol string) string {
	code, ex, ok := strings.Cut(symbol, ".")
	if !ok || ex == "" || strings.ContainsFunc(ex, func(r rune) bool { return !unicode.IsLetter(r) }) {
		return symbol
	}
	return strings.ToLower(ex) + "." + code
}

// FetchDaily 取 [start,end] 的日线收盘序列(桥已按日期升序返回,close 为空的行由桥跳过)。
func (c *Client) FetchDaily(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	q := url.Values{
		"code":  {bridgeCode(symbol)},
		"start": {start.Format("2006-01-02")},
		"end":   {end.Format("2006-01-02")},
	}
	resp, err := c.hc.Get(c.baseURL + "/daily?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("baostock: daily %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("baostock: daily %s: HTTP %d: %s", symbol, resp.StatusCode, string(body))
	}
	var rows []struct {
		Date  string  `json:"date"`
		Close float64 `json:"close"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("baostock: daily %s: decode: %w", symbol, err)
	}
	out := make([]core.OHLCV, 0, len(rows))
	for _, r := range rows {
		d, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			continue
		}
		out = append(out, core.OHLCV{
			Symbol: symbol, Interval: "1d", Close: r.Close, Time: d,
		})
	}
	return out, nil
}
