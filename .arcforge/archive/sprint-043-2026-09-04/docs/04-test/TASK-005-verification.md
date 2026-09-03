# TASK-005 验证报告 · ingest 接通知：pending⇒P0、权威表⇒P2、失败⇒P1，发送失败响亮但不级联

- 验证者：test-m1d-b（第 3 轮；第 1–2 轮由 test-m1d-a）　判定：**VERIFIED（第 3 轮，epoch 2，rework 2，review_fix 第 1 轮返工）**
- 第 3 轮 verify_baseline.head：`b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`（master，验证时 HEAD 同值）；discovery sha256 与基线一致 `282984013ff742e73b3c71659a9b4bd3fa1e0631e8149a70c70ed3a5fe0cdb00`
- 返工 commit：`ec93e13475a5a100e50fb93859820362abea2299`（父 `f3d6eb282c83e0ca730b1713907f5220114ee86b`；merge `208a77c5e7da5038304e42b00c432300385d23f2`），改 `internal/hestia/ingest.go` 3+/1-、`internal/hestia/ingest_test.go` 30+/2-；`git show --name-only` 恰这两文件，在 `writes` 内
- 基线与 merge 之间（`208a77c..b47e440`）只有 TASK-004/007 返工的 merge，对 `ingest.go`/`ingest_test.go` 的 numstat 为空（派验消息里「004 返工顺带改了 ingest.go 1/3」不成立：`bef5572` 只改 `notify.go`/`notify_test.go`，那组数字是 `ec93e13` 的反向 diff）
- 验证树：`git worktree add --detach ../wt-verify-TASK-005-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`；变异在该树副本上做，每次后 `git checkout --` 还原、核实 sha256 前缀 `b767d986e4a36c42` 与 porcelain 为 0；主仓库 `ingest.go` 指纹 harness 前后同值
- 改派说明：原 verifier test-m1d-a 529 唤不回，Leader 15:21:52Z 走 `verifying → verifying` 逃生边改派，基线不刷新；本报告覆盖写，前两轮记录保留在 §5

## 1. 结论

QA 终审（`docs/05-review/verdict.md`）挂在本任务的三条 fix_items 全部落地且各有独家守卫：A4 汇总行分子改用 `len(failedPeriods)`，`TestIngestP1SendFailureIsReported` 末尾断言含 `1/1 期失败`；A8-a 新增 `TestIngestNotifyFailureOnPendingIsLoud`；A8-b `TestIngestFailsPeriodWhenSnapshotUnwritable` 改为 wrap 前缀形态 `(<id>): snapshot: `。三条判据变异各自转红且**只红那一条用例**（每组 FAIL 数 = 1）。前两轮已过的九条无回退：基线两包 `-count=1` 全绿，回归变异 M1/M5/M6 仍 KILLED。门禁全绿、覆盖率 96.4%（A/B 背对背两轮 `ec93e13` 与父 `f3d6eb2` 均 96.4%，无变化）、可选项未做、提交锚匹配、merge 早于 `dev_done`。

## 2. 覆盖矩阵（第 3 轮：fix_items 逐条 + 上一轮项回退检查）

| # | 标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| fix A4 | 汇总行分子用 `len(failedPeriods)`；Parse 失败 + sender 失败 ⇒ 含 `1/1 期失败` | `ingest.go:205` 改用 `len(failedPeriods)` 带理由注释；`TestIngestP1SendFailureIsReported` 末尾 `ErrorContains "1/1 期失败"`；变异 M-A4（改回 `len(errs)`）⇒ `--- FAIL: TestIngestP1SendFailureIsReported`，FAIL 数 1 | PASS |
| fix A8-a | 落 pending + 发送失败 ⇒ 错误链含 `(<id>): send P0: notify: `、pending=1、texts 恰 1 | 新增 `TestIngestNotifyFailureOnPendingIsLoud`（`DepositSumTolerance=1e-9` + `fakeSender{err: errBoom}`）：`ErrorContains "("+annualID+"): send P0: notify: "`、`ErrorIs errBoom`、`countRows(TablePending)==1`、`Len(texts,1)`；变异 M-A8a（`send P0`→`send P2`）⇒ 该用例独家 FAIL | PASS |
| fix A8-b | Snapshot 用例改 wrap 前缀形态 | `TestIngestFailsPeriodWhenSnapshotUnwritable` 改为 `ErrorContains "("+annualID+"): snapshot: "`（其余断言不动）；变异 M-A8b（`wrap("snapshot"`→`wrap("fetch"`）⇒ 该用例独家 FAIL | PASS |
| fix 非功能 | 只改两文件；可选项不做；全包绿、覆盖率 ≥ 96.3%；锚 `fix(TASK-005): M1d …`；完整交付流程 | `git show --name-only ec93e13` 恰两文件；基线代码 `grep -c 'NOTIFY FAILED\|snapshots:' ingest.go` = 0（唯一命中在提交信息里）；树 `b47e440`：`gofmt -l` 仅 `backtest_test.go`、`crisis_test.go`，`go vet` rc=0，两包 `-count=1` ok，`-cover` **96.4%**；五个不动文件 `git diff --stat 4916106 b47e440` 0 行；`go.mod/go.sum/types.go` 相对 `f3d6eb2` 0 行；提交信息 `fix(TASK-005): M1d QA 返工——…` 匹配锚；merge `208a77c` 12:36:08Z 早于 `dev_done` 12:37:31Z（`transitions.jsonl`）；discovery `rework_2` 含 my_commit / merge / mutations / master_gates（锚 `208a77c`）；无 `wt-TASK-005-m1d-fix` worktree 残留（分支 `task/TASK-005-m1d`、`task/TASK-005-m1d-fix` 仍在，归 Leader 收） | PASS |
| functional[0]（含子句、R-005a） | 前两轮已 PASS，本轮查回退 | `TestIngestNotifiesP2OnObservation`、`TestForceOnObservedPeriodIsDuplicate` 在基线绿；回归变异 M1（`Fprintf` 移到 `send` 之后）⇒ `TestIngestNotifyFailureIsLoudButNotCascading` 独家 FAIL | PASS |
| functional[1] / functional[2] | 同上 | `TestIngestNotifiesP0OnPending`、`TestIngestNotifiesP1OnParseFailure` 基线绿 | PASS |
| boundary[0] | 同上 | `TestIngestSendsNothingOnEmptyRun`、`TestIngestNilNotifyIsNoop` 基线绿 | PASS |
| error_handling[0]（含 R-005b） | 同上 | `TestIngestNotifyFailureIsLoudButNotCascading`、`TestIngestP1SendFailureIsReported` 基线绿；回归变异 M6（阶段名改 `notify`）⇒ NotCascading 独家 FAIL；M5（P1 失败不并进 errs）⇒ P1SendFailureIsReported 独家 FAIL | PASS |
| non_functional[0] | 不新增导出函数、既有用例断言不改 | 返工 diff 对既有用例只改 A8-b 那一条断言（QA 明令）；`TestPackageExposesNoWriteFunctions` 在全包绿内 | PASS |
| non_functional[1] / [2] | 门禁 / 交付流程 | 见 fix 非功能行 | PASS |

**覆盖率函数级（树 `b47e440`）**：`send`/`Error`/`Unwrap` 100%，`Ingest` 97.8%，`ingestOne` 97.2%，total 96.4%。

**越界申报核对**：`git show --name-only ec93e13` ⇒ `internal/hestia/ingest.go`、`internal/hestia/ingest_test.go`，与 `writes` 完全一致；两文件在 `208a77c` 与 `b47e440` 逐字节一致（numstat 为空）。

## 3. 变异汇总（第 3 轮，6/6 KILLED，每组 FAIL 数恒为 1）

| 变异 | 期望转红 | 实测 |
|---|---|---|
| M-A4 分子改回 `len(errs)`（**A4 判据**） | P1SendFailureIsReported | KILLED |
| M-A8a `"send P0"` → `"send P2"`（**A8-a 判据**） | NotifyFailureOnPendingIsLoud | KILLED |
| M-A8b `wrap("snapshot"` → `wrap("fetch"`（**A8-b 判据**） | FailsPeriodWhenSnapshotUnwritable | KILLED |
| M6-reg `"send P2"` → `"notify"`（回归） | NotCascading | KILLED |
| M5-reg P1 发送失败不并进 `errs`（回归） | P1SendFailureIsReported | KILLED |
| M1-reg `Fprintf` 移到 `send` 之后（回归，第 2 轮判据） | NotCascading | KILLED |

每组：`python3` 按字面替换 → `go build ./internal/hestia/` 有效性闸 → 打印 diff → `go test ./internal/hestia/ -count=1` 全包 → `git checkout --` 还原 → sha256 前缀与 porcelain 核实。

## 4. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-005-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b
cd ../wt-verify-TASK-005-m1d-b
GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1        # ok / ok
GOTOOLCHAIN=local go test ./internal/hestia/... -count=1 -cover                  # 96.4%
# A4：ingest.go 汇总行 len(failedPeriods) → len(errs)
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestIngestP1SendFailureIsReported -count=1        # ⇒ FAIL
git checkout -- internal/hestia/ingest.go
# A8-a：ingest.go `renderP0(obs, rep), "send P0"` → `"send P2"`
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestIngestNotifyFailureOnPendingIsLoud -count=1  # ⇒ FAIL
git checkout -- internal/hestia/ingest.go
# A8-b：ingest.go `wrap("snapshot", err)` → `wrap("fetch", err)`
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestIngestFailsPeriodWhenSnapshotUnwritable -count=1  # ⇒ FAIL
git checkout -- internal/hestia/ingest.go
# A/B 覆盖率（背对背两轮）
git checkout --detach ec93e13475a5a100e50fb93859820362abea2299 && GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 96.4%
git checkout --detach f3d6eb282c83e0ca730b1713907f5220114ee86b && GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 96.4%
```

## 5. 前两轮记录（test-m1d-a）

### 5.1 第 2 轮（VERIFIED，epoch 2，rework 1）

- verify_baseline.head `1b6ef242a1bda4ad889540f6b360c07aef7c8c9e`（master）；discovery sha256 `5f1eb1db…acc2`。返工 commit `f7fba511ada555fa079149be02f78e813c5d9ac6`（父 `1efbcfec7cfad2d08fa49fea3fde13aa5ffb127a`；merge `1b6ef24`），只改 `ingest_test.go` 5+/0-。验证树 `../wt-verify-TASK-005-r2-m1d @ 1b6ef24`。
- 结论：首轮唯一缺陷（「通知在 `Out` 打印之后发」零断言）已修——`TestIngestNotifyFailureIsLoudButNotCascading` 末尾补 `assert.Contains(out.String(), "→ "+TableObservations)`；把 P2/P0 的 `send` 移到 `Fprintf` 之前（numstat 1/2）⇒ 该用例 FAIL、整包红；原状绿。首轮八条无回退。
- 覆盖矩阵：functional[0] 主句 `TestIngestNotifiesP2OnObservation` PASS；子句 M1 KILLED、新断言 `ingest_test.go:993-997`；R-005a `TestForceOnObservedPeriodIsDuplicate` PASS、M7' KILLED；functional[1] `TestIngestNotifiesP0OnPending`；functional[2] `TestIngestNotifiesP1OnParseFailure`；boundary[0] `TestIngestSendsNothingOnEmptyRun`、`TestIngestNilNotifyIsNoop`；error_handling[0] NotCascading + M2/M6 KILLED；R-005b `TestIngestP1SendFailureIsReported`；non_functional 门禁（树 `1b6ef24`：gofmt 两欠账、vet 0、两包 ok、96.4%、五个不动文件 0 行、`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行）；交付流程（锚匹配、merge `1b6ef24` 早于 `dev_done` 07:14:17Z、discovery `rework_1` 完整、无 worktree 残留）——全部 PASS。
- 变异 4/4 KILLED：M1 send 移到 Fprintf 前、M2 去 `errors.As` continue、M6 阶段名改 `notify`、M7' Duplicate 不发 P2。

### 5.2 第 1 轮（REJECTED，task_defect，2026-09-03 07:01:33Z）

- 基线 `1efbcfec7cfad2d08fa49fea3fde13aa5ffb127a`；dev commits `8fec932`（主体，父 `656fe05`）+ `75961fc`（补丁：通知失败用例按 wrap 前缀形态钉阶段名）。
- 缺陷：DoD functional[0]「通知在 `Out` 打印之后发」子句零断言；M1 变异两包全绿。`TestIngestNotifyFailureIsLoudButNotCascading` 捕获了 `out` 却未断言。
- 首轮其余全部 PASS：9 组有效变异 8 组 KILLED（M2 去 continue / M3 pending 也发 P2 / M4 去 nil 守卫 panic / M5 P1 失败不并 errs / M6 阶段名改 notify / M7' Duplicate 不发 / M8 循环不发 P1 / M9 空跑也发）。阶段名 `send P2`/`send P0` 偏离在 DoD 二选一范围内且记 decision；`err.Error()` 含 `notify`、`errors.Is(errBoom)` 穿透均有测试。
- 门禁（树 `1efbcfe`）：gofmt 两欠账、vet 0、两包 ok、覆盖率 96.4%（`send`/`Error`/`Unwrap` 100%，`Ingest` 97.3%，`ingestOne` 97.2%）；A/B 背对背 `75961fc` 96.4% / `656fe05` 96.4%；五个不动文件 0 行；两文件改动恰等于 `writes`；两 commit 锚匹配。
- 首轮给出的修法（对已捕获的 `out` 断言含 `"→ "+TableObservations`，隔离副本实测变异态红、原状绿）即第 2 轮 dev 落地的形态。
