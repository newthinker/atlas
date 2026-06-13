# 终验收报告 — signal-eval 基准参数化（支持港股）

日期：2026-06-13
执行模式：Arcforge 单 dev 串行（dev-agent-1 + test-agent-1 + qa-agent-1）

## 交付概述
signal-eval 事件研究基准由硬编码 `SH000300` 改为 `--benchmark` 参数（atlas 形式），
启用港股（atlas_hk + `^HSI`）；A股路径默认 `000300.SH` 零回归。美股推迟另开一轮（参数化天然就绪）。

## 任务完成（4/4 accepted）

| 任务 | 标题 | rework | 结果 |
|---|---|---|---|
| TASK-001 | QlibPriceSource benchmark 参数化（prices.py） | 0 | ✅ |
| TASK-002 | evaluate.py --benchmark（透传 + _meta） | 1（QA F2/F3） | ✅ |
| TASK-003 | Makefile signal-eval --benchmark + signal-eval-hk | 0 | ✅ |
| TASK-004 | 集成：港股事件研究非空验证 | 0 | ✅ |

## 质量结果
- `pytest scripts/qlib_eval/` **58 passed**（54 基线 + 4 新）；`go build ./...` 通过。
- benchmark() 残留硬编码防护：注入 fake qlib 断言 D.features 实际请求 `^HSI`→`HSI`、默认→`SH000300`（reviewer 最高风险点）。
- **决定性集成对照**：港股 signal-eval（8129 信号）→ 丢弃 **3**（修复前同口径 8129/8129 全丢弃），基准 `^HSI`，超额收益相对恒生已算（如 price_percentile buy h60 mean_excess 0.0115 / win_rate 50.92%，非 nan）。
- A股零回归：默认 `000300.SH`，单测钉住默认路径（本环境 eastmoney 屏蔽，A股全链路由单测覆盖）。

## Code Review（两轮 + 修复迭代）
- 第 1 轮（常规 + 跨视角对抗，纯 Claude 三视角）：**PASS**（无 CRITICAL）+ 3 WARNING。
- Leader 裁决：
  - **F2**（report.py fallback 仍 SH000300 与 atlas-form 显示不一致）→ 对齐 `000300.SH`。
  - **F3**（render 层无基准端到端断言，F2 盲区）→ 加 render_report 基准文案断言（000300.SH / ^HSI）。
  - **F6**（signal-eval-hk 带 --config 而 A股不带）→ 港股走 yahoo，--config 无害且有益，判为有意，文档说明不改。
  - F4/F5/F7/F8 INFO 非阻断。
- touch-up 后全量 58 passed。

## 独立 reviewer 反审
最高风险（benchmark() D.features 入参断言防残留硬编码）已并入 TASK-001 并落实。

## 环境降级（已生效）
- ecc=false → brainstorming（spec 已产出）；codex/gemini=false → QA 纯 Claude 三视角。
- validator 缺失 → Leader 手动任务图校验（线性 DAG，全过）；arcforge-write.sh 缺失 → with-task-lock.sh。

## 已知遗留（非阻断）
- 美股 signal-eval：本轮仅参数化就绪，未建 atlas_us / 未加美股 watchlist（另开一轮）。
- `region="cn"` 在 HK 路径沿用（pre-existing，raw-OHLC 路径容忍）。

## 验收结论
全部 done_criteria 通过，两轮 Code Review（PASS + WARNING touch-up）闭合，全量回归无失败。**验收通过（accepted）**。
