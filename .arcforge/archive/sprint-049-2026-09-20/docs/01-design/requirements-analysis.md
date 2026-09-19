# 需求分析 — crisis 双时态迁移（M4-crisis，2026-09-18）

需求原文：`hestia/docs/superpowers/plans/2026-09-18-crisis-bitemporal.md`
Spec：`hestia/docs/superpowers/specs/2026-09-18-crisis-bitemporal-design.md`

> ⚠️ **命名撞车**：本 sprint 是「crisis 双时态迁移」。上一个归档 sprint（`sprint-048-2026-09-18`）
> 也叫 M4，内容是「队列可观测与自动触发」，**两者无关**。本 sprint 在文档里一律写 **M4-crisis**。

## 目标
`macro_observations` 的修订不再被静默覆盖（当前是 `INSERT OR REPLACE`，store.go:77），
并让 `crisis replay` 能回答「我当时看到的是什么」。

## 模块划分
| 模块 | 约束 | 任务 |
|---|---|---|
| schema 与视图 | C1（fetched_at NOT NULL）、C9（不自动迁移） | TASK-001 |
| 迁移命令 | C2（重建索引）、C3（留旧表） | TASK-002 |
| 漂移守卫 | C9 | TASK-003 |
| 读走视图 | C5（WHERE 不动）+ **裁决 R1** | TASK-004 |
| 写改追加 | C6（裸 INSERT，冲突响亮失败） | TASK-005 |
| as-of | C4（AsOfQuery 不能建视图）、C7、C8 | TASK-006 |
| CLI flag | C7 | TASK-007 |
| 迁移执行与判据一～四 | 生产动作，人类在场 | TASK-008 |

## Leader 核实的前提（逐条实测，2026-09-18）
| 计划的说法 | 核实结果 |
|---|---|
| 生产库 31092 行 | **属实**（`select count(*)` = 31092） |
| 当前是两段主键、`fetched_at` 可空 | **属实**（`PRIMARY KEY (ts, indicator)`，`fetched_at TEXT` 无 NOT NULL） |
| 生产库 `fetched_at` 零空值 ⇒ NOT NULL 加得上 | **属实**（空值计数 0） |
| `INSERT OR REPLACE` 是缺陷所在 | **属实**（store.go:77） |
| bitemporal 基座有 NewSpec / CurrentQuery / AsOfQuery | **属实**（internal/macro/bitemporal/{spec,query}.go） |
| AsOfQuery 用 `<=`（边界含） | **属实**（query.go 注释与实现一致） |
| hestia 有「漂移即拒绝启动」先例 | **属实**（`TestNewStoreRejectsSchemaDriftOnLegacyDB`，store_test.go:208） |
| 「五个读取点，只改 obsSelect 即可」 | 🔴 **不成立，见下** |

## 🔴 计划的一处缺口（人类裁决 R1：并入 TASK-004）
`obsSelect`（store.go:91）只有 **4 个调用点**（Observation / LatestObservation / SeriesWindow / SeriesSince），
计划所说的「第五处」是常量定义本身。而真正的第五个读取点是 **`EvalDates`（store.go:142-144）**，
它**内联**了 `SELECT ts FROM macro_observations WHERE …`，不经 `obsSelect`。

后果：迁移后只要有修订，`EvalDates` 返回**重复日期**，回测把同一天评估两次。
⚠️ **今天库里全是单版本，所以判据二（迁移前后 replay 逐行相同）照样通过**——
该缺陷要等第一次真实修订才发作，正是本功能开始起作用的那一刻。

## 人类裁决
- **R1**：`EvalDates` 并入 TASK-004 一并改走视图；并加包内守卫（非测试代码不得再出现裸表 `FROM macro_observations`）。
- **R2**：agent 交付 001–007 代码，并在**人类在场时**执行 TASK-008 的备份、迁移、判据一～四；
  判据五（人造修订）、六（同键异值）、七（等真实采集）仍由人类定夺。

## 不在本 sprint
删除旧表 `macro_observations_v1`（C3 明确「迁移当天不删」）；SeriesReader / 评估逻辑 / RenderReplayHTML（C7 全不动）。
