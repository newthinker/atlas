// Package tushare 直连 Tushare Pro HTTP API(POST JSON,token 鉴权,积分制)。
// 2026-08-02 实测(基础积分档):daily/index_daily/daily_basic/hk_daily ✅;
// us_daily/income ❌(40203)。角色 = A/H 估值+行情备源(spec §2)。
// token 只入 runtime configs/config.yaml(gitignored),不入仓不入日志。
package tushare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// token 走 POST body,必须 https:明文 http 会让凭证对路径上任意中间节点可见。
const defaultBaseURL = "https://api.tushare.pro"

// code 40203 是**重载码**:无权限与限频共用它,只能靠 msg 区分。两者的处置相反,
// 故拆成两个哨兵错误——分错不会走错执行路径(该跳本就只调一次),但会让降级链的
// Degraded 文案把「等一会儿就好」写成「去改配置」,误导运维。
var (
	// ErrNoPermission:积分/权限不足——**永久性**,改配置前重试无意义,
	// Degraded 文案应注明配置性问题(降级链原则,spec §2)。
	ErrNoPermission = errors.New("tushare: no api permission")
	// ErrRateLimited:访问频率超限——**临时性**,窗口过后自愈,
	// 调用方可按可重试/下一跳处理,不应提示改配置。
	ErrRateLimited = errors.New("tushare: rate limited")
)

// rateLimitMarker 是 tushare 限频 msg 的判别子串(实测原文形如
// 「抱歉,您访问接口(daily_basic)频率超限(1次/分钟)」;窗口口径会自升级到 1 次/小时,
// 故只认「频率超限」四字,不匹配具体窗口)。
const rateLimitMarker = "频率超限"

type Client struct {
	token, baseURL string
	hc             *http.Client
	mu             sync.Mutex
	lastReq        time.Time
}

const minInterval = 200 * time.Millisecond // 基础档限频兜底

func New(token string) *Client { return NewWithBaseURL(token, defaultBaseURL) }
func NewWithBaseURL(token, baseURL string) *Client {
	return &Client{token: token, baseURL: baseURL, hc: &http.Client{Timeout: 60 * time.Second}}
}

type ValuationPoint struct {
	Date             time.Time
	PETTM, PB, PSTTM float64
}
type PricePoint struct {
	Date  time.Time
	Close float64
}

// row 是一行返回数据:date 取自 trade_date,values 按 fields 列名索引。
type row struct {
	date   time.Time
	values map[string]float64
}

// call POST 一次 api 并返回按 fields 列名索引的行(按日期升序)。
func (c *Client) call(apiName string, params map[string]string, fields string) ([]row, error) {
	c.mu.Lock()
	if wait := minInterval - time.Since(c.lastReq); wait > 0 {
		time.Sleep(wait)
	}
	c.lastReq = time.Now()
	c.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"api_name": apiName, "token": c.token, "params": params, "fields": fields,
	})
	resp, err := c.hc.Post(c.baseURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tushare: %s: %w", apiName, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tushare: %s: read body: %w", apiName, err)
	}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Fields []string `json:"fields"`
			Items  [][]any  `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("tushare: %s: decode: %w", apiName, err)
	}
	if env.Code == 40203 {
		// 限频分支优先:同码不同义,msg 是唯一判据。
		if strings.Contains(env.Msg, rateLimitMarker) {
			return nil, fmt.Errorf("%w: %s (%s)", ErrRateLimited, apiName, env.Msg)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrNoPermission, apiName, env.Msg)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("tushare: %s: code %d: %s", apiName, env.Code, env.Msg)
	}
	idx := map[string]int{}
	for i, f := range env.Data.Fields {
		idx[f] = i
	}
	di, ok := idx["trade_date"]
	if !ok {
		return nil, fmt.Errorf("tushare: %s: no trade_date field", apiName)
	}
	rows := make([]row, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		if di >= len(item) {
			continue
		}
		ds, _ := item[di].(string)
		d, err := time.Parse("20060102", ds)
		if err != nil {
			continue
		}
		values := map[string]float64{}
		for f, i := range idx {
			values[f] = math.NaN() // 非数值列(如 ts_code)与缺列一律 NaN
			if i < len(item) {
				if v, ok := item[i].(float64); ok {
					values[f] = v
				}
			}
		}
		rows = append(rows, row{date: d, values: values})
	}
	// items 为倒序,统一转升序
	sort.Slice(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })
	return rows, nil
}

func dateParams(symbol string, start, end time.Time) map[string]string {
	return map[string]string{"ts_code": symbol,
		"start_date": start.Format("20060102"), "end_date": end.Format("20060102")}
}

// FetchDailyBasic 返回 A 股每日估值指标(官方计算口径)。
func (c *Client) FetchDailyBasic(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	rows, err := c.call("daily_basic", dateParams(symbol, start, end),
		"ts_code,trade_date,pe_ttm,pb,ps_ttm")
	if err != nil {
		return nil, err
	}
	out := make([]ValuationPoint, len(rows))
	for i, r := range rows {
		out[i] = ValuationPoint{
			Date:  r.date,
			PETTM: r.values["pe_ttm"], PB: r.values["pb"], PSTTM: r.values["ps_ttm"],
		}
	}
	return out, nil
}

func (c *Client) fetchClose(apiName, symbol string, start, end time.Time) ([]PricePoint, error) {
	rows, err := c.call(apiName, dateParams(symbol, start, end), "ts_code,trade_date,close")
	if err != nil {
		return nil, err
	}
	out := make([]PricePoint, len(rows))
	for i, r := range rows {
		out[i] = PricePoint{Date: r.date, Close: r.values["close"]}
	}
	return out, nil
}

func (c *Client) FetchIndexDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("index_daily", symbol, start, end)
}
func (c *Client) FetchHKDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("hk_daily", symbol, start, end)
}
func (c *Client) FetchDaily(symbol string, start, end time.Time) ([]PricePoint, error) {
	return c.fetchClose("daily", symbol, start, end)
}
