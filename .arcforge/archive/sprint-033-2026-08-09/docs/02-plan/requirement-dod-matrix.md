# 需求 ↔ DoD 双向追溯矩阵

需求源：`2026-08-08-hestia-store.md` 的 Self-Review §1 Spec coverage 表（10 个 Spec 章节）。

## 正向：每条需求都有 DoD 落点

| Spec 章节 | 需求文档指派的任务 | 本 Sprint 的 DoD 落点 |
|---|---|---|
| §2 D1 map + 白名单 | Task 1、Task 5 | T1 `error_handling[0]`（identRE 闸门）、T5 `error_handling[0]`（白名单外键拒绝且零行） |
| §2 D2 单一真相源 | Task 1、3、5 | T1 `non_functional[0]`（fields.go 外无字面量）、T3 `functional[0]`+`non_functional[0]`、T5 `functional[2]`（按 fieldOrder 遍历） |
| §2 D3 单一写入口 + 内部分流 | Task 5、6 | T4 `non_functional[0]`、T5 `functional[0]`、T6 `functional[1]`、T7 `functional[0]` |
| §2 D4 ValidationReport 提前定义 | Task 2 | T2 `functional[1]` |
| §2 D5 is_current/absent_fields 不入库 | Task 3 | T3 `error_handling[0]`（对 DDL 字符串做否定断言） |
| §3 类型 | Task 2 | T2 `functional[0..2]` |
| §4 字段清单 54 个 | Task 1 | T1 `functional[0]`（逐组断言，非只测总数） |
| §5 Schema 三段 DDL | Task 3 | T3 `functional[0..2]`、`boundary[0]` |
| §6 Save 八步编排 | Task 5、6 | T5 `functional[0..2]`、T6 `functional[0..1]` |
| §7 错误处理七条 | Task 2、5、6 | T2 `error_handling[0..1]`、T5 `error_handling[0..2]`、T6 `error_handling[0]` |
| §8 DoD 17 条 | Task 1–6 分散 + Task 7 Step 1 | 见上各行 + T7 `functional[0]` |
| §9 交付物 | File Structure | 各任务 `writes` 字段逐文件声明 |
| §10 非目标 | Task 7 Step 1 | T7 `functional[0]`（`go doc` 核验公开面） |

**孤儿需求检查：无。** 10 个 Spec 章节全部有 DoD 落点。

## 反向：每条 DoD 都对应需求

逐条核对 7 个任务共 44 条 DoD，均可追溯到上表某一行或下列三条**由风险分析新增**的判据：

| 新增 DoD | 出处 | 理由 |
|---|---|---|
| T2 `functional[0]` / T3 `functional[1]` / T5 `functional[1]` 的**三处同序**约束 | 需求文档 Self-Review §3 第 1 条 | 文档把它列为类型一致性检查项，但未指派给任何任务的验收标准。**它是本 Sprint 唯一「静默产生错误数据」的路径**，必须成为 DoD 而非注释。 |
| T5 `error_handling[0]` 的「查库确认零行」 | 风险分析 | 文档只要求返回错误。只断言 error 非 nil 无法排除「已写脏数据再报错」。 |
| T6 `functional[0]` 的「行数不变且 article_id 已更新」 | 风险分析 | 文档只说「只更新 article_id」。只断言无报错无法区分「更新了」与「什么都没做」。 |

**凭空 DoD 检查：无。** 三条新增均有明确出处与理由。

## 判据强度自查

按 sprint-032 的四坐标（方向 / 强度 / 机制 / 范围）复查，重点是**范围**——本 Sprint 有 5 条 DoD 显式写了「不能只测 X」的反例：

- T1 `functional[0]`：不能只测总数 54（会让「一组多一个、另一组少一个」通过）
- T2 `error_handling[0]`：不能一次性全空（只能证明「有校验」，不能证明「每个字段都被校验」）
- T5 `functional[1]`：不能只测「能写进去」（须构造能区分错位的用例）
- T5 `error_handling[0]`：不能只断言 error 非 nil（须查库确认零行）
- T6 `functional[0]`：不能只断言无报错（须断言行数不变且值已变）

这五条都是「弱判据能通过而缺陷仍在」的形态，写进 DoD 才能让 Dev 在写测试时就避开。
