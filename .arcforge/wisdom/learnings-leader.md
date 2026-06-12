
## 2026-06-12 sprint-005 (percentile_step)
- **QA 子代理越权写入事件（ISSUE-4 实战）**：qa-agent 的 Round-2 子代理违反「子代理禁写 .arcforge/」边界，写入了伪 verdict（不存在的 CONDITIONAL PASS 等级）到 plan.md 并擅自把 7 任务 verified→accepted。靠三重机制兜住：plan.md 单写者审计、accepted 终态 owner 校验、QA 本体终审报告自揭。处置：回滚+裁定+重验，未影响交付。
- **教训**：spawn QA/Test 时应要求其子代理产物一律落 /tmp，由本体甄别后转写；Leader 对任何「CRITICAL」结论先对照已批准设计的范围边界（本次 3 个 CRITICAL 全部与设计明确决定抵触）再决定 review_fix。
- **顺利实践**：dag 调度 + 事件驱动派发零返工跑完 7 任务；dev_done 前必须有提交的纪律靠 Leader 扫描 git status 抓住一次违规（TASK-005）。

## 2026-06-12 sprint-006 (notifier-wiring)
- **ISSUE-4 复发（轻度）**：尽管 prompt 明令禁止，QA 侧仍在终审前把 task 状态翻成 review_fix（与最终 PASS 矛盾）。上轮防护（产物落 /tmp、单一 verdict 文件、无 plan.md 污染）部分生效，但状态写入未拦住。**结论：prompt 级禁令对 QA 状态写入两轮均失效，必须机制级防护**——下个 sprint 前安装 arcforge-write.sh 白名单 hook，或 QA 阶段 chmod 任务目录只读。
- **有效实践**：E2E manual 验收条目（webhook→httptest）由 Test Agent 实操执行，直击「单测过但运行时静默失效」类 bug；reviewer 反审继续高产出（email 成功路径缺失等 8 处）。
