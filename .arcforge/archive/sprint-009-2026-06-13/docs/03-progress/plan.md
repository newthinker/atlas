# 进度看板 — signal-eval 基准参数化

> 真相源：`.arcforge/tasks/*.json` status。执行模式：**Arcforge 单 dev 串行**。

## 阻塞高亮
（无）

## 任务状态

| 任务 | 标题 | wave | 依赖 | status | owner | rework |
|---|---|---|---|---|---|---|
| TASK-001 | QlibPriceSource benchmark 参数化 | 1 | — | pending | — | 0 |
| TASK-002 | evaluate.py --benchmark | 2 | 001 | pending | — | 0 |
| TASK-003 | Makefile signal-eval-hk | 3 | 002 | pending | — | 0 |
| TASK-004 | 集成港股事件研究非空验证 | 4 | 001/002/003 | pending | — | 0 |

## 阶段
- [x] Step 2 需求分析
- [x] Step 3 任务拆分 + DoD + 追溯矩阵 + 手动校验
- [ ] Step 4 dod-gate ← **当前**
- [ ] Step 5 dev/test 串行
- [ ] Step 6 QA 两轮
- [ ] Step 7 交付 + 归档

## 降级备注
ecc/codex/gemini 不可用；validator 缺失（手动）；arcforge-write.sh 缺失（with-task-lock.sh）。
