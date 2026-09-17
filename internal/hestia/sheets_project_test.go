package hestia

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/newthinker/atlas/internal/hestia/sheets"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-004)
// functional[0]     选行三条：monthly 优先 / 累计取最新发布 / Year,Month 取自 Period
//                     → TestSelectRowsPrefersMonthly / TestSelectRowsFallsBackToCumulative /
//                       TestSelectRowsTakesLatestPublishedAmongCumulative / TestBuildRowYearMonthComeFromPeriod
// functional[1]     testdata 77 条 → 恰 61 行                       → TestGroupRowsCollapsesSeventySevenToSixtyOne
// functional[2]     35 列锚点 + 月份/发布日期 string、其余 float64   → TestSheetColumnsCoverEntryArea /
//                       TestSheetColumnFieldsAllExist / TestBuildRowSendsNumbersAsNumbers
// boundary[0]       C4 两向：NULL 不产生 Cell（缺 3 ⇒ 32 格）；0 仍要写 → TestBuildRowOmitsAbsentFields /
//                       TestBuildRowOmitsOnlyAbsentFields / TestBuildRowWritesZeroValues
// error_handling[0] >1 monthly 报错含 Period；同日 tie 报错；Current 读不到报错不跳过
//                     → TestSelectRowsErrorsOnDuplicateMonthly / TestSelectRowsErrorsOnUndecidableTie /
//                       TestBuildSheetRowsErrorsWhenCurrentMissing
// non_functional[0] C2 BuildSheetRows 在父包；守卫登记（review）  → TestBuildSheetRowsReadsStore（真 Store 端到端）+
//                       ../store_test.go TestPackageExposesNoWriteFunctions

// —— 选行策略从「二选一」改为「同期合并」（2026-09-17）——
//
// 原规则「有 monthly 就用 monthly」建立在一个假设上：monthly 那条是完整记录。
// 对季末月**这个假设是假的**——央行季末发的是季度报告，抽取器为同一篇文章产出两条
// 记录（monthly + q1/h1/q1_q3），累计类字段全在季度那条里。实测 16 个多行期中有 14 个
// monthly 只占 33 个数据列中的 2 列，而季度行占 31 列；旧规则每期丢 29 格，共 442 格。
//
// **反转方向也不对**：2024-03 与 2025-03 恰好相反（monthly=29、q1=4）。所以换一个
// 优先级只是把错误挪个地方。实测 16/16 期两条记录**交集为 0、并集恰好铺满 33 列、
// 零取值冲突、同一 article_id 同一发布日**——它们是同一篇文章的互补切片，合并才是正解。

// TestGroupRowsKeepsEveryRecordOfAPeriod：同期多条一条都不丢，全部进同一组待合并。
func TestGroupRowsKeepsEveryRecordOfAPeriod(t *testing.T) {
	got, err := groupRows([]PeriodKey{
		{Period: "2025-06", PeriodType: "h1", PublishedAt: "2025-07-14"},
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1, "同一 period 收敛成一组")
	require.Len(t, got[0], 2, "组内两条都在，一条都不丢")
}

// TestGroupRowsSortsDeterministically：组序按 period 升序、组内按 period_type 升序。
// 不依赖 map 遍历顺序——否则同一份库两次投影可能产出不同的行序。
func TestGroupRowsSortsDeterministically(t *testing.T) {
	got, err := groupRows([]PeriodKey{
		{Period: "2025-09", PeriodType: "q1_q3", PublishedAt: "2025-10-13"},
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
		{Period: "2025-09", PeriodType: "monthly", PublishedAt: "2025-10-13"},
		{Period: "2025-06", PeriodType: "h1", PublishedAt: "2025-07-14"},
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "2025-06", got[0][0].Period)
	require.Equal(t, []string{"h1", "monthly"}, []string{got[0][0].PeriodType, got[0][1].PeriodType})
	require.Equal(t, "2025-09", got[1][0].Period)
	require.Equal(t, []string{"monthly", "q1_q3"}, []string{got[1][0].PeriodType, got[1][1].PeriodType})
}

// TestGroupRowsKeepsSoleCumulativeRecord：没有 monthly 的月份照常成组。
//
// 实测库里 9 个月份没有 monthly，其中**六个是 12 月**——央行 12 月数据随年报发，
// 没有单独的 12 月月报。丢掉它们会让每年的 12 月行都空着。
func TestGroupRowsKeepsSoleCumulativeRecord(t *testing.T) {
	got, err := groupRows([]PeriodKey{
		{Period: "2025-12", PeriodType: "annual", PublishedAt: "2026-01-13"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "annual", got[0][0].PeriodType)
}

// TestGroupRowsErrorsOnDuplicateMonthly：同一 Period 两条 monthly 是权威表不该有的形状
// （DoD error_handling[0]，需求 line 871）。报错要带上是哪个月。
//
// 这条在改为合并之后**仍然保留**：它查的不是「选哪条」，而是「库的形状对不对」。
// 两条 monthly 意味着同一期被重复入库，合并会把它们悄悄揉成一条、掩盖掉这个事实。
func TestGroupRowsErrorsOnDuplicateMonthly(t *testing.T) {
	got, err := groupRows([]PeriodKey{
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-15"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2025-06")
	require.Nil(t, got)
}

// TestMergeObservationsUnionsComplementaryFields 是本次改动的核心断言。
func TestMergeObservationsUnionsComplementaryFields(t *testing.T) {
	got, err := mergeObservations([]Observation{
		{Meta: Meta{Period: "2023-06", PeriodType: "monthly", PublishedAt: "2023-07-11"},
			Values: map[string]float64{FieldTSFStock: 365.0, FieldTSFStockYoY: 9.0}},
		{Meta: Meta{Period: "2023-06", PeriodType: "h1", PublishedAt: "2023-07-11"},
			Values: map[string]float64{FieldM2: 287.3, FieldDepositFlowYTD: 189900}},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]float64{
		FieldTSFStock: 365.0, FieldTSFStockYoY: 9.0,
		FieldM2: 287.3, FieldDepositFlowYTD: 189900,
	}, got.Values)
	require.Equal(t, "2023-06", got.Meta.Period)
}

// TestMergeObservationsErrorsOnConflict：同名字段取值不同就报错，不猜。
//
// 实测 16/16 期零冲突，所以这条**不该触发**；真触发了说明抽取器对同一期给出了两个
// 互相矛盾的数，那是要人知道的事——静默取其一会让一个错数进表而无人察觉。
func TestMergeObservationsErrorsOnConflict(t *testing.T) {
	_, err := mergeObservations([]Observation{
		{Meta: Meta{Period: "2023-06", PeriodType: "monthly"}, Values: map[string]float64{FieldM2: 287.3}},
		{Meta: Meta{Period: "2023-06", PeriodType: "h1"}, Values: map[string]float64{FieldM2: 287.4}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2023-06")
	require.Contains(t, err.Error(), FieldM2)
}

// TestMergeObservationsSameValueIsNotAConflict：同名同值不算冲突（合并幂等）。
func TestMergeObservationsSameValueIsNotAConflict(t *testing.T) {
	got, err := mergeObservations([]Observation{
		{Meta: Meta{Period: "2023-06", PeriodType: "monthly"}, Values: map[string]float64{FieldM2: 287.3}},
		{Meta: Meta{Period: "2023-06", PeriodType: "h1"}, Values: map[string]float64{FieldM2: 287.3}},
	})
	require.NoError(t, err)
	require.Equal(t, 287.3, got.Values[FieldM2])
}

// TestMergeObservationsTakesLatestPublishedAt：发布日列取组内最新。
//
// 实测同期各条同日发布，所以这条也不该有分歧；定死取最新是为了让输出不依赖组内顺序
// ——「恰好相同」不是可以省掉规则的理由，那正是它将来变了却没人发现的形态。
func TestMergeObservationsTakesLatestPublishedAt(t *testing.T) {
	got, err := mergeObservations([]Observation{
		{Meta: Meta{Period: "2025-09", PeriodType: "q1_q3", PublishedAt: "2025-10-13"}, Values: map[string]float64{}},
		{Meta: Meta{Period: "2025-09", PeriodType: "annual", PublishedAt: "2025-10-20"}, Values: map[string]float64{}},
	})
	require.NoError(t, err)
	require.Equal(t, "2025-10-20", got.Meta.PublishedAt)
}

// loadRealPeriodKeys 从 testdata 的期次快照读，不连真库——单元测试不该依赖一个会变的本机文件。
// 快照由 sqlite3 对 v_hestia_current 导出（order by period, period_type，与 AllPeriods 同 SQL）。
func loadRealPeriodKeys(t *testing.T) []PeriodKey {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "period-keys-2026-09-16.json"))
	require.NoError(t, err)
	var keys []PeriodKey
	require.NoError(t, json.Unmarshal(raw, &keys))
	return keys
}

// cellsByLabel 把一行的格按表头标签索引，便于「某列在不在、值是多少」两类断言共用。
// 用 map 而不是按下标断言：库缺的列不产生 Cell，下标会随缺失情况漂移。
func cellsByLabel(row sheets.Row) map[string]any {
	byLabel := make(map[string]any, len(row.Cells))
	for _, c := range row.Cells {
		byLabel[c.Label] = c.Value
	}
	return byLabel
}

// TestGroupRowsCollapsesSeventySevenToSixtyOne 把 spec §4 的交叉验算钉成测试。
//
// 77 条记录 → 61 行，差的 16 条是与同月 monthly 撞行的累计记录。
// 25 条累计记录中 9 条是所在月份唯一记录得以保留，25 − 9 = 16 = 77 − 61。
// 这条用真实库的快照跑，数字随库增长会变——变了要同步改，别直接删。
func TestGroupRowsCollapsesSeventySevenToSixtyOne(t *testing.T) {
	keys := loadRealPeriodKeys(t)
	got, err := groupRows(keys)
	require.NoError(t, err)
	require.Len(t, keys, 77)
	require.Len(t, got, 61, "61 组（= 61 个 period），而不是 61 条记录")

	var total int
	for _, g := range got {
		total += len(g)
	}
	require.Equal(t, 77, total, "77 条记录一条都没丢——改为合并之后这才是不变量")
}

// TestSheetColumnsCoverEntryArea：录入区恰 35 列，顺序即 A…AI。
func TestSheetColumnsCoverEntryArea(t *testing.T) {
	require.Len(t, SheetColumns, 35)
	require.Equal(t, "月份", SheetColumns[0].Label)
	require.Equal(t, "发布日期", SheetColumns[1].Label)
	require.Equal(t, "汇率 USD/CNY", SheetColumns[34].Label)
	// 前两列由 Meta 生成，其余 33 列必须都指向一个库字段
	require.Empty(t, SheetColumns[0].Field)
	require.Empty(t, SheetColumns[1].Field)
	for _, c := range SheetColumns[2:] {
		require.NotEmptyf(t, c.Field, "列「%s」没有对应库字段", c.Label)
	}
}

// TestSheetColumnFieldsAllExist：映射里的字段名必须真的在 fieldOrder 里。
//
// 写错一个字段名的后果是那列**永远空着**，而 dry-run 会把它显示成「库缺」——
// 与「这期报告真的没这个数」长得一模一样，不会有人察觉。
func TestSheetColumnFieldsAllExist(t *testing.T) {
	known := make(map[string]bool, len(fieldOrder))
	for _, f := range fieldOrder {
		known[f] = true
	}
	for _, c := range SheetColumns[2:] {
		require.Truef(t, known[c.Field], "列「%s」映到了不存在的字段 %q", c.Label, c.Field)
	}
}

// mustBuildRow：buildRow 在返工后返回 (Row, error)，既有用例只关心成功路径。
// 用 helper 而不是逐处 `row, err := …; require.NoError` —— 那会让每条用例多两行噪声，
// 而它们要验的性质与「会不会报错」无关。
func mustBuildRow(t *testing.T, obs Observation) sheets.Row {
	t.Helper()
	row, err := buildRow(obs)
	require.NoError(t, err)
	return row
}

// TestBuildRowOmitsAbsentFields 是 C4 的核心测试。
//
// 「缺失就写空」是最自然的写法，也正是会抹掉人工值的那一种：库说「这期报告
// 里没这个数」，不等于「这格该是空的」。实测库缺占数值格的 41%（819/2013）。
func TestBuildRowOmitsAbsentFields(t *testing.T) {
	obs := Observation{
		Meta:   Meta{Period: "2026-06", PeriodType: "h1", PublishedAt: "2026-07-15"},
		Values: map[string]float64{"tsf_stock": 462.06}, // 只有一个字段有值
	}
	row := mustBuildRow(t, obs)

	require.Equal(t, 2026, row.Year)
	require.Equal(t, 6, row.Month)
	// 月份 + 发布日期 + 唯一有值的那列 = 3 格，其余 32 列**不出现**
	require.Len(t, row.Cells, 3)

	byLabel := cellsByLabel(row)
	require.Equal(t, "6月", byLabel["月份"])
	require.Equal(t, "2026-07-15", byLabel["发布日期"])
	require.Equal(t, 462.06, byLabel["社融存量"])
	require.NotContains(t, byLabel, "M2余额")
}

// TestBuildRowOmitsOnlyAbsentFields：DoD boundary[0] 的另一半形状——33 个数值字段缺 3 个，
// 得 2 + 30 = 32 格；缺的三列不在，其余都在。
func TestBuildRowOmitsOnlyAbsentFields(t *testing.T) {
	vals := make(map[string]float64, 33)
	for i, c := range SheetColumns[2:] {
		vals[c.Field] = float64(i + 1)
	}
	delete(vals, FieldM2)
	delete(vals, FieldLoanBillYTD)
	delete(vals, FieldFXRate)

	row := mustBuildRow(t, Observation{
		Meta:   Meta{Period: "2025-03", PeriodType: "monthly", PublishedAt: "2025-04-13"},
		Values: vals,
	})
	require.Len(t, row.Cells, 32)
	byLabel := cellsByLabel(row)
	for _, missing := range []string{"M2余额", "·票据融资累计", "汇率 USD/CNY"} {
		require.NotContainsf(t, byLabel, missing, "库缺的列「%s」不该出现", missing)
	}
	require.Contains(t, byLabel, "社融存量")
}

// TestBuildRowWritesZeroValues：C4 的反向——值为 0 的字段**仍要写**。
//
// Store.Current 对 NULL 是「键不存在」，对 0 是「键存在且为 0」（scanObservation 只在
// NullFloat64.Valid 时才放进 Values）。库里 loan_hh_short_ytd = -5881、deposit_* 都可能真 0，
// 用零值表缺失会把它们抹掉。
func TestBuildRowWritesZeroValues(t *testing.T) {
	row := mustBuildRow(t, Observation{
		Meta:   Meta{Period: "2026-06", PeriodType: "h1", PublishedAt: "2026-07-15"},
		Values: map[string]float64{FieldM2YoY: 0, FieldLoanHHShortYTD: -5881},
	})
	require.Len(t, row.Cells, 4)
	byLabel := cellsByLabel(row)
	require.Contains(t, byLabel, "M2同比")
	require.Equal(t, float64(0), byLabel["M2同比"])
	require.Equal(t, float64(-5881), byLabel["·住户短期累计"])
}

// TestBuildRowYearMonthComeFromPeriod：规则③——Year/Month 取自 Period，不由 PublishedAt 决定。
// 12 月的年报次年 1 月才发，按 PublishedAt 会落到下一张年度表。
func TestBuildRowYearMonthComeFromPeriod(t *testing.T) {
	row := mustBuildRow(t, Observation{
		Meta:   Meta{Period: "2025-12", PeriodType: "annual", PublishedAt: "2026-01-13"},
		Values: map[string]float64{},
	})
	require.Equal(t, 2025, row.Year)
	require.Equal(t, 12, row.Month)
	require.Len(t, row.Cells, 2)
	require.Equal(t, "12月", row.Cells[0].Value)
}

// TestBuildRowSendsNumbersAsNumbers 钉住 RAW 模式下的类型语义。
//
// 🔴 RAW 模式把 JSON 字符串存成**文本**。数值列若发字符串，那格变成文本单元格，
// AJ–BB 的 19 个公式拿它没办法——而表面上数字还都在，没人会察觉。
func TestBuildRowSendsNumbersAsNumbers(t *testing.T) {
	obs := Observation{
		Meta:   Meta{Period: "2026-06", PeriodType: "h1", PublishedAt: "2026-07-15"},
		Values: map[string]float64{"m2_yoy": 8},
	}
	cells := mustBuildRow(t, obs).Cells
	require.Len(t, cells, 3)
	for _, c := range cells {
		switch c.Label {
		case "月份", "发布日期":
			require.IsTypef(t, "", c.Value, "列「%s」应为 string", c.Label)
		default:
			require.IsTypef(t, float64(0), c.Value, "列「%s」应为 float64，发字符串会让它变成文本格", c.Label)
			require.Equal(t, reflect.Float64, reflect.TypeOf(c.Value).Kind())
		}
	}
}

// TestBuildSheetRowsReadsStore：BuildSheetRows 端到端——走真 Store 的 AllPeriods + Current，
// 同月 monthly/h1 **合并**成一行，返回的是纯数据（C2：子包拿不到 *Store）。
func TestBuildSheetRowsReadsStore(t *testing.T) {
	st := newTestStore(t)
	saveObs(t, st, "2025-06", "h1", "2025-07-15")
	saveObs(t, st, "2025-06", "monthly", "2025-07-14")
	saveObs(t, st, "2025-12", "annual", "2026-01-13")

	rows, err := BuildSheetRows(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 2025, rows[0].Year)
	require.Equal(t, 6, rows[0].Month)
	// 发布日期取组内**最新**（改为合并后的新契约，见 mergeObservations）：
	// 合并行的数据来自两条记录，最晚那条才是这一行变完整的时刻。
	require.Equal(t, "2025-07-15", rows[0].Cells[1].Value)
	require.Equal(t, 12, rows[1].Month)
	// saveObs 只写 m2=300 ⇒ 月份 + 发布日期 + M2余额。两条记录的 m2 同值，
	// 合并不算冲突（TestMergeObservationsSameValueIsNotAConflict 单独钉住）。
	require.Len(t, rows[0].Cells, 3)
	require.Equal(t, float64(300), rows[0].Cells[2].Value)
}

// TestBuildSheetRowsErrorsWhenCurrentMissing：AllPeriods 说有、Current 说没有 ⇒ 报错，不静默跳过
// （DoD error_handling[0]，需求 line 1039–1042）。两条读同一视图，单元测试里造不出这种分叉，
// 所以经 assembleRows 注入一个「读不到」的 current。
func TestBuildSheetRowsErrorsWhenCurrentMissing(t *testing.T) {
	keys := []PeriodKey{{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"}}
	rows, err := assembleRows(context.Background(), keys,
		func(context.Context, string, string) (Observation, bool, error) { return Observation{}, false, nil })
	require.Error(t, err)
	require.Contains(t, err.Error(), "2025-06/monthly")
	require.Nil(t, rows)
}

// TestBuildSheetRowsPropagatesCurrentError：Current 本身报错 ⇒ 原样带出（errors.Is 成立）。
func TestBuildSheetRowsPropagatesCurrentError(t *testing.T) {
	boom := errors.New("boom")
	keys := []PeriodKey{{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"}}
	_, err := assembleRows(context.Background(), keys,
		func(context.Context, string, string) (Observation, bool, error) { return Observation{}, false, boom })
	require.ErrorIs(t, err, boom)
}

// —— TASK-010 返工（QA round2 SUGGESTION-5）——
//
// fix_items[2]：buildRow 的 Period[:4] / [5:7] 是定长切片，读路径不重校
// Meta.validate() 的正则（那是 Save 时的闸）⇒ 迁移脚本、手工 SQL、旧版本写入的短
// Period 到得了这里。
//
// 🔴 修法不是「取不出来就当 0」：Year=0 会让 tabName 得到 "0年"，那张表必然缺，
// 而 ingest 固定 Apply+CreateSheets ⇒ **它会去建一张叫「0年」的工作表**。
// 所以正确行为是整批报错返回，与同文件「AllPeriods 里但 Current 读不到 ⇒ 报错，不跳过」
// 同一条原则：宁可整批停下，不要静默产出一行垃圾。
func TestBuildRowRejectsMalformedPeriod(t *testing.T) {
	for _, tc := range []struct{ name, period string }{
		{"短于 YYYY-MM", "2025"},
		{"只有年", "2025-"},
		{"空串", ""},
		{"月份不是数字", "2025-XX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				_, err := buildRow(Observation{Meta: Meta{Period: tc.period, PublishedAt: "2026-01-15"}})
				require.Error(t, err, "形如 %q 的 Period 必须报错，而不是产出一行 Year=0 的垃圾", tc.period)
				require.Contains(t, err.Error(), "hestia sheets: ")
			})
		})
	}
}

// 合法 Period 仍照常取出年月——防止上一条被一个「永远报错」的实现满足。
func TestBuildRowAcceptsWellFormedPeriod(t *testing.T) {
	row, err := buildRow(Observation{Meta: Meta{Period: "2025-06", PublishedAt: "2025-07-15"}})
	require.NoError(t, err)
	require.Equal(t, 2025, row.Year)
	require.Equal(t, 6, row.Month)
}
