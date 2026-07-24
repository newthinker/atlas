package akshare

import (
	"fmt"
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
	return clipSort(pts, start, end), nil
}

// fetchHKStock 由 Task 3 实现;当前为 stub,保证本 Task 可编译且 US market 用例可过。
func (c *Client) fetchHKStock(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	return nil, fmt.Errorf("akshare: hk not implemented yet")
}
