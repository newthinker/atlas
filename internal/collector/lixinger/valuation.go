package lixinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/newthinker/atlas/internal/collector"
)

// usHKIndexCodes maps phase-1 international index symbols to Lixinger codes.
// Candidate values — verify against the basic-info/samples API on first
// implementation day and freeze (design §2.4).
var usHKIndexCodes = map[string]struct{ endpoint, code string }{
	"^GSPC": {"us/index/fundamental", "SPX"},
	"^IXIC": {"us/index/fundamental", "COMP"},
	"^DJI":  {"us/index/fundamental", "DJI"},
	"^HSI":  {"hk/index/fundamental", "HSI"},
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
		// 0700.HK → 00700（理杏仁港股 5 位代码）
		c := fmt.Sprintf("%05s", strings.TrimSuffix(symbol, ".HK"))
		return "hk/company/fundamental", c
	case strings.HasSuffix(symbol, "=F"), strings.Contains(symbol, "-USD"):
		return "", ""
	default: // 美股个股
		return "us/company/fundamental", symbol
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
// a stock or index, using Lixinger's cvpos metric. lookbackYears maps to the
// closest supported granularity (y3/y5/y10). It returns (-1, error) for any
// unsupported symbol, transport/HTTP failure, business error code, or missing
// metric — callers degrade to "percentile unavailable" (design §5).
func (l *Lixinger) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	endpoint, code := endpointFor(symbol)
	if endpoint == "" {
		return -1, fmt.Errorf("lixinger: valuation percentile unsupported for %s", symbol)
	}
	gran := lookbackGranularity(lookbackYears)
	metric := fmt.Sprintf("pe_ttm.%s.cvpos", gran)

	url := fmt.Sprintf("%s/%s", l.baseURL, endpoint)
	payload := map[string]any{
		"token":       l.apiKey,
		"date":        "latest",
		"stockCodes":  []string{code},
		"metricsList": []string{metric},
	}

	raw, err := l.postJSONRaw(url, payload)
	if err != nil {
		return -1, err
	}

	var result struct {
		Code    int              `json:"code"`
		Message string           `json:"message"`
		Data    []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return -1, fmt.Errorf("lixinger: decode valuation response: %w", err)
	}
	if result.Code != 0 {
		return -1, fmt.Errorf("lixinger: API error %d: %s", result.Code, result.Message)
	}
	if len(result.Data) == 0 {
		return -1, fmt.Errorf("lixinger: no valuation data for %s", symbol)
	}

	cvpos, ok := digFloat(result.Data[0], "pe_ttm", gran, "cvpos")
	if !ok {
		return -1, fmt.Errorf("lixinger: metric %s missing for %s", metric, symbol)
	}
	return cvpos * 100, nil
}

// digFloat walks a nested map[string]any along path and returns the terminal
// value as a float64, reporting false if any segment is absent or mistyped.
func digFloat(m map[string]any, path ...string) (float64, bool) {
	cur := any(m)
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur, ok = obj[key]
		if !ok {
			return 0, false
		}
	}
	f, ok := cur.(float64)
	return f, ok
}

// postJSONRaw posts payload as JSON and returns the raw response body. Unlike
// postJSON it does not decode into the flat lixingerResponse, so callers can
// parse nested metric trees (e.g. pe_ttm.y5.cvpos). The HTTP status guard
// mirrors postJSON: a non-200 response is an error regardless of body content.
func (l *Lixinger) postJSONRaw(url string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lixinger: unexpected HTTP status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
