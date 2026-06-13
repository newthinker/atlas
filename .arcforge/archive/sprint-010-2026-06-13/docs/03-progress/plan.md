# 进度看板 — 美股 signal-eval（atlas_us）

> 单写者：Leader · 调度 dag · 更新 2026-06-13
> 真相源：`.arcforge/tasks/*.json` 的 status 字段；本文件为聚合视图。

## 🚦 当前阶段：TASK-006 收口（端到端 region=us 验收进行中）

## 任务总览

| Task | 标题 | package | wave | status |
|---|---|---|---|---|
| TASK-001 | toQlibInstrument 美股分支 + 契约迁移 | cmd/atlas | 1 | ✅ verified |
| TASK-002 | benchmark/inMarket/market 校验美股分支 | cmd/atlas | 2 | ✅ verified |
| TASK-003 | symbols.py 美股对称镜像 | scripts/qlib_eval | 2 | ✅ verified |
| TASK-004 | prices.py region 参数化 + evaluate --region | scripts/qlib_eval | 1 | ✅ verified |
| TASK-005 | Makefile 美股 target + 守门测试 | scripts/qlib_eval | 3 | ✅ verified |
| TASK-006 | config 美股 watchlist + 端到端验收收尾 | configs | 4 | 🔄 收口中 |

## 执行记录（dag 实际推进）

- 窗口1：T001(dev1)+T004(dev2) 并行 → 双 PASS → verified
- 窗口2：T002(dev1)+T003(dev2) 并行 → T002 PASS；T003 上报跨任务 blocker（test_report.py AAPL fixture 失效）→ Leader 授权同 package 修复（AAPL→GC=F）→ PASS → verified
- 窗口3：T005(dev2) → PASS（9 passed，make -n 展开正确）→ verified
- 窗口4：T006(dev1) config 部分 → PASS（build ok / go test ok / pytest 63 passed / go vet 净 / smoke CSV 1308 行）
  - ✅ code-simplifier：无需简化（忠实镜像 HK，复跑全绿）
  - 🔄 端到端 make qlib-data-us / signal-eval-us（region=us 真实验收）— 进行中

## 门禁记录

- ✅ validator 规则：每窗口放行前 PASS
- ✅ 需求↔DoD 追溯 + 独立 reviewer 反审（采纳 #1/#6、证伪 #5）
- ✅ 人类确认门（dod-gate）：放行；符号一致性机器守门按「忠实镜像 HK」不加，记 backlog
- ✅ code-simplifier（全局规范，commit 前）
- ⏳ 端到端 region=us → 待结果
- ⬜ QA 两轮审查（Step 6）
- ⬜ 最终提交 + final-report（Step 7）

## Backlog（reviewer 提出、本 sprint 未做）

- SIGNAL_SYMBOLS_US ↔ config watchlist 双写机器守门（CN/HK/US 统一处理）
- US resolver 集成测试 TestResolveOHLCVSymbols_USMarket（DoD 未要求）

## 降级声明

ecc/codex/gemini=false → 纯 Claude 跨视角；validator/arcforge-write.sh 缺失 → Leader 直接原子写。
