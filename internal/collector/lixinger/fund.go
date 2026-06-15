package lixinger

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

type fundNetValue struct {
	Date     string  `json:"date"`
	NetValue float64 `json:"netValue"`
}

// fetchNetValues pulls the unit-NAV series (newest-first) from cn/fund/net-value.
func (l *Lixinger) fetchNetValues(code string, start, end time.Time) ([]fundNetValue, error) {
	payload := map[string]any{
		"token":     l.apiKey,
		"stockCode": code, // 单数
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
	}
	raw, err := l.request("cn/fund/net-value", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []fundNetValue `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode net-value: %w", err)
	}
	return result.Data, nil
}

// FetchFundHistory fetches historical unit NAV. OHLC are all set to the NAV.
func (l *Lixinger) FetchFundHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	if err := l.requireKey(); err != nil {
		return nil, err
	}
	rows, err := l.fetchNetValues(l.toLixingerSymbol(symbol), start, end)
	if err != nil {
		return nil, err
	}
	data := make([]core.OHLCV, 0, len(rows))
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339, r.Date)
		if err != nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: "1d",
			Open:     r.NetValue,
			High:     r.NetValue,
			Low:      r.NetValue,
			Close:    r.NetValue,
			Volume:   0,
			Time:     t,
		})
	}
	// Lixinger returns newest-first; reverse to chronological (oldest-first) to
	// match the eastmoney FetchHistory contract the backtest replay assumes.
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	return data, nil
}

// FetchFundQuote fetches the latest fund NAV plus aggregated metadata.
func (l *Lixinger) FetchFundQuote(symbol string) (*core.Quote, error) {
	if err := l.requireKey(); err != nil {
		return nil, err
	}
	code := l.toLixingerSymbol(symbol)
	start, end := recentWindow()
	rows, err := l.fetchNetValues(code, start, end)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("lixinger: no fund nav for %s", symbol)
	}
	latest := rows[0]
	q := &core.Quote{
		Symbol:   symbol,
		Market:   core.MarketCNA,
		Price:    latest.NetValue,
		Time:     time.Now(),
		Source:   "lixinger",
		FundInfo: l.fetchFundInfo(code),
	}
	if len(rows) > 1 {
		setChangeFromPrevClose(q, rows[1].NetValue)
	}
	return q, nil
}

// FetchFundInfoPublic exposes fetchFundInfo for the eastmoney fallback.
func (l *Lixinger) FetchFundInfoPublic(code string) *core.FundInfo {
	return l.fetchFundInfo(code)
}

// fetchFundInfo aggregates profile + manager + drawdown + latest NAV. Each
// sub-fetch is best-effort: a failure leaves its fields empty, never fatal.
// Returns nil only if the core profile call fails.
func (l *Lixinger) fetchFundInfo(code string) *core.FundInfo {
	if l.requireKey() != nil {
		return nil
	}
	info := &core.FundInfo{}

	// profile：名称、公司、成立日、运作方式
	if raw, err := l.request("cn/fund/profile", map[string]any{
		"token": l.apiKey, "stockCodes": []string{code},
	}); err == nil {
		var res struct {
			Data []struct {
				ETShortName   string `json:"e_t_short_name"` // 基金简称(真名)，c_name 实为托管行
				FCName        string `json:"f_c_name"`
				InceptionDate string `json:"inception_date"`
				OpMode        string `json:"op_mode"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			d := res.Data[0]
			info.Name = d.ETShortName
			info.ManagementCompany = d.FCName
			info.FundType = d.OpMode
			if t, err := time.Parse(time.RFC3339, d.InceptionDate); err == nil {
				info.InceptionDate = t
			}
		}
	} else {
		return nil // 核心概况都拿不到则视为无信息
	}

	// manager：现任（无 departureDate）经理
	if raw, err := l.request("cn/fund/manager", map[string]any{
		"token": l.apiKey, "stockCodes": []string{code},
	}); err == nil {
		var res struct {
			Data []struct {
				Managers []struct {
					Name          string `json:"name"`
					DepartureDate string `json:"departureDate"`
				} `json:"managers"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			for _, m := range res.Data[0].Managers {
				if m.DepartureDate == "" { // 在任
					info.Manager = m.Name
					break
				}
			}
		}
	}

	// drawdown：最深回撤（最小 value）
	if raw, err := l.request("cn/fund/drawdown", map[string]any{
		"token": l.apiKey, "stockCode": code, "granularity": "y1",
		"startDate": time.Now().AddDate(-1, 0, 0).Format("2006-01-02"),
		"endDate":   time.Now().Format("2006-01-02"),
	}); err == nil {
		var res struct {
			Data []struct {
				Value float64 `json:"value"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			min := math.Inf(1)
			for _, d := range res.Data {
				if d.Value < min {
					min = d.Value
				}
			}
			if !math.IsInf(min, 1) {
				info.MaxDrawdown = min * 100 // drawdown value 是原始比率(-0.3369)，归一为百分数(-33.69)
			}
		}
	}

	// latest NAV
	navStart, navEnd := recentWindow()
	if rows, err := l.fetchNetValues(code, navStart, navEnd); err == nil && len(rows) > 0 {
		info.LatestNAV = rows[0].NetValue
		info.NAVDate = rows[0].Date
	}

	return info
}
