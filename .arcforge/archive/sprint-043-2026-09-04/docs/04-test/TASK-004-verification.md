# TASK-004 验证报告 · Sender 窄接口与 P0/P1/P2 三类消息渲染（纯函数）

- 验证者：test-m1d-b（第 2 轮；第 1 轮由 test-m1d-a）　判定：**VERIFIED（第 2 轮，epoch 2，rework 1，review_fix 第 1 轮返工）**
- 第 2 轮 verify_baseline.head：`b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`（master，验证时 HEAD 同值）；discovery sha256 与基线一致 `c438df56ec4c9acbc0b9b32571213af7e7c9b47e14f8c30317487cbf62696a5a`
- 返工 commit：`bef5572b1edf1a919360be345750694f7d09ec4d`（父 `f3d6eb282c83e0ca730b1713907f5220114ee86b`；merge `687e43ac5265292734be7cf444e36452dc2e356d`），改 `internal/hestia/notify.go` 13+/2-、`internal/hestia/notify_test.go` 17+/0-；`git show --name-only` 恰这两文件，在 `writes` 内
- provenance（discovery `rework[0]` 如实记录）：代码由 dev-m1d-c 编写并提交，dev-m1d-c 撞账号上限失联后 Leader 改派 dev-m1d-d（epoch 2）做 master 重采、补 discovery 与状态推进，未改代码。验证者核对：`git log --format=%cI bef5572` = 12:36:33Z，早于 dev-m1d-d 认领（15:22:45Z）
- 验证树：`git worktree add --detach ../wt-verify-TASK-004-007-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`（与 TASK-007 共用，串行）；变异在该树副本上做，每次后 `git checkout --` 还原、核实 sha256 前缀 `eabee8c1f601469e` 与 porcelain 为 0；主仓库 `notify.go` 指纹 harness 前后同值

## 1. 结论

QA 终审 A7（Leader 拍板）已落地：`renderP2` 在 `out.Verdict == bitemporal.Duplicate` 时把「入库」改为「已在库（本次抽取值未写入）」，其它 Verdict 措辞不变；新增 `TestRenderP2DuplicateSaysValuesNotWritten` 同时守住两个方向（Duplicate 含「未写入」且仍含 `Duplicate`、不含「入库 Duplicate」；New/Revision/OutOfOrder 不含「未写入」且仍是「入库 <Verdict>」）。三组 A7 判据变异各自转红且只红这一条用例。首轮已过的全部项无回退：基线两包 `-count=1` 全绿，字面量守卫与导出面守卫在全包内绿，回归变异 N8（吞 Verdict）KILLED。门禁全绿、覆盖率 96.4%（A/B 背对背两轮 `bef5572` 与父 `f3d6eb2` 均 96.4%）、提交锚匹配、merge 早于 `dev_done`。

## 2. 覆盖矩阵（第 2 轮：fix_items 逐条 + 首轮项回退检查）

| # | 标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| fix A7 | Duplicate ⇒ 含「未写入」且仍含 Duplicate；New ⇒ 不含「未写入」；New/Revision/OutOfOrder 措辞不变 | `notify.go:66-69` `action` 二选一 + 理由注释；`TestRenderP2DuplicateSaysValuesNotWritten`（Duplicate 分支 3 条断言 + 三种 Verdict 循环各 2 条）；变异 N-A7a（条件取反）/ N-A7b（分支改回「入库」）/ N-A7c（条件改 Revision）⇒ 各自 `--- FAIL: TestRenderP2DuplicateSaysValuesNotWritten`，FAIL 数 1 | PASS |
| fix 非功能 | 只改 `notify.go`/`notify_test.go`；`Field*` 常量守卫与导出面守卫绿；锚 `fix(TASK-004): M1d …`；完整交付流程 | `git show --name-only bef5572` 恰两文件；`TestFieldNamesAppearOnlyInFieldsGo`、`TestPackageExposesNoWriteFunctions` 在基线全包绿内（notify.go 新增 import 仅 `bitemporal`，无字段名字面量）；提交信息 `fix(TASK-004): M1d renderP2 …` 匹配锚；merge `687e43a` 12:37:41Z 早于 `dev_done` 15:24:53Z（`transitions.jsonl`）；discovery `rework[0]` 含 fix_commit / merge_commit / master_sample（锚 `b47e440`，content_check 两文件 sha256 与 bef5572 一致）/ provenance；`wt-TASK-004-m1d-fix` 已拆（`git worktree list` 无） | PASS |
| functional[0..2] | 首轮已 PASS，本轮查回退 | `TestRenderP0ListsOnlyFailedChecks`、`TestRenderP1KeepsFirstLineOnly`、`TestRenderP2CarriesVerdictAndAnchors` 基线绿；回归 N8（P2 吞 Verdict）⇒ 4 用例红（CarriesVerdictAndAnchors / DuplicateSaysValuesNotWritten / ingest 层 `TestIngestNotifiesP2OnObservation`、`TestForceOnObservedPeriodIsDuplicate`） | PASS |
| boundary[0] / error_handling[0] | 同上 | `TestRenderP2MissingAnchorIsNA`、`TestRenderP2UsesMomWhenYtdAbsent`、`TestRenderP2BothFlowsAbsent`、`TestRenderP2PrefersYtdWhenBothPresent`、`TestRenderP0NilValueIsNA` 基线绿 | PASS |
| non_functional[0] | 只用 `Field*` 常量；非导出；纯函数 | 返工 diff 未引入 I/O 或字段名字面量；两守卫绿 | PASS |
| non_functional[1] 门禁 | gofmt / vet / 两包 / ≥ 96.3% / 五个不动文件 / 无新依赖 | 树 `b47e440`：`gofmt -l` 仅 `backtest_test.go`、`crisis_test.go`；`go vet` rc=0；两包 `-count=1` ok；`internal/hestia` **96.4%**；A/B 背对背两轮 `bef5572` 96.4% / `f3d6eb2` 96.4%；`git diff --stat 4916106 b47e440 -- {store,validate,parse,extract,fields}.go` 0 行；`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行；注释写 `M1d 的 TASK-004 返工` | PASS |
| non_functional[2] 交付流程 | 见 fix 非功能行 | — | PASS |

**越界申报核对**：`git show --name-only bef5572` ⇒ `internal/hestia/notify.go`、`internal/hestia/notify_test.go`，与 `writes` 完全一致；两文件在 `687e43a` 与 `b47e440` 逐字节一致（TASK-007 的 merge 未碰它们）。

**下游一致性**：TASK-005 的 R-005a 断言（`TestForceOnObservedPeriodIsDuplicate` 要求 `texts` 恰 1 且含 `Duplicate`）在新措辞下仍绿；措辞变更不影响 ingest 层契约。

## 3. 变异汇总（第 2 轮，4/4 KILLED）

| 变异 | 期望转红 | 实测 |
|---|---|---|
| N-A7a `== Duplicate` → `!= Duplicate`（**A7 判据**） | DuplicateSaysValuesNotWritten | KILLED（FAIL 数 1） |
| N-A7b 分支内改回「入库」（**A7 判据**） | DuplicateSaysValuesNotWritten | KILLED（FAIL 数 1） |
| N-A7c 条件改 `Revision`（**A7 判据**） | DuplicateSaysValuesNotWritten | KILLED（FAIL 数 1） |
| N8-reg P2 吞 Verdict（回归） | CarriesVerdictAndAnchors | KILLED（4 用例红） |

每组：`python3` 字面替换 → `go build ./internal/hestia/` 有效性闸 → 打印 diff → `go test ./internal/hestia/ -count=1` 全包 → `git checkout --` 还原 → sha256 前缀与 porcelain 核实。

## 4. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-004-007-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b
cd ../wt-verify-TASK-004-007-m1d-b
GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestRenderP2DuplicateSaysValuesNotWritten|TestRenderP|TestFieldNamesAppearOnlyInFieldsGo|TestPackageExposesNoWriteFunctions' -v -count=1   # 全 PASS
# A7 判据：notify.go 把 `if out.Verdict == bitemporal.Duplicate {` 改成 `!=`
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestRenderP2DuplicateSaysValuesNotWritten -count=1   # ⇒ FAIL
git checkout -- internal/hestia/notify.go
# A/B 覆盖率（背对背两轮）
git checkout --detach bef5572b1edf1a919360be345750694f7d09ec4d && GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 96.4%
git checkout --detach f3d6eb282c83e0ca730b1713907f5220114ee86b && GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 96.4%
```

## 5. 第 1 轮记录（test-m1d-a，VERIFIED，epoch 1）

- verify_baseline.head `2f5ad513f7c0e7539c824e2e8f8a0f078baec316`（master）；discovery sha256 `0dafc1fa…7f34`；dev commit `f8e3c8e68dc97e728d3a65adbc57966d1bf2c1e3`（父 `ae088eb253b64b36e10558a02587e3fa657f5f3e`；merge `69cb71d`）；A/B 覆盖率钉在 `f8e3c8e` 与 `ae088eb`（96.4% / 96.3%）。
- 结论：`notify.go` 与需求原文逐字一致（仅注释加 milestone 前缀与两句说明）；9 条测试全绿；8 组变异 7 组 KILLED（N1 P0 不过滤 / N2 P1 不截首行 / N4 anchorFlow 先查 mom / N5 去 nil 守卫 / N6 缺失写 0 / N7 两缺标 ytd / N8 P2 吞 Verdict），唯一存活 N3（`fmtNum` 改 `%g`）判定为非缺陷：Go `%g` 对 177600 本就打 `177600`（≥ 1e6 才切指数记法），需求原文「`%g` 会把 177600 打成 `1.776e+05`」这句理由为假，但实现按 DoD 用 `FormatFloat('f', -1, 64)`，DoD 字面满足。六个函数逐函数覆盖率 100%，字面量守卫与导出面守卫绿。
- 覆盖矩阵：functional[0] `TestRenderP0ListsOnlyFailedChecks`；functional[1] `TestRenderP1KeepsFirstLineOnly`；functional[2] `TestRenderP2CarriesVerdictAndAnchors`（格式串与 DoD 逐字一致）；boundary[0] `TestRenderP2MissingAnchorIsNA`、`TestRenderP2UsesMomWhenYtdAbsent`、`TestRenderP2BothFlowsAbsent`、R-004 `TestRenderP2PrefersYtdWhenBothPresent`；error_handling[0] `TestRenderP0NilValueIsNA`（`require.NotPanics`）；non_functional 三项（守卫、门禁于 `2f5ad51`、交付流程：锚匹配、merge `69cb71d` 早于 `dev_done` 03:13:08Z、discovery 三个锚）——全部 PASS。
- 发现（不阻断）：`%g` 理由为假，dev decision「NotContains `1.776e+05` 把 `%g` 错误路径钉死」不成立（该断言在任何实现下都不可能失败）；若要真钉住，一行 `assert.Equal(t, "1776000", fmtNum(1776000))` 即可杀死 N3。建议 TASK-008 写 CONTRACTS §A 时把理由改成「≥ 1e6 的值 `%g` 会切指数记法」。
- 复现：`git worktree add --detach ../wt-verify-TASK-004-m1d 2f5ad513f7c0e7539c824e2e8f8a0f078baec316` 后 `GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestRenderP|TestFieldNamesAppearOnlyInFieldsGo|TestPackageExposesNoWriteFunctions' -v -count=1`。
