package akshare

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// mapAStockSymbol: "600519.SH"/"000001.SZ" → "600519"(乐咕接口用裸六位代码)。
func mapAStockSymbol(symbol string) string {
	if i := strings.IndexByte(symbol, '.'); i > 0 {
		return symbol[:i]
	}
	return symbol
}

// FetchStockValuationSeries 返回个股 [start,end] 日频估值时序,按 market 分派数据源。
func (c *Client) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]ValuationPoint, error) {
	switch market {
	case "CN_A":
		return c.fetchAStock(symbol, start, end)
	case "HK":
		return c.fetchHKStock(symbol, start, end) // Task 3 实现
	default:
		return nil, fmt.Errorf("akshare: unsupported market %q for %s", market, symbol)
	}
}

// fetchAStock: 乐咕 stock_a_indicator_lg 返回全历史,客户端过滤窗口。
// ⚠ 字段键 live 校验点: trade_date/pe_ttm/pb/ps_ttm(ps 兜底)。
func (c *Client) fetchAStock(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	rows, err := c.get("stock_a_indicator_lg", url.Values{"symbol": {mapAStockSymbol(symbol)}})
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(rows))
	for _, row := range rows {
		d, err := fdate(row, "trade_date", "date")
		if err != nil {
			return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
		}
		pts = append(pts, ValuationPoint{
			Date:  d,
			PETTM: fnum(row, "pe_ttm"),
			PB:    fnum(row, "pb"),
			PSTTM: fnum(row, "ps_ttm", "ps"),
		})
	}
	if err := guardSchemaDrift(pts, "stock_a_indicator_lg"); err != nil {
		return nil, err
	}
	return clipSort(pts, start, end), nil
}

// mapHKSymbol: "0700.HK" → "00700"(百度接口用五位代码)。
func mapHKSymbol(symbol string) string {
	code := symbol
	if i := strings.IndexByte(symbol, '.'); i > 0 {
		code = symbol[:i]
	}
	for len(code) < 5 {
		code = "0" + code
	}
	return code
}

// fetchHKStock: 百度股市通 stock_hk_valuation_baidu 每次一个指标,两次调用按日期合并。
// 合并以 PE 日期为主键: 有 PE 缺 PB 的日 PB 记 NaN;仅有 PB 无 PE 的日不产出行
// (无 PE 无法算分位)。PSTTM 恒 NaN。
// ⚠ live 校验点: 接口名/indicator 取值(市盈率(TTM)/市净率)/period=全部/date+value 字段键。
func (c *Client) fetchHKStock(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	code := mapHKSymbol(symbol)
	fetch := func(indicator string) (map[string]float64, error) {
		rows, err := c.get("stock_hk_valuation_baidu",
			url.Values{"symbol": {code}, "indicator": {indicator}, "period": {"全部"}})
		if err != nil {
			return nil, err
		}
		m := make(map[string]float64, len(rows))
		for _, row := range rows {
			d, err := fdate(row, "date")
			if err != nil {
				return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
			}
			m[d.Format("2006-01-02")] = fnum(row, "value")
		}
		return m, nil
	}
	pe, err := fetch("市盈率(TTM)")
	if err != nil {
		return nil, err
	}
	pb, err := fetch("市净率")
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(pe))
	for ds, v := range pe {
		d, _ := time.Parse("2006-01-02", ds)
		p := ValuationPoint{Date: d, PETTM: v, PB: math.NaN(), PSTTM: math.NaN()}
		if b, ok := pb[ds]; ok {
			p.PB = b
		}
		pts = append(pts, p)
	}
	if err := guardSchemaDrift(pts, "stock_hk_valuation_baidu"); err != nil {
		return nil, err
	}
	return clipSort(pts, start, end), nil
}
