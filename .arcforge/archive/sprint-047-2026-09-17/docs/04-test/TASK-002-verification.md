# TASK-002 验证报告 — Store.AllPeriods + PeriodKey

- 验证者: test-m2b-a　　时间: 2026-09-16T11:29:23Z
- 判定对象: master @ `79878c79339f8ea2849948152bd6d082c2c3c63a`（= verify_baseline.head，merge commit）；交付 commit `e13ce10455f1d41506a6f0b4673667631b3d329a`（已在 79878c7 祖先链）；base `28fca4c67ce4e303a983ddd13c4de4921849a428`
- discovery sha256: `3abc3f196d7f8e7a79e685cf57591a287fdedaadde18e9b42d798921c6b5bef9`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）。discovery 由 dev-m2b-b 写、代码由 dev-m2b-a 写（`provenance` 在），本报告全部数字由验证者自采。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v002`
- 范围核对: `git diff --stat 28fca4c..79878c7` 仅 `store.go`(33/0)、`store_test.go`(91/2)、`types.go`(8/0)，与 `writes` 声明完全一致，无越界；`go.mod`/`go.sum` 无 diff。

## 结论：**VERIFIED**（5/5 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `PeriodKey{Period, PeriodType, PublishedAt string}`；`AllPeriods(ctx)` 只读 `v_hestia_current`；与需求夹具期望逐项相等含顺序；复用既有夹具 | test | types.go: 三字段均 `string`、注释 YYYY-MM / YYYY-MM-DD；SQL（store.go:497–499）`SELECT period, period_type, published_at FROM `+viewCurrent+` ORDER BY period, period_type`，`viewCurrent` 定义于 schema.go:19 `= "v_hestia_current"`（grep 字面量会假阴，已按常量名跳到定义）；`TestAllPeriodsReturnsEveryObservationKey` 的三元素期望切片与需求 Step 1 逐字相同（h1 → monthly → annual）；`saveObs` 走 `Store.Save`，未另建建库辅助。验证者 `TestV_AllPeriodsReadsCurrentViewNotTable`：同键修订两次 ⇒ 只回 published_at 最新的一行（证明读的是视图而非表；变异 M2 改查表 ⇒ 该测试红）；`TestV_AllPeriodsIgnoresPending`：未过闸观测不出现 | PASS |
| functional[1] | 两条守卫先红后绿；`len(want)` 不手写；追加注释段 | test | python 解析源码 `want`：reflect 版 28fca4c=14 → 79878c7=15（含 `AllPeriods`），AST 版 38 → 39（含 `Store.AllPeriods`），两处均保持排序；断言文案用 `len(want)`；`red_phase.step5_guards_both_red` 记 `len=14/15`、`len=38/39` 两条 FAIL（与源码解析一致；Leader 派发消息里的 40→41 是假数，plan.md 验证者注已说明）；注释段「—— 为什么名单里多了 Store.AllPeriods（M2b 的 TASK-002 追加）——」在 store_test.go:643，diff 中 `+` 行 1 次；`TestStoreExposesNoWriteMethods`、`TestPackageExposesNoWriteFunctions` 现均 PASS | PASS |
| boundary[0] | 空视图 ⇒ 空切片 `err==nil`；顺序由 `ORDER BY` 明确、不依赖插入序 | test | dev: `TestAllPeriodsEmptyStore`（`require.Empty`）、主测试乱序插 3 条；验证者 `TestV_AllPeriodsOrderIsFromOrderBy`：乱序插 6 条（同 2025-06 的 h1/monthly、同 annual 的 2024-12/2025-12、q1、monthly 2025-01），断言输出 == 独立按 (Period, PeriodType) 字节序排序的结果、首项 2024-12、第 4/5 项 h1/monthly、两次调用稳定；变异 M3（`ORDER BY period_type, period`）⇒ dev 与验证者测试均红 | PASS |
| error_handling[0] | `ctx` 已取消 ⇒ 返回 `ctx.Err()` 包裹的错误、不返回半截结果 | test | dev: `TestAllPeriodsPropagatesContextCancellation`（`ErrorIs(context.Canceled)`、`got == nil`）；验证者 `TestV_AllPeriodsEmptyAndCtx` 补 Deadline 形态（`ErrorIs(context.DeadlineExceeded)`、`got == nil`）；错误前缀 `hestia store all periods` 与 store.go 既有 31 处惯例一致 | PASS |
| non_functional[0] | C2 不 import `sheets`；全绿；gofmt/vet 零输出；code-simplifier | manual | `GOTOOLCHAIN=local go test ./internal/hestia/ -count=1` exit 0，`-v` 计数 **823 PASS / 0 FAIL**；`gofmt -l internal/hestia/` 空；`go vet` exit 0；`hestia/sheets` 只在 exported_funcs_test.go 两处注释、import 语句 0 处；`PeriodKey` 纯数据不带句柄。code-simplifier：改动已由验证者**逆向重建并以 sha256 精确证实**（见下节「code-simplifier 改动逆向重建」）——仅两处：错误前缀加 `store`、删 3 处冗余 `defer st.Close()`；`types.go` 未动。改动在提交里；discovery `key_findings[3]`/`degradations[1]` 有申报（写 discovery 时尚无交接稿故记「不可还原」，现已还原，事实以本报告为准） | PASS |

## 变异测试（worktree 内改 store.go 副本，每次 `git checkout` 还原，前后 sha256 `56565d51…` 一致）

| 变异 | 结果 | 说明 |
|---|---|---|
| M1 去掉 `ORDER BY` | **SURVIVED** | observations 主键为 `(period, period_type, published_at)`，SQLite 沿主键 b-tree 遍历的自然序与 `ORDER BY period, period_type` 一致——该 schema 下属等价变异，dev 与验证者夹具都杀不掉。代码里 `ORDER BY` 确实在（grep 见 F0），契约由 SQL 明确而非靠遍历序，判据满足。非缺陷。 |
| M2 `FROM viewCurrent` → `FROM TableObservations` | KILLED | 仅验证者 `TestV_AllPeriodsReadsCurrentViewNotTable` 红（dev 夹具无修订样本，区分不了视图与表；DoD 此项判据是 grep SQL，已满足） |
| M3 `ORDER BY` 列序反转 | KILLED | dev 主测试 + 验证者 V1 红 |
| M4 吞掉 `rows.Err()` 检查 | SURVIVED | 无夹具能触发迭代中途错；DoD 未要求。非缺陷 |

## 测试质量评审
- 断言均为整切片 `require.Equal`、`ErrorIs`、`Nil`，无空洞断言，无 mock；夹具经 `newTestStore`（t.Cleanup 关闭）隔离。
- `saveObs` 是既有 `saveMonthly` 造不出「同月 monthly/h1」形状的合理补充，同走 `Store.Save`（discovery decisions[1] 有申报）。
- 与需求原文的两处刻意偏离（错误前缀 `hestia store …`、不写 `defer st.Close()`）均在 discovery `decisions` 记明理由，与文件既有惯例一致。

## code-simplifier 改动逆向重建（Leader 补充要求；落点本报告，不改 discovery）

来源：dev-m2b-a 交接稿 `scratchpad/dev-m2b-a-TASK-002-discovery-handover.json` 声称 simplifier 改了两处；`scratchpad/dev-m2b-a-TASK-002-pre-simplifier.sha` 只留 sha 不留内容。验证方法：从提交版 `e13ce10` 取出文件，**施加声称改动的逆操作**，算 sha256 与留痕比对——相等即证明「声称的两处 = 全部改动」，多一字少一字都对不上。

| 文件 | 留痕 pre-simplifier sha256 | 提交版 sha256 | 逆操作 | 重建后 sha256 == 留痕 |
|---|---|---|---|---|
| `store.go` | `03ec11fb…` | `56565d51…` | `"hestia store all periods"` → `"hestia all periods"`（3 处） | **True**（`03ec11fb11f3…`） |
| `store_test.go` | `64ba63f7…` | `8d082c33…` | 3 条 AllPeriods 测试的 `st := newTestStore(t)` 后各补回一行 `defer func() { _ = st.Close() }()` | **True**（`64ba63f7ecae…`；行数 3096 → 3093，恰少 3 行） |
| `types.go` | `cc86cc16…` | `cc86cc16…` | 无 | 相同，未动 |

对照变体（用于排除其他改动）：store.go 若同时把 `rows.Err()` 改回需求原文的 `return out, rows.Err()` 形态 ⇒ sha 不等（`330b0ad5…`）；只改 rows.Err 不改前缀 ⇒ 不等（`691f0654…`）；store_test.go 若补的是 `defer st.Close()` 而非需求原文形态 ⇒ 不等（`bfab230f…`）。⇒ **rows.Err() 的显式包裹是 dev 写的，不是 simplifier 改的**；simplifier 的改动恰为交接稿所述两处，无其他。

同时确认：`len(want)` 以 **39**（AST）/ **15**（reflect）为准；交接稿 `key_findings[0]` 的「40→41」是错数（同一份稿的 `red_phase.step5` 打的 `len=38/39` 与源码解析一致），Leader 转述沿用了它。

## 备注（非缺陷）
- 代码作者 dev-m2b-a 失联，discovery 由 dev-m2b-b 零代码改动接手记账（`provenance` 完整）；本报告数字全部由验证者在 79878c7 重采。 discovery `degradations[1]`「simplifier 改动无法还原」已被上节证据推翻，但 discovery 不改（改了 `discovery_sha256` 基线就漂），事实归本报告。
- 验证者自构测试 `zz_verify_m2b_a_test.go` 只存在于验证 worktree，不进交付。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go）
```go
package hestia

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v002 存在）
import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// V1 boundary[0]：乱序插入 6 条（含同 period 不同 period_type、同 period_type 不同 period），
// 断言输出 == 按 (Period, PeriodType) 字节序独立排序的结果，且两次调用稳定。
func TestV_AllPeriodsOrderIsFromOrderBy(t *testing.T) {
	st := newTestStore(t)
	ins := []PeriodKey{
		{"2025-12", "annual", "2026-01-13"},
		{"2025-03", "q1", "2025-04-15"},
		{"2025-06", "monthly", "2025-07-14"},
		{"2024-12", "annual", "2025-01-14"},
		{"2025-06", "h1", "2025-07-15"},
		{"2025-01", "monthly", "2025-02-14"},
	}
	for _, k := range ins {
		saveObs(t, st, k.Period, k.PeriodType, k.PublishedAt)
	}
	want := append([]PeriodKey(nil), ins...)
	sort.Slice(want, func(i, j int) bool {
		if want[i].Period != want[j].Period {
			return want[i].Period < want[j].Period
		}
		return want[i].PeriodType < want[j].PeriodType
	})
	got, err := st.AllPeriods(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, "2024-12", got[0].Period)
	require.Equal(t, []string{"h1", "monthly"}, []string{got[3].PeriodType, got[4].PeriodType})
	got2, err := st.AllPeriods(context.Background())
	require.NoError(t, err)
	require.Equal(t, got, got2)
	t.Logf("got=%v", got)
}

// V2 functional[0]「只读 v_hestia_current」的行为面：同业务键修订 ⇒ 视图只给最新 published_at 的一行
func TestV_AllPeriodsReadsCurrentViewNotTable(t *testing.T) {
	st := newTestStore(t)
	saveObs(t, st, "2025-06", "monthly", "2025-07-14")
	saveObs(t, st, "2025-06", "monthly", "2025-07-20") // 修订：同键、更晚发布
	got, err := st.AllPeriods(context.Background())
	require.NoError(t, err)
	require.Equal(t, []PeriodKey{{"2025-06", "monthly", "2025-07-20"}}, got)
}

// V3：未过闸（pending 表）的观测不出现
func TestV_AllPeriodsIgnoresPending(t *testing.T) {
	st := newTestStore(t)
	rep := passing()
	rep.Passed = false
	_, err := st.Save(context.Background(), Observation{
		Meta: Meta{Period: "2025-09", PeriodType: "monthly", PublishedAt: "2025-10-14",
			ArticleID: "art-pending", CaliberVersion: "2025-01", Extractor: extractorV2},
		Values: map[string]float64{FieldM2: 300},
	}, rep)
	require.NoError(t, err)
	saveObs(t, st, "2025-08", "monthly", "2025-09-14")
	got, err := st.AllPeriods(context.Background())
	require.NoError(t, err)
	require.Equal(t, []PeriodKey{{"2025-08", "monthly", "2025-09-14"}}, got)
}

// V4 boundary[0] 空视图；error_handling[0] ctx 已过期（Deadline 形态，补 dev 的 Cancel 形态）
func TestV_AllPeriodsEmptyAndCtx(t *testing.T) {
	st := newTestStore(t)
	got, err := st.AllPeriods(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 0)

	saveObs(t, st, "2025-06", "monthly", "2025-07-14")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	got, err = st.AllPeriods(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, got)
}
```
