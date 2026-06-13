package lixinger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/newthinker/atlas/internal/collector"
)

// usHKIndexCodes maps supported international index symbols to Lixinger codes.
// Frozen against the live us/index & hk/index listing APIs (2026-06-13): the
// Lixinger open API US-index universe contains only the S&P 500 (.INX) — Dow
// Jones (^DJI) and Nasdaq Composite (^IXIC) are NOT covered, so they are
// intentionally absent and degrade to "percentile unavailable".
var usHKIndexCodes = map[string]struct{ endpoint, code string }{
	"^GSPC": {"us/index/fundamental", ".INX"}, // 标普500；live 验证（SPX 返回空，.INX 有数据）
	"^HSI":  {"hk/index/fundamental", "HSI"},  // 恒生指数；live 验证
}

// endpointFor returns the Lixinger fundamental endpoint (relative path) and the
// Lixinger security code for a watchlist symbol. An empty endpoint means a
// valuation percentile is not available for this symbol class (commodities,
// crypto, or indexes outside the phase-1 list).
func endpointFor(symbol string) (endpoint, code string) {
	switch {
	case collector.IsAShareIndex(symbol):
		return "cn/index/fundamental", strings.SplitN(symbol, ".", 2)[0]
	case strings.HasSuffix(symbol, ".SH"), strings.HasSuffix(symbol, ".SZ"):
		// 金融股（银行/券商/保险）走 non_financial 会失败 → 调用方按不可用降级（一期边界）
		return "cn/company/fundamental/non_financial", strings.SplitN(symbol, ".", 2)[0]
	case strings.HasPrefix(symbol, "^"):
		if m, ok := usHKIndexCodes[symbol]; ok {
			return m.endpoint, m.code
		}
		return "", ""
	case strings.HasSuffix(symbol, ".HK"):
		// 0700.HK → 00700（理杏仁港股 5 位代码）。live: hk/company/fundamental 404，
		// 正确端点为 hk/company/fundamental/non_financial。
		c := fmt.Sprintf("%05s", strings.TrimSuffix(symbol, ".HK"))
		return "hk/company/fundamental/non_financial", c
	case strings.HasSuffix(symbol, "=F"), strings.Contains(symbol, "-USD"):
		return "", ""
	default: // 美股个股：理杏仁开放 API 无个股基本面端点(404) → 降级，无端点
		return "", ""
	}
}

// lookbackGranularity maps a lookback in years to Lixinger's nearest supported
// cvpos window (y3/y5/y10).
func lookbackGranularity(lookbackYears int) string {
	switch {
	case lookbackYears <= 3:
		return "y3"
	case lookbackYears <= 5:
		return "y5"
	default:
		return "y10"
	}
}

// FetchValuationPercentile returns the PE-TTM historical percentile (0-100) for
// a stock or index via Lixinger's cvpos metric. Index endpoints require the
// market-cap-weighted (.mcw) variant. The metric string doubles as the flat
// response key (e.g. "pe_ttm.y5.cvpos"). Returns (-1, error) for unsupported
// symbols or any failure — callers degrade to "percentile unavailable".
func (l *Lixinger) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	endpoint, code := endpointFor(symbol)
	if endpoint == "" {
		return -1, fmt.Errorf("lixinger: valuation percentile unsupported for %s", symbol)
	}
	gran := lookbackGranularity(lookbackYears)

	metric := fmt.Sprintf("pe_ttm.%s.cvpos", gran)
	if strings.Contains(endpoint, "/index/") {
		metric = fmt.Sprintf("pe_ttm.%s.mcw.cvpos", gran) // 指数为市值加权
	}

	payload := map[string]any{
		"token":       l.apiKey,
		"date":        "latest",
		"stockCodes":  []string{code},
		"metricsList": []string{metric},
	}
	raw, err := l.request(endpoint, payload)
	if err != nil {
		return -1, err
	}

	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return -1, fmt.Errorf("lixinger: decode valuation response: %w", err)
	}
	if len(result.Data) == 0 {
		return -1, fmt.Errorf("lixinger: no valuation data for %s", symbol)
	}

	v, ok := result.Data[0][metric].(float64) // 扁平 dotted key
	if !ok {
		return -1, fmt.Errorf("lixinger: metric %s missing for %s", metric, symbol)
	}
	return v * 100, nil
}
