// Package akshare 经本地 aktools HTTP 侧车调用 AKShare 数据接口
// (设计: docs/superpowers/specs/2026-07-24-prism-akshare-source-design.md)。
// ⚠ 接口名与字段键为 live 校验点: AKShare 随上游变动是常态。
// 已于 2026-07-24 对 akshare 1.18.75 实测校正: 乐咕 stock_a_indicator_lg 已被移除,
// A 股/港股个股统一改走百度股市通 stock_zh/hk_valuation_baidu(见 stock.go),指数
// stock_index_pe_lg 仍可用(日频,键名 日期/滚动市盈率 不变)。后续若再不符,以 aktools
// 实际响应修正本包常量并同步测试(仿 M1 Task 3 mcw 处理惯例)。
package akshare

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ValuationPoint 是一天的估值指标,缺失字段为 NaN(store 写入时转 NULL)。
// akshare 无官方分位,分位由 prism 编排层本地计算。
type ValuationPoint struct {
	Date             time.Time
	PETTM, PB, PSTTM float64
}

// Client 是 aktools 本地侧车的 HTTP 客户端。
type Client struct {
	baseURL string
	hc      *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 60 * time.Second}, // lg 全历史响应较大
	}
}

// get 调用 aktools 的 /api/public/{api} 并解析 JSON 数组响应。
func (c *Client) get(api string, params url.Values) ([]map[string]any, error) {
	u := c.baseURL + "/api/public/" + api
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.hc.Get(u)
	if err != nil {
		return nil, fmt.Errorf("akshare: %s: %w", api, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("akshare: %s: HTTP %d: %s", api, resp.StatusCode, string(body))
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("akshare: %s: decode: %w", api, err)
	}
	return rows, nil
}

// fnum 取 row 中第一个存在且可解析为数值的键,均缺失返回 NaN。
// 兼容 aktools 偶把数值序列化为 JSON 字符串的情形(string 值经 strconv.ParseFloat)。
// ⚠ live 校验点: 传入的键名(pe_ttm/ps_ttm/ps 等)须与 aktools 实际响应一致。
func fnum(row map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch v := row[k].(type) {
		case float64:
			return v
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return math.NaN()
}

// guardSchemaDrift 侦测上游 schema 漂移: 拉取 >0 行但全部 PETTM 为 NaN,极可能是
// 字段键变动导致 PE 解析全落空。返回 error 让静默漂移变成 Report.Failed 告警,
// 而非产出一整段空 PE 的序列被下游当正常数据。
func guardSchemaDrift(pts []ValuationPoint, api string) error {
	if len(pts) == 0 {
		return nil
	}
	for _, p := range pts {
		if !math.IsNaN(p.PETTM) {
			return nil
		}
	}
	return fmt.Errorf("akshare: %s: fetched %d rows but all PE are NaN (probable schema drift)", api, len(pts))
}

// fdate 取 row 中第一个存在的日期键,支持 "2006-01-02" 与 ISO8601 前缀。
func fdate(row map[string]any, keys ...string) (time.Time, error) {
	for _, k := range keys {
		s, ok := row[k].(string)
		if !ok {
			continue
		}
		if len(s) >= 10 {
			if d, err := time.Parse("2006-01-02", s[:10]); err == nil {
				return d, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("akshare: no parsable date in row (keys tried: %v)", keys)
}

// clipSort 按 [start,end] 闭区间过滤并升序排序。
func clipSort(pts []ValuationPoint, start, end time.Time) []ValuationPoint {
	out := pts[:0]
	s, e := start.Format("2006-01-02"), end.Format("2006-01-02")
	for _, p := range pts {
		d := p.Date.Format("2006-01-02")
		if d >= s && d <= e {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
