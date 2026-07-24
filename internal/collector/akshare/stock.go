package akshare

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// mapAStockSymbol: "600519.SH"/"000001.SZ" → "600519"(百度接口用裸六位代码,实测可用)。
func mapAStockSymbol(symbol string) string {
	if i := strings.IndexByte(symbol, '.'); i > 0 {
		return symbol[:i]
	}
	return symbol
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

// FetchStockValuationSeries 返回个股 [start,end] 日频估值时序,按 market 分派数据源。
// A 股与港股均走百度股市通双指标路径(stock_zh/hk_valuation_baidu),仅 symbol 映射不同。
func (c *Client) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]ValuationPoint, error) {
	switch market {
	case "CN_A":
		return c.fetchBaidu("stock_zh_valuation_baidu", mapAStockSymbol(symbol), start, end)
	case "HK":
		return c.fetchBaidu("stock_hk_valuation_baidu", mapHKSymbol(symbol), start, end)
	default:
		return nil, fmt.Errorf("akshare: unsupported market %q for %s", market, symbol)
	}
}

// baiduPeriods 按请求窗口跨度选 period 粒度(实测: 近一年/近三年=日频,近五年≈2日,
// 近十年≈5日,全部=15日降采样——「全部」不可用于日频序列)。span<=1y→近一年;
// <=3y→近三年;更长→近十年(宽覆盖≈5日)+近三年(近端日频,覆盖同日稀疏值)双拉。
func baiduPeriods(start, end time.Time) []string {
	switch {
	case !start.Before(end.AddDate(-1, 0, 0)): // 跨度 <= 1 年
		return []string{"近一年"}
	case !start.Before(end.AddDate(-3, 0, 0)): // 跨度 <= 3 年
		return []string{"近三年"}
	default:
		return []string{"近十年", "近三年"}
	}
}

// fetchBaidu: 百度股市通每次返回单指标(字段 date/value),按 period 策略拉取市盈率(TTM)
// 与市净率两指标,再以 PE 日期为主键合并(有 PE 缺 PB 的日 PB 记 NaN;仅有 PB 无 PE 的日
// 不产出行,无 PE 无法算分位)。PSTTM 恒 NaN——原乐咕 stock_a_indicator_lg(带 ps)已被
// akshare 1.18.75 移除,百度接口无 PS 能力。
// ⚠ live 校验点(已于 2026-07-24 对 akshare 1.18.75 实测校正): 接口名 stock_zh/hk_valuation_baidu、
// indicator 取值 市盈率(TTM)/市净率、period 粒度、date+value 字段键。
func (c *Client) fetchBaidu(api, code string, start, end time.Time) ([]ValuationPoint, error) {
	periods := baiduPeriods(start, end)
	// fetchMetric 按 period 顺序拉取并合并到 date→value;后拉的 period(近三年日频)覆盖
	// 先拉 period(近十年降采样)同日的稀疏值。
	fetchMetric := func(indicator string) (map[string]float64, error) {
		m := make(map[string]float64)
		for _, period := range periods {
			rows, err := c.get(api, url.Values{
				"symbol": {code}, "indicator": {indicator}, "period": {period},
			})
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				d, err := fdate(row, "date")
				if err != nil {
					return nil, fmt.Errorf("%w (symbol %s)", err, code)
				}
				m[d.Format("2006-01-02")] = fnum(row, "value")
			}
		}
		return m, nil
	}

	pe, err := fetchMetric("市盈率(TTM)")
	if err != nil {
		return nil, err
	}
	pb, err := fetchMetric("市净率")
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
	if err := guardSchemaDrift(pts, api); err != nil {
		return nil, err
	}
	return clipSort(pts, start, end), nil
}
