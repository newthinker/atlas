package lixinger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// candlestickBar is one row of the cn/company/candlestick response. Dates are
// RFC3339 and rows arrive newest-first.
type candlestickBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// fetchCandlestick is the shared call behind FetchHistory and FetchQuote.
func (l *Lixinger) fetchCandlestick(symbol string, start, end time.Time) ([]candlestickBar, error) {
	if err := l.requireKey(); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"token":     l.apiKey,
		"stockCode": l.toLixingerSymbol(symbol), // 单数，复数会 404
		"type":      "fc_rights",                // 标准前复权，与 eastmoney 一致
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
	}
	raw, err := l.request("cn/company/candlestick", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []candlestickBar `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode candlestick: %w", err)
	}
	return result.Data, nil
}

// FetchHistory fetches forward-adjusted daily OHLCV from cn/company/candlestick.
func (l *Lixinger) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	bars, err := l.fetchCandlestick(symbol, start, end)
	if err != nil {
		return nil, err
	}
	data := make([]core.OHLCV, 0, len(bars))
	for _, b := range bars {
		t, err := time.Parse(time.RFC3339, b.Date)
		if err != nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     b.Open,
			High:     b.High,
			Low:      b.Low,
			Close:    b.Close,
			Volume:   int64(b.Volume),
			Time:     t,
		})
	}
	return data, nil
}

// FetchQuote approximates a quote from the latest candlestick bar. Lixinger has
// no real-time quote API, so this is delayed data (Source "lixinger-delayed").
func (l *Lixinger) FetchQuote(symbol string) (*core.Quote, error) {
	start, end := recentWindow()
	bars, err := l.fetchCandlestick(symbol, start, end)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("lixinger: no candlestick data for %s", symbol)
	}
	latest := bars[0] // newest-first
	q := &core.Quote{
		Symbol: symbol,
		Market: core.MarketCNA,
		Price:  latest.Close,
		Open:   latest.Open,
		High:   latest.High,
		Low:    latest.Low,
		Volume: int64(latest.Volume),
		Time:   time.Now(),
		Source: "lixinger-delayed",
	}
	if len(bars) > 1 {
		setChangeFromPrevClose(q, bars[1].Close)
	}
	return q, nil
}
