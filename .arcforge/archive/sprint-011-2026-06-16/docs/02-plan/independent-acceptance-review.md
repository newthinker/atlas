# 独立验收标准反审报告

> reviewer：独立 agent，**仅读需求文档**，不参考已生成 DoD（避免锚定）。

## 总体判断
验收标准空间**总体充分、可测试、边界较齐全**。三大高风险点（可降级/零回归、selector 不递归、原子写）在计划中均有对应测试设计或显式告警。

## 最易遗漏的验收标准（reviewer 提出 → Leader 处置）

| # | reviewer 指出的缺口 | Leader 处置 |
|---|---|---|
| 1 | 原子写测试只验「无 .tmp 残留」≠ 验「原子」；真正保证是 os.replace 这一刻覆盖 | **已采纳**：T3 boundary 增「原子性以 os.replace 实现而非原地改写，code review 可证」 |
| 2 | `SelectExternalForSymbol` GetAll 兜底跳过 qlib 缺专测；NeverReturnsQlib 用 AAPL 走主路由可能绕开兜底 | **已采纳**：T9 boundary 增「仅注册 qlib 无外部时返回 nil」直击兜底递归路径 |
| 3 | serve.go 装配的缺库/关库/不可读降级几乎无自动化测试，DoD「零回归」靠人工冒烟 | **残留风险**：计划刻意选 build+冒烟（serve.go 难单测）。强制抽函数会破坏 Realistic Scope。交人类确认门裁决；T12 已含 Enabled=false/ Ping 失败跳过的 boundary criteria（code review + 冒烟覆盖） |

## 次要项（非阻断）
- 补尾段与仓库段在 last_date 边界不重复：已由 T7「tail start=last_date 次日」间接保证。
- `adj_close` 已落库但 Go readRange 不读取：**确认在范围内**——本期 FetchHistory 返回 raw OHLCV（计划 readRange 只选 close），复权价消费留待后续；非缺陷。
