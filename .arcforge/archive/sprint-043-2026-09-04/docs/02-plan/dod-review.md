# 独立 DoD 反审 · Sprint M1d

**reviewer**：`dod-reviewer-m1d`（Agent tool，只读；先只读需求与 spec 写出自己的验收要点，再读 8 个任务文件比对）
**判定**：NEEDS WORK —— 1 条阻断（一行修正），其余建议/备注
**Leader 处置**：逐条核实（每条都给了文件:行号，Leader 逐一打开看过）后**全部采纳**，DoD 已经写通道 `update` 落盘，validator 重跑通过。处置表见 `01-design/architecture-decisions.md` AD-11。

## 阻断（1）

**TASK-008 functional[0]**「第一步 `git status --short` 必须为空」在本仓库 sprint 进行中**恒不成立**——主仓库此刻就有 `M CLAUDE.md`、`M .claude/hooks/arcforge-write.sh`（会话外运行时同步）、`?? .arcforge/docs/`、`?? .arcforge/tasks/`（本 sprint 的任务与文档，整个 sprint 都是未跟踪状态）。dev 到这一步只能违反 DoD 或走澄清环把收口卡死；验证者看到非空输出只能判 NEEDS WORK。需求原文「前置纪律」一节就这么写，AD-7 照抄了，是上游笔误。
⇒ 修为 `git status --short -- internal/hestia cmd/atlas configs deploy go.mod go.sum` 必须为空（与后面的 numstat 同口径）。

## 建议（已全部采纳）

| 任务 | 缺口 | 修法 |
|---|---|---|
| 001 | 仓库 yaml 的 `snapshot_dir` 行无测试守卫（`TestShippedConfigLoadsAndIsCalibrated` 只比 `magnitude_ranges` 键集） | 加一条 `cfg.Storage.SnapshotDir == "data/hestia-snapshots"` 断言 |
| 002 | `writeAtomic` 的 `WriteFile` 失败、`Rename` 失败、`snapshot read` 默认分支无测试；覆盖率零余量 | 只读目录用例覆盖 `WriteFile` 失败（root 下 skip）；其余允许不测但要说明 |
| 003 | 第 25 处：`ingest_test.go:178` `TestIngestRejectsPeriodMismatch` 手写 `Config{}` 无 `SnapshotDir`，接线后 `MkdirAll("")` 报错转红 | 点名补 `Storage: StorageCfg{SnapshotDir: t.TempDir()}`，不改断言 |
| 003 | `snapshot diverged` 只有反向断言 | 正向用例：Force 重跑 + 尾部多一换行的 raw |
| 004 | ytd 与 mom 同在时取 ytd 无测试 | 一行断言 |
| 005 | Duplicate 路径 ingest 层无「会调 send」断言（切换清单第 5 步的判据正是它） | 扩 `TestForceOnObservedPeriodIsDuplicate` 加 `fakeSender` |
| 005 | 循环里 `send(renderP1)` 自身失败分支无测试 | Parse 失败 + `fakeSender{err}`，断言错误链含 `parse` 与 `notify` |
| 005 | `wrap("notify", notifyError)` 文案会成 `notify: notify: boom` | 二选一记 discovery |
| 006 | 「Discover 之前拦下」无可观测量 | 断言 `fakeFetcher.calls` 为空 |
| 006 | 0 匹配时 `kept 0 of N` 行仍先打出 | 备注给验证者 |
| 007 | `buildHestiaSender` 无测试（字面量 nil 那个坑没守） | nil 用例照 `TestBuildCrisisSenderNoConfig` + 临时主配置非 nil 用例 |
| 007 | 「不低于 75.7」与 floor 75 会在 75.0–75.6 区间扯皮 | 改「≥ floor 75 且新增代码有测试」 |
| 008 | `$S` 未定义；`$BASE` 要反查 | 钉死 scratchpad 路径（带实例名）与 `ae088eb…` |

## 备注（保留、不阻断）

- AD-10 的「未核验」可撤：`ingest_test.go:318-330` 已用相同阈值证实前提。已撤。
- description 里 `errors.As(err, &notifyError{})` 不是可编译写法。已改为需求原文的 `var ne notifyError` 形态。
- 「TDD 红阶段输出记进 discovery」只能靠 dev 自述，弱证据。保留。

## reviewer 核实过的代码事实（转录，供验证者复用）

`config.go:57-58` `%w` 包装 ✓ ｜ `types.go:276-278/284/293/299` ｜ `discover.go:267-269` ｜ `fields.go:36,101,103,126,137` ｜ `classify.go:21` ｜ `hestia.go:262-276,301` ｜ `hestia_test.go:105,541` ｜ `crisis.go:425-435`、`crisis_test.go:711` ｜ `telegram.go:40,62,110` ｜ `store_test.go:437` want 恰 22 项 ｜ 五个不动文件 `git diff --stat 4916106 HEAD` 为空 ｜ 两包覆盖率 96.3% / 75.7% 于 `ae088eb`。
