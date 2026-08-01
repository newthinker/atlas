package prism

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// SegmentClient is the subset of *edgar.Client used by RefreshSegments.
type SegmentClient interface {
	FetchSegmentRevenue(cik, axis string, since time.Time) ([]edgar.SegmentPeriod, error)
}

// AkshareFinancialsClient is the subset of *akshare.Client used by the A-share
// segment path (利润表 + 主营构成,2026-08-01).
type AkshareFinancialsClient interface {
	FetchProfitQuarters(symbol string) ([]akshare.ProfitQuarter, error)
	FetchSegments(symbol string) ([]akshare.SegmentPoint, error)
}

// 期间分类阈值,与 edgar 客户端的 isSingleQuarter/isAnnual 同口径。FY 期与季度期在
// FetchSegmentRevenue 的返回值里形态完全相同,只能靠跨度区分。
const (
	minQuarterDays, maxQuarterDays = 70, 100
	minAnnualDays, maxAnnualDays   = 350, 380
	// fiscalPeriodToleranceDays 是分部期 period_end 与 fundamental_q period_end 的
	// 允许偏差:两者来自不同申报口径,同一季度的日期可能差几天。
	fiscalPeriodToleranceDays = 3
)

// RefreshSegments 对配置了模板的标的增量拉取分部营收并落库。
//
// 五步语义(design-spec §4.2):
//  1. 增量锚点 since = LatestSegmentPeriodEnd;force=true 时忽略锚点全量重拉
//     (AD-12:纯增量下「跑一次看 member 清单 → 改模板 → 再跑」第二次拉不到任何
//     数据,模板迭代流程走不通)。
//  2. member → segment_key 由模板映射;未映射的 member 记入 Degraded,不失败。
//  3. 落库主键是 period_end(AD-9);fiscal_period 只是展示标签,由 period_end 在
//     fundamental_q 中 ±3 天容差反查 —— 反查不到仍照常落库(空标签 + Degraded),
//     数据不丢且可观测。
//  4. Q4 由同财年的年度期减去已存三季还原;年度期本身不落库(AD-17)。
//  5. manual 兜底数据最后 upsert 覆盖同键自动行。
//
// 单个标的失败只记入 Report.Failed,不影响其余标的。
//
// 注意:Report.Degraded 只含本函数可观察到的降级。FetchSegmentRevenue 对单份报告
// 失败(404/解析错)只写日志并跳过,不经返回值上报,故那部分明细不在此处体现。
func RefreshSegments(cfg config.PrismConfig, store Store, seg SegmentClient,
	ak AkshareFinancialsClient, templates map[string]*sankey.Template,
	manualDir string, force bool) Report {
	var rep Report
	for _, inst := range cfg.Instruments {
		tpl, ok := templates[strings.ToUpper(inst.Symbol)]
		if !ok {
			continue // 未配模板的标的不参与分部刷新
		}
		var deg []string
		var err error
		if inst.Source == "akshare" {
			// A 股路径:利润表 + 主营构成,一次全量幂等(接口整段返回,无增量锚点)。
			deg, err = refreshAkshareSymbol(store, ak, inst, tpl, manualDir)
		} else {
			deg, err = refreshSymbolSegments(store, seg, inst, tpl, manualDir, force)
		}
		for _, d := range deg {
			rep.degrade(d)
		}
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", inst.Symbol, err))
			continue
		}
		rep.Refreshed++
	}
	return rep
}

func refreshSymbolSegments(store Store, seg SegmentClient, inst config.PrismInstrument,
	tpl *sankey.Template, manualDir string, force bool) ([]string, error) {
	// AD-16(3):模板串号会把别家的分部数据写进本标的名下 —— 合法值、错误语义,
	// 静默拉取比拉不到更糟,故先校验再发请求。
	if tpl.CIK == "" {
		// 模板层 CIK 已放宽为可空(A 股模板无 CIK),EDGAR 路径在此守卫。
		return nil, fmt.Errorf("template cik is required for edgar-source segments")
	}
	if inst.CIK != "" && inst.CIK != tpl.CIK {
		return nil, fmt.Errorf("template cik %s does not match configured cik %s", tpl.CIK, inst.CIK)
	}
	id, err := upsertMeta(store, inst)
	if err != nil {
		return nil, err
	}
	since, err := segmentSince(store, id, force)
	if err != nil {
		return nil, err
	}
	periods, err := seg.FetchSegmentRevenue(tpl.CIK, tpl.SegmentAxis, since)
	if err != nil {
		return nil, err
	}
	funds, err := store.QuarterlyFundamentals(id)
	if err != nil {
		return nil, err
	}
	byMember := tpl.KeyByName()

	var deg []string
	var quarters []prismstore.SegmentRow
	var annuals []edgar.SegmentPeriod
	for _, p := range periods {
		days := p.PeriodEnd.Sub(p.PeriodStart).Hours() / 24
		switch {
		case days >= minQuarterDays && days <= maxQuarterDays:
			rows, d, err := quarterRows(p, byMember, funds, inst.Symbol)
			deg = append(deg, d...)
			if err != nil {
				return deg, err
			}
			quarters = append(quarters, rows...)
		case days >= minAnnualDays && days <= maxAnnualDays:
			annuals = append(annuals, p) // AD-17:只用于 Q4 推导,不落库
		}
	}
	if err := store.UpsertSegments(id, quarters); err != nil {
		return deg, err
	}

	// Q4 推导读回已存季度 —— 读回而非只看本轮拉到的期,才能覆盖「三季来自历史、
	// 年度期来自本轮 10-K」这个最常见的组合。
	stored, err := store.SegmentRows(id)
	if err != nil {
		return deg, err
	}
	q4rows, d, err := deriveQ4(annuals, stored, byMember, funds, inst.Symbol)
	deg = append(deg, d...)
	if err != nil {
		return deg, err
	}
	if err := store.UpsertSegments(id, q4rows); err != nil {
		return deg, err
	}

	// manual 最后写,覆盖同键自动行。注意它不是粘性的:下一轮刷新会先写自动值,
	// 再由本步骤按当时的配置重新施加 —— 配置移除后自动值即回归。
	mrows, d, err := manualRows(manualDir, inst.Symbol, funds)
	deg = append(deg, d...)
	if err != nil {
		return deg, err
	}
	return deg, store.UpsertSegments(id, mrows)
}

// segmentSince 返回增量下界。force 时返回零值,让解析器全量重拉(AD-12)。
func segmentSince(store Store, id int64, force bool) (time.Time, error) {
	if force {
		return time.Time{}, nil
	}
	anchor, err := store.LatestSegmentPeriodEnd(id)
	if err != nil {
		return time.Time{}, err
	}
	if anchor == "" {
		return time.Time{}, nil // 首次刷新,无锚点可用
	}
	d, err := time.Parse("2006-01-02", anchor)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad segment anchor %q: %w", anchor, err)
	}
	return d, nil
}

// quarterRows 把一个季度期的 member 值转成落库行。
func quarterRows(p edgar.SegmentPeriod, byMember map[string]string,
	funds []prismstore.FundamentalRow, symbol string) ([]prismstore.SegmentRow, []string, error) {
	pe := p.PeriodEnd.Format("2006-01-02")
	fp, err := lookupFiscalPeriod(funds, p.PeriodEnd)
	if err != nil {
		return nil, nil, err
	}
	var deg []string
	if fp == "" {
		deg = append(deg, fmt.Sprintf(
			"%s: no fundamental quarter within ±%dd of segment period %s, stored with empty fiscal_period",
			symbol, fiscalPeriodToleranceDays, pe))
	}
	rows := make([]prismstore.SegmentRow, 0, len(p.Values))
	for _, member := range sortedKeys(p.Values) {
		key, ok := byMember[member]
		if !ok {
			deg = append(deg, unmappedMsg(symbol, member, pe))
			continue
		}
		rows = append(rows, prismstore.SegmentRow{PeriodEnd: pe, FiscalPeriod: fp,
			SegmentKey: key, Revenue: p.Values[member], Source: "edgar_segment"})
	}
	return rows, deg, nil
}

// deriveQ4 用年度期减去同财年已存的三个季度还原 Q4。Q4 无独立申报,不这样还原就永远缺席。
//
// 「同财年三季」取 period_end 严格落在 (FY.start, FY.end) 内的行:上界用严格小于,
// 天然排除 period_end 恰为 FY.end 的 Q4 自身 —— 否则第二轮刷新会数到 4 个季度,
// 从此再也不重算 Q4(重述数据将永远更新不进来)。
func deriveQ4(annuals []edgar.SegmentPeriod, stored []prismstore.SegmentRow,
	byMember map[string]string, funds []prismstore.FundamentalRow,
	symbol string) ([]prismstore.SegmentRow, []string, error) {
	var rows []prismstore.SegmentRow
	var deg []string
	for _, a := range annuals {
		bySeg := map[string][]float64{}
		ends := map[string]bool{}
		for _, r := range stored {
			d, err := time.Parse("2006-01-02", r.PeriodEnd)
			if err != nil || !d.After(a.PeriodStart) || !d.Before(a.PeriodEnd) {
				continue
			}
			ends[r.PeriodEnd] = true
			bySeg[r.SegmentKey] = append(bySeg[r.SegmentKey], r.Revenue)
		}
		if len(ends) != 3 {
			continue // 该财年季度不齐,无从推导
		}
		pe := a.PeriodEnd.Format("2006-01-02")
		fp, err := lookupFiscalPeriod(funds, a.PeriodEnd)
		if err != nil {
			return rows, deg, err
		}
		for _, member := range sortedKeys(a.Values) {
			key, ok := byMember[member]
			if !ok {
				deg = append(deg, unmappedMsg(symbol, member, pe))
				continue
			}
			vals := bySeg[key]
			if len(vals) != 3 {
				continue // 该 segment 三季凑不齐,跳过而非用两季凑出错误的 Q4
			}
			q4 := a.Values[member]
			for _, v := range vals {
				q4 -= v
			}
			if q4 < 0 {
				// 年度与季度口径不一致(重述/分部重组)才会为负,落库只会污染桑基。
				deg = append(deg, fmt.Sprintf(
					"%s: derived Q4 for segment %s in %s is negative (%g), not stored", symbol, key, pe, q4))
				continue
			}
			rows = append(rows, prismstore.SegmentRow{PeriodEnd: pe, FiscalPeriod: fp,
				SegmentKey: key, Revenue: q4, Source: "edgar_segment"})
		}
	}
	return rows, deg, nil
}

// manualRows 把人工兜底数据转成落库行。manual 按 fiscal_period 组织,而落库主键是
// period_end(AD-9),故须用 fundamental_q 反查;反查不到就跳过并记降级——凭空构造
// period_end 只会造出一行永远 join 不上的孤儿数据。
func manualRows(manualDir, symbol string,
	funds []prismstore.FundamentalRow) ([]prismstore.SegmentRow, []string, error) {
	if manualDir == "" {
		return nil, nil, nil
	}
	manual, err := sankey.LoadManualSegments(manualDir, symbol)
	if err != nil {
		return nil, nil, err
	}
	endByFP := make(map[string]string, len(funds))
	for _, f := range funds {
		endByFP[strings.ToUpper(f.FiscalPeriod)] = f.PeriodEnd
	}
	var rows []prismstore.SegmentRow
	var deg []string
	for _, fp := range sortedKeys(manual) {
		pe, ok := endByFP[fp]
		if !ok {
			deg = append(deg, fmt.Sprintf(
				"%s: manual segment period %s has no matching quarter in fundamental_q, skipped", symbol, fp))
			continue
		}
		for _, key := range sortedKeys(manual[fp]) {
			rows = append(rows, prismstore.SegmentRow{PeriodEnd: pe, FiscalPeriod: fp,
				SegmentKey: key, Revenue: manual[fp][key], Source: "manual"})
		}
	}
	return rows, deg, nil
}

// lookupFiscalPeriod 用 ±3 天容差在 fundamental_q 中反查展示标签。未命中返回
// ("", nil),调用方落空标签并记降级;命中 >1 条返回 error —— 取首个会把某季的标签
// 安到相邻季上,又是一个「合法值但语义错误」。
func lookupFiscalPeriod(funds []prismstore.FundamentalRow, periodEnd time.Time) (string, error) {
	var hits []string
	for _, f := range funds {
		d, err := time.Parse("2006-01-02", f.PeriodEnd)
		if err != nil {
			continue // fundamental_q 里的坏日期不该阻断分部落库
		}
		if withinDays(d, periodEnd, fiscalPeriodToleranceDays) {
			hits = append(hits, f.FiscalPeriod)
		}
	}
	switch len(hits) {
	case 0:
		return "", nil
	case 1:
		return hits[0], nil
	default:
		sort.Strings(hits)
		return "", fmt.Errorf("segment period %s matches %d fundamental quarters (%s) within ±%dd",
			periodEnd.Format("2006-01-02"), len(hits), strings.Join(hits, ", "), fiscalPeriodToleranceDays)
	}
}

func unmappedMsg(symbol, member, periodEnd string) string {
	return fmt.Sprintf("%s: unmapped segment member %s in period %s (add it to the template)",
		symbol, member, periodEnd)
}

func withinDays(a, b time.Time, days int) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= time.Duration(days)*24*time.Hour
}

// sortedKeys 让遍历 map 产生的行与降级说明保持确定顺序(map 遍历本身是随机的)。
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// refreshAkshareSymbol 是 A 股标的的财报桥数据路径(2026-08-01):
//   - 利润表(东财,累计已在采集层差分为单季)落 fundamental_q,filing_date 用真实披露日;
//     Equity/DilutedShares 利润表不含,显式 NaN(零值 0 会污染 PB/ROE)。
//   - 主营构成经模板 KeyByName(xbrl_member 语义为构成名,含 aliases)映射落 segment_revenue;
//     未映射构成名**按名聚合**为一条 Degraded —— 对照 edgar 路径每期一条的告警噪音教训。
//   - 半年披露公司(招行/东阿阿胶式)的构成块标期末季(Q2/Q4):年度粒度聚合正确,
//     季度粒度下为半年累计,属已声明取舍(设计 §7 A 股注记)。
//   - manual 兜底语义与 edgar 路径一致,最后写覆盖同键自动行。
func refreshAkshareSymbol(store Store, ak AkshareFinancialsClient, inst config.PrismInstrument,
	tpl *sankey.Template, manualDir string) ([]string, error) {
	if ak == nil {
		return nil, fmt.Errorf("akshare financials client not configured")
	}
	id, err := upsertMeta(store, inst)
	if err != nil {
		return nil, err
	}

	quarters, err := ak.FetchProfitQuarters(inst.Symbol)
	if err != nil {
		return nil, fmt.Errorf("profit sheet: %w", err)
	}
	frows := make([]prismstore.FundamentalRow, len(quarters))
	for i, q := range quarters {
		frows[i] = prismstore.FundamentalRow{
			FiscalPeriod: q.FiscalPeriod,
			PeriodEnd:    q.PeriodEnd.Format("2006-01-02"),
			FilingDate:   q.NoticeDate.Format("2006-01-02"),
			Revenue:      q.Revenue, NetIncome: q.NetIncome, EPSDiluted: q.EPSDiluted,
			Equity: math.NaN(), DilutedShares: math.NaN(),
			GrossProfit: q.GrossProfit, RnD: q.RnD, SGnA: q.SGnA,
			OperatingIncome: q.OperatingIncome, IncomeTax: q.IncomeTax,
			Source: "akshare",
		}
	}
	if err := store.UpsertFundamentals(id, frows); err != nil {
		return nil, err
	}

	points, err := ak.FetchSegments(inst.Symbol)
	if err != nil {
		return nil, fmt.Errorf("segment composition: %w", err)
	}
	// 差分在映射到 key 之后进行(DiffSegments):同一业务年内改名(茅台「系列酒」→
	// 「其他系列酒」)经 aliases 归一到同 key,差分天然连续,不在改名处断裂。
	byName := tpl.KeyByName()
	blocks, unmapped := akshare.DiffSegments(points, func(name string) (string, bool) {
		key, ok := byName[name]
		return key, ok
	})
	rows := make([]prismstore.SegmentRow, 0, len(blocks))
	for _, b := range blocks {
		rows = append(rows, prismstore.SegmentRow{
			PeriodEnd: b.PeriodEnd.Format("2006-01-02"), FiscalPeriod: b.FiscalPeriod,
			SegmentKey: b.Key, Revenue: b.Revenue, Source: "akshare",
		})
	}
	var deg []string
	for _, name := range unmapped {
		deg = append(deg, fmt.Sprintf("%s: unmapped segment name %q (add it or an alias to the template)",
			inst.Symbol, name))
	}
	if err := store.UpsertSegments(id, rows); err != nil {
		return deg, err
	}

	mrows, d, err := manualRows(manualDir, inst.Symbol, frows)
	deg = append(deg, d...)
	if err != nil {
		return deg, err
	}
	return deg, store.UpsertSegments(id, mrows)
}
