# Sprint 终验收报告 — sprint-003 Qlib 回测验证管线（2026-06-12）

**需求源**: docs/plans/2026-06-12-qlib-eval-pipeline-implementation.md（rev4）| 设计 rev3
**改动规模**: 10 个实现 commit / 24 files / **+2049 −44**（基于 `945ced8`）
**QA 终判**: **PASS**（round1+2 PASS+2 WARNING → 修复一轮 → round3 PASS，崩溃反例 QA 亲证闭合）

## 需求达成（R1-R8 全部 ✅，首个跨语言 Sprint）

| 能力 | 任务 | 交付 |
|------|------|------|
| 引擎机制性盖戳 | 001 | GeneratedAt 统一覆写为 bar 时间（stampStub 反例锁定）+ SkippedBars 计数 |
| 策略墙钟修复 | 002 | ma_crossover 两处 time.Now → ctx.Now |
| export-signals CLI | 003 | 白名单动态拒绝（全 5 策略注册）/warm-up 前移+from 过滤/golden CSV 逐字节/Makefile |
| Python 评估层 | 004-007 | 符号映射、次日开盘入场对齐（顺延边界钉死 >）、事件研究（超额/sell 规避/三桶/基准前值对齐+负索引防护）、严格 CSV 读取、markdown 报告、缺数据指引 |
| 端到端 | 008 | make signal-eval 串联、README 六要素口径、双语言全量回归 |

## 质量数据

- **8/8 verified→accepted；功能开发零返工零阻塞（连续第二个 Sprint）**；QA 后 1 轮 review_fix（4 项，~6 分钟回流）
- 门禁：go build/vet/test + -race 全绿；pytest 28+ 用例全绿（venv，零 qlib 依赖）；CSV 跨语言 round-trip QA 实测一致；plan 验收对照 6/6
- DoD 27 条全测试映射（04-test/ 9 份验证矩阵含复验）

## 计划外修复（QA 发现）

- W1 空信号文件 NaTType 崩溃 → 入口短路（QA 构造同场景亲证闭合）
- W2 基准缺失整跑崩溃 → try/except 降级 + 报告警示节
- S3 收盘到收盘口径注明、S7 utf-8-sig BOM 容忍

## 遗留（CARRYOVER，不阻塞）

- S4: exit_date 超基准末日取末行无 gap 标记（与起点防御不对称）
- S5: confidence %.2f 舍入对桶边界（0.799→0.80）影响备查
- S6: backtest.New 在循环内构造（微优化）
- **真实数据包端到端**：qlib cn_data 未在本环境，make signal-eval 的真实 markdown 报告未实跑（链路与单测充分，ADR-S3-4 口径）——数据包就位后人工跑一次

## 跨语言机制沉淀（建议回流上游模板）

1. task-completed.sh 2e 分流：无 .go scope 不进 Go 门禁；Python scope 跑 pytest（从仓库根，与 conftest 约定绑定）
2. Leader 预置统一 venv 并写入设计文档（避免环境坑澄清风暴——本 Sprint 零环境澄清）
3. 反审 H2 类「DoD 验证命令必须与 hook 调用方式同款」应成为跨语言任务的固定检查项

## 流程统计

- 团队 dev×3（dev-3 为 Python 链专员，5 任务上下文连续）+ test×2 + qa×1；全程零澄清、零 blocked
- Sprint 时长约 70 分钟（dod-gate 确认到 QA 终判）
