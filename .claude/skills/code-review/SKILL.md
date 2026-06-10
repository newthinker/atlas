---
name: code-review
description: 两轮 Code Review（常规审查 + 跨视角对抗式验证），产出分级问题报告与 verdict。QA Agent 终审时使用。
---

# Code Review Skill

## 心智模型：默认 NEEDS WORK
每条 PASS 都必须附带具体证据（文件:行号、测试名、覆盖率数字）。只有所有 CRITICAL/WARNING
解决后才给最终 PASS。

## 第一轮：常规 Code Review
优先用 ECC `code-reviewer` + `security-reviewer`；不可用时按清单内置审查。可叠加
`code-simplifier`（复杂度）与 `wooyun-legacy`（安全知识库）。

检查维度：代码质量、架构设计（含循环依赖/SOLID）、安全性（注入/敏感信息/输入验证）、
性能（含并发安全）、测试质量。

产出 → `.arcforge/docs/05-review/code-review-{timestamp}.md`、`security-review.md`、
`simplification-report.md`。

## 第二轮：Adversarial Review（跨视角）
自行 spawn 不同 lens 的 reviewer（独立 context + 不同 persona）：
- **Skeptic**：逻辑漏洞、边界、隐含假设。
- **Architect**：设计合理性、扩展性、依赖。
- **Minimalist**：过度设计、可删除代码。

规模决定数量：Small(<50 行)→ Skeptic；Medium(50-200)→ +Architect；Large(200+)→ +Minimalist。
可选跨模型增强：`cross_model` 启用且 `codex`/`gemini` CLI 可用时，由 Leader 经 Bash 额外审一轮。

产出 → `.arcforge/docs/05-review/adversarial-review.md`。

## verdict 与报告
- **PASS** — 无 high-severity 发现。
- **CONTESTED** — 有 high-severity 但 reviewer 间分歧 → Leader 人工判断。
- **REJECT** — 有 high-severity 且共识一致 → 对相关任务原子写 status=`review_fix` + `fix_items`。

报告每个问题格式：`[CRITICAL|WARNING|SUGGESTION] 标题 / 文件:行号 / 描述 / 建议修复`。
