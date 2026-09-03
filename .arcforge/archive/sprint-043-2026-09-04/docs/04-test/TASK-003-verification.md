# TASK-003 验证报告 · ingest 在 Parse 之前落盘快照，写盘失败即该期失败

- 验证者：test-m1d-a　承接 epoch：1　判定：**VERIFIED**
- verify_baseline.head：`656fe0577f2713010468223f27f0954d58a54ce9`（master）；discovery sha256 与基线一致 `5ed7d51b…06ce`
- dev commit：`4b829e1d8f6476517093e21ef8aa3d2bc3ec7838`（父 `abebb76057c6fb847ac11fd632fdd28b3b316d47`；merge `656fe05`）；改动 `ingest.go` 12+/0-、`ingest_test.go` 134+/21-、`discover_test.go` 3+/3-
- 验证树：`git worktree add --detach ../wt-verify-TASK-003-m1d 656fe0577f2713010468223f27f0954d58a54ce9`；变异在独立树 `…-mut`（同 sha）上做，每次后 `git checkout --` 还原并核实 porcelain 为 0；A/B 覆盖率钉在 `4b829e1` 与 `abebb76`

## 1. 结论

接线位置、文案、错误阶段名与需求原文逐字一致；`ingestCfg(t)` 24 处替换齐、`ingestCfg()` 残留 0；reviewer 的第 25 处手写 `Config` 补了 `Storage` 且断言未改；R-003 正向用例 `TestIngestSnapshotDivergedOnForce` 独立成条。五条新测试全绿，既有 ingest/discover 测试全绿且 diff 里没有任何断言改动（只有调用点替换）。7 组变异 6 组 KILLED，唯一存活的 M3（阶段名改错）不是 DoD 缺口（见 §4）。门禁全绿、覆盖率 96.4%、无越界。

## 2. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | `ingestCfg(t)` 带 `t.Helper()` + `t.TempDir()`；24 处调用点全替换；vet 干净；第 25 处手写 `Config` 补 `Storage`、不改断言 | diff：定义行 `func ingestCfg(t *testing.T) Config`；树 `656fe05` `git grep 'ingestCfg()'` = 0，`ingestCfg(` 共 30 = 24 调用 + 1 定义 + 5 新用例；`ingest_test.go:186` 补 `Storage: StorageCfg{SnapshotDir: t.TempDir()}`，`TestIngestRejectsPeriodMismatch` PASS 且断言行未在 diff 中；`go vet` rc=0 | PASS |
| functional[1] | Fetch 成功后、Parse 之前调 `saveSnapshot(d.Cfg.Storage.SnapshotDir, c.ArticleID, raw, time.Now())`；快照字节等于 `readTestdata(annualFile)` | `ingest.go:178` 位于 `wrap("fetch")` 之后、`Parse(raw)` 之前；`TestIngestSnapshotBytesMatchRaw` PASS；变异 M1（移到 Parse 后）⇒ SurvivesParseFailure FAIL；M6（articleID 传 URL）⇒ BytesMatchRaw 等 4 条 FAIL；M7（内容传 nil）⇒ 3 条 FAIL | PASS |
| functional[2] | diverged 打 `%s snapshot diverged from %s: saved as %s\n`；同字节 `--force` 不重写、`Out` 不含该行 | `ingest.go:186` 格式串逐字；`TestIngestSnapshotIdempotentOnForce` PASS（Chtimes 2020 后重跑 mtime 不变）；变异 M5（任何 Kind 都打）⇒ FAIL | PASS |
| functional[2] R-003 | 正向：Force 重跑 + 尾部多一换行 ⇒ `Out` 含 `snapshot diverged from <annualID>` 且目录两个文件 | `TestIngestSnapshotDivergedOnForce` PASS（`assert.Len(entries, 2)`）；变异 M4（删打印）⇒ FAIL | PASS |
| boundary[0] | Parse 失败仍有快照、observations 0 行 | `TestIngestSnapshotSurvivesParseFailure` PASS；M1 ⇒ FAIL | PASS |
| error_handling[0] | `SnapshotDir` 是普通文件 ⇒ 该期失败、`err` 含 `snapshot`、两表 0 行 | `TestIngestFailsPeriodWhenSnapshotUnwritable` PASS（observations、pending 均 `assert.Zero`）；变异 M2（吞错继续）⇒ FAIL | PASS |
| non_functional[0] | 既有测试全绿且不改断言；四条新测试先红；import 按需补 | `-v` 列出 TestIngest*/TestForce*/TestDiscover* 共 39 条 PASS；diff 中既有用例只有 `ingestCfg()`→`ingestCfg(t)`；discovery `tdd_red` 附五条真实 FAIL 行（含行号与消息）；import 补 `time`（ingest.go）与 `os`/`path/filepath`/`time`（测试，`io` 已有） | PASS |
| non_functional[1] 门禁 | gofmt / vet / 两包 / 覆盖率 ≥ 96.3% / 五个不动文件 / 无新依赖 / 注释前缀 | 树 `656fe05`：`gofmt -l` 仅 `backtest_test.go`、`crisis_test.go`；`go vet` rc=0；两包 `-count=1` ok；`-cover` **96.4%**；A/B 背对背 `4b829e1` 96.4% / `abebb76` 96.4%；`git diff --stat 4916106 656fe05 -- {store,validate,parse,extract,fields}.go` 0 行；`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行；`ingest.go:174` 注释 `M1d 的 TASK-003` | PASS |
| non_functional[2] 交付流程 | worktree / 提交锚 / code-simplifier / merge 先于 dev_done / 重采 / discovery | `feat(TASK-003): M1d …` 匹配锚；merge `656fe05` 早于 `dev_done`（03:44:16Z）；discovery 含 my_commit / master_after_merge / master_gates_after_merge 与 code-simplifier 的 -3 行说明（`var out` → `io.Discard`，语义不变，diff 核实）；无 `wt-TASK-003-m1d` 残留 | PASS |

**越界申报核对**：`git show --stat 4b829e1` ⇒ 恰 `ingest.go`、`ingest_test.go`、`discover_test.go`，与 `writes` 一致；三文件在 `4b829e1` 与 `656fe05` 逐字节一致。`cmd/atlas` 未动且测试 ok（discovery 称 `hestia_test.go:774` 的 `hestia.Config{}` 不走 ingest 路径，cmd 包全绿佐证）。

## 3. 变异汇总（独立变异树，6/7 KILLED，1 存活判定为非缺陷）

| 变异 | 期望 | 实测 |
|---|---|---|
| M1 快照移到 Parse 之后 | SurvivesParseFailure 红 | KILLED |
| M2 写盘失败吞错继续 | FailsPeriod 红 | KILLED |
| M3 `wrap("snapshot")` 改 `wrap("fetch")` | FailsPeriod 红 | **SURVIVED**（见 §4） |
| M4 删 diverged 打印 | DivergedOnForce 红 | KILLED |
| M5 任何 Kind 都打 diverged | IdempotentOnForce 红 | KILLED |
| M6 articleID 传 `c.URL` | BytesMatchRaw 红 | KILLED（4 条红） |
| M7 落盘内容传 nil | BytesMatchRaw 红 | KILLED（3 条红） |

## 4. 发现（不阻断，供 TASK-005 与 TASK-008 参考）

`TestIngestFailsPeriodWhenSnapshotUnwritable` 的 `assert.Contains(err.Error(), "snapshot")` 被 `saveSnapshot` 内层错误文本（`snapshot dir <path>: …`）满足，与 `wrap("snapshot", …)` 的阶段名无关——阶段名改成 `fetch` 测试仍绿。DoD 原句判据就是「`err.Error()` 含 `snapshot`」，字面满足，故不退回。若要钉住阶段名，断言 wrap 前缀形态（既有 `requireWrappedStageError` 或 `Contains "): snapshot:"`）即可。TASK-005 接通知时 `renderP1` 取错误首行，阶段名会进 Telegram 文案，届时值得用更强的断言。

## 5. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-003-m1d 656fe0577f2713010468223f27f0954d58a54ce9
cd ../wt-verify-TASK-003-m1d
GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestIngestSnapshot|TestIngestFailsPeriodWhenSnapshotUnwritable|TestIngestRejectsPeriodMismatch' -v -count=1
git grep -n 'ingestCfg()' -- internal | wc -l     # 0
GOTOOLCHAIN=local go test ./internal/hestia/ -cover -count=1     # 96.4%
```
