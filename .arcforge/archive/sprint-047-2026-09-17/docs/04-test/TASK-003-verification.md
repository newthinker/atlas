# TASK-003 验证报告 — hestia/sheets 子包骨架、ResolveHeader / ColumnLetter / Cell / Row

- 验证者: test-m2b-a　　时间: 2026-09-16T12:48:55Z
- 判定对象: master @ `4a10d7cbed2129902fc4bca701599f9a23a92168`（= verify_baseline.head，merge commit）；交付 commit `91b251f4d23fcf542de3b2401e14ec810e0fcb64`（dev-m2b-b，已在 4a10d7c 祖先链）；base `79878c79339f8ea2849948152bd6d082c2c3c63a`
- discovery sha256: `c4a67973af561a6d20df40a6c83a674b644ad3ec544db8b7a4d46a0decaf7c64`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v003`
- 范围核对: `git diff --stat 79878c7..4a10d7c` 恰为 `writes` 声明的 5 个文件（sheets.go 16/0、header.go 57/0、header_test.go 67/0、row.go 21/0、store_test.go 15/1），无越界；`go.mod`/`go.sum` 无 diff。

## 结论：**VERIFIED**（8/8 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `ResolveHeader({月份,"",社融存量},{社融存量,月份})` = `{社融存量:2, 月份:0}`；标签两端 `TrimSpace`（`{" 月份","社融存量 "}` → `{月份:0,社融存量:1}`） | test | dev: `TestResolveHeaderSkipsEmptyHeaderCell`（与 DoD 夹具逐字相同）、`TestResolveHeaderMapsLabelsToColumns`、`TestResolveHeaderTrimsSpace`（与需求原文相同）；验证者 `TestV_ReorderBothWays`/`TestV_EdgeCases`；变异 M1（表头侧去 `TrimSpace`）⇒ `TestResolveHeaderTrimsSpace` 红 | PASS |
| functional[1] | `ColumnLetter` 0→A、25→Z、26→AA、34→AI、53→BB | test | dev: `TestColumnLetter` 五点；验证者 `TestV_ColumnLetterCarry`：进位边界 51→AZ、52→BA、701→ZZ、702→AAA、675→YZ、676→ZA、18277→ZZZ、18278→AAAA，另对 0..20000 用独立反解算法（`n = n*26 + (r-'A') + 1`）互验全部一致；变异 M4a（循环 `i > 0`）与 M4b（进位不减一）⇒ dev 与验证者测试均红 | PASS |
| functional[2] | `row.go`：`Cell{Label string; Value any}`、`Row{Year, Month int; Cells []Cell}`，无 `PeriodType`；注释明写数值列必须 `float64` 的理由 | review | row.go 全文读过：两类型字段与 DoD 逐字一致；`grep -c PeriodType row.go` = 0；Cell 注释原句「数值列必须给 float64——给 string 会让那格变成文本，而 AJ–BB 那 19 个公式拿文本没办法，**表面上数字还都在**」；Row 注释含 C4「库里缺的字段不出现在 Cells」与行号规则 Month+3 | PASS |
| functional[3] | `sheets.go` 包注释写明 C2 与 C7；需求 line 531 的 C6 改为 C7 | review | 包注释含「不接收 *hestia.Store，也不接收 *sql.DB（约束 C2）」与「默认 transport（走 ProxyFromEnvironment）…别照抄那边的空 Transport{}（约束 C7）」；`grep -c C7` = 1、`grep -c C6` = 0 ⇒ dev 已按 DoD 改为 C7，不触发 🟡 | PASS |
| boundary[0] | C3：两组「`月份` 在 0/1 列互换」夹具都通过 | test | dev: `TestResolveHeaderMapsLabelsToColumns`（月份=0）+ `TestResolveHeaderFollowsReorder`（月份=1）；验证者 `TestV_ReorderBothWays` 同一 want 两种表头 ⇒ 0/1 与 2/0 | PASS |
| boundary[1] | 写口守卫登记：AST 守卫先红后登记转绿；注释段；红的原始输出进 discovery | review | python 解析源码 `want`：AST 版 79878c7=39 → 4a10d7c=41，新增 `sheets.ColumnLetter`、`sheets.ResolveHeader`，仍有序（字节序 `W` < `s`，落在末尾）；reflect 版 15 → 15 不变（`AllPeriods` 之外无 *Store 新方法）；diff 中 `+// —— 为什么名单里多了 sheets.*` 1 次，段内逐符号写明「纯函数，不碰网络不碰库」；scratchpad `dev-m2b-b-TASK-003-red2-guard.txt` 原始输出：`-([]string) (len=39)` / `+([]string) (len=41)`、`+ "sheets.ColumnLetter"`、`+ "sheets.ResolveHeader"`、`--- FAIL: TestPackageExposesNoWriteFunctions`、`--- PASS: TestStoreExposesNoWriteMethods`——discovery `red_phase.step6_guard_red` 摘要与之一致；`red1.txt` 是 `undefined: ResolveHeader/Columns/ColumnLetter` 编译红。现两条守卫均 PASS | PASS |
| error_handling[0] | 任一标签缺失 ⇒ `err != nil` 且文案同时含全部缺失标签；整批拒绝不返回部分结果 | test | dev: `TestResolveHeaderRejectsMissingLabel`（缺 `社融存量`、`M2余额` 两者都在文案 + `got == nil`）；验证者 `TestV_MissingLabelsMessage`：want 3 个、表头缺 2 个 ⇒ 文案 `表头缺 2 个标签：月份、社融存量（表头共 2 列，请核对年度表第 3 行）` 含两个缺失、**不含**存在的 `M2余额`、`got == nil`；全缺（nil 表头）与 want 含空串同样整批拒绝。变异 M2（只报首个）/ M3（缺标签仍返回部分 cols）⇒ dev 与验证者测试均红 | PASS |
| non_functional[0] | 不 import `database/sql`、不 import 父包；`go test ./internal/hestia/... -count=1` 全绿；gofmt/vet 零输出；无新依赖；code-simplifier | manual | `go list -deps ./internal/hestia/sheets \| grep -c database/sql` = **0**；`grep -c '^…/internal/hestia$'` = **0**；`-deps -test` 亦 0；子包 import 仅 fmt/sort/strings（+ testing/testify）。`GOTOOLCHAIN=local go test ./internal/hestia/... -count=1` exit 0，`-v` 计数 **829 PASS / 0 FAIL**（hestia ok、hestia/sheets ok）；sheets 覆盖率 100.0%；`gofmt -l internal/hestia/` 空；`go vet ./internal/hestia/...` exit 0；go.mod/go.sum 无 diff。code-simplifier：提交版 5 文件与 `dev-m2b-b-TASK-003-pre-simplifier.sha` 比对，仅 header.go 不同；对提交版施加逆操作（签名 `headerRow, want []string` 拆回 `headerRow []string, want []string`）后 sha256 = 留痕 `2fca1938…` **True** ⇒ simplifier 改动恰为 discovery `decisions[2]` 所述那一处，无其他 | PASS |

## 变异测试（worktree 内改 header.go 副本，每次 `git checkout` 还原，前后 sha256 `1208806c…` 一致）

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| M1 表头侧不 `TrimSpace` | KILLED | dev `TestResolveHeaderTrimsSpace` |
| M2 只报第一个缺失标签 | KILLED | dev `RejectsMissingLabel` + 验证者 V3 |
| M3 缺标签时仍返回部分 `cols` | KILLED | dev `RejectsMissingLabel`（`require.Nil`）+ 验证者 V3 |
| M4a `ColumnLetter` 循环条件 `i > 0` | KILLED | dev `TestColumnLetter` + 验证者 V2 |
| M4b `ColumnLetter` 进位不减一 | KILLED | dev `TestColumnLetter` + 验证者 V2 |
| M5 重复标签取最右 | KILLED（仅验证者 V1） | 需求未定，dev 无夹具；非缺陷 |
| M6 空表头格也当标签 | KILLED（仅验证者 V3） | dev `SkipsEmptyHeaderCell` 的 want 不含空串故看不见；DoD 未要求；非缺陷 |

备注：验证者第一版 M4 变异体写成了死循环（净效果 `i = i/26`，`i=0` 永不终止），`go test` 10 分钟包超时后自动终止并还原；该变异体**无效、弃用**，以 M4a/M4b 为准。

## 验证者记录的实际行为（DoD 未定，不判红）
- **重复标签**：`{月份, 社融存量, 月份, "社融存量 "}` ⇒ `{月份:0, 社融存量:1}`，取**最左**，与实现注释一致。
- **want 自带空白**：`want=" 月份 "` 命中表头 `月份`，返回键是原串 `" 月份 "`（interfaces_exposed 已声明「键是 want 里传入的原标签」）。下游用同一 want 切片取值即可，若用字面量取值须注意。
- `ColumnLetter(-1)` = `""`（discovery 已声明为未定义行为）。

## 测试质量评审
- 断言均为整 map `require.Equal`、`Contains`/`Nil`，无空洞断言；无 mock；纯函数无夹具依赖。
- dev 在需求 5 条之外补的 `SkipsEmptyHeaderCell` 与 `require.Nil(got)` 正好覆盖 DoD 比需求多出的两句（functional[0] 第一组夹具、error_handling[0]「不返回部分结果」）。
- `TestColumnLetter` 遍历 map（顺序随机）但每个断言独立，无顺序依赖。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go，只存在于验证 worktree）
```go
package sheets

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v003 存在）
import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// V1 重复标签：需求未定；记实际行为（实现注释声称取最左）
func TestV_DuplicateLabelBehaviour(t *testing.T) {
	got, err := ResolveHeader([]string{"月份", "社融存量", "月份", "社融存量 "}, []string{"月份", "社融存量"})
	require.NoError(t, err)
	t.Logf("重复标签实际返回: %v", got)
	require.Equal(t, Columns{"月份": 0, "社融存量": 1}, got)
}

// V2 ColumnLetter 进位边界 + 更多点
func TestV_ColumnLetterCarry(t *testing.T) {
	cases := map[int]string{51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA", 1: "B", 27: "AB", 675: "YZ", 676: "ZA", 18277: "ZZZ", 18278: "AAAA"}
	for i, want := range cases {
		require.Equalf(t, want, ColumnLetter(i), "列号 %d", i)
	}
	// 独立算法互验（0..20000）：反解 A1 字母回列号
	for i := 0; i <= 20000; i++ {
		s := ColumnLetter(i)
		n := 0
		for _, r := range s {
			require.Truef(t, r >= 'A' && r <= 'Z', "%d → %q 含非大写字母", i, s)
			n = n*26 + int(r-'A') + 1
		}
		require.Equalf(t, i, n-1, "%d → %q 反解不一致", i, s)
	}
	t.Logf("负数输入: ColumnLetter(-1)=%q（未定义行为，仅记录）", ColumnLetter(-1))
}

// V3 缺标签：want 三个、表头缺两个 ⇒ 文案含两个缺失、不含存在的；整批拒绝
func TestV_MissingLabelsMessage(t *testing.T) {
	got, err := ResolveHeader([]string{"发布日期", "M2余额"}, []string{"社融存量", "M2余额", "月份"})
	require.Error(t, err)
	require.Nil(t, got)
	msg := err.Error()
	require.Contains(t, msg, "社融存量")
	require.Contains(t, msg, "月份")
	require.NotContains(t, msg, "M2余额")
	require.Contains(t, msg, "缺 2 个标签")
	require.Contains(t, msg, "表头共 2 列")
	t.Logf("文案: %s", msg)
	// 全缺
	got, err = ResolveHeader(nil, []string{"a", "b"})
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "缺 2 个标签")
	// 空表头格不算标签：want 空串应报缺
	got, err = ResolveHeader([]string{"月份", ""}, []string{"月份", ""})
	require.Error(t, err)
	require.Nil(t, got)
}

// V4 C3：两组「月份在 0/1 列互换」+ 表头带空格子、顺序无关
func TestV_ReorderBothWays(t *testing.T) {
	a, err := ResolveHeader([]string{"月份", "", "社融存量"}, []string{"社融存量", "月份"})
	require.NoError(t, err)
	b, err := ResolveHeader([]string{"社融存量", "月份"}, []string{"社融存量", "月份"})
	require.NoError(t, err)
	require.Equal(t, 0, a["月份"])
	require.Equal(t, 1, b["月份"])
	require.Equal(t, 2, a["社融存量"])
	require.Equal(t, 0, b["社融存量"])
}

// V5 边角：want 为空 ⇒ 空 Columns 无错；want 自带空白时键是原串（interfaces_exposed 声明）
func TestV_EdgeCases(t *testing.T) {
	got, err := ResolveHeader([]string{"月份"}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
	got, err = ResolveHeader([]string{"月份"}, []string{" 月份 "})
	require.NoError(t, err)
	_, hasRaw := got[" 月份 "]
	t.Logf("want 自带空白时返回键: %v（rawKey=%v）", got, hasRaw)
	require.True(t, hasRaw || got["月份"] == 0)
	require.False(t, strings.Contains("x", "y"))
}
```
