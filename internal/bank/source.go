package bank

import (
	"encoding/json"
	"errors"
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

// 东方财富「主要指标」经 aktools 侧车调用。
// ⚠ live 校验点：字段名随上游变动，2026-09-26 对 600036.SH 实测（设计 §2）。
// 易混字段 LOAN_PROVISION_RATIO（拨贷比）、NEWCAPITALADER（资本充足率）、
// FIRST_ADEQUACY_RATIO（一级资本充足率）不得使用。
const (
	emAPI     = "stock_financial_analysis_indicator_em"
	emDateKey = "REPORT_DATE"
)

var emFieldKeys = [numIndicators]string{
	IndNPL:      "NONPERLOAN",
	IndCoverage: "BLDKBBL",
	IndCET1:     "HXYJBCZL",
}

type EMSource struct {
	baseURL string
	hc      *http.Client
}

func NewEMSource(baseURL string) *EMSource {
	return &EMSource{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *EMSource) Fetch(symbol string) (Series, error) {
	q := url.Values{"symbol": {symbol}, "indicator": {"按报告期"}}
	resp, err := s.hc.Get(s.baseURL + "/api/public/" + emAPI + "?" + q.Encode())
	if err != nil {
		return Series{}, fmt.Errorf("%s %s: %w", emAPI, symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		// 按字节截断可能切坏多字节字符，去掉残缺尾字节。
		return Series{}, fmt.Errorf("%s %s: HTTP %d: %s", emAPI, symbol, resp.StatusCode, strings.ToValidUTF8(string(body), ""))
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return Series{}, fmt.Errorf("%s %s: decode: %w", emAPI, symbol, err)
	}
	series, err := parseEMRows(rows)
	if err != nil {
		return Series{}, fmt.Errorf("%s %s: %w", emAPI, symbol, err)
	}
	return series, nil
}

func parseEMRows(rows []map[string]any) (Series, error) {
	if len(rows) == 0 {
		return Series{}, errors.New("无数据")
	}
	var s Series
	for _, row := range rows {
		ds, _ := row[emDateKey].(string)
		if len(ds) < 10 {
			continue
		}
		p, err := time.Parse("2006-01-02", ds[:10])
		if err != nil {
			continue
		}
		o := Observation{Period: p}
		for i, k := range emFieldKeys {
			o.Values[i] = toFloat(row[k])
		}
		s.Obs = append(s.Obs, o)
	}
	if len(s.Obs) == 0 {
		return Series{}, errors.New("无可解析报告期")
	}
	sort.Slice(s.Obs, func(i, j int) bool { return s.Obs[i].Period.Before(s.Obs[j].Period) })
	for _, k := range emFieldKeys {
		if !anyRowHas(rows, k) {
			s.MissingFields = append(s.MissingFields, k)
		}
	}
	return s, nil
}

// anyRowHas 只看键是否存在：值为 null 的列是「未披露」，不是数据源结构变化。
func anyRowHas(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

// toFloat 兼容 aktools 偶把数值序列化为字符串；null / 缺键 / 不可解析 / 非有限值 → NaN。
func toFloat(v any) float64 {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case string:
		var err error
		if f, err = strconv.ParseFloat(x, 64); err != nil {
			return math.NaN()
		}
	default:
		return math.NaN()
	}
	if math.IsInf(f, 0) {
		return math.NaN()
	}
	return f
}
