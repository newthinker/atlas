package lixinger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// FetchFundamental fetches latest valuation metrics for an A-share stock from
// cn/company/fundamental/non_financial. Note: ROE is not a valid price metric
// on this endpoint, so core.Fundamental.ROE is left zero.
func (l *Lixinger) FetchFundamental(symbol string) (*core.Fundamental, error) {
	if err := l.requireKey(); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"token":       l.apiKey,
		"date":        "latest",
		"stockCodes":  []string{l.toLixingerSymbol(symbol)},
		"metricsList": []string{"pe_ttm", "pb", "ps_ttm", "dyr", "mc"},
	}
	raw, err := l.request("cn/company/fundamental/non_financial", payload)
	if err != nil {
		return nil, fmt.Errorf("lixinger: fetch fundamental: %w", err)
	}
	var result struct {
		Data []struct {
			PETTM float64 `json:"pe_ttm"`
			PB    float64 `json:"pb"`
			PSTTM float64 `json:"ps_ttm"`
			DYR   float64 `json:"dyr"`
			MC    float64 `json:"mc"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode fundamental: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no data for symbol %s", symbol)
	}
	d := result.Data[0]
	return &core.Fundamental{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Date:          time.Now(),
		PE:            d.PETTM,
		PB:            d.PB,
		PS:            d.PSTTM,
		DividendYield: d.DYR * 100, // dyr 是原始比率(0.0403)，归一为百分数(4.03) 供策略消费
		MarketCap:     d.MC,
		Source:        "lixinger",
	}, nil
}

// FetchFundamentalHistory returns the latest fundamental as a single-element
// slice (the endpoint exposes point-in-time valuation, not a true series).
func (l *Lixinger) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	f, err := l.FetchFundamental(symbol)
	if err != nil {
		return nil, err
	}
	return []core.Fundamental{*f}, nil
}
