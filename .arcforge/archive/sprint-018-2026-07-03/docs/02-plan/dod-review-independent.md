# 独立 reviewer 反审报告 — Sprint 018（dod-reviewer-018）

> 完整原文见会话记录；此处存档 Leader 裁决对照（详表见 architecture-decisions.md AD-8）。

## 要点

- plan 代码事实引用高度准确（serve.go:99-170 迁移安全、buildFundamental 签名、lixinger A 股专用、
  PercentileRank 严格小于语义/空序列 -1 有守卫等 —— 全部核实一致）。
- 3 处 spec 一致性问题：C1 JSON gaps 位置（接受偏离，backlog 回写 spec）；C2 config 缺失 exit 语义
  （接受 plan，与既有命令一致，TASK-004 补 DoD）；C3 价格窗口"不同源"（**误报**——策略侧价格窗口
  since-inception 即 ~100 年，-90d 缓冲属 EPS 拉取且 buildFundamental 内部自动同源，维持 plan）。
- 4 处测试缺口全部采纳回写 DoD：B3 并发等价（Workers 1 vs 4）、B4 ≥3 标的保序断言、
  B5 价格百分位极值边界（>max→100 / ==min→0 / 内部值）、B10 collector 名称集合零变化锚点。
- B7/B8/B9（stdout 构造保证、退出码 proxy、空表 exit 0）：接受，验证阶段措辞不夸大 + 手工冒烟兜底。

## 修订后 DoD 条数

T1=4 / T2=8 / T3=5 / T4=8，全部 ≤8 ✓
