# 独立验收标准反审报告 — 第二期 Part B PIT

> reviewer：独立 agent，仅读需求文档，不参考已生成 DoD。

## 总体判断
验收标准空间**基本充分、可测试、边界较齐全**。三大高风险点（防前视、修订升序、零回归/降级）计划自身 DoD 已点名，Task 4/5 测试桩可直接转化为验收。

## 最易遗漏项（reviewer 提出 → Leader 处置）

| # | reviewer 指出的缺口 | Leader 处置 |
|---|---|---|
| 1 | **`observe_date == end` 等值边界**（off-by-one）：所有用例都是严格之前/之后，无等值用例；`<=` 误写 `<` 时防前视测试仍过，只有等值能抓 | **已采纳**：T4 boundary 增「observe_date==end 必须包含」专测 |
| 2 | **serve 装配时序无可执行验收**：拆分开库/注册的结构改动仅靠 build+既有测试，未端到端证明「qlib 启用 + 符号无基本面 → EPS 走 yahoo」 | **部分采纳/残留**：该端到端语义由 T5（qlibpit 兜底委托：缺符号→fallback）+ T6（db!=nil 构造 qlibpit.New(db,yahoo) 的 wiring 测试）组合覆盖；serve 层真端到端集成测试如第一期 T12 一样难做，作残留风险交人类确认门裁决 |
| 3 | **eps_ttm 必填却允许空入库的契约矛盾**：契约称 eps_ttm 必填，但 `_f("")→None` 允许空；半空行污染主源 | **行为已安全**：T4 boundary「eps_ttm IS NULL 的行被排除」保证 Go 侧不产出该 EPSPoint（存而不服务）；Python 侧按计划保留 `_f` 宽松解析。如需更严可在 T1 拒绝空 eps_ttm——交人类确认门决定是否收紧 |

## 次要项
- N7 原子性（崩溃不残留半成品）：第一期 writer 已用 `.tmp`+os.replace 保证，本期沿用，无需新增强 DoD。
