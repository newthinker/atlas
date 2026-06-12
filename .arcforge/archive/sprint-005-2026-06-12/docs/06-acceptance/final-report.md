# Final Report — percentile_step Sprint

> 日期：2026-06-12｜分支：feature/percentile-step（基于 master @ 4c1555a）
> 需求：docs/plans/2026-06-12-percentile-step-implementation.md（rev4.1 final）
> 设计：docs/plans/2026-06-12-percentile-step-design.md（rev4，用户批准）

## 交付总览

为 router 实现百分位步进提醒门控（`|当前分位−上次通知分位| ≥ step` 才重新放行），步长策略级可配（Signal.Metadata 传递、全局值回退），并修复 cfg.Router 死配置预存 bug。

**7/7 任务 accepted，0 返工、0 澄清阻塞，QA verdict PASS。**

| 任务 | 内容 | 提交 | 覆盖率 |
|---|---|---|---|
| TASK-001 | router 步进门控核心（分流+策略级步长） | 055d062 | router 包（终态 86.8%） |
| TASK-002 | RouteBatch/Clear/GetStats | 7dd171a | 同上 |
| TASK-003 | price_percentile 步长参数 | fa9ee68 | 各包 83.8%–96.3% |
| TASK-004 | pe_percentile 步长参数 | 10984ba | 同上 |
| TASK-005 | config percentile_step 字段+校验 | 55668d2 | 同上 |
| TASK-006 | app.New() 接线（修死配置 bug，含实证测试） | eb9c12b | 同上 |
| TASK-007 | configs 交付 + code-simplifier + 全量回归 + gitnexus 重索引 | fa60303 | go vet/test ./... 全绿 |

全部包覆盖率 ≥ 80% 门槛（coverage.dev_minimum）。验证证据：.arcforge/docs/04-test/ 七份逐条 done_criteria 覆盖矩阵报告。

## ⚠️ 存量行为变更（发布须知）

修复死配置 bug 后，未显式配置 router 节的部署：冷却 1h→4h、置信阈值 0.5→0.6（config 默认值开始真正生效）。详见 eb9c12b 提交信息 BREAKING-ish 段落。

## QA 审查结果

两轮审查（常规 + 纯 Claude 三视角对抗降级），verdict **PASS**：无 CRITICAL，客观门禁全绿（vet/test/-race/cover）。
审查期间发生一起越权写入事件，已按 ISSUE-4 流程处置（状态回滚+排查+重验），详见 05-review/leader-adjudication.md。

## 流程降级记录（per capabilities）

- ECC 不可用 → 设计已获用户批准（4 轮评审），直接沉淀，未走 brainstorming。
- codex/gemini CLI 不可用 → QA 第二轮降级为纯 Claude 跨视角（运维/对抗测试/维护者）。
- Go validator 不存在 → 任务图与 transition-audit 降级为 Leader 手工核对（两次全绿）。
- arcforge-write.sh 不存在 → teammate 状态写入降级为 with-task-lock.sh 临界区纪律。

## 后续建议（非阻断，源自 QA WARNING/SUGGESTION 处置）

1. Metadata 键常量化（internal/core 定义 MetadataKey* 常量）——范围外重构，本次按设计 YAGNI 豁免。
2. 配置文件职责说明：config.example.yaml 为模板、percentile-watchlist.yaml 为部署实例，多环境部署时注意同步。
3. GetStats 可观测性增强、symbol 含 `|` 防御性校验——按需排期。

## 验收声明

done_criteria 15 条需求全覆盖（02-plan/requirement-dod-matrix.md 双向追溯，含 reviewer 反审补充的 R16）；设计 §6 十条测试全部落地；7 任务全部置 accepted。
