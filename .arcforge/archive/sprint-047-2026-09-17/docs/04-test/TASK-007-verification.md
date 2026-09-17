# TASK-007 验证报告 — sheets.Push 编排与 dry-run（httptest 他证零写）

- 验证者: test-m2b-a　　时间: 2026-09-16T15:51:19Z
- 判定对象: master @ `e0f4f8b4fdca4575d7b1bd1010126e85c69d8697`（= verify_baseline.head，merge commit）；交付 commit `5b18b1b8d83fe6e38ef061e8cab56d56511a6a83`（已在 e0f4f8b 祖先链）；base `e923173d9cbbe72e36ffb9fd1bcd0502d91f0c89`
- discovery sha256: `39f6417935a2206d6509a86cca5aad8cc824552385a22c2d74603a93c712beb0`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）
- provenance：代码由 dev-m2b-b 编写（含 code-simplifier），dev-m2b-a 按 AD-21 接手（epoch 1→2）。验证者核 `scratchpad/handover-TASK-007/SHA256SUMS`：push.go / push_test.go / store_test.go 三文件 sha256 与交付 commit **逐字节一致** ⇒ dev-m2b-a 未改一行；`red_phase` 引用的是 dev-m2b-b 留痕。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v007`；测试在 `HTTP_PROXY=HTTPS_PROXY=http://127.0.0.1:9`（不可达代理）下运行，httptest 是唯一出口。
- 范围核对: `git diff --stat e923173..e0f4f8b` 恰为 `writes` 的 3 个文件（push.go 132/0、push_test.go 198/0、store_test.go 6/1），无越界；go.mod/go.sum 无 diff。

## 结论：**VERIFIED**（6/6 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `Options{Apply, CreateSheets}`、`Result{Changes, MissingTabs, WillWrite, Same, AbsentInDB}`、`Push(ctx, c *Client, rows, wantLabels, opts) (Result, error)` 无 `*Store`/`*sql.DB`；表名 `%d年` | test | push.go:12–24 两类型逐字；:48 签名逐字，文件中 `Store`/`sql.` 出现 0 次；:27 `tabName` = `"%d年"`。dev `TestPushApplyWritesOnlyWillWrite` 断 `'2026年'!C9`；验证者 `TestV_ApplyPerTabColumns` 断 `'2025年'!A4`/`'2025年'!B15`。变异 M6（表名无「年」）⇒ 10 条测试全红 | PASS |
| functional[1] | 判据三：`Apply=false` ⇒ 全部请求 GET（逐条 `HasPrefix`），`res.WillWrite > 0` | test | dev `TestPushDryRunSendsNoWriteRequest`：`Greater(WillWrite, 0)`（=1）、`requireOnlyGETs` 逐条 `require.Truef(HasPrefix(m,"GET "))`、`writeBodies` 空。验证者 `TestV_DryRunIndependent`（独立夹具：2025/2026 两张表列序不同、2025 表短行 `["","1月"]`、含 0 值与越界月）：`WillWrite=4 > 0`、5 个请求（Tabs 1 + 两表各 header/entry）**逐条 GET**、写体空。变异 M1（短路挪到 WriteCells 之后）⇒ dev 2 条 + 验证者 2 条红 | PASS |
| functional[2] | `Apply=true` ⇒ `len(Data)==WillWrite`；不变量 `W+S+A == rows×labels` | test | dev `TestPushApplyWritesOnlyWillWrite`（`Len(Data, WillWrite)`、RAW、`requireInvariant` 1×4）。验证者 `TestV_ApplyPerTabColumns`：Data 4 == WillWrite 4，四个 range 值逐一核（462.06 / 400.0 / `float64(0)` / `"12月"`），Same 与 Absent 的格**不在**写体，POST 是最后一个请求且之前全 GET；`TestV_DryRunIndependent` 不变量 3×4 = 12 = 4+3+5 且 `len(Changes)=12`。变异 M2（写全部 Changes）/ M9（Same 计入 WillWrite）⇒ 红 | PASS |
| error_handling[0] | 表头缺任一标签 ⇒ error 且 `writeBodies(rec)` 空 | test | dev `TestPushRejectsUnknownLabel`（含 `M2余额`、写体空）。验证者 `TestV_RejectOrdering`：缺标签发生在第二张表（第一张已读完）⇒ 文案含 `2026年` 与 `M2余额`、写体空。变异 M4（缺标签跳过该表继续）⇒ 红 | PASS |
| error_handling[1] | 缺年度表且 `CreateSheets=false` ⇒ 文案同时含表名与 `--create-sheets`、`MissingTabs` 列全、零写 | test | dev `TestPushRefusesMissingTabs`（含 `2019年` 与 `--create-sheets`、`MissingTabs` 七张有序、写体空）。验证者：缺表拒绝发生在读表头**之前**（只 1 个 GET）、`MissingTabs=["2019年"]`；读侧 503 ⇒ 错误含 `Service Unavailable`、零写。变异 M3（不拒绝）/ M7（文案无 `--create-sheets`）/ M8（不排序）⇒ 红 | PASS |
| non_functional[0] | 守卫登记 `sheets.Push` + 注释段 + 红留痕；全绿/gofmt/vet；code-simplifier；**不得放松 GET-only** | manual | python 解析 `want` AST 49 → 50，`sheets.Push` idx 47、邻居 (`sheets.NewClient`, `sheets.ResolveHeader`)、有序；reflect 15 不变；注释段挂既有「sheets.*」节（「签名里没有 *Store / *sql.DB…dry-run 由 httptest 他证」）；`dev-m2b-b-TASK-007-red2-guard.txt` 原始 `-([]string) (len=49)`/`+([]string) (len=50)`/`+ "sheets.Push"`、`--- FAIL: TestPackageExposesNoWriteFunctions`、reflect PASS；`red1.txt` 是 `undefined: Result/Push/Options` 编译红。`go test ./internal/hestia/... -count=1 -v` exit 0，**870 PASS / 0 FAIL**；`gofmt -l` 空；`go vet` exit 0；子包 deps database/sql=0、父包=0。GET-only 断言仍是逐条 `HasPrefix`（push_test.go:81–86）。code-simplifier：dev-m2b-b 的 `pre-simplifier.sha` 显示 push.go / push_test.go 被 simplifier 改过、store_test.go 未改；前版只有 sha 无内容，不可逆操作重建；验证者逐行审读交付树两文件（本报告全部判定基于它），dev-m2b-a 接手未再跑 simplifier（`decisions[1]`，Leader 指示） | PASS |

## 变异测试（worktree 内改 push.go 副本；每个变异体先 `go vet`、`-timeout 120s`、出网被堵；每次 `git checkout` 还原，前后 sha256 `be8503e3…` 一致）

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| M1 dry-run 短路挪到 WriteCells 之后 | KILLED | dev DryRunSendsNoWriteRequest / DryRunReportsMissingTabsWithCreateFlag + 验证者 2 条 |
| M2 写全部 Changes（含 Same/Absent） | KILLED | dev ApplyWritesOnlyWillWrite + 验证者 |
| M3 缺表不拒绝 | KILLED | dev RefusesMissingTabs + 验证者 |
| M4 缺标签跳过该表继续 | KILLED | dev RejectsUnknownLabel + 验证者 |
| M5 建表挂点挪到写之后 | KILLED | dev CreateSheetsHookRunsBeforeWrite |
| M6 表名不带「年」 | KILLED | 10 条全红 |
| M7 缺表文案不提 `--create-sheets` | KILLED | dev RefusesMissingTabs + 验证者 |
| M8 MissingTabs 不排序 | KILLED | dev RefusesMissingTabs |
| M9 计数把 Same 算进 WillWrite | KILLED | dev 3 条 + 验证者 3 条 |
| M10 缺表的行也参与 diff | KILLED | dev DryRunReportsMissingTabsWithCreateFlag / CreateSheetsHookRunsBeforeWrite + 验证者 |

## 验证者记录的实际行为（DoD 未定，不判红）
- 缺表 + `CreateSheets=true` + dry-run：不报错、`MissingTabs` 列全、缺表的行**不进 Changes**，不变量只覆盖已有表的行（discovery key_findings[3] 已声明；建表是 TASK-008 的活）。
- `rows` 为空 ⇒ 只调一次 `Tabs`，`Result` 零值，零写。
- `createTabs` 挂点本任务恒报错并提 TASK-008；Apply + CreateSheets + 缺表 ⇒ 报错且已有表的格也不写（dev 测试钉住）。
- `Push` 只把 `WillWrite` 的 `Change` 交给 `WriteCells`，不读 `Same` 的 `Want`（005 报告 M10 提示的风险不成立）。
- 验证者自报：首版夹具把 WillWrite 数成 5（真值 4，2025/1 行的「发布日期」是 Absent 不是 WillWrite），是验证者算术错；修正后 4/4 PASS 并重跑全部变异，结论不变。

## 测试质量评审
- dry-run 的「没动表」由 httptest 服务端逐请求他证，而非检查代码里的 `if !opts.Apply`；`writeBodies` 只收非 GET，`/token` 不入 recorder。
- `requireInvariant` 在 dry-run 与 apply 两条路径都断言；缺表/缺标签两种拒绝都断「零写」。
- 断言均精确（Equal 整切片、Len、Contains、Truef 逐条），无 mock。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go，只存在于验证 worktree）
```go
package sheets

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v007 存在）
import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	vHeader25 = "/v4/spreadsheets/sheet-id/values/'2025年'!A3:AI3"
	vEntry25  = "/v4/spreadsheets/sheet-id/values/'2025年'!A4:AI15"
)

// 两张年度表：2025 表列序 [社融存量, 月份, 发布日期, M2余额]，2026 表列序 [月份, 发布日期, 社融存量, M2余额]
func vTwoYearResponses() map[string]string {
	return map[string]string{
		pathTabs:     `{"sheets":[{"properties":{"title":"2025年"}},{"properties":{"title":"2026年"}},{"properties":{"title":"说明"}}]}`,
		vHeader25:    `{"values":[["社融存量","月份","发布日期","M2余额"]]}`,
		vEntry25:     `{"values":[["", "1月"]]}`, // 短行：1 月只有两格
		pathHeader26: `{"values":[["月份","发布日期","社融存量","M2余额"]]}`,
		pathEntry26:  `{"values":[["1月"],["2月"],["3月"],["4月"],["5月"],["6月","2026-07-15",999.0,300.0]]}`,
	}
}

func vRows() []Row {
	return []Row{
		{Year: 2026, Month: 6, Cells: []Cell{{Label: "月份", Value: "6月"}, {Label: "发布日期", Value: "2026-07-15"}, {Label: "社融存量", Value: 462.06}}},
		{Year: 2025, Month: 1, Cells: []Cell{{Label: "月份", Value: "1月"}, {Label: "社融存量", Value: 400.0}, {Label: "M2余额", Value: 0.0}}},
		{Year: 2025, Month: 12, Cells: []Cell{{Label: "月份", Value: "12月"}}},
	}
}

// V1 独立 dry-run 他证：多年度 + 各表列序不同 + 短行；WillWrite>0、全 GET、零写体、不变量
func TestV_DryRunIndependent(t *testing.T) {
	c, rec := newTestClient(t, vTwoYearResponses(), 0)
	res, err := Push(context.Background(), c, vRows(), sampleLabels(), Options{Apply: false})
	require.NoError(t, err)
	require.Greater(t, res.WillWrite, 0)
	require.NotEmpty(t, rec.methods)
	for _, m := range rec.methods {
		require.Truef(t, strings.HasPrefix(m, "GET "), "非 GET：%s", m)
	}
	require.Empty(t, writeBodies(rec))
	require.Equal(t, 3*4, res.WillWrite+res.Same+res.AbsentInDB)
	require.Len(t, res.Changes, 12)
	// 2026/6：月份 Same、发布日期 Same、社融存量 WillWrite、M2余额 Absent；
	// 2025/1：月份 Same（短行第 2 格 "1月"，列序按 2025 表）、社融存量 WillWrite（现值 ""=无值）、M2余额 0 WillWrite、发布日期 Absent；
	// 2025/12：月份 WillWrite、其余 3 Absent ⇒ W 1+2+1=4、S 2+1+0=3、A 1+1+3=5（验证者首版把 W 数成 5，是自己的算术错）
	require.Equal(t, 4, res.WillWrite)
	require.Equal(t, 3, res.Same)
	require.Equal(t, 5, res.AbsentInDB)
	require.Empty(t, res.MissingTabs)
	// 2025 表按自己的表头取列：社融存量 在 A（col 0）、月份在 B（col 1）
	for _, ch := range res.Changes {
		if ch.Sheet == "2025年" && ch.Label == "社融存量" {
			require.Equal(t, 0, ch.Col)
		}
		if ch.Sheet == "2025年" && ch.Label == "月份" {
			require.Equal(t, 1, ch.Col)
		}
	}
	// 读请求：Tabs 1 次 + 每张已有表 header+entry 各 1 次（"说明" 表不读）
	require.Len(t, rec.methods, 1+2*2)
}

// V2 Apply：写体 Data 数 == WillWrite，range 按各表列序，0 值原样、Same/Absent 不在写体
func TestV_ApplyPerTabColumns(t *testing.T) {
	c, rec := newTestClient(t, vTwoYearResponses(), 0)
	res, err := Push(context.Background(), c, vRows(), sampleLabels(), Options{Apply: true})
	require.NoError(t, err)
	require.Len(t, writeBodies(rec), 1)
	var got batchBody
	require.NoError(t, json.Unmarshal([]byte(lastWriteBody(t, rec)), &got))
	require.Len(t, got.Data, res.WillWrite)
	require.Equal(t, 4, len(got.Data))
	ranges := map[string]any{}
	for _, d := range got.Data {
		ranges[d.Range] = d.Values[0][0]
	}
	require.Equal(t, 462.06, ranges["'2026年'!C9"])
	require.Equal(t, 400.0, ranges["'2025年'!A4"])   // 社融存量 在 2025 表的 A 列
	require.Equal(t, float64(0), ranges["'2025年'!D4"]) // M2余额 0 值也写
	require.Equal(t, "12月", ranges["'2025年'!B15"])
	require.NotContains(t, ranges, "'2026年'!A9") // Same 不写
	require.NotContains(t, ranges, "'2026年'!D9") // Absent 不写
	// 写请求发生在全部读之后
	require.True(t, strings.HasPrefix(rec.methods[len(rec.methods)-1], "POST "))
	for _, m := range rec.methods[:len(rec.methods)-1] {
		require.True(t, strings.HasPrefix(m, "GET "))
	}
}

// V3 rows 空 ⇒ 只调 Tabs，零写，Result 全 0；缺表 + CreateSheets + dry-run：不变量只覆盖已有表的行（记录）
func TestV_EmptyRowsAndMissingTabDryRun(t *testing.T) {
	c, rec := newTestClient(t, vTwoYearResponses(), 0)
	res, err := Push(context.Background(), c, nil, sampleLabels(), Options{Apply: true})
	require.NoError(t, err)
	require.Equal(t, Result{}, res)
	require.Empty(t, writeBodies(rec))
	t.Logf("rows 空时请求: %v", rec.methods)

	c2, rec2 := newTestClient(t, vTwoYearResponses(), 0)
	rows := append(vRows(), Row{Year: 2019, Month: 12, Cells: []Cell{{Label: "月份", Value: "12月"}}})
	res, err = Push(context.Background(), c2, rows, sampleLabels(), Options{Apply: false, CreateSheets: true})
	require.NoError(t, err)
	require.Equal(t, []string{"2019年"}, res.MissingTabs)
	requireOnlyGETs(t, rec2)
	t.Logf("缺表+CreateSheets+dry-run：Changes=%d（3 行×4=12，缺表那行不计）sum=%d", len(res.Changes), res.WillWrite+res.Same+res.AbsentInDB)
	require.Equal(t, 12, res.WillWrite+res.Same+res.AbsentInDB)
}

// V4 缺标签发生在第二张表：第一张表已读完也不写；缺表且 !CreateSheets 时连 header 都不读
func TestV_RejectOrdering(t *testing.T) {
	m := vTwoYearResponses()
	m[pathHeader26] = `{"values":[["月份","发布日期","社融存量"]]}` // 2026 表缺 M2余额
	c, rec := newTestClient(t, m, 0)
	_, err := Push(context.Background(), c, vRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2026年")
	require.Contains(t, err.Error(), "M2余额")
	require.Empty(t, writeBodies(rec))

	c2, rec2 := newTestClient(t, vTwoYearResponses(), 0)
	res, err := Push(context.Background(), c2, append(vRows(), Row{Year: 2019, Month: 1}), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2019年")
	require.Contains(t, err.Error(), "--create-sheets")
	require.Equal(t, []string{"2019年"}, res.MissingTabs)
	require.Len(t, rec2.methods, 1, "缺表拒绝应在读表头之前")
	require.Empty(t, writeBodies(rec2))

	// 读侧 5xx ⇒ 原样带出且零写
	c3, rec3 := newTestClient(t, nil, 503)
	_, err = Push(context.Background(), c3, vRows(), sampleLabels(), Options{Apply: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Service Unavailable")
	require.Empty(t, writeBodies(rec3))
}
```
