package sheets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-005)
// functional[0]     接口逐字；6 月 ⇒ Row 9、Col 取 cols            → TestDiffMarksEmptyCellAsWillWrite（编译即校验签名）
// functional[1]     三类判定 + AbsentInDB 带现值 + 不变量 rows×cols → TestDiffMarksIdenticalAsSame / TestDiffMarksAbsentInDB /
//                                                                   TestDiffCoversEveryRowAndColumn（裁决 A，questions[0] 2026-09-16T13:23Z）
// boundary[0]       相对容差 1e-9：噪声 Same、真差异 WillWrite     → TestDiffToleratesFloatNoise / TestDiffReportsRealDifference
// boundary[1]       短行/越界按无值不 panic；新表全空 ⇒ 全 WillWrite → TestDiffTreatsShortRowsAsEmpty / TestDiffEmptySheetIsAllWillWrite
// error_handling[0] Cell.Label 不在 cols ⇒ 不校验不 panic           → TestDiffIgnoresLabelsOutsideColumns
// non_functional[0] 纯函数；守卫登记 sheets.Diff（review）          → ../store_test.go TestPackageExposesNoWriteFunctions

func cols() Columns { return Columns{"月份": 0, "发布日期": 1, "社融存量": 2} }

// find 按标签取出某一格的变更；找不到就让测试当场失败，而不是拿零值往下比。
// 用查找而不是 got[0]：一行产出多少条、按什么顺序，是 Diff 的输出形状，不是这几条测试要钉的事。
func find(t *testing.T, got []Change, label string) Change {
	t.Helper()
	for _, c := range got {
		if c.Label == label {
			return c
		}
	}
	require.Failf(t, "缺少变更", "没有标签为「%s」的变更（共 %d 条）", label, len(got))
	return Change{}
}

func TestDiffMarksEmptyCellAsWillWrite(t *testing.T) {
	got := Diff("2025年", cols(), [][]any{}, []Row{{
		Year: 2025, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 462.06}},
	}})
	c := find(t, got, "社融存量")
	require.Equal(t, WillWrite, c.Kind)
	require.Equal(t, 9, c.Row) // 6月 ⇒ 第 9 行（Month + 3）
	require.Equal(t, 2, c.Col)
	require.Equal(t, "2025年", c.Sheet)
	require.Nil(t, c.Current)
	require.Equal(t, 462.06, c.Want)
	// 裁决 A：cols 有而 Cells 没有的列也各产出一条（AbsentInDB），一行恰 len(cols) 条
	require.Len(t, got, 3)
	for _, label := range []string{"月份", "发布日期"} {
		a := find(t, got, label)
		require.Equal(t, AbsentInDB, a.Kind, label)
		require.Nil(t, a.Current, label)
	}
}

func TestDiffMarksIdenticalAsSame(t *testing.T) {
	cur := make([][]any, 12)
	cur[5] = []any{"6月", "2026-07-15", 462.06} // 索引 5 = 6月
	got := Diff("2026年", cols(), cur, []Row{{
		Year: 2026, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 462.06}},
	}})
	c := find(t, got, "社融存量")
	require.Equal(t, Same, c.Kind)
	require.Equal(t, 462.06, c.Current)
	// 月份/发布日期不在 Cells ⇒ AbsentInDB，且把表中现值带出来
	require.Len(t, got, 3)
	month := find(t, got, "月份")
	require.Equal(t, AbsentInDB, month.Kind)
	require.Equal(t, "6月", month.Current)
	published := find(t, got, "发布日期")
	require.Equal(t, AbsentInDB, published.Kind)
	require.Equal(t, "2026-07-15", published.Current)
}

// TestDiffToleratesFloatNoise 直接决定判据六能不能达成。
//
// 浮点表示噪声（实测契约 JSON 里出现过 64400.00000000001）会让某些格**永远**
// 报「不一致」，于是「再跑一次 dry-run 将写 0 格」永远达不到，而每次 apply
// 都在写同一个值。用相对容差，不用 ==。
func TestDiffToleratesFloatNoise(t *testing.T) {
	cur := make([][]any, 12)
	cur[5] = []any{nil, nil, 64400.0}
	got := Diff("2026年", cols(), cur, []Row{{
		Year: 2026, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 64400.00000000001}},
	}})
	require.Equal(t, Same, find(t, got, "社融存量").Kind, "浮点噪声不该被当成差异，否则幂等永远达不成")
}

// TestDiffReportsRealDifference：容差不能大到盖住真实差异。
func TestDiffReportsRealDifference(t *testing.T) {
	cur := make([][]any, 12)
	cur[5] = []any{nil, nil, 64400.0}
	got := Diff("2026年", cols(), cur, []Row{{
		Year: 2026, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 64400.01}},
	}})
	c := find(t, got, "社融存量")
	require.Equal(t, WillWrite, c.Kind)
	require.Equal(t, 64400.0, c.Current)
	require.Equal(t, 64400.01, c.Want)
}

// TestDiffMarksAbsentInDB：Row.Cells 里没有的列要显式报告，
// 而不是无声消失——dry-run 必须能把「库缺」与「已一致」分开显示（spec §7.2）。
func TestDiffMarksAbsentInDB(t *testing.T) {
	cur := make([][]any, 12)
	cur[5] = []any{nil, nil, 999.0} // 表里有人工值
	got := Diff("2026年", cols(), cur, []Row{{
		Year: 2026, Month: 6, Cells: []Cell{{Label: "月份", Value: "6月"}}, // 社融存量缺
	}})
	c := find(t, got, "社融存量")
	require.Equal(t, AbsentInDB, c.Kind)
	require.Equal(t, 999.0, c.Current, "库缺时要把表中现值带出来，人要看见自己填的那个数没被动")
	require.Nil(t, c.Want)
	require.Equal(t, WillWrite, find(t, got, "月份").Kind) // 表中无值 ⇒ 要写
	// 裁决 A：「发布日期」同样不在 Cells ⇒ 也是 AbsentInDB（表中无值 ⇒ Current nil）；absent 共 2 条
	absent := 0
	for _, ch := range got {
		if ch.Kind == AbsentInDB {
			absent++
		}
	}
	require.Equal(t, 2, absent)
	published := find(t, got, "发布日期")
	require.Nil(t, published.Current)
	require.Equal(t, AbsentInDB, published.Kind)
}

// TestDiffTreatsShortRowsAsEmpty：Sheets API 会截断尾部空格，current 的行可能比 cols 短、
// 甚至整张表行数不足 12；越界一律按「表中无值」，不 panic。
func TestDiffTreatsShortRowsAsEmpty(t *testing.T) {
	cur := [][]any{{"1月"}} // 只有 1 行、1 列
	got := Diff("2026年", cols(), cur, []Row{
		{Year: 2026, Month: 1, Cells: []Cell{{Label: "月份", Value: "1月"}, {Label: "社融存量", Value: 1.0}}},
		{Year: 2026, Month: 12, Cells: []Cell{{Label: "社融存量", Value: 2.0}}},
	})
	require.Equal(t, Same, find(t, got, "月份").Kind)
	jan := find(t, got, "社融存量") // 第一条命中的是 1 月那行
	require.Equal(t, 4, jan.Row)
	require.Equal(t, WillWrite, jan.Kind)
	require.Nil(t, jan.Current)
	var dec []Change
	for _, c := range got {
		if c.Row == 15 {
			dec = append(dec, c)
		}
	}
	require.NotEmpty(t, dec, "12 月越过 current 的行数，也要按无值产出")
	for _, c := range dec {
		require.Nil(t, c.Current)
		if c.Label == "社融存量" {
			require.Equal(t, WillWrite, c.Kind)
		}
	}
}

// TestDiffEmptySheetIsAllWillWrite：新建的年度表全空 ⇒ Cells 里的每一格都要写。
func TestDiffEmptySheetIsAllWillWrite(t *testing.T) {
	got := Diff("2027年", cols(), nil, []Row{
		{Year: 2027, Month: 3, Cells: []Cell{{Label: "月份", Value: "3月"}, {Label: "发布日期", Value: "2027-04-12"}, {Label: "社融存量", Value: 1.5}}},
	})
	require.Len(t, got, 3)
	for _, c := range got {
		require.Equal(t, WillWrite, c.Kind, "列「%s」", c.Label)
		require.Equal(t, 6, c.Row)
		require.Nil(t, c.Current)
	}
}

// TestDiffIgnoresLabelsOutsideColumns：Cell.Label 不在 cols 里时不校验、不 panic——
// C3 由 push 层的 ResolveHeader(wantLabels) 兑现，Diff 不重复做。
func TestDiffIgnoresLabelsOutsideColumns(t *testing.T) {
	var got []Change
	require.NotPanics(t, func() {
		got = Diff("2026年", cols(), nil, []Row{{
			Year: 2026, Month: 6, Cells: []Cell{{Label: "不存在的列", Value: 1.0}, {Label: "社融存量", Value: 2.0}},
		}})
	})
	for _, c := range got {
		require.NotEqual(t, "不存在的列", c.Label)
	}
	require.Equal(t, WillWrite, find(t, got, "社融存量").Kind)
}

// TestDiffCoversEveryRowAndColumn 是裁决 A 的显式不变量：任意 rows/cols 组合下
// len(got) == len(rows)×len(cols)，且三类计数之和等于它——dry-run 的
// 「将写 / 一致 / 库缺」三数相加就是格数（判据四：1282+34+819 = 61×35）。
// 同时钉住输出顺序：rows 按输入顺序，行内按列号升序，行号只由 Month 决定。
func TestDiffCoversEveryRowAndColumn(t *testing.T) {
	cur := make([][]any, 12)
	cur[11] = []any{"12月", "2026-01-13", 1.0}
	rows := []Row{
		{Year: 2025, Month: 12, Cells: []Cell{{Label: "月份", Value: "12月"}, {Label: "社融存量", Value: 2.0}}}, // 刻意 12 月在前
		{Year: 2025, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 3.0}}},
		{Year: 2025, Month: 1, Cells: nil}, // 一格都没有 ⇒ 全 AbsentInDB
	}
	got := Diff("2025年", cols(), cur, rows)
	require.Len(t, got, len(rows)*len(cols()))

	counts := map[ChangeKind]int{}
	for _, c := range got {
		counts[c.Kind]++
	}
	require.Equal(t, len(got), counts[WillWrite]+counts[Same]+counts[AbsentInDB])
	require.Equal(t, 1, counts[Same])       // 12 月的「月份」
	require.Equal(t, 2, counts[WillWrite])  // 12 月社融存量（1.0→2.0）、6 月社融存量（无值→3.0）
	require.Equal(t, 6, counts[AbsentInDB]) // 12 月发布日期 + 6 月两列 + 1 月三列

	// 顺序：行按输入序（12 月、6 月、1 月），行内列号 0,1,2
	wantRows := []int{15, 15, 15, 9, 9, 9, 4, 4, 4}
	wantCols := []int{0, 1, 2, 0, 1, 2, 0, 1, 2}
	for i, c := range got {
		require.Equal(t, wantRows[i], c.Row, "第 %d 条", i)
		require.Equal(t, wantCols[i], c.Col, "第 %d 条", i)
	}
}
