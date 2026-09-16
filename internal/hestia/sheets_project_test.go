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
// functional[1]     testdata 77 条 → 恰 61 行                       → TestSelectRowsCollapsesSeventySevenToSixtyOne
// functional[2]     35 列锚点 + 月份/发布日期 string、其余 float64   → TestSheetColumnsCoverEntryArea /
//                       TestSheetColumnFieldsAllExist / TestBuildRowSendsNumbersAsNumbers
// boundary[0]       C4 两向：NULL 不产生 Cell（缺 3 ⇒ 32 格）；0 仍要写 → TestBuildRowOmitsAbsentFields /
//                       TestBuildRowOmitsOnlyAbsentFields / TestBuildRowWritesZeroValues
// error_handling[0] >1 monthly 报错含 Period；同日 tie 报错；Current 读不到报错不跳过
//                     → TestSelectRowsErrorsOnDuplicateMonthly / TestSelectRowsErrorsOnUndecidableTie /
//                       TestBuildSheetRowsErrorsWhenCurrentMissing
// non_functional[0] C2 BuildSheetRows 在父包；守卫登记（review）  → TestBuildSheetRowsReadsStore（真 Store 端到端）+
//                       ../store_test.go TestPackageExposesNoWriteFunctions

// TestSelectRowsPrefersMonthly：D1 决定——同月撞行时 monthly 赢。
func TestSelectRowsPrefersMonthly(t *testing.T) {
	got, err := selectRows([]PeriodKey{
		{Period: "2025-06", PeriodType: "h1", PublishedAt: "2025-07-15"},
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
	})
	require.NoError(t, err)
	require.Equal(t, []PeriodKey{
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
	}, got)
}

// TestSelectRowsFallsBackToCumulative 不是可选分支。
//
// 实测库里 9 个月份没有 monthly，其中**六个是 12 月**——央行 12 月数据随年报发，
// 没有单独的 12 月月报。只认 monthly 会让每年的 12月 行都空着。
func TestSelectRowsFallsBackToCumulative(t *testing.T) {
	got, err := selectRows([]PeriodKey{
		{Period: "2025-12", PeriodType: "annual", PublishedAt: "2026-01-13"},
	})
	require.NoError(t, err)
	require.Equal(t, "annual", got[0].PeriodType)
}

// TestSelectRowsTakesLatestPublishedAmongCumulative：多条累计时取最新发布的。
func TestSelectRowsTakesLatestPublishedAmongCumulative(t *testing.T) {
	got, err := selectRows([]PeriodKey{
		{Period: "2025-09", PeriodType: "q1_q3", PublishedAt: "2025-10-13"},
		{Period: "2025-09", PeriodType: "annual", PublishedAt: "2025-10-20"},
	})
	require.NoError(t, err)
	require.Equal(t, "annual", got[0].PeriodType)
}

// TestSelectRowsErrorsOnUndecidableTie 是本规则唯一允许失败的地方。
//
// 实测 77 期里一个月最多一条累计记录，所以它**不该触发**。真触发了说明库的
// 形状变了，那时候需要人知道——而不是得到一个静默的、依赖 map 遍历顺序的选择。
func TestSelectRowsErrorsOnUndecidableTie(t *testing.T) {
	_, err := selectRows([]PeriodKey{
		{Period: "2025-09", PeriodType: "q1_q3", PublishedAt: "2025-10-13"},
		{Period: "2025-09", PeriodType: "annual", PublishedAt: "2025-10-13"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2025-09")
}

// TestSelectRowsErrorsOnDuplicateMonthly：同一 Period 两条 monthly 是权威表不该有的形状
// （DoD error_handling[0]，需求 line 871）。报错要带上是哪个月。
func TestSelectRowsErrorsOnDuplicateMonthly(t *testing.T) {
	got, err := selectRows([]PeriodKey{
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14"},
		{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-15"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2025-06")
	require.Nil(t, got)
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

// TestSelectRowsCollapsesSeventySevenToSixtyOne 把 spec §4 的交叉验算钉成测试。
//
// 77 条记录 → 61 行，差的 16 条是与同月 monthly 撞行的累计记录。
// 25 条累计记录中 9 条是所在月份唯一记录得以保留，25 − 9 = 16 = 77 − 61。
// 这条用真实库的快照跑，数字随库增长会变——变了要同步改，别直接删。
func TestSelectRowsCollapsesSeventySevenToSixtyOne(t *testing.T) {
	keys := loadRealPeriodKeys(t)
	got, err := selectRows(keys)
	require.NoError(t, err)
	require.Len(t, keys, 77)
	require.Len(t, got, 61)
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

// TestBuildRowOmitsAbsentFields 是 C4 的核心测试。
//
// 「缺失就写空」是最自然的写法，也正是会抹掉人工值的那一种：库说「这期报告
// 里没这个数」，不等于「这格该是空的」。实测库缺占数值格的 41%（819/2013）。
func TestBuildRowOmitsAbsentFields(t *testing.T) {
	obs := Observation{
		Meta:   Meta{Period: "2026-06", PeriodType: "h1", PublishedAt: "2026-07-15"},
		Values: map[string]float64{"tsf_stock": 462.06}, // 只有一个字段有值
	}
	row := buildRow(obs)

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

	row := buildRow(Observation{
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
	row := buildRow(Observation{
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
	row := buildRow(Observation{
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
	cells := buildRow(obs).Cells
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
// 同月 monthly/h1 收敛成一行，返回的是纯数据（C2：子包拿不到 *Store）。
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
	require.Equal(t, "2025-07-14", rows[0].Cells[1].Value) // monthly 赢 ⇒ 发布日期是 monthly 的
	require.Equal(t, 12, rows[1].Month)
	// saveObs 只写 m2=300 ⇒ 月份 + 发布日期 + M2余额
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
