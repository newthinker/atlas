# TASK-004 验证报告 — 选行规则与 35 列字段映射，BuildSheetRows 编排

- 验证者: test-m2b-a　　时间: 2026-09-16T13:19:43Z
- 判定对象: master @ `bacfd461e1d8e193083355f94a2022da3e731618`（= verify_baseline.head，merge commit）；交付 commit `c2484e84dc3dce04d4fcc2ec2324208308ae7e0c`（dev-m2b-b，已在 bacfd46 祖先链）；base `4a10d7cbed2129902fc4bca701599f9a23a92168`
- discovery sha256: `9bcd0bc3c15958e21991ff4994b179676bd94be6b9b7927fa81f530c7756e979`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v004`
- 范围核对: `git diff --stat 4a10d7c..bacfd46` 恰为 `writes` 的 4 个文件（sheets_project.go 179/0、sheets_project_test.go 289/0、store_test.go 12/1、testdata/period-keys-2026-09-16.json 387/0），无越界；`go.mod`/`go.sum` 无 diff。

## 结论：**VERIFIED**（6/6 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 选行三条规则；四组夹具期望以需求 `require.*` 为准；Year/Month 取自 `Period` | test | dev 四组与需求 Step 1 逐字一致：`PrefersMonthly`（整切片 Equal monthly）、`FallsBackToCumulative`（`got[0].PeriodType=="annual"`）、`TakesLatestPublishedAmongCumulative`（annual 2025-10-20 胜 q1_q3 10-13）、`ErrorsOnUndecidableTie`（含 `2025-09`）；规则③ `TestBuildRowYearMonthComeFromPeriod`（2025-12/annual 发于 2026-01-13 ⇒ Year 2025、Month 12）。验证者 `TestV_TieAmongThreeCumulative`：三条累计同日 ⇒ 错误文案 `2025-09 有多条同日发布的累计期次（q1_q3、annual、h1）`；tie 之后来更晚一条 ⇒ 不报错取最晚；最晚在前、后两条打平 ⇒ 不报错；monthly 与同日累计并存 ⇒ 取 monthly；输出按 Period 升序；空输入 ⇒ 空。变异 M1（monthly 不优先）/ M2（tie 不报错）/ M4（Year/Month 取 PublishedAt）/ M8（best 更新不清 tie）⇒ 全红 | PASS |
| functional[1] | `testdata/period-keys-2026-09-16.json` 77 条 → 恰 61 行，不连真库 | test | dev `TestSelectRowsCollapsesSeventySevenToSixtyOne`（`Len 77`/`Len 61`，读 testdata 不连库）。验证者独立复算（python，不用 Go 实现）：77 条、字段恰 Period/PeriodType/PublishedAt、分布 monthly 52 / annual 6 / h1 6 / q1 7 / q1_q3 6、distinct period 61、无 monthly 月份 9（六个 12 月 + 2025-09/2026-03/2026-06）、重复 monthly 0、同日累计 tie 0、按三规则独立选行 **61**、已按 (period, period_type) 排序；sha256 `36c3f111…` 与 discovery 一致。来源核对：`sqlite3 -readonly` runtime 库（mtime 09-16 07:55）`count 77`、distinct 61、分布相同，导出 JSON 与 testdata **逐行相等**；主仓库 `data/hestia.db`（09-02）为 76 条，证实 Leader 注 1 的来源差异。验证者 `TestV_TestdataSelectionShape`：首行 2019-12/annual、末行 2026-08/monthly、非 monthly 恰 9 行 | PASS |
| functional[2] | 35 列锚点；前两列 string，其余 `float64` | test | dev `TestSheetColumnsCoverEntryArea`（len 35、[0]/[1]/[34] Label、前两列 Field 空、其余非空）、`TestSheetColumnFieldsAllExist`（33 个 Field 都在 `fieldOrder`）、`TestBuildRowSendsNumbersAsNumbers`（`IsType` + `reflect.Float64`）、`OmitsAbsentFields`（`"6月"`、`"2026-07-15"`）。验证者 `TestV_SheetColumnsUnique`：Label 唯一、Field 唯一且恰 33 个；fieldOrder 76 字段中 43 个不在录入区（spec §5.5 只定义 35 列，符合） | PASS |
| boundary[0] | C4 两向：NULL 不产生 Cell（缺 3 ⇒ 32）；0 仍要写 | test | dev `OmitsOnlyAbsentFields`（缺 M2/票据/汇率 ⇒ 32 格且三列不在）、`WritesZeroValues`（`m2_yoy:0` ⇒ `float64(0)`，`-5881` 保留）。验证者独立看 `scanObservation`（store.go:560 起）：`vals []sql.NullFloat64`，仅 `if vals[i].Valid { obs.Values[f] = … }` ⇒ NULL=键不存在、0=键存在；`TestV_NullVsZeroThroughRealStore` 走真 Store：Save `{m2:300, m2_yoy:0}` → `Current.Values` 恰 2 键、`m1` 键不存在 → `BuildSheetRows` 4 格、`M2同比==float64(0)`、`M1余额` 不在。变异 M3（`v==0` 视为缺）⇒ dev 与验证者测试均红 | PASS |
| error_handling[0] | >1 monthly ⇒ error 含 Period；同日 tie 按需求处理；`Current` 读不到 ⇒ 报错不跳过 | test | dev `ErrorsOnDuplicateMonthly`（含 `2025-06`、`got nil`）、`ErrorsOnUndecidableTie`、`ErrorsWhenCurrentMissing`（经 `assembleRows` 注入 ok=false ⇒ 错误含 `2025-06/monthly`、rows nil）、`PropagatesCurrentError`（`ErrorIs`）。验证者 V3 补 ctx 取消经 `AllPeriods` 原样带出（`ErrorIs context.Canceled`）。变异 M5（读不到静默跳过）/ M6（重复 monthly 取首条）⇒ 红 | PASS |
| non_functional[0] | C2 `BuildSheetRows` 在父包；守卫登记 + 注释段 + 红留痕；全绿/gofmt/vet；code-simplifier | manual | `BuildSheetRows(ctx, *Store)` 在 `internal/hestia`，子包 `go list -deps ./internal/hestia/sheets` 仍 0 处 `database/sql`/父包。python 解析 `want`：AST 41 → 42，`BuildSheetRows` 在 idx 4、邻居 (`BuildHistory`, `Calibrate`)、仍有序；reflect 15 → 15（`assembleRows`/`currentFunc` 未导出，导出面不变）；diff `+// —— 为什么名单里多了 BuildSheetRows` 1 段，写明「读方法编排、纯数据交出、不碰写路径」；scratchpad `dev-m2b-b-TASK-004-red2-guard.txt` 原始输出 `-([]string) (len=41)`/`+([]string) (len=42)`/`+ "BuildSheetRows"`、`--- FAIL: TestPackageExposesNoWriteFunctions`、`TestStoreExposesNoWriteMethods PASS`；`red1.txt` 是 `undefined: selectRows` 编译红。`GOTOOLCHAIN=local go test ./internal/hestia/... -count=1` exit 0，`-v` 计数 **845 PASS / 0 FAIL**；`gofmt -l` 空；`go vet` exit 0。code-simplifier：4 文件与 `pre-simplifier.sha` 比对，仅 `sheets_project.go`、`sheets_project_test.go` 不同、testdata 与 store_test.go 相同——与 `decisions[3]` 申报范围一致（两文件我已逐行读过：`slices.Sorted(maps.Keys)`、`cellsByLabel` 助手）；本次无法像 002/003 那样逆操作精确重建（改动跨多处、无原文），以 sha 范围 + 逐行审读为据 | PASS |

## 变异测试（worktree 内改 sheets_project.go 副本；每个变异体先 `go vet`、`-timeout 120s`；每次 `git checkout` 还原，前后 sha256 `cde52088…` 一致）

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| M1 monthly 不优先（全按最新发布） | KILLED | dev PrefersMonthly / 77→61 / ReadsStore + 验证者 |
| M2 同日 tie 不报错 | KILLED | dev UndecidableTie + 验证者 V1 |
| M3 值为 0 视为缺失 | KILLED | dev WritesZeroValues + 验证者 V3 |
| M4 Year/Month 取自 PublishedAt | KILLED | dev YearMonthComeFromPeriod / OmitsAbsentFields / ReadsStore |
| M5 Current 读不到静默跳过 | KILLED | dev ErrorsWhenCurrentMissing |
| M6 重复 monthly 不报错取首条 | KILLED | dev ErrorsOnDuplicateMonthly |
| M7 输出不排序（直接遍历 map） | KILLED（仅验证者 V1/V5） | dev 无确定性顺序断言（ReadsStore 两期靠 map 顺序碰运气）；interfaces_exposed 声明「按 Period 升序」，DoD 未明列，非缺陷，建议下游 005 消费时勿依赖 dev 测试守这一点 |
| M8 best 更新不清 tie 标志 | KILLED（仅验证者 V1） | 需要 ≥3 条累计才暴露，dev 夹具最多 2 条；非缺陷 |

## 验证者记录的实际行为（DoD 未定，不判红）
- `buildRow` 对长度 <7 的 `Period`（`"2025-6"`、`""`）会 **panic**（slice bounds）；`"2025-13"` 得 Month 13 不校验；`"abcd-ef"` 得 Year/Month 0。**不可达**：`Meta.validate` 的 `periodRE = \A[0-9]{4}-[0-9]{2}\z`（types.go:88）在写入侧已保证形状，`Current` 读回的 Period 必为 YYYY-MM。
- 同日 tie 文案：`hestia sheets: 2025-09 有多条同日发布的累计期次（q1_q3、annual、h1），无法判定用哪条；实测 2026-09 前不该出现，请核对权威表`。

## 测试质量评审
- 断言均为整切片/整 map 精确相等、`ErrorIs`、`Nil`、`IsType`；无空洞断言。`assembleRows` seam 是最小注入点（函数值），未引入 mock 框架，导出面未变。
- 端到端 `TestBuildSheetRowsReadsStore` 走真 Store（AllPeriods + Current + 选行 + 组装），与单元级测试互补。
- testdata 由与 `AllPeriods` 同一 SQL 导出，快照不连库；数字随库增长会变，测试注释已交代「变了要同步改」。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go，只存在于验证 worktree）
```go
package hestia

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v004 存在）
import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// V1 tie：同 Period 三条累计同日发布 ⇒ error 含 Period 与各 period_type；tie 被更晚一条打破则不报错
func TestV_TieAmongThreeCumulative(t *testing.T) {
	keys := []PeriodKey{
		{"2025-09", "q1_q3", "2025-10-13"},
		{"2025-09", "annual", "2025-10-13"},
		{"2025-09", "h1", "2025-10-13"},
	}
	got, err := selectRows(keys)
	require.Error(t, err)
	require.Nil(t, got)
	for _, s := range []string{"2025-09", "q1_q3", "annual", "h1", "同日"} {
		require.Contains(t, err.Error(), s)
	}
	t.Logf("tie 文案: %s", err.Error())
	// 打平的两条之后来一条更晚的 ⇒ 不报错，取最晚
	keys2 := append(keys[:2:2], PeriodKey{"2025-09", "h1", "2025-10-20"})
	got, err = selectRows(keys2)
	require.NoError(t, err)
	require.Equal(t, []PeriodKey{{"2025-09", "h1", "2025-10-20"}}, got)
	// 最晚在前、后面两条打平 ⇒ 不报错
	keys3 := []PeriodKey{{"2025-09", "h1", "2025-10-20"}, {"2025-09", "q1_q3", "2025-10-13"}, {"2025-09", "annual", "2025-10-13"}}
	got, err = selectRows(keys3)
	require.NoError(t, err)
	require.Equal(t, "h1", got[0].PeriodType)
	// monthly 与同日累计并存 ⇒ monthly 赢，不报 tie
	keys4 := append(keys, PeriodKey{"2025-09", "monthly", "2025-10-13"})
	got, err = selectRows(keys4)
	require.NoError(t, err)
	require.Equal(t, "monthly", got[0].PeriodType)
	// 输出按 Period 升序，与输入顺序无关
	mixed := []PeriodKey{{"2026-03", "q1", "2026-04-13"}, {"2019-12", "annual", "2020-01-16"}, {"2025-06", "monthly", "2025-07-14"}, {"2025-06", "h1", "2025-07-15"}}
	got, err = selectRows(mixed)
	require.NoError(t, err)
	require.True(t, sort.SliceIsSorted(got, func(i, j int) bool { return got[i].Period < got[j].Period }))
	require.Len(t, got, 3)
	// 空输入
	got, err = selectRows(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

// V2 Period 异常形状：只记录行为（DoD 未定）
func TestV_BuildRowOddPeriods(t *testing.T) {
	try := func(period string) (row any, panicked any) {
		defer func() { panicked = recover() }()
		r := buildRow(Observation{Meta: Meta{Period: period, PublishedAt: "2026-01-01"}})
		return fmt.Sprintf("Year=%d Month=%d cells=%d 月份=%v", r.Year, r.Month, len(r.Cells), r.Cells[0].Value), nil
	}
	for _, p := range []string{"2025-13", "2025-6", "2025", "", "abcd-ef", "2025-06-01"} {
		row, pan := try(p)
		t.Logf("Period=%q ⇒ row=%v panic=%v", p, row, pan)
	}
}

// V3 真 Store 路径上的 NULL/0：只写 m2 与 m2_yoy=0 ⇒ Current 的 Values 恰含这两键，其余键不存在；BuildSheetRows 得 4 格且 M2同比==float64(0)
func TestV_NullVsZeroThroughRealStore(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Save(context.Background(), Observation{
		Meta:   Meta{Period: "2025-06", PeriodType: "monthly", PublishedAt: "2025-07-14", ArticleID: "art-v3", CaliberVersion: "2025-01", Extractor: extractorV2},
		Values: map[string]float64{FieldM2: 300, FieldM2YoY: 0},
	}, passing())
	require.NoError(t, err)
	obs, ok, err := st.Current(context.Background(), "2025-06", "monthly")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, obs.Values, 2)
	_, hasYoY := obs.Values[FieldM2YoY]
	require.True(t, hasYoY)
	require.Equal(t, float64(0), obs.Values[FieldM2YoY])
	_, hasM1 := obs.Values[FieldM1]
	require.False(t, hasM1, "NULL 字段不应出现在 Values")

	rows, err := BuildSheetRows(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Cells, 4)
	by := cellsByLabel(rows[0])
	require.Equal(t, float64(0), by["M2同比"])
	require.Equal(t, float64(300), by["M2余额"])
	require.NotContains(t, by, "M1余额")
	// ctx 取消经 AllPeriods 原样带出
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = BuildSheetRows(ctx, st)
	require.ErrorIs(t, err, context.Canceled)
}

// V4 SheetColumns 完整性：Label/Field 各自唯一；fieldOrder 中未映射的字段列出来（只记录）
func TestV_SheetColumnsUnique(t *testing.T) {
	labels, fields := map[string]bool{}, map[string]bool{}
	for _, c := range SheetColumns {
		require.Falsef(t, labels[c.Label], "重复 Label %q", c.Label)
		labels[c.Label] = true
		if c.Field != "" {
			require.Falsef(t, fields[c.Field], "重复 Field %q", c.Field)
			fields[c.Field] = true
		}
	}
	require.Len(t, fields, 33)
	var unmapped []string
	for _, f := range fieldOrder {
		if !fields[f] {
			unmapped = append(unmapped, f)
		}
	}
	t.Logf("fieldOrder 共 %d 个字段，未映射到录入区的 %d 个: %v", len(fieldOrder), len(unmapped), unmapped)
}

// V5 用 testdata 跑 selectRows，与独立 python 复算的首末/计数对齐；且所有无 monthly 月份都保留了一行
func TestV_TestdataSelectionShape(t *testing.T) {
	keys := loadRealPeriodKeys(t)
	got, err := selectRows(keys)
	require.NoError(t, err)
	require.Len(t, got, 61)
	require.Equal(t, PeriodKey{"2019-12", "annual", "2020-01-16"}, got[0])
	require.Equal(t, PeriodKey{"2026-08", "monthly", "2026-09-14"}, got[60])
	nonMonthly := 0
	for _, k := range got {
		if k.PeriodType != "monthly" {
			nonMonthly++
		}
	}
	require.Equal(t, 9, nonMonthly)
	require.Equal(t, 52, 61-nonMonthly)
}
```
