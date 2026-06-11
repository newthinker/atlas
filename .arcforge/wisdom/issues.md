# 跨任务问题清单（仅 Leader 写，聚合自各实例与验证报告）

## CARRYOVER（sprint-001 遗留，下一 Sprint 候选）

- ~~**I3 [latent]** 仲裁合成信号未设 Price~~ ✅ **已于 sprint-002 修复**（cc0182a：referencePrice 取冲突信号首个正价 + 反例锁定测试）——sprint-002 双策略示例使其条件可达，QA 对抗轮拦截后裁决即修。
- **理杏仁口径核对**（sprint-002 遗留）：cvpos 边界口径（≤ vs <）、usHKIndexCodes 四候选代码（SPX/COMP/DJI/HSI）、metricsList 键名——需 LIXINGER_API_KEY，代码已注明首日核对项。
- **I1** confirm/batch 模式仅入队但日志 "signal executed" 语义误导；paper 模式无自动 confirm，pending 单需消费方。
- **I2** PaperBroker.CancelOrder 非终态分支不可达死代码。
- **I4 [trivial]** router.Route 恒返回 nil err，app.go 对应 err 分支为死代码。
- FutuBroker 真实实现 + live 模式（ADR-7 出范围）；执行确认 UI/API。

## ISSUE-1: HTTP collector 缺 StatusCode 检查（fantasy assertion 模式）

- **来源**: TASK-010 验证拒绝（test-agent-1，2026-06-10）
- **模式**: `client.Do` 后直接 `json.Decode`，无 `resp.StatusCode` 守卫 → HTTP 503 + 合法 JSON body 被当成功；「HTTP 错误」测试仅靠非 JSON body 触发 decode 失败碰巧通过，与畸形 JSON 测试同一代码路径。
- **影响范围**: 同模式可能存在于 eastmoney（TASK-009）、yahoo（TASK-011）——已提示两个 test agent 重点核查。
- **修复模板**: Do 后 Decode 前 `if resp.StatusCode != http.StatusOK { return error }`；测试必须用「合法 JSON + 非 200」断言 error（与畸形 JSON 用例区分路径）。
- **QA 提示**: Code Review 阶段应全局 grep 各 HTTP collector 的 StatusCode 处理。

## ISSUE-3: 整包覆盖门禁对 package main 不可行（已机制化修复）

- **来源**: TASK-003 澄清（dev-agent-4，2026-06-10）
- **问题**: cmd/atlas 含大量任务范围外的存量未测 CLI 样板，整包 80% 对接线类任务不可达。
- **裁决**: hook 支持任务级 `coverage_minimum`（Leader 裁决写入 task JSON），TASK-003=45 / TASK-007=35 / TASK-008=35；验收质量由 DoD 端到端测试兜底。
- **附带发现**: dev-agent-4 修复了 ExecutionManager.Execute 市价单不带 Price 导致 paper BUY 永被拒的真实集成缺陷（internal/broker，已防护性纳入 003 packages）——QA 阶段重点回看。

## ISSUE-4: TeammateIdle hook 保活条件过宽导致空转（qa-* 与 test-* 两例，均已修复）

- **qa-* 例（sprint-001）**: review_fix 阶段 qa-agent-1 被无限唤醒（8 次/40s）——保活条件「存在 verified 任务」，但 verified 在等修复回流。修复：改「终审就绪」语义（无在途任务且存在 verified 才 exit 2）。
- **test-* 例（sprint-002，两轮）**: ①test-agent-1 被**已派给他人**的 dev_done 唤醒（3 次/50s），修复为过滤 verifier；②修复后仍被 **verifier 为空**（Leader 派验前窗口）的 dev_done 唤醒（6+ 次/分钟）——test agent 工作循环只认 verifier==自己，醒来无事 idle，hook 再唤醒成环。最终修复：PENDING_VERIFY 仅匹配 `verifier == 自己`，verifier 空窗口属 Leader 职责不唤醒任何 test 实例。
- **模式教训**: 保活条件必须按「该实例可执行的动作」过滤，而非按任务状态泛匹配——建议回流上游模板时全角色复查此原则。

## ISSUE-2: 流程类（已修复，供复盘）

- TaskCompleted hook 的 OTHERS 排除集原漏 verified/accepted → 已 verified 未 commit 的改动被误判他人 drift（dev-agent-2 上报，已修 hook + 要求及时 commit）。
- validator 必须从项目根运行（相对路径 discovery 校验），统一用 `~/.arcforge/bin/arcforge-validate .arcforge/tasks`。
- 子代理调用的 scope 约束可能被 dev 误内化为自身约束（TASK-009 澄清案例）。
