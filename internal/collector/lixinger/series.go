package lixinger

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ValuationPoint is one day of Lixinger valuation metrics. Missing metrics are
// NaN; cvpos percentiles are converted to 0-100.
type ValuationPoint struct {
	Date                              time.Time
	PETTM, PB, PSTTM, Pctl5Y, Pctl10Y float64
}

// seriesMetrics returns the flat metric keys for the endpoint class. Index
// endpoints require the market-cap-weighted (.mcw) variant, mirroring
// FetchValuationPercentile.
//
// ⚠ live 校验点:指数原始估值指标名(pe_ttm.mcw / pb.mcw / ps_ttm.mcw)按现有
// cvpos 命名规则(见 valuation.go:74-77)外推,未经 live 验证。Task 7 首次真实
// 运行时若报 metric missing,以真实响应修正此处常量并同步 series_test.go。
func seriesMetrics(endpoint string) (pe, pb, ps, cv5, cv10 string) {
	if strings.Contains(endpoint, "/index/") {
		return "pe_ttm.mcw", "pb.mcw", "ps_ttm.mcw",
			"pe_ttm.y5.mcw.cvpos", "pe_ttm.y10.mcw.cvpos"
	}
	return "pe_ttm", "pb", "ps_ttm", "pe_ttm.y5.cvpos", "pe_ttm.y10.cvpos"
}

// parseSeriesDate accepts both ISO8601 timestamps and bare YYYY-MM-DD dates.
func parseSeriesDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// metricAt reads a flat dotted metric key, scaling it (cvpos ×100); a missing
// or non-numeric value degrades to NaN.
func metricAt(row map[string]any, key string, scale float64) float64 {
	if v, ok := row[key].(float64); ok {
		return v * scale
	}
	return math.NaN()
}

// FetchValuationSeries returns the daily valuation series of symbol within
// [start, end], ordered by ascending date. Callers must fetch incrementally
// (check the local latest date first) — Lixinger bills per request volume
// (理杏豆). Unsupported symbols return an error so callers degrade gracefully.
func (l *Lixinger) FetchValuationSeries(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	endpoint, code := endpointFor(symbol)
	if endpoint == "" {
		return nil, fmt.Errorf("lixinger: valuation series unsupported for %s", symbol)
	}
	pe, pb, ps, cv5, cv10 := seriesMetrics(endpoint)

	payload := map[string]any{
		"token":       l.apiKey,
		"startDate":   start.Format("2006-01-02"),
		"endDate":     end.Format("2006-01-02"),
		"stockCodes":  []string{code},
		"metricsList": []string{pe, pb, ps, cv5, cv10},
	}
	raw, err := l.request(endpoint, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode series response: %w", err)
	}

	pts := make([]ValuationPoint, 0, len(result.Data))
	for _, row := range result.Data {
		ds, _ := row["date"].(string)
		d, err := parseSeriesDate(ds)
		if err != nil {
			return nil, fmt.Errorf("lixinger: bad date %q for %s: %w", ds, symbol, err)
		}
		pts = append(pts, ValuationPoint{
			Date:    d,
			PETTM:   metricAt(row, pe, 1),
			PB:      metricAt(row, pb, 1),
			PSTTM:   metricAt(row, ps, 1),
			Pctl5Y:  metricAt(row, cv5, 100),
			Pctl10Y: metricAt(row, cv10, 100),
		})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })
	return pts, nil
}
