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

// selectRows 把每个月份的多条记录收敛成一条（spec §4，M2b-1 的 D1）。
//
// 1. 有 monthly ⇒ 用它（多于一条 ⇒ 报错，权威表不该有这种形状）
// 2. 否则取 published_at 最新的累计期次
// 3. 仍打平 ⇒ 报错，不猜
func selectRows(keys []PeriodKey) ([]PeriodKey, error) {
	byPeriod := make(map[string][]PeriodKey)
	for _, k := range keys {
		byPeriod[k.Period] = append(byPeriod[k.Period], k)
	}
	out := make([]PeriodKey, 0, len(byPeriod))
	// 按 period 升序输出，与 map 遍历顺序无关
	for _, p := range slices.Sorted(maps.Keys(byPeriod)) {
		cand := byPeriod[p]
		var monthly []PeriodKey
		for _, k := range cand {
			if k.PeriodType == "monthly" {
				monthly = append(monthly, k)
			}
		}
		if len(monthly) > 1 {
			return nil, fmt.Errorf("hestia sheets: %s 有 %d 条 monthly，权威表不该出现这种形状", p, len(monthly))
		}
		if len(monthly) == 1 {
			out = append(out, monthly[0])
			continue
		}

		best := cand[0]
		tie := false
		for _, k := range cand[1:] {
			switch {
			case k.PublishedAt > best.PublishedAt:
				best, tie = k, false
			case k.PublishedAt == best.PublishedAt:
				tie = true
			}
		}
		if tie {
			return nil, fmt.Errorf(
				"hestia sheets: %s 有多条同日发布的累计期次（%s），无法判定用哪条；"+
					"实测 2026-09 前不该出现，请核对权威表", p, typesOf(cand))
		}
		out = append(out, best)
	}
	return out, nil
}

// typesOf 把候选的 period_type 拼成「q1_q3、annual」，只给错误文案用。
func typesOf(keys []PeriodKey) string {
	ts := make([]string, len(keys))
	for i, k := range keys {
		ts[i] = k.PeriodType
	}
	return strings.Join(ts, "、")
}

// buildRow 把一条观测变成一行待写的格。库里缺的字段**不产生 Cell**（C4）。
//
// Year/Month 取自 Period 而不是 PublishedAt：12 月的年报次年 1 月才发，按发布日会落到
// 下一张年度表。缺失判定用 map 的 ok：scanObservation 只在 NULL 之外的值才放进 Values，
// 所以「键不存在」= 库缺、「键存在且为 0」= 真 0，后者仍要写（loan_hh_short_ytd 可为负、
// deposit_* 可为 0）。
func buildRow(obs Observation) sheets.Row {
	year, _ := strconv.Atoi(obs.Meta.Period[:4])
	month, _ := strconv.Atoi(obs.Meta.Period[5:7])

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
	return sheets.Row{Year: year, Month: month, Cells: cells}
}

// currentFunc 是 Store.Current 的形状，assembleRows 经它读每一期，测试用它注入「读不到」。
type currentFunc func(ctx context.Context, period, periodType string) (Observation, bool, error)

// assembleRows 对 keys 选行后逐期读取并组装。AllPeriods 说有而 current 说没有 ⇒ 报错，
// 不静默跳过：两者读同一视图，分叉只可能来自并发写或视图不一致，那是要人知道的事。
func assembleRows(ctx context.Context, keys []PeriodKey, current currentFunc) ([]sheets.Row, error) {
	chosen, err := selectRows(keys)
	if err != nil {
		return nil, err
	}
	rows := make([]sheets.Row, 0, len(chosen))
	for _, k := range chosen {
		obs, ok, err := current(ctx, k.Period, k.PeriodType)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("hestia sheets: %s/%s 在 AllPeriods 里但 Current 读不到", k.Period, k.PeriodType)
		}
		rows = append(rows, buildRow(obs))
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
