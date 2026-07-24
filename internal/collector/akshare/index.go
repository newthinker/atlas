package akshare

import (
	"fmt"
	"math"
	"net/url"
	"time"
)

// lgIndexName 是 Prism symbol → 乐咕指数中文名映射(兜底范围内按需登记)。
// ⚠ live 校验点: 名称须与 stock_index_pe_lg 的 symbol 取值一致。
var lgIndexName = map[string]string{
	"000300.SH": "沪深300",
	"000905.SH": "中证500",
}

// FetchIndexValuationSeries 返回 A 股指数 [start,end] 日频 PE(滚动市盈率,加权口径,
// 近似理杏仁 mcw——差异见 spec §3)。PB/PSTTM 恒 NaN。
// ⚠ live 校验点: 接口名 stock_index_pe_lg 与中文字段键 日期/滚动市盈率。
func (c *Client) FetchIndexValuationSeries(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	name, ok := lgIndexName[symbol]
	if !ok {
		return nil, fmt.Errorf("akshare: no lg index mapping for %q", symbol)
	}
	rows, err := c.get("stock_index_pe_lg", url.Values{"symbol": {name}})
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(rows))
	for _, row := range rows {
		d, err := fdate(row, "日期", "date")
		if err != nil {
			return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
		}
		pts = append(pts, ValuationPoint{
			Date:  d,
			PETTM: fnum(row, "滚动市盈率"),
			PB:    math.NaN(),
			PSTTM: math.NaN(),
		})
	}
	return clipSort(pts, start, end), nil
}
