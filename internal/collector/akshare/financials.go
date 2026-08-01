package akshare

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"time"
)

// A 股财报采集(桑基 A 股支持,2026-08-01 实测定案):
//   - 利润表走东财 stock_profit_sheet_by_report_em(102 期历史,含 NOTICE_DATE 真实披露日,
//     防前视口径与 EDGAR filing date 对齐);科目为**年初至今累计**,需差分成单季。
//   - 分部走东财主营构成 stock_zygc_em(按产品分类),同为累计;茅台类每季披露 → 单季块,
//     招行/东阿阿胶类仅中报/年报披露 → 半年块标期末季(Q2/Q4)——年度粒度聚合后正确,
//     季度粒度下 Q2/Q4 为半年累计,属已声明取舍(设计 §7 A 股注记)。
// ⚠ live 校验点:接口名与字段键随 AKShare 上游变动是常态,以 aktools 实际响应修正。

// ProfitQuarter 是差分后的一个单季利润表,字段与 prismstore.FundamentalRow 对齐,
// 缺失/无法差分 = NaN。
type ProfitQuarter struct {
	FiscalPeriod string // "2026Q1"(A 股无财年错位,自然年+季)
	PeriodEnd    time.Time
	NoticeDate   time.Time // 该单季数值的公开可得日(较新一期报告的披露日)
	Revenue      float64
	GrossProfit  float64 // 营收−营业成本;银行无营业成本 → NaN(降级显示)
	RnD          float64
	SGnA         float64 // 销售费用+管理费用
	OperatingIncome, IncomeTax, NetIncome, EPSDiluted float64
}

// SegmentBlock 是差分后的一个分部营收块。半年披露的公司产出半年块,FiscalPeriod
// 标期末季(H1→Q2、H2→Q4)。
type SegmentBlock struct {
	FiscalPeriod string
	PeriodEnd    time.Time
	Name         string // 主营构成名称,经模板 xbrl_member/aliases 映射为 segment_key
	Revenue      float64
}

// mapEMSymbol: "600519.SH" → "SH600519"(东财接口用交易所前缀+代码)。
func mapEMSymbol(symbol string) string {
	code := mapAStockSymbol(symbol)
	if len(symbol) > 3 && symbol[len(symbol)-2:] == "SZ" {
		return "SZ" + code
	}
	return "SH" + code
}

// parseEMDate 解析东财两种日期形态:"2026-03-31 00:00:00" 与 "2026-03-31T00:00:00.000"。
func parseEMDate(s string) (time.Time, error) {
	if len(s) < 10 {
		return time.Time{}, fmt.Errorf("akshare: bad date %q", s)
	}
	return time.Parse("2006-01-02", s[:10])
}

// fiscalOf 由期末日推财期标签(自然年)。
func fiscalOf(end time.Time) string {
	return fmt.Sprintf("%dQ%d", end.Year(), (int(end.Month())+2)/3)
}

// spanKind 按差分块跨度分类:单季(70~100 天)/半年(170~190 天)/异常(丢弃)。
func spanKind(days float64) (quarter, half bool) {
	return days >= 70 && days <= 100, days >= 170 && days <= 190
}

// diffNaN: 累计差分,任一操作数缺失 → NaN(不产假 0)。
func diffNaN(cur, prev float64) float64 {
	if math.IsNaN(cur) || math.IsNaN(prev) {
		return math.NaN()
	}
	return cur - prev
}

// cumSpan 为 end 升序序列在下标 i 处判定累计差分的上一期,利润表与主营构成共用同一套
// 锚点/跨度规则:同自然年有前一期 → 返回其下标;否则为年内首期(prev = -1),累计以
// 「上一自然年末」为零值锚点判跨度。keep 表示该块跨度落在可保留的单季/半年区间
// (异常跨度 → false,由调用方丢弃)。
func cumSpan[T any](items []T, i int, endOf func(T) time.Time) (prev int, keep bool) {
	cur := endOf(items[i])
	prevEnd := time.Date(cur.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC)
	prev = -1
	if i > 0 && endOf(items[i-1]).Year() == cur.Year() {
		prev, prevEnd = i-1, endOf(items[i-1])
	}
	quarter, half := spanKind(cur.Sub(prevEnd).Hours() / 24)
	return prev, quarter || half
}

// FetchProfitQuarters 拉取利润表并差分成单季。年内首期以「上一自然年末」为零值锚点:
// 03-31 首期跨度 ~90 天为单季;仅半年披露的公司首期 06-30 跨度 ~181 天,同样按块产出
// (标 Q2,与分部半年块口径一致);异常跨度(缺季导致的跨两季差分)丢弃。
func (c *Client) FetchProfitQuarters(symbol string) ([]ProfitQuarter, error) {
	params := url.Values{"symbol": {mapEMSymbol(symbol)}}
	rows, err := c.get("stock_profit_sheet_by_report_em", params)
	if err != nil {
		return nil, err
	}

	type cum struct {
		end    time.Time
		notice time.Time
		vals   [8]float64 // revenue, cost, rnd, sgna, opinc, tax, ni, eps
	}
	var cums []cum
	for _, row := range rows {
		end, err := parseEMDate(str(row, "REPORT_DATE"))
		if err != nil {
			continue
		}
		notice, err := parseEMDate(str(row, "NOTICE_DATE"))
		if err != nil {
			notice = end // 披露日缺失回退期末日(与 yahoo 路径同口径,轻微前视)
		}
		sale, manage := fnum(row, "SALE_EXPENSE"), fnum(row, "MANAGE_EXPENSE")
		sgna := math.NaN()
		if !math.IsNaN(sale) && !math.IsNaN(manage) {
			sgna = sale + manage
		}
		cums = append(cums, cum{end: end, notice: notice, vals: [8]float64{
			fnum(row, "TOTAL_OPERATE_INCOME", "OPERATE_INCOME"), // 银行回退链
			fnum(row, "OPERATE_COST"),
			fnum(row, "RESEARCH_EXPENSE"),
			sgna,
			fnum(row, "OPERATE_PROFIT"),
			fnum(row, "INCOME_TAX"),
			fnum(row, "PARENT_NETPROFIT"),
			fnum(row, "DILUTED_EPS", "BASIC_EPS"),
		}})
	}
	sort.Slice(cums, func(i, j int) bool { return cums[i].end.Before(cums[j].end) })

	var out []ProfitQuarter
	for i := range cums {
		prev, keep := cumSpan(cums, i, func(c cum) time.Time { return c.end })
		if !keep {
			continue
		}
		cur := cums[i]
		var prevVals [8]float64 // 年首锚点:累计从零起算(diffNaN(cur,0)==cur)
		if prev >= 0 {
			prevVals = cums[prev].vals
		}
		var d [8]float64
		for k := range d {
			d[k] = diffNaN(cur.vals[k], prevVals[k])
		}
		gross := math.NaN()
		if !math.IsNaN(d[0]) && !math.IsNaN(d[1]) {
			gross = d[0] - d[1]
		}
		out = append(out, ProfitQuarter{
			FiscalPeriod: fiscalOf(cur.end), PeriodEnd: cur.end, NoticeDate: cur.notice,
			Revenue: d[0], GrossProfit: gross, RnD: d[2], SGnA: d[3],
			OperatingIncome: d[4], IncomeTax: d[5], NetIncome: d[6], EPSDiluted: d[7],
		})
	}
	return out, nil
}

// FetchSegments 拉取主营构成(按产品分类)并按构成名差分。差分规则与利润表一致:
// 单季块/半年块产出,异常跨度丢弃。
func (c *Client) FetchSegments(symbol string) ([]SegmentBlock, error) {
	params := url.Values{"symbol": {mapEMSymbol(symbol)}}
	rows, err := c.get("stock_zygc_em", params)
	if err != nil {
		return nil, err
	}

	type point struct {
		end time.Time
		val float64
	}
	byName := map[string][]point{}
	for _, row := range rows {
		if str(row, "分类类型") != "按产品分类" {
			continue
		}
		end, err := parseEMDate(str(row, "报告日期"))
		if err != nil {
			continue
		}
		name := str(row, "主营构成")
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], point{end: end, val: fnum(row, "主营收入")})
	}

	var out []SegmentBlock
	for name, pts := range byName {
		sort.Slice(pts, func(i, j int) bool { return pts[i].end.Before(pts[j].end) })
		for i := range pts {
			prev, keep := cumSpan(pts, i, func(p point) time.Time { return p.end })
			if !keep {
				continue
			}
			cur := pts[i]
			val := cur.val // 年内首期:累计值本身
			if prev >= 0 {
				val = diffNaN(cur.val, pts[prev].val)
			}
			if math.IsNaN(val) {
				continue
			}
			out = append(out, SegmentBlock{
				FiscalPeriod: fiscalOf(cur.end), PeriodEnd: cur.end, Name: name, Revenue: val,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PeriodEnd.Equal(out[j].PeriodEnd) {
			return out[i].PeriodEnd.Before(out[j].PeriodEnd)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// str 取 row 中的字符串键,缺失/非字符串返回 ""。
func str(row map[string]any, key string) string {
	s, _ := row[key].(string)
	return s
}
