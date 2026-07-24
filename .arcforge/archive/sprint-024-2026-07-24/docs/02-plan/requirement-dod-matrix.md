# 需求 ↔ DoD 双向追溯矩阵(Prism M1)

> 需求来源: `docs/superpowers/plans/2026-07-23-prism-m1.md`(计划 Task N ↔ TASK-00N 一一映射)
> 机器检查结论: 无孤儿需求、无凭空 DoD(逐条核对见下)。

## 正向: 需求 → DoD

| 需求条目(计划出处) | DoD 落点 |
|---|---|
| Task 1 ApplyDefaults 默认值/显式值保持 | TASK-001 functional×2 + boundary×1 |
| Task 2 store 七方法/NaN↔NULL/Board 最新行/Series from 过滤 | TASK-002 functional×4 |
| Task 2 upsert 幂等(instrument/valuation) | TASK-002 boundary×2 |
| Task 3 公司/指数(.mcw) metricsList、cvpos×100、双日期格式 | TASK-003 functional×3 |
| Task 3 缺指标→NaN、乱序→升序、⚠指数指标名 live 校验点 | TASK-003 boundary×1 + non_functional(review) |
| Task 4 阶梯对齐/滚动分位/minPoints/EPS≤0 跳过/ErrInsufficientEPS | TASK-004 全部 6 条 |
| Task 4 alignPE 重构行为不变(既有测试安全网) | TASK-004 non_functional(test) |
| Task 5 首次回填/增量 latest+1/零请求/部分失败 | TASK-005 functional×3 + boundary×2 |
| Task 6 引擎路径重算/252 minPoints/PB·PS·10Y=NaN/幂等 | TASK-006 全部 5 条 |
| Task 7 子命令/告警/退化打印/enabled=false/exit code 语义 | TASK-007 functional×2 + boundary×2 + error×1 |
| Task 7 复用 crisis helper(禁自建) | TASK-007 non_functional(review) |
| Task 8 board/series JSON、status 规则、NaN→null、400/404 | TASK-008 functional×3 + boundary×1 |
| Task 8 ErrNotFound 增补、server/serve 接线 | TASK-008 error×1 + non_functional(review) |
| Task 9 board 卡片+group 筛选/detail 图表/static | TASK-009 functional×3 |
| Task 9 未启用 404/NaN→"—"/vendor 无新 CDN | TASK-009 boundary×1 + non_functional(review) |
| Task 10 配置示例/plist 08:30/部署文档只追加 | TASK-010 functional(review)×3 |
| 设计 §9 M1 五条验收 | TASK-010 non_functional(manual)×5 |

## 全局约束传导

| 全局约束 | 传导落点 |
|---|---|
| sqlite 固定 v1.38.2 / GOTOOLCHAIN=local | TASK-002 non_functional(review);dev prompt 全局注入 |
| testify+httptest+Context Checkpoint 注释 | 各任务测试风格约束,Test Agent 核对 |
| NaN↔NULL 缺失约定 | TASK-002/006/008 显式条目 |
| 阈值 15/85 配置可覆盖 | TASK-001 默认值 + TASK-008 status 规则 |
| 理杏豆增量计费 | TASK-005 boundary(零请求)+ TASK-010 验收⑤ |
| 美股指数仅标普500 | TASK-010 配置示例 review 条目 |
| 无新外部 CDN | TASK-009 non_functional(review) |
| 每 Task 单独 commit + detect_changes | 流程门禁(dev 工作流),不入 DoD |
| deployment.md 只追加不重排 | TASK-010 functional(review) |

## 反向: DoD → 需求(凭空检查)

TASK-001..010 全部 done_criteria 逐条可回指计划对应 Task 的 Step 1 测试代码、Interfaces 或 Global Constraints,无凭空条目。

## Realistic Scope 偏差(如实记录)

- TASK-008 跨 4 包、TASK-009 跨 2 包 6 文件: 接线任务按计划原文保留边界,依赖边(T7→T8→T9)保证同包任务不并发在途,validator scope 互斥通过。
- TASK-010 纯产物任务: 按「无代码任务声明」全部 done_criteria 为对象形态 verify_by review|manual。
