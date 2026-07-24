# Sprint 终验收报告 — crisis NORMAL 态周报

> 日期：2026-07-23 | Leader: 主 session | 需求入口：docs/plans/atlas-macro-crisis-monitor-design.md
> Sprint 范围（差距分析后判定，dod-gate 用户确认）：NORMAL 态周报增量
> 权威设计：docs/superpowers/specs/2026-07-23-crisis-normal-weekly-report-design.md

## 交付总览（4/4 任务 verified，QA PASS）

| 任务 | 内容 | commit | 覆盖率 | 验证 |
|---|---|---|---|---|
| TASK-001 | SummaryKind 三值枚举 + Messages 路由（SummaryDue→Summary） | 8ae94fc | crisis 包 94.6% | 矩阵 5/5 |
| TASK-002 | renderWeekly NORMAL 尾段分叉（去退出进度行） | b717e9b | crisis 包 94.6%（renderWeekly 100%） | 矩阵 4/4，WATCH 字节不变量三重核验 |
| TASK-003 | cmd 层 summaryKind 判日 + buildNotifyContext 组装 | f5d7b82 | crisis.go 文件级 86.4%（summaryKind 100%） | 矩阵 5/5，Trends 对偶护栏✓ |
| TASK-004 | 运维手册 §2/§5 更新 + 残留清理 + 全量终验 | cbfbed9 + d59bc74 | —（文档任务） | review/manual 逐条✓ |

## 功能交付
- NORMAL 态每周一（评估日）发周报（复用 WATCH 骨架、无退出进度行、尾行「下次周报：下周一 · 状态变更即时通知」）。
- 当月首个交易日仍发月报；撞日（首交易日∧周一）只发月报——分支顺序天然实现，测试锚定 2026-06-01 / 2026-08-03。
- 不变量全部保持：WATCH 周报逐字节不变；结构化消息每日至多 1 条；评估成本不变（NORMAL 周报不拉 Trends 窗口）；周一假日顺延由既有 data-not-ready 机制覆盖（零新增日历逻辑，discovery 显式记录）。

## 终验证据
- 验证者亲跑 `GOTOOLCHAIN=local go test ./...` 全绿；`go build ./...` 干净；grep 全仓无 SummaryDue/summaryDue 残留（代码+运维文档）。
- 覆盖率门禁：internal/crisis 94.6%；cmd/atlas 按 AD-4 文件级口径 crisis.go 86.4%（AD-6 处置：临时阈值 80→75 放行一次 transition 后立即恢复 80，test-agent 独立复核数字一致）。
- QA 两轮审查 PASS（qa-agent-1，报告 05-review/sprint-crisis-normal-weekly-review.md）：第一轮常规全过；第二轮**真跨模型对抗**（codex exec read-only 独立复核）未发现真实缺陷。唯一 [INFO]：notify.go:38,40 防御性状态守卫属冗余但建议保留（cmd 层输入收敛之外的纵深防御）。
- 独立 DoD reviewer 反审 PASS，3 条非阻塞建议全部闭环（假日顺延 discovery 记录✓、Trends 对偶断言核验✓、互斥结构备查✓）。

## 流程数据
- 返工：0（四任务 rework_count 均 0；TASK-003 经历一轮 blocked_clarification 澄清环——门禁口径问题，非代码缺陷）。
- 机制：写通道 arcforge-write.sh + token 全程生效（dev×2/test×1/qa×1 均登记）；validator 任务图校验 3 次全过；transition-audit 无子命令→Leader 手工降级审计全绿（合法边/epoch/零越权）。
- 降级路径：ECC 不可用（AD-1 采纳定稿设计）；gitnexus 索引降级（AD-3，Leader 代跑 diff 范围核对：5 commit 全部仅触及声明文件）；codex 对抗轮实测可用未降级。
- code-simplifier（协议 v2 Leader 代跑）：无需改动。

## 技术债 / 框架反馈（登记）
1. **[框架] task-completed.sh 覆盖率门禁仅支持整包口径**：历史低基线包（cmd/atlas 75.9%）结构上无法靠 in-scope 改动过门禁，本 Sprint 走 AD-6 临时阈值处置。建议上游 ArcForge 支持 changed-files 口径（AD-4 语义机制化）。
2. **[框架] 分发的 arcforge-validate 缺 transition-audit 子命令**，本 Sprint 由 Leader 手工审计替代。
3. cmd/atlas 历史 CLI 入口（main/runServe/runBacktest 等）0% 覆盖欠账，与本需求无关，择机补测。

## 部署说明
无部署产物需求（07-deploy 无产物）：无 DB 迁移、无配置变更、无新 plist——周报判定在既有 daily eval 内完成（设计 §3.1），launchd 调度零改动。生效方式：下次 crisis-daily 唤起自动按新逻辑执行；无需人工操作。

### 待同步 hooks 清单(人类执行)
| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 Sprint 未改动 project-template/hooks/、project-template/scripts/ 与 CLAUDE.md 模板 | — |

备注：上表为空即无需人类同步动作；框架侧两项改进需求（changed-files 门禁口径、transition-audit 子命令）见「技术债 / 框架反馈」，属上游 ArcForge 仓库变更，不在本仓运行时同步范围。
