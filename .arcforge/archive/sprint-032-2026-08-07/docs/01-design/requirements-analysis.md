# 需求分析 — M1a `internal/macro/bitemporal`

**需求文档**：`hestia/docs/superpowers/plans/2026-08-07-macro-bitemporal.md`（1173 行）
**设计依据**：`superpowers/specs/2026-08-07-macro-bitemporal-design.md`
**实现位置**：**atlas 仓库**（文档在 hestia，Goal 第一句写明实现在 atlas——
文档位置 ≠ 实现位置，这一点在环境检查时差点误判）

## 目标

新建 `internal/macro/bitemporal` 包，为双时态表提供时态语义：
判断一次写入是**新增/重复/修订/乱序**，并构造「当前行」与「某时点视图」的查询。

## 与 Sprint 031 的形态差异（决定了本 Sprint 的风险面）

| | Sprint 031（collector policy gate） | 本 Sprint（M1a bitemporal） |
|---|---|---|
| 改动性质 | 改造八家既有 collector + 删装饰器 | **只新增一个包** |
| 回归面 | 大（全部下游） | **零**（无生产代码改动、无新依赖） |
| 跨包 | 是 | 否（**单包**） |
| 任务数 | 21（含 5 次计划外范围扩大） | 6（文档已拆好） |
| 主要风险 | 覆盖缺口、断言不到场 | **断言存在但无效**（见下） |

⇒ **本 Sprint 的风险不在「改坏什么」，而在「测试看起来齐全但守不住」。**
需求文档已把 DoD 写在各测试文件头部的 `Context Checkpoint` 注释里，
**但它给的是「有哪些测试」，没给「怎么证明这些测试有效」。**

## 架构要点（决定 DoD 该盯什么）

1. **薄机制层**：包只认识键的形状（业务键哪几列、哪列是发布时间轴），
   不知道表里还有什么业务列，**不导出写操作、不建表、不管事务**。
2. **「当前行」由 revision 列派生（`MAX`）而非 `is_current` 列** ——
   写入是调用方一句普通 INSERT，本包不参与。
3. **注入面为零**：拼进 SQL 的每个标识符先过 `^[A-Za-z_][A-Za-z0-9_]*$`，
   业务键取值一律走 `?` 占位符。
4. **两套 Spec 形状并行**（hestia 形状与 crisis 形状，**两者都是两列业务键**——⚠ 本文初稿误写「crisis 单列键」，test-agent-18 在 TASK-003 验收时追到此处为失真源头；`singleKeyShape` 是**另一条独立的边界需求**，不在「两套形状」之内）——
   所有涉及数据库的测试都要对两套跑。

## 硬约束（来自 Global Constraints，逐条可验）

| # | 约束 | 可验方式 |
|---|---|---|
| C1 | 不新增任何依赖 | `git diff master --stat go.mod go.sum` 无输出 |
| C2 | 标识符必过 `^[A-Za-z_][A-Za-z0-9_]*$` | 注入用例 |
| C3 | 业务键取值走 `?` 占位符 | SQL 串不含字面值 |
| C4 | 包不导出写操作/建表/事务 | `go doc` 公开面核对 |
| C5 | 测试用真实 SQLite + `t.TempDir()` | 读测试代码 |
| C6 | 每个测试文件头部写 Context Checkpoint 映射 | grep |
| C7 | SQL 构造器测试**跑真 SQL 断言结果**，不断言字符串字面量 | 读测试；唯一例外是注入防护那条 |
| C8 | 所有 DB 测试对两套 Spec 形状并行跑 | `bothShapes` 覆盖 |
| C9 | 注释用中文、解释「为什么」 | 抽读 |
| C10 | 不触及既有包 | 全量回归 + `detect_changes` |

## 依赖图

```
Task 1 (Spec/NewSpec)   ← 无依赖
Task 2 (Classify 纯函数) ← 无依赖        ⇒ 可与 Task 1 并行
Task 3 (fixture 测试基建) ← Task 1
Task 4 (Current/AsOfQuery) ← Task 1, 3
Task 5 (Lookup)          ← Task 1, 2, 3  ⇒ 可与 Task 4 并行
Task 6 (收尾验证)        ← Task 1–5
```

## 一处调度约束（Sprint 031 的教训直接适用）

**6 个任务全在 `internal/macro/bitemporal` 一个包内** ⇒ validator 的 scope 互斥
（子树口径）会拦住全部并行。

处置：`packages` 保持包路径（门禁要跑测试），**`writes` 用文件级精确化**——
各任务的文件互不重叠：

| 任务 | writes |
|---|---|
| 1 | `spec.go` `spec_test.go` |
| 2 | `classify.go` `classify_test.go` |
| 3 | `fixture_test.go` |
| 4 | `query.go` `query_test.go` |
| 5 | `lookup.go` `lookup_test.go` |
| 6 | 无（纯验证） |

这样 1‖2、4‖5 可以真并行。
