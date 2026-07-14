# Sprint 019 进度 — 宏观危机监控（Cassandra）

> 唯一真相源：`.arcforge/tasks/*.json` 的 status 字段；本文件由 Leader 单写者维护。
> 需求源：`docs/plans/2026-07-13-macro-crisis-monitor-impl.md` ｜ 设计：`atlas-macro-crisis-monitor-design.md` v0.2
> 调度：dag（依赖全 verified 即就绪）｜ autonomy: dod-gate ｜ max_rework: 3
> 分支：`feature/crisis-monitor`（首个 Dev 开工前创建）

## 当前阶段

**已交付（终验收完成）**：15/15 accepted；QA CONDITIONAL PASS 的 SEC-1 已双路径闭合复验；manual 三清单移交人工。
- TASK-004 → dev-agent-1（in_progress, epoch 1）；流程已调整为「先提交后简化」（wave 1 教训）。
- QA 待核对项（非阻断）：TASK-003 impact 前置记录（Leader 亲跑，存 discovery）。

⚠ 运行事故记录（已处置，事后修正）：三个 dev slot 的 code-simplifier 子代理一度占据 slot 身份上报、协议报告延迟，TeammateIdle 空转。实况：dev-2/dev-3 其实已自行提交（b8c6c13/76e9f59），正式 dev_done 报告迟到送达且与 Leader 复核结论一致；仅 TASK-001 提交（452c06d）由 Leader 代完成。Leader 全程亲自复核回归/覆盖率/diff 后代写 discovery 并推进状态；拒绝了两次「解除 .arcforge 禁写」请求（降级规则：Leader 单写者不动摇）。

## 降级备忘

- ECC 缺失 → 设计 v0.2 已评审，直接以实施方案为需求源（不重新 brainstorm）。
- Go validator 缺失 → `scratchpad/arcforge_validate.py`（venv python）手工校验，本轮 PASS（15 任务，唯二 WARN 为 AD-3 裁决的 TASK-014/015 双 package）。
- `arcforge-write.sh` 缺失 → `.arcforge/` 状态 Leader 单写者维护（with-task-lock.sh 临界区）；dev/test agent 不写 `.arcforge/`。

## 任务看板

| ID | 标题 | wave | deps | packages | 状态 |
|---|---|---|---|---|---|
| TASK-001 | crisis 类型/日期/Store | 1 | — | internal/crisis | accepted |
| TASK-002 | FRED 采集器 | 1 | — | collector/fred | accepted |
| TASK-003 | yahoo JPY=X | 1 | — | collector/yahoo | accepted |
| TASK-004 | 配置与加载 | 2 | 001 | internal/crisis | accepted |
| TASK-005 | derive.go | 3 | 004 | internal/crisis | accepted |
| TASK-006 | ingest.go | 4 | 002,003,005 | internal/crisis | accepted |
| TASK-007 | CLI+backfill（一阶段收口） | 5 | 006 | cmd/atlas | accepted |
| TASK-008 | suppress.go | 5 | 006 | internal/crisis | accepted |
| TASK-009 | rules.go | 6 | 008 | internal/crisis | accepted |
| TASK-010 | statemachine+memhistory | 7 | 009 | internal/crisis | accepted |
| TASK-011 | eval.go | 8 | 010 | internal/crisis | accepted |
| TASK-012 | eval/status 子命令 | 9 | 007,011 | cmd/atlas | accepted |
| TASK-013 | replay 回测（二阶段收口） | 10 | 012 | cmd/atlas | accepted |
| TASK-014 | notify.go+telegram | 11 | 011,013 | crisis+cmd | accepted |
| TASK-015 | intraday+launchd（三阶段收口） | 12 | 014 | cmd+crisis | accepted |

并行度：wave 1 三路并行；wave 5 二路并行（007∥008）；其余同包串行（AD-2）。
建议团队：dev × 3、test × 1。

## 人工待办（manual 验收，终验收阶段执行）

- TASK-007：真实 FRED backfill + HY OAS CSV 导入 + sqlite 抽查 + 附录基线核对
- TASK-013：三段历史回测达标（2020/2024/2008 提前预警、2015–19 误报 ≤1）
- TASK-015：launchd 部署 + kickstart 幂等验证 + 两周试运行

## 事件日志

- 2026-07-13 环境检查通过；需求分析（降级）完成；15 任务拆分 + DoD 初稿；追溯矩阵无孤儿/无凭空；validator（降级）PASS。
- 2026-07-13 独立 reviewer 反审 PASS_WITH_NOTES（12 项）：TASK-013 boundary 方向性矛盾（阻断级）已改为「无观测 → run backfill first 报错」；TASK-005/012/015 boundary 漏项补齐；5 处 error_handling 悬空统一裁决为 Dev 补最小注入用例（TASK-001 半开连接关闭改 QA review 核对）；TASK-007 boundary 措辞对齐 TestImportCSVFrom；TASK-015 补 intraday plist 并合并 functional 保持 ≤8 条。修正后 validator 重跑 PASS。DoD 定稿，暂停于 dod-gate 等人工确认。
- 2026-07-13 dod-gate 人工确认通过 → 建分支 → wave 1 三路并行开发（协议波折见上方事故记录）→ test-agent-1 全部 VERIFIED（452c06d / b8c6c13 / 76e9f59）。
- 2026-07-13 dag 放行 TASK-004 → dev-agent-1（复用同包上下文）；dev 协议 v2：TDD 全绿先提交，再跑 code-simplifier，简化另行提交。
- 2026-07-13 TASK-005 VERIFIED（78ba314，84.4%，反审守卫双证）→ 放行 TASK-006。期间 dev-agent-1/test-agent-1 遭瞬时 API 中断，SendMessage 续接恢复，无产物损失（协议 v2 主产物先入库）。
- 2026-07-13 TASK-006 首验 REJECTED（Reality Checker 逮到「FRED 失败返回 error」四分支零覆盖）→ rework_count=1、epoch=2 重派 dev-agent-1，最小修复=补一个 fakeFRED 缺序列的 require.Error 用例。其余条目全 PASS。
- 2026-07-13 TASK-006 返工复验 VERIFIED（7664f49，86.6%）→ 进入二路并行：TASK-007→dev-agent-2、TASK-008→dev-agent-1。
- 2026-07-13 TASK-008 首验 REJECTED（rawFromDetail 回退分支覆盖=0，「间接覆盖」注释被 profile 证伪）→ rework 1、epoch 2 重派，最小修复=一个 Detail 为空的判别性用例。
- 2026-07-13 TASK-007 交付（f104ba1+38bd622）；覆盖率门禁裁决 AD-6：cmd/atlas 任务按新增文件计（crisis.go 80.3% PASS，全包 70.9% 记录不阻断）。TASK-008 返工 bc85498 复验中；TASK-007 排队验证。
- 2026-07-13 TASK-008 复验 VERIFIED（bc85498，88.1%）→ 放行 TASK-009（rules.go）→ dev-agent-1。
- 2026-07-13 TASK-007 VERIFIED → **第一阶段（数据流跑通，Task 1–7）代码全部 verified**；人工验收项（真实 FRED backfill/CSV/基线核对）留终验收。在途仅 TASK-009（rules.go, dev-agent-1）。
- 2026-07-13 TASK-009 首验 REJECTED（分位轨 AMBER 半边覆盖=0，dev 自查的"剩余全是 err 分支"声称仍有遗漏）→ rework 1、epoch 2 重派。fixture/零阈值/基线锚点等其余条目全 PASS。
- 2026-07-13 TASK-009 复验 VERIFIED（0a301e6，89.1%）→ 放行 TASK-010（statemachine+memhistory）→ dev-agent-1。
- 2026-07-13 TASK-010 首验即 VERIFIED（5a0fa1c，90.8%，complex 任务首次一次通过）→ 放行 TASK-011（eval.go）→ dev-agent-1。
- 2026-07-13 TASK-011 首验 REJECTED（prevState resume-from-history 分支零命中——replay 状态链根基，全部用例只测冷启动半）→ rework 1、epoch 2 重派。
- 2026-07-13 TASK-011 复验 VERIFIED（1253f3d，90.7%）→ 引擎层全部收口 → 放行 TASK-012（eval/status CLI）→ dev-agent-2。
- 2026-07-13 TASK-012 首验 REJECTED（nfci 模式零覆盖 + 假测试注释引用不存在的用例）→ Leader 裁决方案 a（抽 executeCrisisEvalNFCI 注入 seam，不改 DoD）→ rework 1、epoch 2 重派。
- 2026-07-14 dev-agent-2 因月度用量限额失败 → TASK-012 返工由 Leader 按方案 a 代工：04ccad1（crisisEvalDeps 增 ingestNFCI seam + executeCrisisEvalNFCI + TestExecuteCrisisEvalNFCI + 假注释修正），crisis.go 覆盖 82.9%（AD-6 PASS）。复验已派 test-agent-1。
- 2026-07-14 TASK-012 复验 VERIFIED（04ccad1，crisis.go 82.9%）→ 放行 TASK-013（replay 回测，二阶段收口）→ dev-agent-2（限额恢复尝试）。
- 2026-07-14 TASK-013 首验即 VERIFIED（263282f，84.9%）→ **第二阶段全部收口** → 放行 TASK-014（notify+telegram，AD-3 双 package）→ dev-agent-1。
- 2026-07-14 TASK-014 首验即 VERIFIED（08b407e）→ 放行 TASK-015（intraday+launchd，最后一个任务）→ dev-agent-1。
- 2026-07-14 TASK-015 首验即 VERIFIED（9850acc）→ **15/15 任务级验证全部完成**。transition-audit（降级手工版）PASS。进入 QA 两轮审查。
- 2026-07-14 QA 终审 CONDITIONAL PASS（0C/2W/3I）。SEC-1（api_key 经传输错误泄漏日志）→ TASK-002 review_fix 重开（dev-agent-2，rework 1，epoch 2）；CLEAN-1（LatestObservation 死导出）Leader 裁决降级 INFO（契约冻结方法，删除违约）。报告：05-review/qa-review.md。
- 2026-07-14 SEC-1 修复 dd2da2a + 残留面 e2f2815（Leader 补构造分支 stripURLError）复验通过；全仓 53 包终回归全绿 → **15/15 accepted，Sprint 019 交付**。
