# 进度计划 — digest「PE%」列

> 真相源：.arcforge/tasks/*.json。Leader 维护。调度：dag。autonomy：dod-gate。

## 当前阶段
**QA 阶段**（两任务 verified，rework 0，transition-audit 通过）
- TASK-001 ✅ verified（renderTable 100%，整包 90.5%）@7889c8b
- TASK-002 ✅ verified（enrichSignalMetadata 100%，B4/B6/N1 PASS）@cebea8a
- 独立评审补强用例(B4/B6/F5/N1)全部落地并通过；全仓 -race 全绿
- → QA 两轮 code-review 进行中

## 任务图（DAG / wave）
```
wave1（并行，无依赖，包不重叠）:
  TASK-001 (./internal/notifier/telegram)   TASK-002 (./internal/app)
```
| 任务 | 标题 | wave | deps | packages | status |
|------|------|------|------|----------|--------|
| TASK-001 | renderTable 加 PE% 列 | 1 | — | ./internal/notifier/telegram | pending |
| TASK-002 | enrichSignalMetadata 盖 pe_percentile_display | 1 | — | ./internal/app | pending |

## 校验
- 人工 validator（Go validator 缺失降级）：DAG 无环 ✅ / 单 owner ✅ / scope 非空+并发互斥（telegram vs app 不重叠）✅
- 追溯矩阵：0 孤儿 / 0 凭空 ✅
- 键名契约：pe_percentile_display 两端锁定 ✅

## 团队
2 dev（wave1 两任务并行）+ 1 test。沿用上 sprint 降级：write-hook 缺失→Leader 单写者；validator 缺失→人工；QA 跨视角纯 Claude。

## 分支
feature/digest-pe-percentile-column（已含 spec+plan 2 提交）
