# TASK-005 验证报告 — sheets.Diff 三类变更集（按不变量 rows×cols，裁决 A）

- 验证者: test-m2b-a　　时间: 2026-09-16T14:08:14Z
- 判定对象: master @ `639c53ef6970ddc5eb140547d9779ce774e06c8c`（= verify_baseline.head，merge commit）；交付 commit `91889cc52d04a77b731dc55bb28545f74ee38f46`（dev-m2b-b，已在 639c53e 祖先链）；base `bacfd461e1d8e193083355f94a2022da3e731618`
- discovery sha256: `f7f4c75b35036f0cf0e5566db3e7398a2800472d7f6f9b9032b288410851e115`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v005`
- 判据解读：按 `questions[0].answer`（Leader 裁决 A，2026-09-16T13:23:55Z）——每行 × 每列恰一条 `Change`；DoD functional[0] 括号里的 `got[0].Row==9/Col==2` 以「社融存量那条」解读；需求原文 `require.Len(got,1)` / absent 1 条不作判据。
- 范围核对: `git diff --stat bacfd46..639c53e` 恰为 `writes` 的 3 个文件（diff.go 123/0、diff_test.go 212/0、store_test.go 5/1），无越界；`go.mod`/`go.sum` 无 diff。

## 结论：**VERIFIED**（6/6 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 接口逐字；`Diff` 无 error 返回；6 月 ⇒ Row 9、Col 取 cols | test | diff.go:10–27 `ChangeKind`（`WillWrite`/`Same`/`AbsentInDB` iota 顺序与需求一致）、`Change{Sheet; Row, Col int; Label; Current, Want any; Kind}` 逐字；diff.go:44 `func Diff(sheetName string, cols Columns, current [][]any, rows []Row) []Change` **无 error**。dev `TestDiffMarksEmptyCellAsWillWrite` 按 Label 取「社融存量」：`Row 9`、`Col 2`、`Sheet "2025年"`、`Current nil`、`Want 462.06`、`Kind WillWrite`。验证者 `TestV_InvariantAndOrder`：行号恒 `Month+3`、列号按 `cols`（含非连续列号 0/2/7/40） | PASS |
| functional[1] | 三类判定；`AbsentInDB` 无论表中有无值且保留现值；不变量 `len == rows×cols` | test | dev：`MarksIdenticalAsSame`（Same + 月份/发布日期 AbsentInDB 且 Current `"6月"`/`"2026-07-15"`）、`MarksAbsentInDB`（社融存量 AbsentInDB、Current 999.0、Want nil；absent 恰 2）、`CoversEveryRowAndColumn`（9 == 3×3，Same 1 / WillWrite 2 / AbsentInDB 6 之和相等，顺序 rows 输入序 + 行内列号升序）。验证者 `TestV_InvariantAndOrder`（4 行 × 4 列含同 Month 重复行、非连续列号：16 条、2/2/12 之和相等、顺序正确）、`TestV_ProductionShape`（12 × 35 = 420 条，36 WillWrite + 384 AbsentInDB）、`TestV_ShapeEdges`（rows=0 / cols=0 / 皆 0 ⇒ 0 条）。变异 M3（丢弃 Absent）/ M4（Absent 不带现值）/ M9（行内不按列号排序）⇒ 全红 | PASS |
| boundary[0] | 相对容差 1e-9：噪声 Same、真差异 WillWrite | test | dev `ToleratesFloatNoise`（64400.0 vs 64400.00000000001 ⇒ Same）、`ReportsRealDifference`（vs 64400.01 ⇒ WillWrite，Current/Want 都带）。验证者 `TestV_Tolerance`：0 vs 1e-10 ⇒ Same、0 vs 1e-8 ⇒ WillWrite（分母 `max(1,…)` 生效）、1e12 vs 1e12+1e2 ⇒ Same、vs 1e12+1e4 ⇒ WillWrite、−5881 vs 5881 ⇒ WillWrite、int 300 / float32 300 现值 vs 300.0 ⇒ Same、string 不同 ⇒ WillWrite / 相同 ⇒ Same。变异 M2（容差 1e-6）⇒ dev + 验证者红；M1（绝对容差 1e-9）**仅验证者夹具杀**（dev 两组在 64400 量级下绝对/相对不可分） | PASS |
| boundary[1] | 短行/越界按无值不 panic；新表全空 ⇒ 全 WillWrite | test | dev `TreatsShortRowsAsEmpty`（current 1 行 1 列，12 月越界 ⇒ 产出且 Current nil）、`EmptySheetIsAllWillWrite`（current nil ⇒ 3 条全 WillWrite、Row 6）。验证者 `TestV_ShapeEdges`：全 nil 行 ⇒ 3 条 WillWrite；`[]any{"6月"}` 只 1 列 ⇒ 月份 Same、其余 WillWrite；current 仅 3 行而 Month=6 ⇒ 3 条 WillWrite Row 9；空串现值 ⇒ 按无值。变异 M7（去掉列越界防护）⇒ panic 红；M8（空串不算无值）⇒ 红 | PASS |
| error_handling[0] | `Cell.Label` 不在 `cols` ⇒ 不校验、不 panic | test | dev `IgnoresLabelsOutsideColumns`（`NotPanics`、不产出「不存在的列」、其余照常）；验证者「外星列」+ 重复 Label（取最后一个 Value）⇒ 不 panic、仍恰 `len(cols)` 条 | PASS |
| non_functional[0] | 纯函数不 import `database/sql`；守卫登记 + 注释段 + 红留痕；全绿/gofmt/vet；code-simplifier | manual | diff.go import 仅 `fmt`/`math`/`sort`；`go list -deps ./internal/hestia/sheets` database/sql=0、父包=0。python 解析 `want`：AST 42 → 43，`sheets.Diff` idx 41、邻居 (`sheets.ColumnLetter`, `sheets.ResolveHeader`)、仍有序；reflect 15 不变；注释段挂在既有「为什么名单里多了 sheets.*」节下（store_test.go:671「TASK-005 追加 sheets.Diff：纯函数…写动作不在这里」），非新标题行；scratchpad `dev-m2b-b-TASK-005-red2-guard.txt` 原始 `-([]string) (len=42)`/`+([]string) (len=43)`/`+ "sheets.Diff"`、`--- FAIL: TestPackageExposesNoWriteFunctions`、reflect PASS；`red1.txt` 是 `undefined: Change/Diff/WillWrite` 编译红。`GOTOOLCHAIN=local go test ./internal/hestia/... -count=1` exit 0，`-v` 计数 **854 PASS / 0 FAIL**，sheets 覆盖率 93.7%；`gofmt -l` 空；`go vet` exit 0。code-simplifier：3 文件与 `pre-simplifier.sha` 比对**全部不同**（子代理回复「Unchanged」为假，dev 已自报），与 `decisions[3]` 申报的三处一致；本例改动跨多处无原文，不可逆操作精确重建，以验证者逐行审读为据：diff.go 的 `Change{…}` 字面量拆行、`max(1, |a|, |b|)` 内建（Go 1.21+）语义等价于嵌套 `math.Max`；diff_test.go 的局部变量提取与 absent 计数器不改断言；store_test.go 注释补「表名」。断言零删减、`want` 未动 | PASS |

## 变异测试（worktree 内改 diff.go 副本；每个变异体先 `go vet`、`-timeout 60s`；每次 `git checkout` 还原，前后 sha256 `65cf2efc…` 一致）

| 变异 | 结果 | 谁杀的 / 说明 |
|---|---|---|
| M1 容差改绝对 1e-9（不乘 scale） | KILLED（仅验证者 V1） | dev 夹具在 64400 量级下分不出绝对/相对；非缺陷（实现确为相对） |
| M2 容差放大到 1e-6 | KILLED | dev ReportsRealDifference + 验证者 |
| M3 AbsentInDB 丢弃不产出 | KILLED | dev 4 条 + 验证者 3 条 |
| M4 AbsentInDB 不带现值 | KILLED | dev MarksIdenticalAsSame / MarksAbsentInDB |
| M5 行号 Month+2 | KILLED | dev 4 条 + 验证者 2 条 |
| M6 现值 nil 也走 sameValue | SURVIVED | 等价变异：`sameValue(nil, v)` 经 `fmt.Sprint` 恒不等（Cells 不会出现 nil Value），行为不变 |
| M7 去掉列越界防护 | KILLED | dev TreatsShortRowsAsEmpty（panic） |
| M8 空串不算无值 | KILLED（仅验证者 V2） | dev 无空串夹具；discovery key_findings[2] 声明了该语义 |
| M9 行内不按列号排序 | KILLED | dev CoversEveryRowAndColumn + 验证者 |
| M10 Same 时不带 Want | SURVIVED | 无测试守 `Same` 的 `Want`；DoD 未要求（Change 注释「将写值；AbsentInDB 时为 nil」隐含 Same 应带值）。**建议下游 007 若读 Same 的 Want，自行断言** |

## 验证者记录的实际行为（DoD 未定，不判红）
- `Month=0` ⇒ `Row 3`（表头行）、`Month=13` ⇒ `Row 16`；`Diff` 不校验 Month 范围——`Row.Month` 类型注释「1–12」，由上游 buildRow（Period 经 periodRE 校验）保证。
- 表里字符串 `"64400"` vs 库 `float64(64400)` ⇒ **Same**（非数值路径走 `fmt.Sprint` 比较）；`true` vs `1.0` ⇒ WillWrite。RAW 读回数字为 float64，生产上不经此路径。
- 重复 `Cell.Label`：取最后一个 Value。

## 测试质量评审
- 断言均精确（`Equal` 整值、`Len`、`Nil`、`NotPanics`），`find` 助手在缺失时 `Failf` 而非零值下比；无 mock。
- 不变量测试刻意把 12 月放在前面，同时钉住计数与顺序；dev 自报首跑期望值数错被断言抓住（discovery `red_phase.invariant_test_first_run`），说明断言在守。
- 需求原文 3 条与不变量冲突的测试已按裁决 A 改为按 Label 查找，discovery `decisions[0]` 附裁决时间戳。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go，只存在于验证 worktree）
```go
package sheets

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v005 存在）
import (
	"testing"

	"github.com/stretchr/testify/require"
)

func vcols() Columns { return Columns{"月份": 0, "发布日期": 1, "社融存量": 2} }

func kindOf(t *testing.T, cur any, want any) ChangeKind {
	t.Helper()
	c := make([][]any, 12)
	c[5] = []any{nil, nil, cur}
	got := Diff("2026年", vcols(), c, []Row{{Year: 2026, Month: 6, Cells: []Cell{{Label: "社融存量", Value: want}}}})
	return find(t, got, "社融存量").Kind
}

// V1 容差两向（含 0 附近以 max(1,…) 为分母）
func TestV_Tolerance(t *testing.T) {
	require.Equal(t, Same, kindOf(t, 64400.0, 64400.00000000001))
	require.Equal(t, WillWrite, kindOf(t, 64400.0, 64400.01))
	require.Equal(t, Same, kindOf(t, 0.0, 1e-10))
	require.Equal(t, WillWrite, kindOf(t, 0.0, 1e-8))
	require.Equal(t, Same, kindOf(t, 1e-10, 0.0))
	require.Equal(t, WillWrite, kindOf(t, 1e-8, 0.0))
	require.Equal(t, Same, kindOf(t, 1e12, 1e12+1e2))   // 相对 1e-10 < 1e-9
	require.Equal(t, WillWrite, kindOf(t, 1e12, 1e12+1e4)) // 相对 1e-8
	require.Equal(t, WillWrite, kindOf(t, -5881.0, 5881.0))
	require.Equal(t, Same, kindOf(t, -5881.0, -5881.0))
	// 表里读回 int / float32 也按数值比
	require.Equal(t, Same, kindOf(t, 300, 300.0))
	require.Equal(t, Same, kindOf(t, float32(300), 300.0))
	// 非数值：string vs string 不同 ⇒ WillWrite；相同 ⇒ Same
	require.Equal(t, WillWrite, kindOf(t, "2026-07-15", "2026-07-16"))
	require.Equal(t, Same, kindOf(t, "2026-07-15", "2026-07-15"))
	// 记录：表里是字符串 "64400"、库给 float64 64400 ⇒ ?（Sprint 比较）
	t.Logf("string \"64400\" vs float64 64400 ⇒ %v", kindOf(t, "64400", 64400.0))
	t.Logf("bool true vs float64 1 ⇒ %v", kindOf(t, true, 1.0))
}

// V2 边界形状：全 nil 行 / 短行只 1 列 / current 只 3 行且 Month=6 越界 / 空串现值
func TestV_ShapeEdges(t *testing.T) {
	row := Row{Year: 2026, Month: 6, Cells: []Cell{{Label: "月份", Value: "6月"}, {Label: "发布日期", Value: "2026-07-15"}, {Label: "社融存量", Value: 1.5}}}
	// 全 nil 行
	cur := make([][]any, 12)
	cur[5] = []any{nil, nil, nil}
	got := Diff("2026年", vcols(), cur, []Row{row})
	require.Len(t, got, 3)
	for _, c := range got {
		require.Equal(t, WillWrite, c.Kind)
		require.Nil(t, c.Current)
	}
	// 短行只 1 列
	cur[5] = []any{"6月"}
	got = Diff("2026年", vcols(), cur, []Row{row})
	require.Equal(t, Same, find(t, got, "月份").Kind)
	require.Equal(t, WillWrite, find(t, got, "发布日期").Kind)
	require.Equal(t, WillWrite, find(t, got, "社融存量").Kind)
	// current 只有 3 行，Month=6 越界
	got = Diff("2026年", vcols(), [][]any{{"1月"}, {"2月"}, {"3月"}}, []Row{row})
	require.Len(t, got, 3)
	for _, c := range got {
		require.Equal(t, WillWrite, c.Kind)
		require.Equal(t, 9, c.Row)
	}
	// 空串现值按无值：WillWrite 且 Current nil；AbsentInDB 时 Current 也是 nil
	cur[5] = []any{"", "", ""}
	got = Diff("2026年", vcols(), cur, []Row{{Year: 2026, Month: 6, Cells: []Cell{{Label: "社融存量", Value: 1.5}}}})
	require.Equal(t, WillWrite, find(t, got, "社融存量").Kind)
	require.Nil(t, find(t, got, "月份").Current)
	require.Equal(t, AbsentInDB, find(t, got, "月份").Kind)
	// 不变量在 rows=0 / cols=0 / 两者皆 0 时都成立（0 条）
	require.Len(t, Diff("x", vcols(), nil, nil), 0)
	require.Len(t, Diff("x", Columns{}, nil, []Row{row}), 0)
	require.Len(t, Diff("x", nil, nil, nil), 0)
	// Cell.Label 不在 cols ⇒ 不 panic、不产出，其余照常；重复 Label 取最后一个
	require.NotPanics(t, func() {
		got = Diff("x", vcols(), nil, []Row{{Month: 6, Cells: []Cell{{Label: "外星列", Value: 1}, {Label: "社融存量", Value: 1.0}, {Label: "社融存量", Value: 2.0}}}})
	})
	require.Len(t, got, 3)
	require.Equal(t, 2.0, find(t, got, "社融存量").Want)
}

// V3 不变量与顺序：随机形状 rows×cols，三类计数之和 == len；行内列号升序；非连续列号也按升序
func TestV_InvariantAndOrder(t *testing.T) {
	cols := Columns{"a": 7, "b": 2, "c": 40, "d": 0}
	cur := make([][]any, 12)
	cur[0] = []any{"x", nil, 1.0}
	rows := []Row{
		{Month: 12, Cells: []Cell{{Label: "a", Value: 1.0}}},
		{Month: 1, Cells: []Cell{{Label: "b", Value: 1.0}, {Label: "d", Value: "x"}}},
		{Month: 7, Cells: nil},
		{Month: 1, Cells: []Cell{{Label: "c", Value: 9.0}}}, // 同一 Month 出现两次也各产出一行
	}
	got := Diff("s", cols, cur, rows)
	require.Len(t, got, len(rows)*len(cols))
	n := map[ChangeKind]int{}
	for i, c := range got {
		n[c.Kind]++
		require.Equal(t, rows[i/4].Month+3, c.Row)
		require.Equal(t, []int{0, 2, 7, 40}[i%4], c.Col)
		require.Equal(t, "s", c.Sheet)
	}
	require.Equal(t, len(got), n[WillWrite]+n[Same]+n[AbsentInDB])
	require.Equal(t, 2, n[Same])      // 1 月 b(1.0) 与 d("x")
	require.Equal(t, 2, n[WillWrite]) // 12 月 a、第 4 行 c
	require.Equal(t, 12, n[AbsentInDB])
	// 记录：Month 0 / 13 的行号（DoD 未定）
	g := Diff("s", vcols(), cur, []Row{{Month: 0, Cells: []Cell{{Label: "月份", Value: "0月"}}}, {Month: 13}})
	t.Logf("Month=0 ⇒ Row=%d Current=%v；Month=13 ⇒ Row=%d", g[0].Row, g[0].Current, g[3].Row)
}

// V4 生产形状：35 列 × 61 行不变量（用 ColumnLetter 造 35 列，rows 全带月份/发布日期）
func TestV_ProductionShape(t *testing.T) {
	cols := Columns{}
	for i := 0; i < 35; i++ {
		cols["col"+ColumnLetter(i)] = i
	}
	var rows []Row
	for m := 1; m <= 12; m++ {
		rows = append(rows, Row{Month: m, Cells: []Cell{{Label: "colA", Value: "x"}, {Label: "colB", Value: "d"}, {Label: "colC", Value: float64(m)}}})
	}
	got := Diff("2025年", cols, nil, rows)
	require.Len(t, got, 12*35)
	n := map[ChangeKind]int{}
	for _, c := range got {
		n[c.Kind]++
	}
	require.Equal(t, 36, n[WillWrite])
	require.Equal(t, 12*32, n[AbsentInDB])
}
```
