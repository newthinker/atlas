package hestia

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/newthinker/atlas/internal/hestia/sheets"
)

// ColumnSpec 是录入区一列的「表头标签 → 库字段」。
type ColumnSpec struct {
	Label string
	Field string // 空串 = 由 Meta 生成
}

// SheetColumns 是录入区 35 列的「表头标签 → 库字段」，顺序即列序（A…AI）。
// 月份与发布日期的库字段为空串，它们由 Meta 生成。
//
// 逐行抄自 spec §5.5（2026-09-16 对账实证：2026年/6月 人工值与库里 2026-06/h1 逐格相等）。
// 这张表是映射的唯一真相源，但不是运行时判定依据——运行时按表头解析（C3）。
var SheetColumns = []ColumnSpec{
	{Label: "月份"},                                         // A：由 period 的月份生成（「6月」）
	{Label: "发布日期"},                                       // B：published_at
	{Label: "社融存量", Field: FieldTSFStock},                 // C
	{Label: "社融存量同比", Field: FieldTSFStockYoY},            // D
	{Label: "社融增量·累计", Field: FieldTSFFlowYTD},            // E
	{Label: "·人民币贷款累计", Field: FieldTSFFlowRMBLoanYTD},    // F
	{Label: "·政府债券净融资累计", Field: FieldTSFFlowGovtBondYTD}, // G
	{Label: "·企业债券净融资累计", Field: FieldTSFFlowCorpBondYTD}, // H
	{Label: "M2余额", Field: FieldM2},                       // I
	{Label: "M2同比", Field: FieldM2YoY},                    // J
	{Label: "M1余额", Field: FieldM1},                       // K
	{Label: "M1同比", Field: FieldM1YoY},                    // L
	{Label: "M0余额", Field: FieldM0},                       // M
	{Label: "M0同比", Field: FieldM0YoY},                    // N
	{Label: "人民币存款余额", Field: FieldDepositBalance},        // O
	{Label: "存款余额同比", Field: FieldDepositBalanceYoY},      // P
	{Label: "新增存款·累计", Field: FieldDepositFlowYTD},        // Q
	{Label: "·住户存款累计", Field: FieldDepositHouseholdYTD},   // R
	{Label: "·非金融企业存款累计", Field: FieldDepositCorpYTD},     // S
	{Label: "·财政性存款累计", Field: FieldDepositFiscalYTD},     // T
	{Label: "·非银金融机构存款累计", Field: FieldDepositNBFIYTD},    // U
	{Label: "人民币贷款余额", Field: FieldLoanBalance},           // V
	{Label: "贷款余额同比", Field: FieldLoanBalanceYoY},         // W
	{Label: "新增贷款·累计", Field: FieldLoanFlowYTD},           // X
	{Label: "·住户短期累计", Field: FieldLoanHHShortYTD},        // Y
	{Label: "·住户中长期累计", Field: FieldLoanHHMLTYTD},         // Z
	{Label: "企业贷款累计（报告值）", Field: FieldLoanCorpTotalYTD},  // AA
	{Label: "·企业短期累计", Field: FieldLoanCorpShortYTD},      // AB
	{Label: "·企业中长期累计", Field: FieldLoanCorpMLTYTD},       // AC
	{Label: "·票据融资累计", Field: FieldLoanBillYTD},           // AD
	{Label: "·非银金融机构贷款累计", Field: FieldLoanNBFIYTD},       // AE
	{Label: "同业拆借月加权利率", Field: FieldRateIBO},             // AF
	{Label: "质押式回购月加权利率", Field: FieldRateRepo},           // AG
	{Label: "外汇储备", Field: FieldFXReserve},                // AH
	{Label: "汇率 USD/CNY", Field: FieldFXRate},             // AI
}

// groupRows 把每个月份的多条记录收敛成**一组**（spec §4 的 D1 于 2026-09-17 修订）。
//
// 🔴 **原规则是「有 monthly 就用它，否则取最新的累计期次」，那是错的。** 它建立在
// 「monthly 那条是完整记录」这个假设上，而对季末月该假设为假：央行季末发的是季度
// 报告，抽取器为**同一篇文章**产出两条记录（monthly + q1/h1/q1_q3），累计类字段全在
// 季度那条里。实测 16 个多行期中 14 个的 monthly 只占 33 个数据列里的 2 列，季度行占
// 31 列——旧规则每期丢 29 格，线上表格共 442 格因此常年空着。
//
// **不能靠反转优先级修**：2024-03 与 2025-03 恰好相反（monthly=29、q1=4），换个优先级
// 只是把错误挪到另外两期。实测 16/16 期两条记录**交集为 0、并集恰好铺满全部 33 个数据列、
// 零取值冲突、同一 article_id、同一发布日** ⇒ 它们是同一篇文章的互补切片，合并才是正解。
//
// 本函数只分组不合并，取数与合并在 assembleRows / mergeObservations——分开是因为分组
// 只需要 PeriodKey，而合并需要真去读 Observation。
//
// 保留的唯一硬失败是「同期多条 monthly」：那查的不是「选哪条」而是「库的形状对不对」，
// 合并反而会把重复入库悄悄揉成一条、掩盖掉它。原先那条「同日发布的累计期次无法判定」
// 的报错**随本次改动作废**——不再二选一，就不存在无法判定。
func groupRows(keys []PeriodKey) ([][]PeriodKey, error) {
	byPeriod := make(map[string][]PeriodKey)
	for _, k := range keys {
		byPeriod[k.Period] = append(byPeriod[k.Period], k)
	}
	out := make([][]PeriodKey, 0, len(byPeriod))
	// 按 period 升序输出，与 map 遍历顺序无关
	for _, p := range slices.Sorted(maps.Keys(byPeriod)) {
		cand := byPeriod[p]
		var monthly int
		for _, k := range cand {
			if k.PeriodType == "monthly" {
				monthly++
			}
		}
		if monthly > 1 {
			return nil, fmt.Errorf("hestia sheets: %s 有 %d 条 monthly，权威表不该出现这种形状", p, monthly)
		}
		// 组内按 period_type 排序：行序与合并结果都不该依赖 AllPeriods 的返回顺序
		slices.SortFunc(cand, func(a, b PeriodKey) int { return strings.Compare(a.PeriodType, b.PeriodType) })
		out = append(out, cand)
	}
	return out, nil
}

// mergeObservations 把同一期的多条观测按字段并成一条。
//
// **冲突报错，不猜**：同名字段取值不同意味着抽取器对同一期给出了两个互相矛盾的数。
// 实测 16/16 期零冲突，所以这条不该触发；真触发了是要人知道的事——静默取其一会让一个
// 错数进表而无人察觉，而表里的数没有任何下游校验会发现它。
//
// Meta 取组内**发布日最新**的那条整体使用（不是逐字段挑）：发布日要进表格的 B 列，
// 而 ArticleID / Extractor 等若拆开取会拼出一个不对应任何真实记录的 Meta。实测同期各条
// 同日发布，定死规则是为了让输出不依赖组内顺序——「恰好相同」不是省掉规则的理由，
// 那正是它将来变了却没人发现的形态。
func mergeObservations(obs []Observation) (Observation, error) {
	if len(obs) == 0 {
		return Observation{}, fmt.Errorf("hestia sheets: mergeObservations 收到空组，调用方应保证每组至少一条")
	}
	best := obs[0]
	values := make(map[string]float64, len(SheetColumns))
	for _, o := range obs {
		if o.Meta.PublishedAt > best.Meta.PublishedAt {
			best = o
		}
		for f, v := range o.Values {
			if prev, ok := values[f]; ok && prev != v {
				return Observation{}, fmt.Errorf(
					"hestia sheets: %s 的字段 %s 在多条记录间取值冲突（%v vs %v），拒绝猜测；请核对权威表",
					o.Meta.Period, f, prev, v)
			}
			values[f] = v
		}
	}
	return Observation{Meta: best.Meta, Values: values}, nil
}

// typesOf 把候选的 period_type 拼成「q1_q3、annual」，只给错误文案用。
func typesOf(keys []PeriodKey) string {
	ts := make([]string, len(keys))
	for i, k := range keys {
		ts[i] = k.PeriodType
	}
	return strings.Join(ts, "、")
}

// periodYearMonth 从 "YYYY-MM" 取出年与月。
//
// 🔴 **读路径不能用定长切片**（QA round2 SUGGESTION-5）：`Meta.validate()` 的格式正则是
// **Save 时**的闸，而本函数走的是读路径、不重校——迁移脚本、手工 SQL、旧版本写入的短
// Period 到得了这里，`Period[:4]` / `[5:7]` 会当场 panic。
//
// 取不出来时**报错返回，不折成 0**：Year=0 会让 `tabName` 得到 "0年"，那张表必然缺，
// 而 ingest 固定 `Apply+CreateSheets` ⇒ **它会真的去建一张叫「0年」的工作表**。
// 与本文件「AllPeriods 里但 Current 读不到 ⇒ 报错，不跳过」同一条原则：
// 宁可整批停下，也不要静默产出一行垃圾。
func periodYearMonth(period string) (int, int, error) {
	y, m, ok := strings.Cut(period, "-")
	if !ok || len(y) != 4 || len(m) != 2 {
		return 0, 0, fmt.Errorf("hestia sheets: period %q 不是 YYYY-MM 形态，无法定位年度表与行号", period)
	}
	year, yerr := strconv.Atoi(y)
	month, merr := strconv.Atoi(m)
	if yerr != nil || merr != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("hestia sheets: period %q 的年或月不是合法数字（月份须 01–12）", period)
	}
	return year, month, nil
}

// buildRow 把一条观测变成一行待写的格。库里缺的字段**不产生 Cell**（C4）。
//
// Year/Month 取自 Period 而不是 PublishedAt：12 月的年报次年 1 月才发，按发布日会落到
// 下一张年度表。缺失判定用 map 的 ok：scanObservation 只在 NULL 之外的值才放进 Values，
// 所以「键不存在」= 库缺、「键存在且为 0」= 真 0，后者仍要写（loan_hh_short_ytd 可为负、
// deposit_* 可为 0）。
func buildRow(obs Observation) (sheets.Row, error) {
	year, month, err := periodYearMonth(obs.Meta.Period)
	if err != nil {
		return sheets.Row{}, err
	}

	cells := make([]sheets.Cell, 0, len(SheetColumns))
	cells = append(cells,
		sheets.Cell{Label: SheetColumns[0].Label, Value: fmt.Sprintf("%d月", month)},
		sheets.Cell{Label: SheetColumns[1].Label, Value: obs.Meta.PublishedAt},
	)
	for _, c := range SheetColumns[2:] {
		v, ok := obs.Values[c.Field]
		if !ok {
			continue // 库缺 ⇒ 不写这格，保留表中现值
		}
		cells = append(cells, sheets.Cell{Label: c.Label, Value: v})
	}
	return sheets.Row{Year: year, Month: month, Cells: cells}, nil
}

// currentFunc 是 Store.Current 的形状，assembleRows 经它读每一期，测试用它注入「读不到」。
type currentFunc func(ctx context.Context, period, periodType string) (Observation, bool, error)

// assembleRows 对 keys 分组后逐期读取**组内全部记录**、合并、再组装。AllPeriods 说有而 current 说没有 ⇒ 报错，
// 不静默跳过：两者读同一视图，分叉只可能来自并发写或视图不一致，那是要人知道的事。
func assembleRows(ctx context.Context, keys []PeriodKey, current currentFunc) ([]sheets.Row, error) {
	groups, err := groupRows(keys)
	if err != nil {
		return nil, err
	}
	rows := make([]sheets.Row, 0, len(groups))
	for _, g := range groups {
		obs := make([]Observation, 0, len(g))
		for _, k := range g {
			o, ok, err := current(ctx, k.Period, k.PeriodType)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("hestia sheets: %s/%s 在 AllPeriods 里但 Current 读不到", k.Period, k.PeriodType)
			}
			obs = append(obs, o)
		}
		merged, err := mergeObservations(obs)
		if err != nil {
			return nil, err
		}
		row, err := buildRow(merged)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// BuildSheetRows 读库并组装成可投影的行集。
//
// 只读：AllPeriods + Current，都不碰写路径。
// 返回值是**纯数据**——子包不认识 *Store 也不认识 *sql.DB（spec §3.1 C2）。
func BuildSheetRows(ctx context.Context, st *Store) ([]sheets.Row, error) {
	keys, err := st.AllPeriods(ctx)
	if err != nil {
		return nil, err
	}
	return assembleRows(ctx, keys, st.Current)
}
