# 设计规格 — Sprint 035（指针文件）

本 Sprint 的设计规格**不在本文件内**。上游已产出完整的实施计划与设计文档，
在此重抄一份只会产生第二个真相源，两者一旦分叉，先错的一定是抄件。

| 内容 | 位置 |
|---|---|
| 实施计划（7 任务，含全部目标代码、测试代码、预期 RED 信息、提交信息） | `~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-11-hestia-validate.md` |
| 设计 spec（D1-D4 决策、7.1 映射表、8.1 ULP 契约、8.3 双重跳过理由） | `~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/specs/2026-08-11-hestia-validate-design.md` |
| M0 契约样本与 schema 验证（阈值标定的实测来源） | vault `Projects/Hestia/M0-契约样本与schema验证.md` |
| 本包既有契约 | `internal/hestia/CONTRACTS.md` |

## 计划中需要回填上游 spec 的三条实现决策

计划在「三个在写计划时定下的实现决策」一节标明这三条 spec 未覆盖、需回填。
本 Sprint **实现它们**，回填 spec 的动作属于交付后事项（计划「完成后」第 2 条）：

1. `minDriftHistory = 3` —— 漂移检测至少需 3 期历史；不足时记 `passed` + `drift_skipped:insufficient_history`，
   与「完全没有历史」的 `no_prior_period` **区分开**（对运维含义不同：前者「再等几期」，后者「这是首期」）。
2. `requiredFields("llm-fallback@v1")` 返回 `nil` —— `Extractor` 同时编码了「抽取方式」与「模板版本」，
   而该值只带前者。这是**刻意的失败信号**，留给 M1c。
3. `yoy_sanity` 用 `_yoy` 后缀筛、`completeness` 走模板表而非 `tsf_` 前缀 ——
   前者是明确的命名规则（约定本身），后者的前缀只是碰巧与板块归属一致。

## 交付后事项（不在本 Sprint 范围）

1. 合并 master，更新 vault 文档状态
2. 把上述三条回填上游 spec
3. 下一子迭代 M1b-4（discover + CLI）
