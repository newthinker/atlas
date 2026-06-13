# 进度看板 — HK qlib bundle (atlas_hk)

> 真相源：`.arcforge/tasks/*.json` 的 status。执行模式：**Arcforge 单 dev 串行**。

## 阻塞高亮
（无）

## 任务状态

| 任务 | 标题 | wave | 依赖 | status | owner | rework |
|---|---|---|---|---|---|---|
| TASK-001 | toQlibInstrument HK 命名（Go） | 1 | — | pending | — | 0 |
| TASK-002 | to_qlib_instrument HK（Python） | 1 | — | pending | — | 0 |
| TASK-003 | export-ohlcv market 参数化 | 2 | 001 | pending | — | 0 |
| TASK-004 | config.yaml watchlist 加 ETF+指数 | 3 | 003 | pending | — | 0 |
| TASK-005 | Makefile qlib-data-hk | 3 | 003 | pending | — | 0 |
| TASK-006 | analyze_watchlist HK/CSI 指数识别 | 1 | — | pending | — | 0 |
| TASK-007 | 集成建 atlas_hk + 分析 | 4 | 002,003,004,005,006 | pending | — | 0 |

## 阶段
- [x] Step 2 需求分析
- [x] Step 3 任务拆分 + DoD + 追溯矩阵 + 手动任务图校验
- [ ] Step 4 dod-gate 人类确认 ← **当前**
- [ ] Step 5 dev/test 串行
- [ ] Step 6 QA 两轮
- [ ] Step 7 交付 + 归档

## 降级备注
ecc/codex/gemini 不可用；validator 缺失（手动校验）；arcforge-write.sh 缺失（with-task-lock.sh）。
