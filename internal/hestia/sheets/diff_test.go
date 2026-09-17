package sheets

import (
	"fmt"
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

// —— TASK-006 返工（QA round2 CRITICAL-1）——
//
// Context Checkpoint: fix_items → test mapping (TASK-006 review_fix 第 1 轮)
// [0][1] values.get 用 UNFORMATTED_VALUE；替身回字符串形态时 Diff 仍判 Same
//                                  → TestDiffTreatsStringNumbersAsSame（本文件）
//                                    TestReadUsesUnformattedValue（client_test.go）
// [2]    sameValue / toFloat 的 string 直接单测；文本格应判 WillWrite 去纠正
//                                  → TestSameValueParsesStringNumbers、TestToFloatAcceptsStrings、
//                                    TestDiffRewritesTextCellIntoNumber
// [3]    把「current 的类型取决于 valueRenderOption」写成文 → diff.go sameValue 注释

// TestSameValueParsesStringNumbers 是 CRITICAL-1 的最小复现。
//
// 🔴 pinned 模块 sheets-gen.go:11695 明写 values.get 默认 FORMATTED_VALUE ⇒ 返回的是
// **格式化文本**。toFloat 原先没有 string 分支 ⇒ sameValue 退化成 fmt.Sprint 比较 ⇒
// 带格式的列全判 WillWrite ⇒ 幂等失效、每次 apply 全量重写、dry-run 的「一致跳过」恒为 0。
//
// 这里逐个钉住真 API 会吐出来的六种形态。
func TestSameValueParsesStringNumbers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current any
		want    any
		same    bool
	}{
		{"纯小数文本", "412.50", 412.5, true},
		{"整数文本", "1282", 1282.0, true},
		{"千分位", "255,800", 255800.0, true},
		{"百分比", "10.70%", 10.7, true},
		{"会计负数", "(933)", -933.0, true},
		{"带空格", " 462.06 ", 462.06, true},
		{"真不相等", "412.50", 999.0, false},
		{"不是数字的文本", "暂无", 412.5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.same, sameValue(tc.current, tc.want),
				"current=%#v want=%#v", tc.current, tc.want)
		})
	}
}

// TestSameValueRewritesStaleResidueOverCleanInteger 守住容差的一个例外。
//
// 背景：amount.toYi 早期用裸的 `v * scale`，把「18.99 万亿元」算成
// 189899.99999999997 并写进了库，再经本包投影进了线上表格。库侧已在
// internal/hestia/amount.go 的 scaleDecimal 修好，但**表里那 44 格不会因此恢复**
// ——1e-9 的相对容差恰好把它们判成「一致」，push 永远跳过。
//
// 于是容差从「防幂等失效」变成了「冻住陈旧残差」。这条例外只在一个方向开口：
// **库值是可精确表示的整数，而表中值不是**。此时表里那个带尾巴的数就是我们
// 自己以前写进去的噪声，应当被纠正。
//
// **幂等不受影响**：写一次之后表中即为 189900，两边都是整数、精确相等，下一轮
// 判 Same。而反方向（库值本身带尾巴，如同比 8.7 那族 float64 固有的不可表示值）
// 不触发本例外，仍走容差——那正是容差当初要防的场景。
func TestSameValueRewritesStaleResidueOverCleanInteger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current any // 表中值
		want    any // 库值
		same    bool
	}{
		// 真实脏格，逐个取自线上表格
		{"表中退位残差/库整数", 189899.99999999997, 189900.0, false},
		{"表中进位残差/库整数", 64400.00000000001, 64400.0, false},
		{"表中负值残差/库整数", -11100.000000000002, -11100.0, false},
		{"文本形态的残差", "255799.99999999997", 255800.0, false},

		// 反方向：库值自身不可精确表示 ⇒ 例外不开口，容差照旧
		{"库值带尾巴/表中相近", 8.699999999999999, 8.7, true},
		{"两边都非整数且相近", 328.64000000000004, 328.64, true},

		// 例外不得波及正常判定
		{"两边同为整数", 189900.0, 189900.0, true},
		{"库整数/表中同整数的文本", "189900", 189900.0, true},
		{"库整数/表中真不同", 189900.0, 190000.0, false},
		{"表中为空", nil, 189900.0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.same, sameValue(tc.current, tc.want),
				"current=%#v want=%#v", tc.current, tc.want)
		})
	}
}

// toFloat 的 string 分支：这是 sameValue 之外的第二个消费点，单独钉住。
func TestToFloatAcceptsStrings(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want float64
		ok   bool
	}{
		{"412.50", 412.5, true},
		{"255,800", 255800.0, true},
		{"10.70%", 10.7, true},
		{"(933)", -933.0, true},
		{"", 0, false},
		{"暂无", 0, false},
		{412.5, 412.5, true},
	} {
		got, ok := toFloat(tc.in)
		require.Equal(t, tc.ok, ok, "toFloat(%#v)", tc.in)
		if tc.ok {
			require.InDelta(t, tc.want, got, 1e-9, "toFloat(%#v)", tc.in)
		}
	}
}

// TestDiffTreatsStringNumbersAsSame：整条 Diff 路径上的同一性质。
//
// 单测 sameValue 还不够——CRITICAL-1 的后果发生在 Diff 的判定里，而「表里的值是字符串」
// 这个形态在本包既有夹具里**一次都没出现过**（全部返回 JSON 数字），所以该性质此前
// 结构上不可观测。这条用例把那个盲区补上。
func TestDiffTreatsStringNumbersAsSame(t *testing.T) {
	cols := Columns{"月份": 0, "发布日期": 1, "社融存量": 2}
	// 表里读回来的是格式化文本——真 API 的默认形态。
	//
	// ⚠️ 社融存量刻意用**带千分位**的 "255,800"：若写 "462.06" 对 462.06，
	// 即使 toFloat 没有 string 分支，fmt.Sprint(462.06) 也恰好是 "462.06" ⇒ 用例
	// 会**因为巧合而绿**，测不到被测性质。消融实测：用 462.06 时删掉 string 分支
	// 这条不红，换成千分位后立刻红。
	current := [][]any{{"6月", "2026-07-15", "255,800"}}
	rows := []Row{{Year: 2026, Month: 1, Cells: []Cell{
		{Label: "月份", Value: "6月"},
		{Label: "发布日期", Value: "2026-07-15"},
		{Label: "社融存量", Value: 255800.0},
	}}}

	changes := Diff("2026年", cols, current, rows)
	require.Len(t, changes, 3)
	for _, ch := range changes {
		require.Equal(t, Same, ch.Kind,
			"格 %s 判成了 %v —— 表里是格式化文本不代表值不一样，这样会让幂等永远达不成", ch.Label, ch.Kind)
	}
}

// TestDiffRewritesTextCellIntoNumber：与上一条**方向相反**，别把两者混成一条。
//
// 上一条说「文本形态的同值不该重写」；这一条说「表里真是文本格且值不同时，仍要写回数字」。
// 理由不是洁癖：文本格会让 AJ–BB 的 19 个公式失效，而那 19 个是人维护的分析逻辑。
func TestDiffRewritesTextCellIntoNumber(t *testing.T) {
	cols := Columns{"社融存量": 0}
	current := [][]any{{"400.00"}}
	rows := []Row{{Year: 2026, Month: 1, Cells: []Cell{{Label: "社融存量", Value: 462.06}}}}

	changes := Diff("2026年", cols, current, rows)
	require.Len(t, changes, 1)
	require.Equal(t, WillWrite, changes[0].Kind)
	require.Equal(t, 462.06, changes[0].Want, "写回去的必须是 float64，不能是字符串")
}

// —— TASK-006 返工第 2 轮（QA [7] 行号钳制 / [8] TotalUpdatedCells / [10] 幂等零写）——

// TestDiffSkipsRowsWithMonthOutOfRange 是 [7] 的核心。
//
// 🔴 `Row: r.Month + entryRowOffset` 原先没有任何范围校验，而 `entryRowOffset = 3`
// 恰好等于 `headerRow`（client.go:18）⇒ **Month=0 会把「0月」的格写进表头行**，
// 破坏 C3 依赖的表头本身，此后所有投影都会因表头解析失败而永久停摆。
// Month=13 则写到第 16 行，落在录入区 4–15 之外。
//
// 判据不是「Row 值对不对」，而是**根本不产出这一行的任何 Change**：
// 产出了就有机会被 WriteCells 写出去。
func TestDiffSkipsRowsWithMonthOutOfRange(t *testing.T) {
	cols := Columns{"社融存量": 0}
	current := [][]any{{100.0}}

	for _, bad := range []int{0, -1, 13, 99} {
		t.Run(fmt.Sprintf("Month=%d", bad), func(t *testing.T) {
			got := Diff("2026年", cols, current,
				[]Row{{Year: 2026, Month: bad, Cells: []Cell{{Label: "社融存量", Value: 1.0}}}})
			require.Empty(t, got, "越界月份不许产出任何 Change —— 产出了就有机会被写出去")
		})
	}

	// 边界内仍照常产出，防止上面被一个「永远返回空」的实现满足。
	for _, ok := range []int{1, 12} {
		t.Run(fmt.Sprintf("Month=%d 仍产出", ok), func(t *testing.T) {
			got := Diff("2026年", cols, current,
				[]Row{{Year: 2026, Month: ok, Cells: []Cell{{Label: "社融存量", Value: 1.0}}}})
			require.Len(t, got, 1)
			require.Equal(t, ok+entryRowOffset, got[0].Row)
		})
	}
}

// 产出的每个 Change 的 Row 都必须落在录入区（4–15）内——这是上一条的性质化表述，
// 一条断言覆盖全部合法输入，而不是逐个月份列举。
func TestDiffNeverTargetsRowsOutsideEntryArea(t *testing.T) {
	cols := Columns{"月份": 0, "社融存量": 1}
	rows := make([]Row, 0, 12)
	for m := 1; m <= 12; m++ {
		rows = append(rows, Row{Year: 2026, Month: m, Cells: []Cell{{Label: "社融存量", Value: float64(m)}}})
	}
	for _, ch := range Diff("2026年", cols, nil, rows) {
		require.GreaterOrEqual(t, ch.Row, 4, "不许碰表头（第 3 行）与其上")
		require.LessOrEqual(t, ch.Row, 15, "不许越过录入区末行")
	}
}
